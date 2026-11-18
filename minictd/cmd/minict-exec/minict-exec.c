/*
 * minict-exec — exec into a RUNNING minict instance via setns (D5 gate)
 *
 * The daemon (minictd) sits in an OUTER user namespace U0. minict instances
 * were forked by the daemon AFTER unshare(CLONE_NEWUSER), so the instance
 * userns U1 is a DESCENDANT of U0. user_namespaces(7): a process in an
 * ancestor userns is capable over all of its descendants — setns() into U1
 * is permitted from U0 and the joined caller holds a FULL capability set in
 * U1 (RootlessKit/podman exec model). Join ORDER matters: user FIRST (the
 * full cap set gained there covers the remaining joins), then mount, pid,
 * uts, ipc, net.
 *
 * Usage:
 *   minict-exec --pid <shim-pid-of-instance> --cwd <abs-path-inside-instance>
 *               --env-file <path> [--no-seccomp] -- cmd [args...]
 *
 * Daemon contract (minictd):
 *   --env-file  one KEY=VALUE per line, written host-side by the daemon
 *               (sanitized there). MUST be read BEFORE the setns chain:
 *               after setns(mount) the host filesystem is unreachable —
 *               the instance mount ns detached its old root (pivot_root +
 *               MNT_DETACH in minict.c).
 *   stdin       fd 0 is inherited untouched (never redirected here): the
 *               exec'd cmd reads the daemon-provided stdin directly.
 *   signals     SIGTERM/SIGINT sent to minict-exec are forwarded to the
 *               cmd child (minict fwd_sig pattern, D19) — the daemon
 *               enforces timeouts by signaling minict-exec itself.
 *   status      exit code mirrors minict: cmd's exit status, 128+sig on
 *               signal death, 126 (setup/die), 127 (exec failure),
 *               125 (usage).
 *
 * SECCOMP (important): a seccomp filter is per-THREAD and does NOT follow
 * setns — an exec'd process does NOT inherit the instance payload's filter,
 * it would run with the JOINER's filter posture (none, here). So
 * install_seccomp() below is a verbatim copy of minict.c's blocklist
 * builder (AUDIT_ARCH_X86_64 gate, flat blocklist, clone() CLONE_NEW*
 * arg-check, socket(AF_ALG) + socket(AF_NETLINK) checks, clone3 -> ENOSYS
 * fallback trick) and is MANDATORY here for an equivalent isolation
 * posture. --no-seccomp is the flag equivalent of minict's
 * MINICT_NO_SECCOMP=1 (A/B overhead control).
 */
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <linux/audit.h>
#include <linux/capability.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <sched.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

static void die(const char *ctx)
{
        fprintf(stderr, "minict-exec: %s: %s\n", ctx, strerror(errno));
        exit(126);
}

static void warn(const char *ctx)
{
        fprintf(stderr, "minict-exec: warn: %s: %s\n", ctx, strerror(errno));
}

/* ---------- capability drop (verbatim from minict.c) ---------- */

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
        /* NOTE: no setgroups(2) here — the instance already wrote "deny" to
         * /proc/self/setgroups (mandatory before gid_map without CAP_SETGID),
         * which makes setgroups() permanently EPERM in this userns and, as a
         * side effect, already forces an empty supplementary group list.
         * See user_namespaces(7). */
}

/* ------------------------------------------------------------------ *
 * seccomp-bpf blocklist — VERBATIM copy of minict.c (issue #2, D11;
 * AF_NETLINK D15). See the header comment: per-thread filters do not
 * cross setns, so this MUST be re-installed after joining the instance.
 * Only deviation: the disable switch is the --no-seccomp FLAG (the
 * equivalent of minict's MINICT_NO_SECCOMP=1 env control).
 * ------------------------------------------------------------------ */

static int g_no_seccomp = 0;

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

        if (g_no_seccomp) {
                warn("seccomp disabled via --no-seccomp");
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
        /* requires NO_NEW_PRIVS (set before the fork, inherited here);
         * kernel validates the whole program atomically — fail closed */
        if (prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &fprog, 0, 0) < 0)
                die("prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER)");
}

/* ---------- env file (daemon-written, KEY=VALUE per line) ---------- */

#define ENVF_MAX_FILE (64 * 1024)
#define ENVF_MAX_ENTRIES 64

static char g_envf_buf[ENVF_MAX_FILE + 1];
static char *g_envkv[ENVF_MAX_ENTRIES];
static int g_envkv_n;

static void load_env_file(const char *path)
{
        char *p;
        size_t off = 0;
        int fd;
        struct stat st;

        /* read BEFORE the setns chain: after setns(mount) this host path
         * no longer exists in our view (see header comment) */
        fd = open(path, O_RDONLY);
        if (fd < 0)
                die("open(--env-file)");
        if (fstat(fd, &st) < 0) {
                close(fd);
                die("fstat(--env-file)");
        }
        if (st.st_size > ENVF_MAX_FILE) {
                close(fd);
                fprintf(stderr, "minict-exec: %s: larger than 64KB cap\n",
                        path);
                exit(125);
        }
        for (;;) {
                ssize_t r = read(fd, g_envf_buf + off,
                                 sizeof(g_envf_buf) - 1 - off);

                if (r < 0) {
                        if (errno == EINTR)
                                continue;
                        close(fd);
                        die("read(--env-file)");
                }
                if (r == 0)
                        break;
                off += (size_t)r;
        }
        close(fd);
        g_envf_buf[off] = '\0';
        if (memchr(g_envf_buf, '\0', off)) {
                fprintf(stderr, "minict-exec: %s: NUL byte in env file\n",
                        path);
                exit(125);
        }

        p = g_envf_buf;
        while (p) {
                char *eol = strchr(p, '\n');
                char *line = p;
                char *end, *eq;

                if (eol) {
                        *eol = '\0';
                        p = eol + 1;
                } else {
                        p = NULL;
                }
                /* trim trailing whitespace incl. CR; skip blank lines */
                end = line + strlen(line);
                while (end > line && (end[-1] == '\r' || end[-1] == ' ' ||
                                      end[-1] == '\t'))
                        *--end = '\0';
                if (!*line)
                        continue;
                eq = strchr(line, '=');
                if (!eq || eq == line) {
                        fprintf(stderr, "minict-exec: %s: line without "
                                        "KEY=VALUE\n", path);
                        exit(125);
                }
                if (g_envkv_n >= ENVF_MAX_ENTRIES) {
                        fprintf(stderr, "minict-exec: %s: too many entries "
                                        "(cap 64)\n", path);
                        exit(125);
                }
                g_envkv[g_envkv_n++] = line;
        }
}

/* ---------- namespace join (order matters, see header comment) ----------
 *
 * ALL fds are opened BEFORE any join: after setns(mount) the mounted /
 * proc is the INSTANCE's pidns-scoped view, where the target pid's host
 * number no longer resolves (the instance's pid-1 child appears as pid 1).
 * Opening lazily per-join would therefore break the pid/uts/ipc/net opens
 * on any kernel where minict's /proc mount succeeds. fds opened earlier
 * stay valid across the joins (namespace fds are open file descriptions).
 */
static void enter_instance_ns(pid_t pid)
{
        static const char *nss[] = { "user", "mnt", "pid", "uts", "ipc",
                                     "net" };
        int fds[6] = { -1, -1, -1, -1, -1, -1 };
        char path[64], why[64];
        size_t i;

        for (i = 0; i < sizeof(nss) / sizeof(nss[0]); i++) {
                snprintf(path, sizeof(path), "/proc/%d/ns/%s", (int)pid,
                         nss[i]);
                fds[i] = open(path, O_RDONLY);
                if (fds[i] < 0)
                        die(path);
        }
        for (i = 0; i < sizeof(nss) / sizeof(nss[0]); i++) {
                /* nstype 0: the fd's own type is authoritative */
                if (setns(fds[i], 0) < 0) {
                        snprintf(why, sizeof(why), "setns(%s)", nss[i]);
                        die(why);
                }
                close(fds[i]);
                fds[i] = -1;
        }
}

/* ---------- signal forwarding to the cmd child (minict fwd_sig) ---------- */

static volatile sig_atomic_t g_child_pid = 0;

/* async-signal-safe forwarder (kill(2) is AS-safe per signal-safety(7)) */
static void fwd_to_child(int sig)
{
        if (g_child_pid > 0)
                kill((pid_t)g_child_pid, sig);
}

/* ---------- hidden selftest: does the filter builder really filter? ---- */

static void run_selftest(void)
{
        int s;

        if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0)
                die("PR_SET_NO_NEW_PRIVS (selftest)");
        install_seccomp();
        /* AF_NETLINK is on the blocklist: with the filter live this must
         * fail with EPERM (D12: blocked calls return ERRNO|EPERM, not
         * SIGSYS — testable via assert on errno) */
        errno = 0;
        s = socket(AF_NETLINK, SOCK_RAW, 0);
        if (s >= 0) {
                close(s);
                fprintf(stderr, "minict-exec: seccomp selftest FAILED: "
                                "socket(AF_NETLINK) was allowed\n");
                _exit(1);
        }
        if (errno != EPERM) {
                fprintf(stderr, "minict-exec: seccomp selftest FAILED: "
                                "socket(AF_NETLINK) errno=%d (want EPERM)\n",
                                errno);
                _exit(1);
        }
        printf("seccomp selftest OK\n");
        fflush(stdout); /* _exit skips the stdio flush — do it by hand */
        _exit(0);
}

static void usage(void)
{
        fprintf(stderr,
                "usage: minict-exec --pid <shim-pid> --cwd <abs-dir> "
                "--env-file <file> [--no-seccomp] -- cmd [args...]\n");
}

int main(int argc, char **argv)
{
        const char *cwd = NULL;
        const char *envfile = NULL;
        long pid = 0;
        int i = 1, selftest = 0, status;
        char **cmd;
        pid_t child;
        sigset_t fwd, old;

        while (i < argc) {
                if (!strcmp(argv[i], "--pid") && i + 1 < argc) {
                        char *end = NULL;

                        errno = 0;
                        pid = strtol(argv[++i], &end, 10);
                        if (errno == ERANGE || !end || *end != '\0' ||
                            pid <= 0) {
                                fprintf(stderr, "minict-exec: bad --pid "
                                                "'%s'\n", argv[i]);
                                return 125;
                        }
                } else if (!strcmp(argv[i], "--cwd") && i + 1 < argc) {
                        cwd = argv[++i];
                } else if (!strcmp(argv[i], "--env-file") && i + 1 < argc) {
                        envfile = argv[++i];
                } else if (!strcmp(argv[i], "--no-seccomp")) {
                        g_no_seccomp = 1;
                } else if (!strcmp(argv[i], "--selftest-seccomp")) {
                        selftest = 1;
                } else if (!strcmp(argv[i], "--")) {
                        i++;
                        break;
                } else {
                        usage();
                        return 125;
                }
                i++;
        }
        cmd = &argv[i];

        if (selftest)
                run_selftest(); /* never returns */

        if (pid <= 0 || !cwd || !envfile || !*cwd || !cmd[0]) {
                usage();
                return 125;
        }
        if (cwd[0] != '/') {
                fprintf(stderr, "minict-exec: --cwd must be an absolute "
                                "path inside the instance\n");
                return 125;
        }

        load_env_file(envfile);
        enter_instance_ns((pid_t)pid);

        /* after setns(mount) the ns root IS the instance root (the kernel
         * re-anchors fs root+pwd to the target mount ns root — which is
         * the pivot_root'd rootfs); re-anchor and fail closed */
        if (chdir("/") < 0)
                die("chdir(/) after setns(mount)");
        if (chdir(cwd) < 0) {
                warn("--cwd (falling back to /)");
                if (chdir("/") < 0)
                        die("chdir(/) --cwd fallback");
        }

        if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0)
                die("PR_SET_NO_NEW_PRIVS");

        /* block TERM/INT around the fork so a racing signal is QUEUED and
         * forwarded once g_child_pid is set (minict.c fork-window pattern;
         * the child restores the mask before exec, caught dispositions are
         * reset to SIG_DFL by exec(2)) */
        sigemptyset(&fwd);
        sigaddset(&fwd, SIGTERM);
        sigaddset(&fwd, SIGINT);
        if (sigprocmask(SIG_BLOCK, &fwd, &old) < 0)
                die("sigprocmask(SIG_BLOCK)");

        signal(SIGTERM, fwd_to_child);
        signal(SIGINT, fwd_to_child);

        child = fork();
        if (child < 0)
                die("fork");
        if (child == 0) {
                /* child: inherits the instance ns, our cwd and fd 0 —
                 * stdin passthrough is simply NOT touching fd 0 here */
                if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
                        die("PDEATHSIG");
                if (sigprocmask(SIG_SETMASK, &old, NULL) < 0)
                        _exit(126);

                install_seccomp(); /* NOT inherited via setns — mandatory */

                drop_all_caps();

                /* start from an EMPTY environment (R2, 2026-11-18): the
                 * exec must not inherit whatever the caller had — only
                 * the built-ins below + the daemon's env-file */
                if (clearenv() != 0)
                        _exit(126);
                /* defaults first, env-file entries after (overwrite=1):
                 * the daemon's env wins over the built-ins */
                setenv("PATH", "/bin:/sbin:/usr/bin", 1);
                setenv("HOME", "/", 1);
                setenv("MINICT", "1", 1);
                {
                        int e;

                        for (e = 0; e < g_envkv_n; e++) {
                                char *eq = strchr(g_envkv[e], '=');

                                *eq = '\0'; /* loader guarantees '=' */
                                setenv(g_envkv[e], eq + 1, 1);
                                *eq = '=';
                        }
                }

                execvp(cmd[0], cmd);
                fprintf(stderr, "minict-exec: exec %s: %s\n", cmd[0],
                        strerror(errno));
                _exit(127);
        }

        g_child_pid = child;
        if (sigprocmask(SIG_SETMASK, &old, NULL) < 0)
                die("sigprocmask(SIG_SETMASK)");

        drop_all_caps(); /* supervisor posture (minict shim, D16/D19) */

        while (waitpid(child, &status, 0) < 0) {
                if (errno != EINTR)
                        die("waitpid");
        }
        if (WIFEXITED(status))
                return WEXITSTATUS(status);
        if (WIFSIGNALED(status))
                return 128 + WTERMSIG(status);
        return 1;
}
