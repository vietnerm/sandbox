/*
 * seccomp_probe.c — probe binary for tests/test_seccomp.sh (issue #2).
 *
 * Runs INSIDE the minict sandbox. For each probe name, attempts the
 * syscall and prints "<name> errno=<N>" (errno=0 means the call
 * succeeded). The suite asserts errno=1 (EPERM) with the filter
 * active, and errno!=1 for the discriminating probes with
 * MINICT_NO_SECCOMP=1 (A/B control proving EPERM comes from OUR
 * filter, not from the host).
 */
#define _GNU_SOURCE
#include <errno.h>
#include <sched.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <unistd.h>

struct result {
        long rc;
        int err;
};

static struct result try_syscall(const char *name)
{
        struct result r;

        errno = 0;
        if (strcmp(name, "keyctl") == 0) {
                /* KEYCTL_JOIN_SESSION_KEYRING with NULL name: unprivileged-
                 * safe on the host; blocked EPERM by our filter */
                r.rc = syscall(SYS_keyctl, 1, 0, 0, 0, 0);
        } else if (strcmp(name, "open_by_handle_at") == 0) {
                r.rc = syscall(SYS_open_by_handle_at, -100 /*AT_FDCWD*/,
                               (void *)0, 0);
        } else if (strcmp(name, "bpf") == 0) {
                r.rc = syscall(SYS_bpf, 0 /*BPF_MAP_CREATE*/, (void *)0, 0);
        } else if (strcmp(name, "setns") == 0) {
                /* fd 0 is not an ns handle -> EBADF unfiltered, EPERM filtered */
                r.rc = syscall(SYS_setns, 0, 0);
        } else if (strcmp(name, "unshare") == 0) {
                r.rc = syscall(SYS_unshare, CLONE_NEWUSER);
        } else if (strcmp(name, "io_uring") == 0) {
                r.rc = syscall(SYS_io_uring_setup, 4, (void *)0);
        } else if (strcmp(name, "alg") == 0) {
                /* socket(AF_ALG, SOCK_SEQPACKET, 0) — issue #8 mitigation */
                r.rc = syscall(SYS_socket, 38 /*AF_ALG*/, 5 /*SOCK_SEQPACKET*/, 0);
        } else if (strcmp(name, "netlink") == 0) {
                /* socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE) — nftables/
                 * ebtables/audit config surface (CVE-2024-1086 /
                 * CVE-2026-53266 class, D15). Unprivileged creatable on the
                 * host, so the A/B control discriminates. */
                r.rc = syscall(SYS_socket, 16 /*AF_NETLINK*/, 3 /*SOCK_RAW*/,
                               0 /*NETLINK_ROUTE*/);
        } else if (strcmp(name, "tcp") == 0) {
                /* socket(AF_INET, SOCK_STREAM) — control: ordinary sockets
                 * MUST keep working under the filter (httpd needs them) */
                r.rc = syscall(SYS_socket, 2 /*AF_INET*/, 1 /*SOCK_STREAM*/, 0);
        } else if (strcmp(name, "clone_userns") == 0) {
                r.rc = syscall(SYS_clone, (unsigned long)(SIGCHLD | CLONE_NEWUSER),
                               (void *)0, (void *)0, (void *)0, (void *)0);
        } else if (strcmp(name, "pvm") == 0) {
                /* process_vm_readv(pid=0,...): ESRCH without filter (no such
                 * process), EPERM with our filter - discriminating control */
                r.rc = syscall(SYS_process_vm_readv, 0, (void *)0, 1,
                               (void *)0, 1, 0);
        } else if (strcmp(name, "ptrace") == 0) {
                r.rc = syscall(SYS_ptrace, 0 /*PTRACE_TRACEME*/, 0, 0, 0);
        } else if (strcmp(name, "clone3") == 0) {
                /* filter returns ENOSYS so libc-style fallbacks retry clone() */
                r.rc = syscall(SYS_clone3, (void *)0, 0);
        } else {
                fprintf(stderr, "unknown probe: %s\n", name);
                exit(2);
        }
        r.err = r.rc < 0 ? errno : 0;
        return r;
}

int main(int argc, char **argv)
{
        struct result r;

        if (argc != 2)
                return 2;
        r = try_syscall(argv[1]);
        printf("%s errno=%d\n", argv[1], r.err);
        return 0;
}
