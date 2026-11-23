/*
 * minict — minimal rootless container (PoC day 2026-11-17)
 *
 * Stack: user ns + mount ns + pid ns + uts ns + ipc ns + NET ns (issue #3),
 *        self-written uid_map, pivot_root + MNT_DETACH of old root,
 *        bind-mounted /dev bits, capability drop + PR_SET_NO_NEW_PRIVS,
 *        seccomp-bpf blocklist (incl. AF_NETLINK — D15).
 *        pidns-1 = signal shim: installs handlers + forwards to the payload
 *        child (issue #6, D19 — pid_namespaces(7): init ignores
 *        default-disposition signals from ancestor namespaces).
 *        No Docker, no root.
 *
 * Usage: minict <rootfs-dir> <cmd> [args...]
 *
 * Env:
 *   MINICT_NO_NETNS=1   opt out of CLONE_NEWNET (A/B control + hosts without
 *                       netns); default is NETNS ON and FAIL-CLOSED (no
 *                       silent fallback to the shared host net, D17)
 *   MINICT_WEB_PORT=N   enable the in-sandbox web relay for this port (D16)
 *   MINICT_PROXY_SOCK=P unix socket path INSIDE the new root (relay binds it;
 *                       host webproxy.py reaches httpd through it because
 *                       AF_UNIX is filesystem-scoped, not netns-scoped)
 *   MINICT_SPEC=F       line-based spec file augmenting the setup (D23):
 *                       hostname/env/bind/webport/tmpfs/shm/ulimit lines;
 *                       '#' comments and blank lines ignored; unknown
 *                       directives warn+skip (forward compatible), parse
 *                       errors abort the run (fail-closed)
 *
 * Verified on kernel 5.10.134 (see journal/2026-11-17/research.md):
 *  - pivot_root works inside unprivileged userns
 *  - mounts over host mountpoints are EPERM (MNT_LOCKED), mounts over plain
 *    dirs inside our rootfs work
 *  - mount(2) of /proc is blocked by the host seccomp filter (warn, continue)
 *  - mknod is EPERM in userns -> bind-mount host /dev/null etc. before pivot
 *  - CLONE_NEWNET in userns works; `lo` comes up DOWN — bring it up while we
 *    still hold caps (AFTER the uid_map write: empty map = no caps at all,
 *    measured 2026-11-17), see D18
 */
#define _GNU_SOURCE
#include <arpa/inet.h>
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <linux/audit.h>
#include <linux/capability.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <net/if.h>
#include <netinet/in.h>
#include <poll.h>
#include <sched.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mount.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

/* ---------- mount_setattr (S3, issue #113, D52) ----------
 *
 * Kernel >= 5.12 makes a recursive read-only bind a ONE-syscall operation:
 * mount_setattr(AT_RECURSIVE, MOUNT_ATTR_RDONLY) walks the whole mount
 * tree under VFS locks — no TOCTOU, covers submounts a plain bind-remount
 * leaves writable. Introduced in 5.12 (glibc wrapper 2.36), so call the
 * raw syscall and fall back on ENOSYS (kernel < 5.12, measured on the
 * 5.10 dev host). x86_64 is the only arch this repo measures; other
 * arches get the syscall number from <sys/syscall.h> when their headers
 * are new enough, else take the fallback path directly.
 */
#ifndef AT_RECURSIVE
#define AT_RECURSIVE 0x8000
#endif
#ifndef MOUNT_ATTR_RDONLY
#define MOUNT_ATTR_RDONLY 0x00000001
#endif
#ifndef AT_SYMLINK_NOFOLLOW
#define AT_SYMLINK_NOFOLLOW 0x100
#endif
struct minict_mount_attr {
        unsigned long long attr_set;
        unsigned long long attr_clr;
        unsigned long long propagation;
        unsigned long long userns_fd;
};

static void die(const char *ctx)
{
        fprintf(stderr, "minict: %s: %s\n", ctx, strerror(errno));
        exit(126);
}

static void warn(const char *ctx)
{
        fprintf(stderr, "minict: warn: %s: %s\n", ctx, strerror(errno));
}

/* write a whole string to a /proc file, fail closed */
static void write_proc_file(const char *path, const char *buf)
{
        int fd = open(path, O_WRONLY);
        size_t len = strlen(buf);

        if (fd < 0)
                die(path);
        if (write(fd, buf, len) != (ssize_t)len) {
                close(fd);
                die(path);
        }
        close(fd);
}

/* map 0 -> real_uid inside our own new userns (unprivileged self-map case,
 * see user_namespaces(7)); setgroups must be denied before gid_map */
static void map_ids(uid_t ruid, gid_t rgid)
{
        char buf[64];

        write_proc_file("/proc/self/setgroups", "deny\n");
        snprintf(buf, sizeof(buf), "0 %d 1\n", (int)ruid);
        write_proc_file("/proc/self/uid_map", buf);
        snprintf(buf, sizeof(buf), "0 %d 1\n", (int)rgid);
        write_proc_file("/proc/self/gid_map", buf);
}

/* ---------- netns (issue #3, D17/D18) ----------
 *
 * Default: CLONE_NEWNET — the sandbox gets its OWN network namespace with
 * only the loopback device (a new netns has NO host interface: "a physical
 * network device can live in exactly one network namespace",
 * network_namespaces(7)). `lo` comes up DOWN and needs CAP_NET_ADMIN in the
 * user namespace that OWNS the netns (user_namespaces(7): privileged ops on
 * a nonuser namespace require the capability in the owning user namespace) —
 * we hold full caps right after unshare + uid_map, so bring it up there,
 * BEFORE capdrop. Fail-closed: if netns cannot be created the run aborts
 * instead of silently sharing the host net (opt out explicitly with
 * MINICT_NO_NETNS=1).
 */

static int want_netns(void)
{
        return getenv("MINICT_NO_NETNS") == NULL;
}

static void bring_lo_up(void)
{
        int fd = socket(AF_INET, SOCK_DGRAM, 0);
        struct ifreq ifr;

        if (fd < 0)
                die("socket(AF_INET) for lo up");
        memset(&ifr, 0, sizeof(ifr));
        strncpy(ifr.ifr_name, "lo", IFNAMSIZ - 1);
        ifr.ifr_name[IFNAMSIZ - 1] = '\0';
        if (ioctl(fd, SIOCGIFFLAGS, &ifr) < 0)
                die("ioctl(SIOCGIFFLAGS lo)");
        ifr.ifr_flags |= (IFF_UP | IFF_RUNNING);
        if (ioctl(fd, SIOCSIFFLAGS, &ifr) < 0)
                die("ioctl(SIOCSIFFLAGS lo) — netns without usable loopback");
        close(fd);
}

/* bind host device files into <rootfs>/dev BEFORE pivot_root (mknod is
 * EPERM inside userns on this kernel; bind mounts of existing nodes work) */
static const char *g_devs[] = { "null", "zero", "urandom", "random" };

static void setup_dev_binds(const char *rootfs)
{
        char src[256], dst[256];
        size_t i;
        int fd;

        for (i = 0; i < sizeof(g_devs) / sizeof(g_devs[0]); i++) {
                snprintf(src, sizeof(src), "/dev/%s", g_devs[i]);
                snprintf(dst, sizeof(dst), "%s/dev/%s", rootfs, g_devs[i]);
                /* placeholder file so the bind target exists */
                fd = open(dst, O_CREAT | O_WRONLY, 0666);
                if (fd >= 0)
                        close(fd);
                if (mount(src, dst, NULL, MS_BIND, NULL) < 0)
                        warn(dst);
        }
}

static void drop_all_caps(void)
{
        struct __user_cap_header_struct hdr;
        struct __user_cap_data_struct data[2];

        memset(&hdr, 0, sizeof(hdr));
        hdr.version = _LINUX_CAPABILITY_VERSION_3;
        hdr.pid = 0;
        memset(data, 0, sizeof(data));
        if (syscall(SYS_capset, &hdr, data) < 0)
                die("capset");
        /* NOTE: no setgroups(2) here — map_ids() wrote "deny" to
         * /proc/self/setgroups (mandatory before gid_map without CAP_SETGID),
         * which makes setgroups() permanently EPERM in this userns and, as a
         * side effect, already forces an empty supplementary group list.
         * See user_namespaces(7). */
}

/* ---------- in-sandbox web relay (issue #3, D16) ----------
 *
 * The sandbox now runs in its own netns (loopback-only), so nothing on the
 * host can open a TCP connection into it (no veth to the host rootless —
 * measured 2026-11-17, needs CAP_NET_ADMIN on the host netns).
 * AF_UNIX sockets are filesystem-scoped (not netns-scoped): a socket bound
 * INSIDE the sandbox on a path in the rootfs disk is reachable from the host
 * via the instance rootfs dir. Bridge:
 *
 *   host:    tools/webproxy.py  TCP 127.0.0.1:<port> --> unix <rootfs>/<sock>
 *   sandbox: relay (below)      unix <sock> --> TCP 127.0.0.1:<port> (httpd
 *                               inside the netns)
 *
 * Precedent: RootlessKit --port-driver=builtin (parent proxies child-ns
 * ports; slirp4netns/pasta are not available in this env). The relay is
 * forked from the container child AFTER pivot (fs is the new root, socket
 * lands on the instance rootfs disk) and BEFORE capdrop; it drops caps
 * itself, never execs, deliberately does NOT get the seccomp filter (forked
 * before install) — attack surface = dumb byte pump with no caps. Teardown:
 * PDEATHSIG (parent = pidns-1) + pidns-1-exit + shared process group kill.
 */

static int g_web_port = 0;              /* MINICT_WEB_PORT, 0 = no relay */
static const char *g_proxy_sock = NULL; /* MINICT_PROXY_SOCK (in new root) */

/* ---------- MINICT_SPEC state (D23) ----------
 *
 * Parsed once, host-side, into these globals BEFORE any namespace work.
 * Hard caps everywhere (64 entries per list, 1024B per line, 64KB per
 * file): a spec file is operator config, not a data format — unbounded
 * growth would be a DoS against our own parser (fail-closed, D17 style).
 */
#define SPEC_MAX_ENTRIES 64
#define SPEC_MAX_LINE 1024
#define SPEC_MAX_FILE (64 * 1024)
/* S3 (D52): per-":ro" bind cap for src-relative submount paths recorded
 * host-side from /proc/self/mountinfo. Exceeding the cap dies — a ro bind
 * whose submounts we refuse to enumerate must not boot half-protected. */
#define SPEC_SUBMOUNT_MAX 16
#define SPEC_SUBMOUNT_PATH 256

static char g_hostname[HOST_NAME_MAX + 1] = "minict";

struct spec_bind {              /* host src bind-mounted into the rootfs */
        char src[512];
        char dst[512];          /* path INSIDE the rootfs (absolute) */
        int ro;                 /* ":ro" suffix -> MS_RDONLY remount */
        /* S3 (D52): MS_REC clones the mounts BENEATH src into the sandbox;
         * a top-only ro remount leaves them writable. Enumerated host-side
         * pre-unshare (the only window with /proc — the new mountns has
         * none and cannot mount one there: EPERM measured, D39) from
         * /proc/self/mountinfo, stored as src-relative suffixes ("/sub").
         * Used by the ENOSYS fallback of the recursive ro enforcement. */
        char submounts[SPEC_SUBMOUNT_MAX][SPEC_SUBMOUNT_PATH];
        int submount_n;
        int mountinfo_ok;       /* enumeration ran (else fail-closed later) */
};
static struct spec_bind g_spec_bind[SPEC_MAX_ENTRIES];
static int g_spec_bind_n;

static char g_spec_env[SPEC_MAX_ENTRIES][SPEC_MAX_LINE]; /* raw "K=V" */
static int g_spec_env_n;

struct spec_relay {             /* D24: one unix-socket relay per entry */
        int port;
        char sock[108];         /* sockaddr_un sun_path */
};
static struct spec_relay g_spec_web[SPEC_MAX_ENTRIES];
static int g_spec_web_n;

struct spec_tmpfs {             /* extra tmpfs like /tmp; size 0 = default */
        char path[512];
        unsigned long long size;
};
static struct spec_tmpfs g_spec_tmpfs[SPEC_MAX_ENTRIES];
static int g_spec_tmpfs_n;

static unsigned long long g_spec_shm_size; /* 0 = no shm directive */

struct spec_rlimit {            /* setrlimit in payload BEFORE capdrop */
        int res;
        rlim_t soft;
        rlim_t hard;
};
static struct spec_rlimit g_spec_rlimit[SPEC_MAX_ENTRIES];
static int g_spec_rlimit_n;
static int g_spec_readonly;  /* "readonly" directive -> Landlock ro-root (GĐ2 mục 6, #149) */

/* ---------- seccomp allowlist (GĐ2 mục 3a, issue #139, D59) ----------
 * Populated host-side BEFORE unshare/setns by spec_seccomp_allow() /
 * record_seccomp() from MINICT_SPEC / MINICT_SECCOMP_ALLOW directives.
 * In allowlist mode the program is: D11 blocklist floor (EPERM) + clone
 * NS-mask + socket AF_ALG/AF_NETLINK arg-checks + clone3 ENOSYS (glibc and
 * musl fall back to the arg-checked clone) + JEQ allowlist -> ALLOW +
 * default EPERM. An allowlist can never re-admit a floor entry: the floor
 * is evaluated first. MD0 (tests/test_minictd.sh seccomp_src) keeps this
 * decl + allow_add + install_seccomp byte-identical with
 * minictd/cmd/minict-exec/minict-exec.c — edit both or nothing. */
#define SECCOMP_ALLOW_MAX 512
static unsigned short g_allowlist[SECCOMP_ALLOW_MAX];
static size_t g_allowlist_n;

/* dedupe on insert; range-checked; full before-first-filter validation */
static void allow_add(unsigned int nr)
{
        size_t i;

        if (nr > 511) {
                fprintf(stderr, "minict-seccomp: allowlist syscall %u out "
                                "of range (cap 511)\n", nr);
                exit(125);
        }
        for (i = 0; i < g_allowlist_n; i++)
                if (g_allowlist[i] == (unsigned short)nr)
                        return;
        if (g_allowlist_n >= SECCOMP_ALLOW_MAX) {
                fprintf(stderr, "minict-seccomp: allowlist exceeds cap %d\n",
                        SECCOMP_ALLOW_MAX);
                exit(125);
        }
        g_allowlist[g_allowlist_n++] = (unsigned short)nr;
}

static int write_all(int fd, const char *buf, size_t len)
{
        size_t off = 0;

        while (off < len) {
                ssize_t k = write(fd, buf + off, len - off);

                if (k < 0) {
                        if (errno == EINTR)
                                continue;
                        return -1;
                }
                off += (size_t)k;
        }
        return 0;
}

/* bidirectional byte pump between two established sockets */
static void pump(int a, int b)
{
        char buf[16384];
        struct pollfd pf[2];

        pf[0].fd = a;
        pf[0].events = POLLIN;
        pf[0].revents = 0;
        pf[1].fd = b;
        pf[1].events = POLLIN;
        pf[1].revents = 0;
        for (;;) {
                int n = poll(pf, 2, -1);

                if (n < 0) {
                        if (errno == EINTR)
                                continue;
                        return;
                }
                if (pf[0].revents & POLLIN) {
                        ssize_t k = read(a, buf, sizeof(buf));

                        if (k <= 0 || write_all(b, buf, (size_t)k) < 0)
                                return;
                }
                if (pf[1].revents & POLLIN) {
                        ssize_t k = read(b, buf, sizeof(buf));

                        if (k <= 0 || write_all(a, buf, (size_t)k) < 0)
                                return;
                }
                if ((pf[0].revents | pf[1].revents) & (POLLERR | POLLNVAL))
                        return;
        }
}

/* relay event loop: never returns */
static void relay_main(int port, const char *sockpath)
{
        int sfd;
        struct sockaddr_un sa;
        struct sigaction ign;

        if (strlen(sockpath) >= sizeof(sa.sun_path))
                _exit(122);
        sfd = socket(AF_UNIX, SOCK_STREAM, 0);
        if (sfd < 0)
                _exit(122);
        unlink(sockpath); /* stale socket from a previous run */
        memset(&sa, 0, sizeof(sa));
        sa.sun_family = AF_UNIX;
        strcpy(sa.sun_path, sockpath);
        if (bind(sfd, (struct sockaddr *)&sa, sizeof(sa)) < 0)
                _exit(122);
        if (listen(sfd, 16) < 0)
                _exit(122);

        memset(&ign, 0, sizeof(ign));
        ign.sa_handler = SIG_IGN;
        sigaction(SIGCHLD, &ign, NULL); /* per-conn children auto-reaped */

        drop_all_caps(); /* relay never needs caps */

        for (;;) {
                int cfd = accept(sfd, NULL, NULL);

                if (cfd < 0) {
                        if (errno == EINTR)
                                continue;
                        continue; /* transient error: keep serving */
                }
                {
                        pid_t p = fork();

                        if (p == 0) {
                                /* conn child: dial httpd on the in-ns loopback */
                                struct sockaddr_in a;

                                close(sfd);
                                int ufd = socket(AF_INET, SOCK_STREAM, 0);

                                if (ufd < 0)
                                        _exit(1);
                                memset(&a, 0, sizeof(a));
                                a.sin_family = AF_INET;
                                a.sin_port = htons((uint16_t)port);
                                a.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
                                if (connect(ufd, (struct sockaddr *)&a,
                                            sizeof(a)) < 0)
                                        _exit(1);
                                pump(cfd, ufd);
                                _exit(0);
                        }
                }
                close(cfd);
        }
}

static void fork_one_relay(int port, const char *sock)
{
        pid_t p = fork();

        if (p < 0)
                die("fork (web relay)");
        if (p == 0) {
                if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
                        warn("relay PDEATHSIG");
                relay_main(port, sock); /* never returns */
                _exit(123);
        }
        /* parent continues: capdrop -> seccomp -> exec payload */
}

static void spawn_web_relay(void)
{
        int i;

        /* D16 env-var relay stays: single port via MINICT_WEB_PORT */
        if (g_web_port > 0)
                fork_one_relay(g_web_port, g_proxy_sock);
        /* D24: spec-file webports — one relay child per entry, same
         * PDEATHSIG + pidns-1-exit + group-kill teardown as above */
        for (i = 0; i < g_spec_web_n; i++)
                fork_one_relay(g_spec_web[i].port, g_spec_web[i].sock);
}

static void load_web_relay_env(void)
{
        const char *ps = getenv("MINICT_WEB_PORT");

        if (ps && *ps) {
                char *end = NULL;
                long v = strtol(ps, &end, 10);

                if (!end || *end != '\0' || v < 1024 || v > 65535) {
                        fprintf(stderr, "minict: bad MINICT_WEB_PORT '%s' "
                                        "(need 1024..65535)\n", ps);
                        exit(125);
                }
                g_web_port = (int)v;
                g_proxy_sock = getenv("MINICT_PROXY_SOCK");
                if (!g_proxy_sock || g_proxy_sock[0] != '/' ||
                    strstr(g_proxy_sock, "..")) {
                        fprintf(stderr, "minict: MINICT_PROXY_SOCK must be an "
                                        "absolute path in the new root\n");
                        exit(125);
                }
        }
}

/* ---------- MINICT_SPEC parsing (D23) ----------
 *
 * Read host-side BEFORE unshare (DAC checked against the REAL uid).
 * Every directive is validated at parse time: a bad value aborts the run
 * (exit 125, same class as the MINICT_WEB_PORT checks) instead of
 * half-applying config the operator believes is active. Exception:
 * unknown directives warn+skip, so a newer daemon can talk to an older
 * binary (forward compatible by design).
 */

static void spec_die(int lineno, const char *what)
{
        fprintf(stderr, "minict-spec: line %d: %s\n", lineno, what);
        exit(125);
}

/* bare bytes or one K/M/G suffix (case-insensitive, "512m"); -1 on error */
static int spec_parse_size(const char *s, unsigned long long *out)
{
        char *end = NULL;
        unsigned long long v, mul = 0, max = (unsigned long long)-1;

        errno = 0;
        v = strtoull(s, &end, 10);
        if (end == s || *s == '\0' || errno == ERANGE)
                return -1;
        if (*end) {
                if (end[1] != '\0')
                        return -1;
                switch (*end) {
                case 'K': case 'k':
                        mul = 1024ULL;
                        break;
                case 'M': case 'm':
                        mul = 1024ULL * 1024ULL;
                        break;
                case 'G': case 'g':
                        mul = 1024ULL * 1024ULL * 1024ULL;
                        break;
                default:
                        return -1;
                }
                if (v > max / mul)
                        return -1; /* would overflow */
                v *= mul;
        }
        *out = v;
        return 0;
}

/* numeric or "unlimited" (RLIM_INFINITY) */
static int spec_parse_rlim(const char *s, rlim_t *out)
{
        char *end = NULL;
        unsigned long long v;

        if (!strcmp(s, "unlimited")) {
                *out = RLIM_INFINITY;
                return 0;
        }
        errno = 0;
        v = strtoull(s, &end, 10);
        if (end == s || *end != '\0' || errno == ERANGE ||
            v > (unsigned long long)RLIM_INFINITY)
                return -1;
        *out = (rlim_t)v;
        return 0;
}

static void spec_copy_field(int lineno, char *dst, size_t cap,
                            const char *src)
{
        if (strlen(src) >= cap)
                spec_die(lineno, "value too long for field");
        strcpy(dst, src);
}

/* container-side paths: absolute, no ".." — bind dst and tmpfs paths are
 * resolved under the rootfs BEFORE pivot_root, so ".." would reach
 * OUTSIDE the rootfs and place a mount on a HOST path. Reject, don't
 * sanitize (same fail-closed spirit as D17). */
static void spec_check_ct_path(int lineno, const char *p)
{
        if (p[0] != '/')
                spec_die(lineno, "container path must be absolute");
        if (strstr(p, ".."))
                spec_die(lineno, "container path must not contain '..'");
}

static void spec_no_more_args(int lineno, const char *arg)
{
        if (strchr(arg, ' ') || strchr(arg, '\t'))
                spec_die(lineno, "unexpected extra argument");
}

static void spec_hostname(char *arg, int lineno)
{
        size_t i;

        spec_no_more_args(lineno, arg);
        if (*arg == '\0' || strlen(arg) > HOST_NAME_MAX)
                spec_die(lineno, "hostname: need 1..64 chars");
        for (i = 0; i < strlen(arg); i++) {
                if (!isalnum((unsigned char)arg[i]) && arg[i] != '-' &&
                    arg[i] != '.' && arg[i] != '_')
                        spec_die(lineno, "hostname: invalid character "
                                         "(use [A-Za-z0-9._-])");
        }
        strcpy(g_hostname, arg);
}

static void spec_env(char *arg, int lineno)
{
        const char *eq;
        size_t k;

        if (*arg == '\0')
                spec_die(lineno, "env: need KEY=VALUE");
        eq = strchr(arg, '=');
        if (!eq || eq == arg)
                spec_die(lineno, "env: need KEY=VALUE");
        for (k = 0; &arg[k] < eq; k++) {
                unsigned char c = (unsigned char)arg[k];

                if (k == 0 ? (!isalpha(c) && c != '_')
                           : (!isalnum(c) && c != '_'))
                        spec_die(lineno, "env: KEY must match "
                                         "[A-Za-z_][A-Za-z0-9_]*");
        }
        if (g_spec_env_n >= SPEC_MAX_ENTRIES)
                spec_die(lineno, "too many env entries (cap 64)");
        spec_copy_field(lineno, g_spec_env[g_spec_env_n], SPEC_MAX_LINE,
                        arg);
        g_spec_env_n++;
}

static void spec_bind(char *arg, int lineno)
{
        struct spec_bind *b;
        char *sp;

        spec_no_more_args(lineno, arg);
        if (g_spec_bind_n >= SPEC_MAX_ENTRIES)
                spec_die(lineno, "too many bind entries (cap 64)");
        sp = strchr(arg, ':');
        if (!sp)
                spec_die(lineno, "bind: need <host-src>:<ct-dst>");
        *sp = '\0';
        b = &g_spec_bind[g_spec_bind_n];
        b->ro = 0;
        spec_copy_field(lineno, b->src, sizeof(b->src), arg);
        arg = sp + 1;
        sp = strchr(arg, ':');
        if (sp) {
                *sp = '\0';
                if (strcmp(sp + 1, "ro"))
                        spec_die(lineno, "bind: only ':ro' suffix supported");
                b->ro = 1;
        }
        spec_copy_field(lineno, b->dst, sizeof(b->dst), arg);
        if (b->src[0] != '/')
                spec_die(lineno, "bind: host src must be absolute");
        spec_check_ct_path(lineno, b->dst);
        g_spec_bind_n++;
}

static void spec_webport(char *arg, int lineno)
{
        struct spec_relay *r;
        char *sock = arg, *end = NULL;
        char msg[SPEC_MAX_LINE + 64];
        long v;

        if (g_spec_web_n >= SPEC_MAX_ENTRIES)
                spec_die(lineno, "too many webport entries (cap 64)");
        while (*sock && *sock != ' ' && *sock != '\t')
                sock++;
        if (!*sock)
                spec_die(lineno, "webport: need <port> <sockpath>");
        *sock++ = '\0';
        while (*sock == ' ' || *sock == '\t')
                sock++;
        if (strchr(sock, ' ') || strchr(sock, '\t'))
                spec_die(lineno, "webport: unexpected extra argument");
        v = strtol(arg, &end, 10);
        if (!end || *end != '\0' || v < 1024 || v > 65535) {
                snprintf(msg, sizeof(msg), "webport: bad port '%s' "
                         "(need 1024..65535)", arg);
                spec_die(lineno, msg);
        }
        if (sock[0] != '/')
                spec_die(lineno, "webport: sockpath must be absolute in "
                                 "the new root");
        if (strstr(sock, ".."))
                spec_die(lineno, "webport: sockpath must not contain '..'");
        r = &g_spec_web[g_spec_web_n];
        r->port = (int)v;
        if (strlen(sock) >= sizeof(r->sock))
                spec_die(lineno, "webport: sockpath too long for AF_UNIX");
        strcpy(r->sock, sock);
        g_spec_web_n++;
}

static void spec_tmpfs(char *arg, int lineno)
{
        struct spec_tmpfs *t;
        char *sp;

        spec_no_more_args(lineno, arg);
        if (g_spec_tmpfs_n >= SPEC_MAX_ENTRIES)
                spec_die(lineno, "too many tmpfs entries (cap 64)");
        t = &g_spec_tmpfs[g_spec_tmpfs_n];
        t->size = 0;
        sp = strchr(arg, ':');
        if (sp) {
                *sp = '\0';
                if (strncmp(sp + 1, "size=", 5))
                        spec_die(lineno, "tmpfs: only ':size=N' option "
                                         "supported");
                if (spec_parse_size(sp + 6, &t->size) < 0 || t->size == 0) {
                        char msg[96];

                        snprintf(msg, sizeof(msg), "tmpfs: bad size '%s' "
                                 "(bytes or K/M/G)", sp + 5);
                        spec_die(lineno, msg);
                }
        }
        spec_copy_field(lineno, t->path, sizeof(t->path), arg);
        spec_check_ct_path(lineno, t->path);
        g_spec_tmpfs_n++;
}

static void spec_shm(char *arg, int lineno)
{
        char msg[SPEC_MAX_LINE + 32];

        spec_no_more_args(lineno, arg);
        if (spec_parse_size(arg, &g_spec_shm_size) < 0 ||
            g_spec_shm_size == 0) {
                snprintf(msg, sizeof(msg), "shm: bad size '%s' "
                         "(bytes or K/M/G)", arg);
                spec_die(lineno, msg);
        }
}

static void spec_ulimit(char *arg, int lineno)
{
        struct spec_rlimit *r;
        char *eq, *colon;
        int res;

        spec_no_more_args(lineno, arg);
        if (g_spec_rlimit_n >= SPEC_MAX_ENTRIES)
                spec_die(lineno, "too many ulimit entries (cap 64)");
        eq = strchr(arg, '=');
        if (!eq || eq == arg)
                spec_die(lineno, "ulimit: need <name>=<soft>[:<hard>]");
        *eq = '\0';
        if (!strcmp(arg, "nofile"))
                res = RLIMIT_NOFILE;
        else if (!strcmp(arg, "nproc"))
                res = RLIMIT_NPROC;
        else
                spec_die(lineno, "ulimit: unknown resource (nofile, nproc)");
        colon = strchr(eq + 1, ':');
        if (colon)
                *colon = '\0';
        r = &g_spec_rlimit[g_spec_rlimit_n];
        if (spec_parse_rlim(eq + 1, &r->soft) < 0)
                spec_die(lineno, "ulimit: bad soft value");
        r->hard = r->soft;
        if (colon && spec_parse_rlim(colon + 1, &r->hard) < 0)
                spec_die(lineno, "ulimit: bad hard value");
        if (r->hard != RLIM_INFINITY && r->soft > r->hard)
                spec_die(lineno, "ulimit: soft > hard");
        r->res = res;
        g_spec_rlimit_n++;
}

/* seccomp_allow <nr>[,<nr>]* (D59) — one chunk of the profile the daemon
 * rendered from minictd/internal/minict/profiles/<name>.allow. Parse errors
 * DIE (fail-closed): a half-loaded allowlist would default-deny syscalls
 * the operator believes are allowed, and an empty line would brick the
 * payload entirely. The floor (blocklist) still wins at filter time, so a
 * listed-but-denied syscall is harmless noise, not a hole. */
static void spec_seccomp_allow(char *arg, int lineno)
{
        char *tok, *end;
        size_t before = g_allowlist_n;
        unsigned long v;

        for (tok = strtok(arg, ","); tok; tok = strtok(NULL, ",")) {
                v = strtoul(tok, &end, 10);
                if (end == tok || *end != '\0')
                        spec_die(lineno, "seccomp_allow: bad syscall number "
                                         "(need decimal 0..511)");
                allow_add((unsigned int)v);
        }
        if (g_allowlist_n == before)
                spec_die(lineno, "seccomp_allow: empty directive");
}

/* one line, in-place (we own g_spec_text) */
static void spec_parse_line(char *line, int lineno)
{
        char *dir, *arg, *end;

        /* leading whitespace off; '#' comments and blank lines are ignored */
        while (*line == ' ' || *line == '\t')
                line++;
        if (*line == '\0' || *line == '#')
                return;
        /* trailing whitespace off (incl. CR from daemon-written files) */
        end = line + strlen(line);
        while (end > line && (end[-1] == '\n' || end[-1] == '\r' ||
                              end[-1] == ' ' || end[-1] == '\t'))
                *--end = '\0';

        dir = line;
        arg = line + strcspn(line, " \t");
        if (*arg) {
                *arg = '\0';
                arg++;
                while (*arg == ' ' || *arg == '\t')
                        arg++;
        }

        if (!strcmp(dir, "hostname"))
                spec_hostname(arg, lineno);
        else if (!strcmp(dir, "env"))
                spec_env(arg, lineno);
        else if (!strcmp(dir, "bind"))
                spec_bind(arg, lineno);
        else if (!strcmp(dir, "webport"))
                spec_webport(arg, lineno);
        else if (!strcmp(dir, "tmpfs"))
                spec_tmpfs(arg, lineno);
        else if (!strcmp(dir, "shm"))
                spec_shm(arg, lineno);
        else if (!strcmp(dir, "ulimit"))
                spec_ulimit(arg, lineno);
        else if (!strcmp(dir, "readonly"))
                g_spec_readonly = 1;
        else if (!strcmp(dir, "seccomp_allow"))
                spec_seccomp_allow(arg, lineno);
        else
                fprintf(stderr, "minict-spec: line %d: unknown directive "
                                "'%s' — skipped\n", lineno, dir);
}

static char g_spec_text[SPEC_MAX_FILE + 1];

/* defined after this file's spec parse section (S3, D52): must run AFTER
 * the full spec is parsed, BEFORE unshare removes /proc from view */
static void spec_scan_bind_submounts(void);

static void load_spec_file(void)
{
        const char *spec = getenv("MINICT_SPEC");
        char *p;
        size_t off = 0;
        int fd, lineno = 0;
        struct stat st;

        if (!spec || !*spec)
                return; /* no spec: defaults stay (hostname "minict") */

        /* host-side open: DAC is checked against the REAL uid (pre-unshare) */
        fd = open(spec, O_RDONLY);
        if (fd < 0)
                die("open(MINICT_SPEC)");
        if (fstat(fd, &st) < 0) {
                close(fd);
                die("fstat(MINICT_SPEC)");
        }
        if (!S_ISREG(st.st_mode)) {
                close(fd);
                fprintf(stderr, "minict-spec: %s: not a regular file\n",
                        spec);
                exit(125);
        }
        if (st.st_size > SPEC_MAX_FILE) {
                close(fd);
                fprintf(stderr, "minict-spec: %s: larger than 64KB cap\n",
                        spec);
                exit(125);
        }
        for (;;) {
                ssize_t r = read(fd, g_spec_text + off,
                                 sizeof(g_spec_text) - 1 - off);

                if (r < 0) {
                        if (errno == EINTR)
                                continue;
                        close(fd);
                        die("read(MINICT_SPEC)");
                }
                if (r == 0)
                        break;
                off += (size_t)r;
        }
        close(fd);
        g_spec_text[off] = '\0';
        if (memchr(g_spec_text, '\0', off)) {
                fprintf(stderr, "minict-spec: %s: NUL byte in spec\n", spec);
                exit(125);
        }

        p = g_spec_text;
        while (p) {
                char *eol = strchr(p, '\n');
                char *line = p;

                if (eol) {
                        *eol = '\0';
                        p = eol + 1;
                } else {
                        p = NULL;
                }
                lineno++;
                spec_parse_line(line, lineno);
        }
        spec_scan_bind_submounts();
}

/* ---------- S3 (issue #113, D52): submount enumeration for :ro binds ----------
 *
 * /proc/self/mountinfo (man 5 proc) lines look like:
 *   36 35 98:0 /mnt1 /mnt2 rw,... master:1 - ext3 /dev/sda1 ...
 * Fields 4 (root) and 5 (mount point) are escaped with exactly four
 * sequences: \040 ' ', \011 '\t', \012 '\n', \134 '\\'. Only field 5 is
 * needed here. Runs host-side BEFORE unshare — inside the new mountns
 * there is no /proc and one cannot be mounted there (EPERM measured on
 * 5.10 and the runner, D39), so this window is the only chance to learn
 * which mounts the MS_REC bind will clone under a ":ro" src.
 */
static int mntinfo_decode(const char *in, char *out, size_t outsz)
{
        size_t o = 0;
        const char *p;

        for (p = in; *p; p++) {
                if (o + 1 >= outsz)
                        return -1; /* truncated mount point — fail-closed
                                    * (a truncated path cannot match src and
                                    * would silently MISS a submount) */
                if (*p == '\\' && p[1] >= '0' && p[1] <= '7' &&
                    p[2] >= '0' && p[2] <= '7' &&
                    p[3] >= '0' && p[3] <= '7') {
                        out[o++] = (char)((p[1] - '0') * 64 +
                                          (p[2] - '0') * 8 +
                                          (p[3] - '0'));
                        p += 3;
                        continue;
                }
                out[o++] = *p;
        }
        out[o] = '\0';
        return 0;
}

static void spec_scan_bind_submounts(void)
{
        int i, any_ro = 0;
        FILE *f;
        char line[2048];

        for (i = 0; i < g_spec_bind_n; i++)
                if (g_spec_bind[i].ro)
                        any_ro = 1;
        if (!any_ro)
                return; /* nothing to enforce recursively */

        f = fopen("/proc/self/mountinfo", "r");
        if (!f) {
                /* fail-closed: mark every ro bind unverified. On a kernel
                 * WITH mount_setattr this flag is unused (the one-syscall
                 * path needs no enumeration); without it a ro bind whose
                 * submounts are unknown must NOT boot half-protected. */
                for (i = 0; i < g_spec_bind_n; i++)
                        g_spec_bind[i].mountinfo_ok = 0;
                warn("open /proc/self/mountinfo — :ro submounts unverified");
                return;
        }

        while (fgets(line, sizeof(line), f)) {
                char *p = line, *tok;
                char mp[1024];
                int k;

                if (!strchr(line, '\n') && !feof(f)) {
                        /* a line longer than the buffer would split into a
                         * bogus second line — refuse to guess (fail-closed,
                         * same class as the >16 submount cap) */
                        fprintf(stderr, "minict-spec: /proc/self/mountinfo "
                                "line exceeds %zu bytes — refusing to "
                                "enumerate :ro submounts\n", sizeof(line));
                        exit(125);
                }
                /* split off fields 1..4 (id, parent, maj:min, root) */
                for (k = 0; k < 4; k++) {
                        while (*p == ' ')
                                p++;
                        tok = p;
                        while (*p && *p != ' ')
                                p++;
                        if (!*p)
                                break;
                        *p++ = '\0';
                }
                if (k < 4)
                        continue; /* malformed line — skip */
                while (*p == ' ')
                        p++;
                tok = p;
                while (*p && *p != ' ')
                        p++;   /* field 5: mount point (escaped) */
                *p = '\0';
                if (mntinfo_decode(tok, mp, sizeof(mp)) < 0) {
                        fprintf(stderr, "minict-spec: mount point exceeds "
                                "%zu bytes — refusing to enumerate :ro "
                                "submounts\n", sizeof(mp));
                        exit(125);
                }

                for (i = 0; i < g_spec_bind_n; i++) {
                        struct spec_bind *b = &g_spec_bind[i];
                        struct stat sst;
                        size_t len = strlen(b->src);

                        if (!b->ro || b->src[0] != '/')
                                continue;
                        /* strictly below src (src itself = the top mount,
                         * already covered by the top remount) */
                        if (strncmp(mp, b->src, len) || mp[len] != '/')
                                continue;
                        /* only directory srcs can carry submounts */
                        if (stat(b->src, &sst) < 0 || !S_ISDIR(sst.st_mode))
                                continue;
                        /* cap check AFTER the prefix match: unrelated host
                         * mounts must not trigger the cap on a bind that
                         * already recorded its full 16 (SIA #114 fix) */
                        if (b->submount_n >= SPEC_SUBMOUNT_MAX) {
                                fprintf(stderr, "minict-spec: bind %s: "
                                        "more than %d submounts — refusing "
                                        "incomplete :ro (S3 cap)\n",
                                        b->src, SPEC_SUBMOUNT_MAX);
                                exit(125);
                        }
                        /* #117/D58: a src-relative submount suffix longer
                         * than the record cap would be silently truncated
                         * by snprintf → wrong path remounted later → a
                         * partial-ro config lie (worse than refusing — same
                         * class as the >16 submount cap above and the
                         * mp>1024 cap). Die fail-closed; the issue (#117,
                         * from-audit SIA PR #114 low) suggested a warning,
                         * but a die is consistent with the sibling caps and
                         * closes the silent-truncation hole outright. */
                        {
                                size_t suffix_len = strlen(mp + len);

                                if (suffix_len >= SPEC_SUBMOUNT_PATH) {
                                        fprintf(stderr, "minict-spec: bind %s: "
                                                "submount suffix %zu bytes "
                                                "exceeds %d cap — refusing "
                                                "truncated :ro (#117)\n",
                                                b->src, suffix_len,
                                                SPEC_SUBMOUNT_PATH);
                                        exit(125);
                                }
                                memcpy(b->submounts[b->submount_n],
                                       mp + len, suffix_len + 1);
                        }
                        b->submount_n++;
                }
        }
        fclose(f);

        for (i = 0; i < g_spec_bind_n; i++)
                g_spec_bind[i].mountinfo_ok = 1; /* enumeration succeeded */
}

/* Create <rootfs><rel> component by component WITHOUT following symlinks
 * (R7, integration review 2026-11-18). The rootfs is a private copy the
 * payload can write to and it PERSISTS across stop/start, so a payload can
 * plant `ln -s /home/u/.ssh /data`; before pivot_root an absolute symlink
 * resolves against the HOST root, and the old mkdir -p + open(O_CREAT) +
 * mount() then created files/dirs on the host and mounted outside the
 * rootfs. Every existing component must be a real directory (the last one
 * a dir or regular file matching the bind source); symlinks die(). No
 * TOCTOU: nothing runs inside this rootfs while the shim is being set up
 * (the previous payload's pidns is gone, a new one does not exist yet). */
static void spec_make_target(const char *rootfs, const char *rel,
                             int want_dir)
{
        char path[1024];
        size_t i, len = strlen(rootfs);
        struct stat st;

        if ((size_t)snprintf(path, sizeof(path), "%s%s", rootfs, rel) >=
            sizeof(path))
                die("spec path too long (build bug)");
        for (i = len + 1;; i++) {
                int last = (path[i] == '\0');

                if (path[i] != '/' && !last)
                        continue;
                path[i] = '\0';
                if (lstat(path, &st) == 0) {
                        if (S_ISLNK(st.st_mode)) {
                                fprintf(stderr, "minict-spec: refusing bind "
                                        "target with symlink component: "
                                        "%s\n", path);
                                exit(125);
                        }
                        if (!last && !S_ISDIR(st.st_mode)) {
                                fprintf(stderr, "minict-spec: bind target "
                                        "parent is not a directory: %s\n",
                                        path);
                                exit(125);
                        }
                } else if (errno == ENOENT) {
                        if (!last || want_dir) {
                                if (mkdir(path, 0755) < 0)
                                        die(path);
                        } else {
                                int fd = open(path, O_CREAT | O_EXCL |
                                              O_WRONLY | O_NOFOLLOW, 0644);

                                if (fd < 0)
                                        die(path);
                                close(fd);
                        }
                } else {
                        die(path);
                }
                if (last)
                        break;
                path[i] = '/';
        }
}

/* mkdir -p for paths INSIDE the new root (after pivot_root: the host is
 * unreachable, so following symlinks can only land inside the instance) */
static void spec_mkdir_p(const char *base, const char *rel)
{
        char path[1024];
        size_t i, len = strlen(base);

        if ((size_t)snprintf(path, sizeof(path), "%s%s", base, rel) >=
            sizeof(path))
                die("spec path too long (build bug)");
        for (i = len + 1; path[i]; i++) {
                if (path[i] == '/') {
                        path[i] = '\0';
                        mkdir(path, 0755);
                        path[i] = '/';
                }
        }
        mkdir(path, 0755);
}

/* Host paths are only visible between setup_dev_binds() and pivot_root —
 * the ONLY window where a bind into the rootfs can be established (same
 * reason the /dev binds happen there). A bind that fails to establish
 * means the container is silently NOT what the operator asked for, so
 * die() — unlike the tmpfs mounts below, which are additive service. */
static void apply_spec_binds(const char *rootfs)
{
        int i;

        for (i = 0; i < g_spec_bind_n; i++) {
                struct spec_bind *b = &g_spec_bind[i];
                char dst[1024], why[1280];
                struct stat sst;

                if ((size_t)snprintf(dst, sizeof(dst), "%s%s", rootfs,
                                     b->dst) >= sizeof(dst))
                        die("spec bind dst too long (build bug)");
                snprintf(why, sizeof(why), "spec bind %s -> %s%s", b->src,
                         b->dst, b->ro ? " (:ro)" : "");
                if (stat(b->src, &sst) < 0)
                        die(why);
                /* dir source -> dir target, anything else -> file target */
                spec_make_target(rootfs, b->dst, S_ISDIR(sst.st_mode));
                if (mount(b->src, dst, NULL, MS_BIND | MS_REC, NULL) < 0)
                        die(why);
                /* one mount() with MS_BIND|MS_RDONLY is NOT read-only: the
                 * kernel ignores MS_RDONLY on the bind step itself — a
                 * second bind-remount applies it (mount(2) semantics).
                 * S3 (issue #113, D52): MS_REC cloned the mounts BELOW the
                 * src into the sandbox; a top-only remount leaves them
                 * writable — measured 2026-11-22 (tmpfs submount under a
                 * :ro bind stayed rw). Enforce the WHOLE tree:
                 *   - kernel >= 5.12: mount_setattr(AT_RECURSIVE,
                 *     MOUNT_ATTR_RDONLY) — one syscall, VFS-locked walk,
                 *     no TOCTOU;
                 *   - ENOSYS (kernel < 5.12): per-submount bind-remounts
                 *     from the host-side mountinfo enumeration, then the
                 *     top. Any submount that cannot go ro -> die(): the
                 *     operator asked for ro; shipping silently-partial ro
                 *     is the exact "silently NOT what the operator asked
                 *     for" class binds already die() for. */
                if (b->ro) {
#ifdef __NR_mount_setattr
                        struct minict_mount_attr ma;

                        memset(&ma, 0, sizeof(ma));
                        ma.attr_set = MOUNT_ATTR_RDONLY;
                        if (syscall(__NR_mount_setattr, AT_FDCWD, dst,
                                    AT_RECURSIVE | AT_SYMLINK_NOFOLLOW,
                                    &ma, sizeof(ma)) == 0) {
                                /* whole tree (top + submounts) is ro */
                                continue;
                        }
                        if (errno != ENOSYS)
                                die(why); /* real failure, fail-closed */
                        /* ENOSYS: kernel < 5.12 — per-submount fallback */
#endif
                        if (!b->mountinfo_ok)
                                die("spec bind :ro: kernel lacks "
                                    "mount_setattr and /proc/self/mountinfo "
                                    "was unreadable — recursive ro cannot "
                                    "be guaranteed");
                        {
                                int j;

                                for (j = 0; j < b->submount_n; j++) {
                                        char sdst[1024];

                                        if ((size_t)snprintf(sdst,
                                                             sizeof(sdst),
                                                             "%s%s", dst,
                                                             b->submounts[j]) >=
                                            sizeof(sdst))
                                                die(why);
                                        if (mount(NULL, sdst, NULL,
                                                  MS_BIND | MS_REMOUNT |
                                                  MS_RDONLY, NULL) < 0)
                                                die(why);
                                }
                        }
                        if (mount(NULL, dst, NULL,
                                  MS_BIND | MS_REC | MS_REMOUNT | MS_RDONLY,
                                  NULL) < 0)
                                die(why);
                }
        }
}

/* extra tmpfs + /dev/shm (D23): same posture as the /tmp mount above —
 * warn, don't die, because mount(2) can be blocked by the HOST seccomp
 * filter in this environment (measured 2026-11-17). A missing tmpfs is
 * degraded service, not a silently-wrong container; binds above differ
 * because a missing bind IS one. Runs in the shim BEFORE relay spawn so
 * the relays see the same fs the payload will. */
static void apply_spec_tmpfs(void)
{
        int i;

        for (i = 0; i < g_spec_tmpfs_n; i++) {
                struct spec_tmpfs *t = &g_spec_tmpfs[i];
                char data[64];
                const char *dp = NULL;

                spec_mkdir_p("", t->path);
                if (t->size > 0) {
                        snprintf(data, sizeof(data), "size=%llu", t->size);
                        dp = data;
                }
                if (mount("tmpfs", t->path, "tmpfs", MS_NOSUID | MS_NODEV,
                          dp) < 0)
                        warn(t->path); /* context = the path + errno */
        }
        if (g_spec_shm_size > 0) {
                char data[64];

                mkdir("/dev/shm", 0755); /* new root may lack it */
                snprintf(data, sizeof(data), "size=%llu", g_spec_shm_size);
                if (mount("tmpfs", "/dev/shm", "tmpfs",
                          MS_NOSUID | MS_NODEV, data) < 0)
                        warn("spec mount /dev/shm");
        }
}

/* setrlimit BEFORE drop_all_caps(): raising the hard limit needs
 * CAP_SYS_RESOURCE and we hold the full set in our own userns until
 * capdrop — after it, limits can only be lowered. die() on failure: an
 * ulimit that silently didn't apply is a config lie. */
static void apply_spec_rlimits(void)
{
        int i;

        for (i = 0; i < g_spec_rlimit_n; i++) {
                struct rlimit rl;

                rl.rlim_cur = g_spec_rlimit[i].soft;
                rl.rlim_max = g_spec_rlimit[i].hard;
                if (setrlimit(g_spec_rlimit[i].res, &rl) < 0)
                        die("setrlimit (spec ulimit)");
        }
}

/* ---------- Landlock ro-root (GĐ2 mục 6, issue #149, D61) ----------
 *
 * spec.ReadOnly enforcement via Landlock (kernel >=5.13, rootless). The rootfs
 * "/" is granted read+execute only; "/tmp" and "/dev/shm" (tmpfs mounts) get
 * write accesses too — a Docker --read-only equivalent without overlayfs (D2
 * noted-only before). Defense-in-depth on top of pivot_root. ENOSYS (<5.13)
 * -> warn (fail-soft; pivot_root + capdrop + seccomp remain primary).
 *
 * Landlock rules: a dir rule grants accesses to its subtree (union of ancestor
 * rules); accesses NOT in handled_access_fs are unrestricted.
 */
#ifndef __NR_landlock_create_ruleset
#define __NR_landlock_create_ruleset 444
#endif
#ifndef __NR_landlock_add_rule
#define __NR_landlock_add_rule 445
#endif
#ifndef __NR_landlock_restrict_self
#define __NR_landlock_restrict_self 446
#endif
#define LL_RULE_PATH_BENEATH 1

#define LL_FS_EXECUTE      (1ULL << 0)
#define LL_FS_WRITE_FILE   (1ULL << 1)
#define LL_FS_READ_FILE    (1ULL << 2)
#define LL_FS_READ_DIR     (1ULL << 3)
#define LL_FS_REMOVE_DIR   (1ULL << 4)
#define LL_FS_REMOVE_FILE  (1ULL << 5)
#define LL_FS_MAKE_DIR     (1ULL << 7)
#define LL_FS_MAKE_REG     (1ULL << 8)
#define LL_FS_MAKE_SYM     (1ULL << 12)
#define LL_FS_REFER        (1ULL << 13)  /* rename/link across dirs + magic-link follow (5.19+) */
#define LL_ACCESS_RO (LL_FS_READ_FILE | LL_FS_READ_DIR | LL_FS_EXECUTE)
#define LL_ACCESS_RW (LL_ACCESS_RO | LL_FS_WRITE_FILE | LL_FS_REMOVE_FILE | \
                      LL_FS_REMOVE_DIR | LL_FS_MAKE_REG | LL_FS_MAKE_DIR | LL_FS_MAKE_SYM | \
                      LL_FS_REFER)
#define LL_HANDLED   (LL_FS_EXECUTE | LL_FS_WRITE_FILE | LL_FS_READ_FILE | LL_FS_READ_DIR | \
                      LL_FS_REMOVE_DIR | LL_FS_REMOVE_FILE | LL_FS_MAKE_DIR | LL_FS_MAKE_REG | \
                      LL_FS_MAKE_SYM | LL_FS_REFER)

struct ll_ruleset_attr { unsigned long long handled_access_fs; };
struct ll_path_beneath_attr { unsigned long long allowed_access_fs; int parent_fd; };

static int landlock_add_writable(int ruleset_fd, const char *path)
{
        struct ll_path_beneath_attr pb;
        int fd = open(path, O_RDONLY | O_CLOEXEC);

        if (fd < 0)
                return -1;
        pb.allowed_access_fs = LL_ACCESS_RW;
        pb.parent_fd = fd;
        if (syscall(__NR_landlock_add_rule, ruleset_fd, LL_RULE_PATH_BENEATH, &pb, 0) < 0) {
                int e = errno;

                close(fd);
                errno = e;
                return -1;
        }
        close(fd);
        return 0;
}

static void landlock_enforce(void)
{
        struct ll_ruleset_attr rsa;
        struct ll_path_beneath_attr pb;
        int rfd, rootfd;
        unsigned long long root_access;

        /* #154/D63: Landlock now runs for EVERY instance (default defense-in-
         * depth restrict-to-rootfs + spec.ReadOnly ro-root), not just spec.ReadOnly. */
        rsa.handled_access_fs = LL_HANDLED;
        rfd = syscall(__NR_landlock_create_ruleset, &rsa, sizeof(rsa), 0);
        if (rfd < 0) {
                if (g_spec_readonly) {
                        /* spec.ReadOnly is operator-requested ro-root. If Landlock is
                         * unavailable (kernel <5.13), refuse to boot half-protected —
                         * the operator asked for ro-root and can't get it. Fail-closed
                         * (die), consistent with bind :ro + seccomp posture. */
                        if (errno == ENOSYS || errno == EOPNOTSUPP)
                                die("spec.ReadOnly requires Landlock (kernel >=5.13) — unavailable, refusing to boot half-protected");
                        else
                                die("landlock_create_ruleset — spec.ReadOnly NOT enforced, refusing to boot");
                }
                /* default (no spec.ReadOnly): Landlock is defense-in-depth. ENOSYS ->
                 * warn (fail-soft; pivot_root+capdrop+seccomp remain primary). */
                if (errno == ENOSYS || errno == EOPNOTSUPP)
                        warn("Landlock unavailable (kernel <5.13) — default fs-policy NOT enforced (defense-in-depth, fail-soft)");
                else
                        warn("landlock_create_ruleset — default fs-policy NOT enforced");
                return;
        }
        rootfd = open("/", O_RDONLY | O_CLOEXEC);
        if (rootfd < 0) {
                warn("open / for Landlock");
                close(rfd);
                return;
        }
        /* "/" rule: spec.ReadOnly -> read-only; default -> all accesses (rw,
         * defense-in-depth: payload restricted to rootfs subtree, host denied). */
        root_access = g_spec_readonly ? LL_ACCESS_RO : LL_ACCESS_RW;
        pb.allowed_access_fs = root_access;
        pb.parent_fd = rootfd;
        if (syscall(__NR_landlock_add_rule, rfd, LL_RULE_PATH_BENEATH, &pb, 0) < 0) {
                warn("landlock_add_rule /");
                close(rootfd);
                close(rfd);
                return;
        }
        close(rootfd);
        if (g_spec_readonly) {
                /* "/tmp" + "/dev/shm" writable (tmpfs mounts) */
                landlock_add_writable(rfd, "/tmp");
                landlock_add_writable(rfd, "/dev/shm");
                /* bind mounts writable: residual — not tracked per-bind; writable
                 * binds inherit "/" read-only. Known limitation (document D61). */
        }
        /* default: "/" LL_ACCESS_RW covers /tmp etc (no extra rules needed). */
        if (syscall(__NR_landlock_restrict_self, rfd, 0) < 0)
                warn("landlock_restrict_self — fs-policy NOT enforced");
        close(rfd);
}

/* ---- payload fd + core-dump hardening (issue #5, D33) ----
 *
 * Two leak classes closed BEFORE the payload exec:
 *
 * 1. Inherited fds: everything the launcher's caller (shell, daemon, IDE)
 *    holds open with no FD_CLOEXEC survives fork()+execvp() into the
 *    payload — an fd pointing at a HOST file then bypasses pivot_root
 *    entirely (openat/fchdir/read via the fd need no privilege at all).
 *    The TOCTOU class this enables is CVE-2018-15664-shaped: an fd opened
 *    before a check, used after. The payload contract is stdio (0/1/2)
 *    only — minict-exec additionally documents its stdin passthrough as
 *    fd 0 — so closing 3..max is strictly narrowing. close_range(2)
 *    (Linux 5.9, syscall 436) does it kernel-side; on older kernels fall
 *    back to a bounded close loop (best effort, errors ignored per fd).
 *
 * 2. Core dumps: RLIMIT_CORE is left at the caller's value, so a payload
 *    crash would write a core FILE of the sandbox process into the rootfs
 *    (disk fill + memory image outside tmpfs). Force RLIMIT_CORE to 0
 *    before exec. Residual, documented in D33: core(5) — "RLIMIT_CORE will
 *    be ignored if the system is configured to pipe core dumps to a
 *    program" — a piped host handler still receives the (payload-owned)
 *    memory; core_pattern itself is unreachable from the payload (no
 *    /proc, AL4) and GĐ2 owns host-side pattern policy.
 */
#ifndef SYS_close_range
#define SYS_close_range 436 /* Linux 5.9 (x86_64) */
#endif
#define CLOSE_LOOP_MAX 65536 /* fallback bound; fd tables are way below */
static void harden_payload_fds(void)
{
        struct rlimit core0 = { 0, 0 };

        if (setrlimit(RLIMIT_CORE, &core0) < 0)
                die("setrlimit(RLIMIT_CORE, 0)");
        if (syscall(SYS_close_range, 3, ~0U, 0) == 0)
                return;
        /* ENOSYS path (kernel < 5.9): bounded loop, per-fd errors ignored */
        {
                struct rlimit rl;
                rlim_t hi;

                if (getrlimit(RLIMIT_NOFILE, &rl) < 0)
                        rl.rlim_max = 1024;
                hi = rl.rlim_max;
                if (hi > CLOSE_LOOP_MAX)
                        hi = CLOSE_LOOP_MAX;
                {
                        rlim_t fd;

                        for (fd = 3; fd < hi; fd++)
                                close((int)fd);
                }
        }
}

/* ------------------------------------------------------------------ *
 * seccomp-bpf blocklist (issue #2, decision D11; AF_NETLINK D15).
 *
 * Installed in the sandboxed child AFTER capdrop + NNP, BEFORE exec.
 * Strategy: blocklist of known-dangerous syscalls (default ALLOW) —
 * NOT an allowlist: busybox httpd/sh need a broad syscall set and a
 * wrong allowlist entry turns into a runtime outage (see D11 for the
 * trade-off and the measured overhead).
 *
 * Blocked (flat): ptrace, mount/umount2/swapon/swapoff/reboot, module
 * loading, keyctl/add_key/request_key, kexec, bpf, perf_event_open,
 * name/open_by_handle_at, setns, unshare, process_vm_*, userfaultfd,
 * io_uring_* (2026-08: ring ops bypass per-syscall filtering entirely),
 * acct, _sysctl, lookup_dcookie, remap_file_pages, iopl/ioperm.
 * Arg-checked: clone() with any CLONE_NEW* flag (no nested namespaces),
 * socket(AF_ALG) (userspace crypto interface — issue #8 mitigation),
 * socket(AF_NETLINK) (netlink is the nftables/ebtables/audit config
 * surface — netns without a netlink lock is half-done, D15),
 * clone3 -> ENOSYS (glibc-style fallbacks then use clone(), which IS
 * arg-checked above).
 *
 * Blocked calls return EPERM (fail-closed, predictable, testable);
 * MINICT_NO_SECCOMP=1 skips the filter (A/B overhead measurement).
 * Syscall numbers are the fixed x86_64 ABI (this sandbox is x86_64
 * only; the arch check makes the filter kill any other-ABI caller).
 * ------------------------------------------------------------------ */

#ifndef BPF_STMT
#define BPF_STMT(code, k) { (unsigned short)(code), 0, 0, k }
#endif
#ifndef BPF_DT
#define BPF_DT(code, jt, jf, k) { (unsigned short)(code), jt, jf, k }
#endif

enum {
        NR_socket = 41,
        NR_clone = 56,
        NR_ptrace = 101,
        NR__sysctl = 156,
        NR_acct = 163,
        NR_mount = 165,
        NR_umount2 = 166,
        NR_swapon = 167,
        NR_swapoff = 168,
        NR_reboot = 169,
        NR_iopl = 172,
        NR_ioperm = 173,
        NR_create_module = 174,
        NR_init_module = 175,
        NR_delete_module = 176,
        NR_nfsservctl = 180,
        NR_lookup_dcookie = 212,
        NR_remap_file_pages = 216,
        NR_kexec_load = 246,
        NR_add_key = 248,
        NR_request_key = 249,
        NR_keyctl = 250,
        NR_unshare = 272,
        NR_perf_event_open = 298,
        NR_name_to_handle_at = 303,
        NR_open_by_handle_at = 304,
        NR_finit_module = 313,
        NR_setns = 308,
        NR_process_vm_readv = 310,
        NR_process_vm_writev = 311,
        NR_bpf = 321,
        NR_userfaultfd = 323,
        NR_clone3 = 435,
        NR_io_uring_setup = 425,
        NR_io_uring_enter = 426,
        NR_io_uring_register = 427,
};

static const unsigned short g_blocklist[] = {
        NR_ptrace, NR__sysctl, NR_acct, NR_mount, NR_umount2, NR_swapon,
        NR_swapoff, NR_reboot, NR_iopl, NR_ioperm, NR_create_module,
        NR_init_module, NR_finit_module, NR_delete_module, NR_nfsservctl,
        NR_lookup_dcookie, NR_remap_file_pages, NR_kexec_load, NR_add_key,
        NR_request_key, NR_keyctl, NR_unshare,
        NR_perf_event_open, NR_name_to_handle_at, NR_open_by_handle_at,
        NR_setns, NR_process_vm_readv, NR_process_vm_writev, NR_bpf,
        NR_userfaultfd, NR_io_uring_setup, NR_io_uring_enter,
        NR_io_uring_register,
};

/* any namespace-creation flag on clone() is denied (fits in 32 bits) */
#define CLONE_NS_MASK (CLONE_NEWNS | CLONE_NEWUTS | CLONE_NEWIPC | \
                       CLONE_NEWUSER | CLONE_NEWPID | CLONE_NEWNET | \
                       CLONE_NEWCGROUP)

#define SECCOMP_EPERM (SECCOMP_RET_ERRNO | (unsigned int)EPERM)
#define SECCOMP_ENOSYS (SECCOMP_RET_ERRNO | (unsigned int)ENOSYS)

static void install_seccomp(void)
{
        struct sock_filter prog[1280];
        struct sock_fprog fprog;
        size_t i, n = 0;

        if (getenv("MINICT_NO_SECCOMP")) {
                warn("seccomp disabled via MINICT_NO_SECCOMP");
                return;
        }

        /* load arch; anything but x86_64 never gets to run */
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 4);
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               1, 0, AUDIT_ARCH_X86_64);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                 SECCOMP_RET_KILL_THREAD);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 0);

        /* flat blocklist: JEQ nr -> RET EPERM (2 instructions per entry) */
        for (i = 0; i < sizeof(g_blocklist) / sizeof(g_blocklist[0]); i++) {
                prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                                       0, 1, g_blocklist[i]);
                prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                         SECCOMP_EPERM);
        }

        /* clone(2): deny any namespace-creating flag (args[0] low 32 bits) */
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               0, 3, NR_clone);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 16);
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JSET | BPF_K,
                                               0, 1, (unsigned int)CLONE_NS_MASK);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K, SECCOMP_EPERM);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 0);

        /* socket(2): deny AF_ALG (38) — userspace crypto interface */
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               0, 3, NR_socket);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 16);
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               0, 1, 38);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K, SECCOMP_EPERM);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 0);

        /* socket(2): deny AF_NETLINK (16) — nftables/ebtables/audit config
         * surface; the netns stack stays read-only-by-design (D15) */
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               0, 3, NR_socket);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 16);
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               0, 1, 16);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K, SECCOMP_EPERM);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, 0);

        /* clone3: ENOSYS so libc-style fallbacks retry via clone(2), which
         * IS arg-checked above; everything else is allowed */
        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                               0, 1, NR_clone3);
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                 SECCOMP_ENOSYS);

        if (g_allowlist_n) {
                /* allowlist mode (D59): a syscall must be listed to pass —
                 * the floor and the arg-checks above still win */
                for (i = 0; i < g_allowlist_n; i++) {
                        prog[n++] = (struct sock_filter)BPF_DT(BPF_JMP | BPF_JEQ | BPF_K,
                                                               0, 1,
                                                               g_allowlist[i]);
                        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                                 SECCOMP_RET_ALLOW);
                }
                prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                         SECCOMP_EPERM);
        } else {
                prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                         SECCOMP_RET_ALLOW);
        }

        if (n > sizeof(prog) / sizeof(prog[0]))
                die("seccomp program overflow (build bug)");

        fprog.len = (unsigned short)n;
        fprog.filter = prog;
        /* requires NO_NEW_PRIVS (set before unshare, inherited here);
         * kernel validates the whole program atomically — fail closed */
        if (prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &fprog, 0, 0) < 0)
                die("prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER)");
}

static volatile sig_atomic_t g_child_pid = 0;
static sigset_t g_parent_sig_old;  /* #30/D62: parent mask saved around pidns fork */

/* ---------- pidns-1 signal shim (issue #6, D19) ----------
 *
 * ROOT CAUSE of issue #6 (session 7, 2026-11-17): the payload used to be
 * exec'd DIRECTLY as PID 1 of the new pidns. pid_namespaces(7):
 *   "a process in an ancestor namespace can ... send signals to the 'init'
 *   process of a child PID namespace only if the 'init' process has
 *   established a handler for that signal ... SIGKILL or SIGSTOP are
 *   treated exceptionally: these signals are forcibly delivered when sent
 *   from an ancestor PID namespace."
 * A default-disposition SIGTERM (kill() rc=0, no delivery — measured day 1,
 * A/B/C matrix) is therefore KERNEL BEHAVIOR, not the host-seccomp
 * hypothesis of D4. The D4 fail-safe layer stays (see below).
 *
 * FIX (tini/runc pattern): a tiny in-ns supervisor stays as pidns-1,
 * INSTALLS handlers for the forwardable termination signals, and fork+execs
 * the real payload as its child. With a handler != SIG_DFL the kernel
 * DELIVERS the signal; the shim forwards it to the payload and mirrors the
 * exit status (signal death -> exit(128+sig), the bash convention —
 * observable parity without relying on self-re-raise through the pidns-init
 * UNKILLABLE semantics).
 *
 * Forwarded: TERM INT HUP QUIT USR1 USR2 WINCH. SIGKILL/SIGSTOP cannot be
 * caught (kernel-forced from the ancestor ns) — D4 teardown unchanged and
 * still guaranteed: the shim keeps PR_SET_PDEATHSIG and is still pidns-1,
 * so its death still SIGKILLs everything left in the namespace.
 *
 * The host-side wrapper now signals the SHIM PID only (docker semantics:
 * signal PID 1, PID 1 manages its tree) — the old group-kill(-pgid) would
 * hit the payload twice (once via the group, once via the shim forward).
 * Payload-tree members are covered by the pidns teardown when the shim
 * exits. The shim deliberately gets NO seccomp filter (same posture as the
 * web relay, D16: forked before install, never execs, caps dropped).
 */

static volatile sig_atomic_t g_payload_pid = 0;
static volatile sig_atomic_t g_payload_done = 0;
static volatile sig_atomic_t g_payload_status = 0;

/* async-signal-safe forwarder (kill(2) is AS-safe per signal-safety(7)) */
static void fwd_to_payload(int sig)
{
        if (g_payload_pid > 0)
                kill((pid_t)g_payload_pid, sig);
}

/* async-signal-safe reaper (SIGCHLD): every orphaned grandchild re-parents
 * to pidns-1 = US, so its SIGCHLD fires here; reap every available child
 * (tini pattern) so a long-running instance does not accumulate zombies,
 * and flag the main loop when the PAYLOAD itself exited so its status is
 * mirrored exactly once (audit 2026-11-17, medium — SIG5 test).
 * errno is saved/restored: sigsuspend() in the main loop must still see
 * the kernel's own errno, not one clobbered by this handler. */
static void shim_reap(int sig)
{
        int saved_errno = errno;

        (void)sig;
        for (;;) {
                int st;
                pid_t w = waitpid(-1, &st, WNOHANG);

                if (w > 0) {
                        if (w == (pid_t)g_payload_pid) {
                                g_payload_status = st;
                                g_payload_done = 1;
                        }
                        continue;
                }
                if (w < 0 && errno == EINTR)
                        continue;
                break; /* 0 = children alive but none dead; ECHILD = none */
        }
        errno = saved_errno;
}

/* shim event loop: orphan grandchildren are reaped by shim_reap() as they
 * die; we only block until the PAYLOAD exits, then mirror its status.
 * never returns. */
static void shim_wait_loop(void)
{
        sigset_t empty;

        sigemptyset(&empty);
        while (!g_payload_done) {
                /* sleep until any signal fires (sigsuspend unblocks all
                 * signals atomically); the SIGCHLD handler does the work */
                (void)sigsuspend(&empty);
        }
        if (WIFEXITED((int)g_payload_status))
                _exit(WEXITSTATUS((int)g_payload_status));
        if (WIFSIGNALED((int)g_payload_status))
                _exit(128 + WTERMSIG((int)g_payload_status));
        _exit(127);
}

/* host-side forwarder: signal the pidns-1 SHIM pid (docker semantics —
 * signal PID 1; the shim forwards to the payload, D19). A group-wide kill
 * (-pgid) would double-signal the payload: once via the group, once via the
 * shim forward. Installed before the pidns fork so the shim/payload chain
 * inherits CAUGHT dispositions, not the launcher's SIG_IGN (SIG3). */
static void fwd_sig(int sig)
{
        if (g_child_pid > 0)
                kill((pid_t)g_child_pid, sig);
}

int main(int argc, char **argv)
{
        uid_t ruid;
        gid_t rgid;
        char olddir[256], path[512];
        struct stat st;
        pid_t pid;

        if (argc < 3) {
                fprintf(stderr, "usage: minict <rootfs-dir> <cmd> [args...]\n");
                return 125;
        }
        const char *rootfs = argv[1];
        char **cmd = &argv[2];

        /* capture real ids BEFORE unshare: getuid() reads 65534 (nobody)
         * between unshare(CLONE_NEWUSER) and the uid_map write */
        ruid = getuid();
        rgid = getgid();

        /* rootfs sanity check (host-side, DAC-checked against real uid).
         * D46 (issue #88): require an executable /bin/sh instead of
         * /bin/busybox — Debian/glibc rootfs ship no busybox, and /bin/sh is
         * what the wrapper always execs (minict.sh default payload). Busybox
         * and alpine rootfs still pass (both have /bin/sh). /bin/sh may be a
         * symlink (alpine: /bin/sh -> /bin/busybox, ABSOLUTE target — valid
         * only INSIDE the rootfs, dangling on the host), so resolve it
         * relative to the rootfs root before the check — same rule as the
         * wrapper's fail-closed check (D43). */
        snprintf(path, sizeof(path), "%s/bin/sh", rootfs);
        {
                /* jbuf sized for the worst join: rootfs(511) + "/bin/"(5) +
                 * lpath(511) + NUL = 1028 — SCR R2 builds with -Werror and
                 * gcc format-truncation proves lpath (known 512 buffer) */
                char lpath[512], jbuf[1100];
                int depth;
                /* SIA audit PR #89 (medium): symlink loops are possible in a
                 * user-provided rootfs (bin/sh -> sh or absolute cycles) —
                 * resolve with a HARD CAP and fail closed; never stat a
                 * stale intermediate symlink (that one used to be reached by
                 * the too-long break). The final lstat+S_ISREG below rejects
                 * any remaining symlink, so only a real executable file
                 * passes. */
                for (depth = 0; depth < 8; depth++) {
                        ssize_t n = readlink(path, lpath, sizeof(lpath) - 1);
                        if (n < 0)
                                break; /* regular file (or ELOOP): lstat below */
                        lpath[n] = '\0';
                        if (lpath[0] == '/')
                                snprintf(jbuf, sizeof(jbuf), "%s%s", rootfs,
                                         lpath);
                        else
                                snprintf(jbuf, sizeof(jbuf), "%s/bin/%s",
                                         rootfs, lpath);
                        if (strlen(jbuf) >= sizeof(path))
                                break; /* too long: lstat fails below */
                        memcpy(path, jbuf, strlen(jbuf) + 1);
                }
                if (depth >= 8) {
                        fprintf(stderr,
                                "minict: /bin/sh symlink chain too deep in %s"
                                " (cap 8 — loop or abuse?)\n",
                                rootfs);
                        return 125;
                }
        }
        /* lstat (not stat): a leftover symlink must NOT pass — SIA PR #89 */
        if (lstat(path, &st) < 0 || access(path, X_OK) != 0 ||
            !S_ISREG(st.st_mode)) {
                fprintf(stderr,
                        "minict: no executable /bin/sh in %s (build a rootfs "
                        "with mkrootfs.sh or pass a usable --rootfs)\n",
                        rootfs);
                return 125;
        }

        /* no new privileges before anything else; inherited across fork/exec */
        if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0)
                die("PR_SET_NO_NEW_PRIVS");

        load_web_relay_env();
        /* MINICT_SPEC (D23): read+parse host-side BEFORE unshare — DAC
         * against the real uid, and parse errors abort before any ns work */
        load_spec_file();

        /* one-shot namespace creation; netns ON by default (D17), fail-closed
         * — no silent fallback to the shared host network namespace */
        if (unshare(CLONE_NEWUSER | CLONE_NEWNS | CLONE_NEWPID |
                    CLONE_NEWUTS | CLONE_NEWIPC |
                    (want_netns() ? CLONE_NEWNET : 0)) < 0)
                die("unshare(user|mount|pid|uts|ipc|net)");

        map_ids(ruid, rgid);

        /* loopback comes up DOWN in a fresh netns; bring it up NOW — while we
         * still hold caps in the userns that owns it (after uid_map: with an
         * empty map we hold no caps at all, measured 2026-11-17 — D18) */
        if (want_netns())
                bring_lo_up();

        /* private propagation so nothing leaks to/from the host mount ns */
        if (mount(NULL, "/", NULL, MS_REC | MS_PRIVATE, NULL) < 0)
                die("mount(/ MS_REC|MS_PRIVATE)");

        /* rootfs must be a mount point for pivot_root: bind it onto itself */
        if (mount(rootfs, rootfs, NULL, MS_BIND | MS_REC, NULL) < 0)
                die("bind rootfs onto itself");
        if (mount(NULL, rootfs, NULL, MS_REC | MS_PRIVATE, NULL) < 0)
                die("rootfs MS_REC|MS_PRIVATE");

        setup_dev_binds(rootfs);

        /* spec binds (D23): host paths are still visible here, and the
         * mounts must exist BEFORE pivot_root — this window is the only
         * chance (same reasoning as setup_dev_binds above) */
        apply_spec_binds(rootfs);

        /* unique put_old dir per invocation => safe concurrent instances */
        snprintf(olddir, sizeof(olddir), "%s/old-%d", rootfs, (int)getpid());
        mkdir(olddir, 0700);

        if (chdir(rootfs) < 0)
                die("chdir(rootfs)");

        /* pivot: new root = rootfs (cwd), old host root parked at ./old-<pid>;
         * put_old is given relative to cwd, same mount as required by pivot(2) */
        const char *rel = olddir + strlen(rootfs) + 1; /* "old-<pid>" */
        if (syscall(SYS_pivot_root, ".", rel) < 0)
                die("pivot_root");

        /* from here the host filesystem is unreachable; re-anchor at new / */
        if (chdir("/") < 0)
                die("chdir(/)");

        /* The old (host) root is detached by the pidns-1 child right after
         * it mounts /proc (issue #31, D39): the kernel only lets a userns
         * mount procfs while a FULLY VISIBLE procfs already exists in the
         * mount ns (fs/namespace.c mount_too_revealing ->
         * mnt_already_visible). Detaching first left none -> EPERM on every
         * kernel. Nothing in this process touches the filesystem between
         * here and the fork, and the payload is only forked after the
         * child has detached the old root (fail-closed there). */

        /* forwarders BEFORE the pidns fork: the shim and the payload
         * inherit these dispositions. Installing them only after the fork
         * (the old order) leaked the launcher's SIG_IGN into the shim:
         * a background job started by a non-interactive shell has
         * SIGINT/SIGQUIT set to SIG_IGN (POSIX "asynchronous list" rule),
         * and SIG_IGN SURVIVES fork+exec — the payload could then never
         * receive SIGINT (SIG3 failure, session 7). The shim still installs
         * its own handlers on top; here we only fix the inherited state.
         * fwd_sig is a no-op until g_child_pid is set below. */
        signal(SIGINT, fwd_sig);
        signal(SIGTERM, fwd_sig);

/* #30/D62: block the forwardable signals in the PARENT around the pidns
 * fork so a SIGTERM/SIGINT arriving between fork() and g_child_pid being
 * set (below) is QUEUED, not delivered to the no-op fwd_sig (g_child_pid=0)
 * and swallowed. The child inherits the blocked mask; the UNBLOCK restores
 * below (shim + payload) give them clean masks. */
        {
                sigset_t pfwd;

                sigemptyset(&pfwd);
                sigaddset(&pfwd, SIGINT);
                sigaddset(&pfwd, SIGTERM);
                if (sigprocmask(SIG_BLOCK, &pfwd, &g_parent_sig_old) < 0)
                        die("sigprocmask(SIG_BLOCK parent)");
        }
        pid = fork();
        if (pid < 0)
                die("fork (enter pidns)");

        if (pid == 0) {
                /* ---- container side: pidns-1 = signal shim (D19) ---- */
                sigset_t fwd, old;
                pid_t payload;
                struct sigaction sa;

                setpgid(0, 0); /* own process group for tree-wide signals */
                if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
                        warn("PR_SET_PDEATHSIG");
                if (sethostname(g_hostname, strlen(g_hostname)) < 0)
                        warn("sethostname");
                /* procfs of THIS pidns, mounted while the host procfs is
                 * still visible under the parked old root (#31, D39).
                 * nosuid/nodev/noexec like every container runtime. Still
                 * warn-only: a host seccomp filter or a host with masked
                 * /proc (overmounted paths = not fully visible) refuses it,
                 * and a sandbox without /proc is degraded, not unsafe. */
                if (mkdir("/proc", 0555) < 0 && errno != EEXIST)
                        warn("mkdir /proc");
                if (mount("proc", "/proc", "proc",
                          MS_NOSUID | MS_NODEV | MS_NOEXEC, NULL) < 0)
                        warn("mount /proc");

                /* now detach + forget the old (host) root: fail closed —
                 * the payload must never start with the host root
                 * reachable */
                if (umount2(rel, MNT_DETACH) < 0)
                        die("umount2(old root) — refusing to continue with "
                            "host root still visible");
                if (rmdir(rel) < 0)
                        warn("rmdir(old)");
                if (mount("tmpfs", "/tmp", "tmpfs", MS_NOSUID | MS_NODEV,
                          NULL) < 0)
                        warn("mount /tmp (child)");
                apply_spec_tmpfs(); /* spec tmpfs + /dev/shm (D23) */

                /* in-sandbox unix-socket relays BEFORE capdrop/seccomp so
                 * they can bind the sockets and (not) inherit the filter
                 * (D16 env pair + D24 spec webports) */
                spawn_web_relay();

                /* block the forwardable signals around the payload fork so
                 * one racing fork->g_payload_pid is QUEUED, not lost; they
                 * are delivered right after the handlers are installed.
                 * SIGCHLD joins the set: without it, a payload dying between
                 * fork() and the shim_reap() install below would lose its
                 * only SIGCHLD (default disposition is ignore) and the wait
                 * loop would hang forever (lost wakeup) */
                sigemptyset(&fwd);
                sigaddset(&fwd, SIGTERM);
                sigaddset(&fwd, SIGINT);
                sigaddset(&fwd, SIGHUP);
                sigaddset(&fwd, SIGQUIT);
                sigaddset(&fwd, SIGUSR1);
                sigaddset(&fwd, SIGUSR2);
                sigaddset(&fwd, SIGWINCH);
                sigaddset(&fwd, SIGCHLD);
                if (sigprocmask(SIG_BLOCK, &fwd, &old) < 0)
                        die("sigprocmask(SIG_BLOCK)");

                payload = fork();
                if (payload < 0)
                        die("fork (payload)");

                if (payload == 0) {
                        /* ---- payload: plain child, normal signal rules;
                         * NOT pidns-1 anymore, so default-disposition
                         * signals from the ancestor namespace ARE delivered
                         * (pid_namespaces(7) rule only targets init) ---- */
                        if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
                                warn("payload PDEATHSIG");
                        /* a blocked mask SURVIVES exec(2) — restore a CLEAN mask
 * (UNBLOCK the fwd set). #30/D62: the parent blocks SIGTERM/SIGINT around
 * the pidns fork; the child inherits that, so restoring &old (which
 * captures the inherited block) would keep them blocked. UNBLOCK fwd
 * gives the payload the clean default mask it expects. */
                        if (sigprocmask(SIG_UNBLOCK, &fwd, NULL) < 0)
                                _exit(126);
                        /* so do signal DISPOSITIONS: reset everything the
                         * launcher/shim chain may have left ignored or
                         * caught (SIG_IGN survives exec — SIG3 lesson) to
                         * the defaults a bare command would see */
                        {
                                static const int rst[] = { SIGHUP, SIGINT,
                                        SIGQUIT, SIGTERM, SIGUSR1,
                                        SIGUSR2, SIGWINCH };
                                size_t k;

                                for (k = 0;
                                     k < sizeof(rst) / sizeof(rst[0]);
                                     k++)
                                        signal(rst[k], SIG_DFL);
                        }

                        harden_payload_fds(); /* fd leak + core dump (D33) */

                        apply_spec_rlimits(); /* setrlimit BEFORE capdrop (D23) */

                        drop_all_caps(); /* after ALL privileged work is done */

                        install_seccomp(); /* last lock: blocklist, before exec */

                        landlock_enforce(); /* GĐ2 mục 6: ro-root if spec.ReadOnly + kernel >=5.13 */

                        setenv("PATH", "/bin:/sbin:/usr/bin", 1);
                        setenv("HOME", "/", 1);
                        setenv("PS1", "minict> ", 1);
                        setenv("MINICT", "1", 1);

                        {
                                int e;

                                /* spec env LAST with overwrite=1: the
                                 * operator's spec wins over the defaults
                                 * above (D23) */
                                for (e = 0; e < g_spec_env_n; e++) {
                                        char *eq = strchr(g_spec_env[e], '=');

                                        *eq = '\0'; /* parser guarantees '=' */
                                        setenv(g_spec_env[e], eq + 1, 1);
                                        *eq = '=';
                                }
                        }

                        execvp(cmd[0], cmd);
                        fprintf(stderr, "minict: exec %s: %s\n", cmd[0],
                                strerror(errno));
                        _exit(127);
                }

                /* ---- shim continues as pidns-1 (never execs) ---- */
                g_payload_pid = payload;
                drop_all_caps(); /* relay posture (D16/D19), nothing to run */

                memset(&sa, 0, sizeof(sa));
                sa.sa_handler = fwd_to_payload;
                if (sigaction(SIGTERM, &sa, NULL) < 0 ||
                    sigaction(SIGINT, &sa, NULL) < 0 ||
                    sigaction(SIGHUP, &sa, NULL) < 0 ||
                    sigaction(SIGQUIT, &sa, NULL) < 0 ||
                    sigaction(SIGUSR1, &sa, NULL) < 0 ||
                    sigaction(SIGUSR2, &sa, NULL) < 0 ||
                    sigaction(SIGWINCH, &sa, NULL) < 0)
                        die("sigaction(shim)");
                sa.sa_handler = shim_reap;
                sa.sa_flags = SA_RESTART | SA_NOCLDSTOP;
                if (sigaction(SIGCHLD, &sa, NULL) < 0)
                        die("sigaction(SIGCHLD shim)");

                /* pending signals queued during the fork window fire NOW,
                 * with g_payload_pid already set (race closed) — including
                 * the queued SIGCHLD of an early payload death */
                /* #30/D62: UNBLOCK the fwd set (not restore &old) so the shim's
 * fwd_to_payload handlers actually receive the signals queued during the
 * fork window. &old captures the parent's inherited SIGTERM/SIGINT block;
 * restoring it would leave them blocked. */
                if (sigprocmask(SIG_UNBLOCK, &fwd, NULL) < 0)
                        die("sigprocmask(SIG_UNBLOCK)");

                shim_wait_loop(); /* never returns */
        }

        /* ---- host side: supervisor, no extra privileges ---- */
        g_child_pid = pid;
/* #30/D62: unblock the signals blocked around the pidns fork — pending
 * SIGTERM/SIGINT now fires fwd_sig with g_child_pid set (forwarded to the
 * shim). Race closed. */
        if (sigprocmask(SIG_SETMASK, &g_parent_sig_old, NULL) < 0)
                die("sigprocmask(SIG_SETMASK parent)");
        setpgid(pid, pid); /* also from parent to avoid the pgid race */
        /* (INT/TERM forwarders installed BEFORE the fork — see above) */

        int status;
        while (waitpid(pid, &status, 0) < 0) {
                if (errno != EINTR)
                        die("waitpid");
        }
        if (WIFEXITED(status))
                return WEXITSTATUS(status);
        if (WIFSIGNALED(status))
                return 128 + WTERMSIG(status);
        return 1;
}
