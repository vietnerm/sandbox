/*
 * minict — minimal rootless container (PoC day 2026-11-17)
 *
 * Stack: user ns + mount ns + pid ns + uts ns + ipc ns + NET ns (issue #3),
 *        self-written uid_map, pivot_root + MNT_DETACH of old root,
 *        bind-mounted /dev bits, capability drop + PR_SET_NO_NEW_PRIVS,
 *        seccomp-bpf blocklist (incl. AF_NETLINK — D15). No Docker, no root.
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
#include <errno.h>
#include <fcntl.h>
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
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

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

static void spawn_web_relay(void)
{
        pid_t p = fork();

        if (p < 0)
                die("fork (web relay)");
        if (p == 0) {
                if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
                        warn("relay PDEATHSIG");
                relay_main(g_web_port, g_proxy_sock); /* never returns */
                _exit(123);
        }
        /* parent continues: capdrop -> seccomp -> exec payload */
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
        struct sock_filter prog[160];
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
        prog[n++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K,
                                                 SECCOMP_RET_ALLOW);

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

/* child runs in its own process group => signal hits the whole tree */
static void fwd_sig(int sig)
{
        if (g_child_pid > 0)
                kill(-(pid_t)g_child_pid, sig);
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

        /* rootfs sanity check (host-side, DAC-checked against real uid) */
        snprintf(path, sizeof(path), "%s/bin/busybox", rootfs);
        if (stat(path, &st) < 0) {
                fprintf(stderr, "minict: no busybox in %s (run mkrootfs.sh)\n",
                        rootfs);
                return 125;
        }

        /* no new privileges before anything else; inherited across fork/exec */
        if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0)
                die("PR_SET_NO_NEW_PRIVS");

        load_web_relay_env();

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

        /* detach + forget the old (host) root: fail closed on error */
        if (umount2(rel, MNT_DETACH) < 0)
                die("umount2(old root) — refusing to continue with host root "
                    "still visible");
        if (rmdir(rel) < 0)
                warn("rmdir(old)");

        pid = fork();
        if (pid < 0)
                die("fork (enter pidns)");

        if (pid == 0) {
                /* ---- container side: we are PID 1 of the new pidns ---- */
                setpgid(0, 0); /* own process group for tree-wide signals */
                if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
                        warn("PR_SET_PDEATHSIG");
                if (sethostname("minict", 6) < 0)
                        warn("sethostname");
                /* /proc is blocked by the HOST seccomp filter in this env;
                 * on a normal kernel this succeeds and gives pidns proc */
                if (mount("proc", "/proc", "proc", 0, NULL) < 0)
                        warn("mount /proc (blocked in this environment)");
                if (mount("tmpfs", "/tmp", "tmpfs", MS_NOSUID | MS_NODEV,
                          NULL) < 0)
                        warn("mount /tmp (child)");

                /* in-sandbox unix-socket relay BEFORE capdrop/seccomp so it
                 * can bind the socket and (not) inherit the filter (D16) */
                if (g_web_port > 0)
                        spawn_web_relay();

                drop_all_caps(); /* after ALL privileged-in-ns work is done */

                install_seccomp(); /* last lock: syscall blocklist, before exec */

                setenv("PATH", "/bin:/sbin:/usr/bin", 1);
                setenv("HOME", "/", 1);
                setenv("PS1", "minict> ", 1);
                setenv("MINICT", "1", 1);

                execvp(cmd[0], cmd);
                fprintf(stderr, "minict: exec %s: %s\n", cmd[0],
                        strerror(errno));
                _exit(127);
        }

        /* ---- host side: supervisor, no extra privileges ---- */
        g_child_pid = pid;
        setpgid(pid, pid); /* also from parent to avoid the pgid race */
        signal(SIGINT, fwd_sig);
        signal(SIGTERM, fwd_sig);

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
