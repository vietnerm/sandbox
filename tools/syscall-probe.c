/*
 * syscall-probe.c — ptrace-based syscall-set recorder for seccomp allowlist
 * profiles (GĐ2 mục 3a, issue #139, D59).
 *
 * Why: profiles must be derived from the template's REAL workload (issue #29:
 * "không chép Docker default"). strace is not available on the dev machine and
 * seccomp SECCOMP_RET_LOG needs root audit access, so we run a minimal
 * PTRACE_SYSCALL tracer: it records the x86_64 syscall numbers of the target
 * command AND every descendant (fork/vfork/clone/exec), which for
 * `syscall-probe -- sandbox/minict <rootfs> sh -c ...` is exactly the payload
 * tree — never the test harness or host-side helpers.
 *
 * Usage:
 *   syscall-probe [--out FILE | --append FILE] -- cmd [args...]
 *
 *   --out FILE    write the sorted unique syscall numbers, one per line
 *   --append FILE like --out, but merge with the numbers already in FILE
 *                 (union across workload runs — busybox sh, stack boot, ...)
 *   (no flag)     print the numbers to stdout
 *
 * The D11 blocklist stays ACTIVE during measurement (no MINICT_NO_SECCOMP):
 * a syscall the floor already denies can never be satisfied by the allowlist,
 * so including it would only lie about the workload. Blocked-but-retried
 * syscalls may appear — harmless, the floor wins at filter time.
 *
 * Same uid rootless posture as the rest of the repo: ptrace of own children,
 * no setuid, no root. PTRACE_O_EXITKILL reaps stragglers on exit.
 */
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ptrace.h>
#include <sys/types.h>
#include <sys/user.h>
#include <sys/wait.h>
#include <unistd.h>

#define MAX_NR 512
#define MAX_PIDS 4096

/* --after-exec N: record only syscalls seen after the Nth PTRACE_EVENT_EXEC
 * in the tree. For `syscall-probe -- minict <rootfs> sh ...` N=2 skips the
 * sandbox SETUP phase (minict's unshare/pivot_root/mount/capdrop run between
 * its own exec and the payload execve) — exactly the phase that must NOT
 * leak into an allowlist profile (D59). */
static int g_after_exec = 0;
static int g_execs = 0;
static int g_recording = 0;

struct tracee {
        pid_t pid;
        int alive;
};

static struct tracee g_tracees[MAX_PIDS];
static size_t g_tracee_n;
static unsigned char g_seen[MAX_NR];

static struct tracee *tracee_find(pid_t pid)
{
        size_t i;
        for (i = 0; i < g_tracee_n; i++)
                if (g_tracees[i].pid == pid)
                        return &g_tracees[i];
        return NULL;
}

static struct tracee *tracee_add(pid_t pid)
{
        struct tracee *t;
        if (tracee_find(pid))
                return tracee_find(pid);
        if (g_tracee_n >= MAX_PIDS) {
                fprintf(stderr, "syscall-probe: too many tracees\n");
                exit(2);
        }
        t = &g_tracees[g_tracee_n++];
        t->pid = pid;
        t->alive = 1;
        return t;
}

static void tracee_dead(pid_t pid)
{
        struct tracee *t = tracee_find(pid);
        if (t)
                t->alive = 0;
}

static const int opts = PTRACE_O_TRACESYSGOOD | PTRACE_O_TRACEFORK |
                        PTRACE_O_TRACEVFORK | PTRACE_O_TRACECLONE |
                        PTRACE_O_TRACEEXEC | PTRACE_O_TRACEEXIT |
                        PTRACE_O_EXITKILL;

static void set_opts(pid_t pid)
{
        if (ptrace(PTRACE_SETOPTIONS, pid, 0, (void *)(long)opts) < 0) {
                /* tracee may have died between stop and call */
                if (errno != ESRCH)
                        tracee_dead(pid);
        }
}

static void record(struct user_regs_struct *r)
{
        if (!g_recording)
                return;
        long nr = (long)r->orig_rax;
        if (nr >= 0 && nr < MAX_NR)
                g_seen[nr] = 1;
}

int main(int argc, char **argv)
{
        const char *outfile = NULL;
        int append = 0;
        int i = 1, status;
        pid_t pid;

        for (; i < argc; i++) {
                if (strcmp(argv[i], "--out") == 0 && i + 1 < argc) {
                        outfile = argv[++i];
                        append = 0;
                } else if (strcmp(argv[i], "--append") == 0 && i + 1 < argc) {
                        outfile = argv[++i];
                        append = 1;
                } else if (strcmp(argv[i], "--after-exec") == 0 &&
                           i + 1 < argc) {
                        g_after_exec = atoi(argv[++i]);
                        if (g_after_exec)
                                g_recording = 0;
                        else
                                g_recording = 1;
                } else if (strcmp(argv[i], "--") == 0) {
                        i++;
                        break;
                } else {
                        fprintf(stderr, "syscall-probe: bad arg %s\n", argv[i]);
                        return 2;
                }
        }
        if (i >= argc) {
                fprintf(stderr, "usage: syscall-probe [--out F|--append F] "
                                "-- cmd [args...]\n");
                return 2;
        }

        if (!g_after_exec)
                g_recording = 1;

        pid = fork();
        if (pid < 0) {
                perror("fork");
                return 2;
        }
        if (pid == 0) {
                if (ptrace(PTRACE_TRACEME, 0, 0, 0) < 0) {
                        perror("ptrace(TRACEME)");
                        _exit(127);
                }
                execvp(argv[i], &argv[i]);
                perror("execvp");
                _exit(127);
        }
        tracee_add(pid);

        for (;;) {
                int events;
                pid_t w = waitpid(-1, &status, __WALL);

                if (w < 0) {
                        if (errno == EINTR)
                                continue;
                        break; /* no tracees left */
                }
                if (WIFEXITED(status) || WIFSIGNALED(status)) {
                        tracee_dead(w);
                        if (w == pid && g_tracee_n == 1)
                                break; /* root tracee done and alone */
                        /* keep waiting for other tracees */
                        continue;
                }
                if (!WIFSTOPPED(status))
                        continue;

                events = (unsigned int)status >> 16;
                set_opts(w);

                if (events == PTRACE_EVENT_EXEC) {
                        g_execs++;
                        if (g_after_exec && g_execs >= g_after_exec)
                                g_recording = 1;
                }

                if (events == PTRACE_EVENT_FORK ||
                    events == PTRACE_EVENT_VFORK ||
                    events == PTRACE_EVENT_CLONE) {
                        unsigned long nw = 0;
                        if (ptrace(PTRACE_GETEVENTMSG, w, 0, &nw) == 0)
                                tracee_add((pid_t)nw);
                }

                if (WSTOPSIG(status) == (SIGTRAP | 0x80)) {
                        struct user_regs_struct r;
                        if (ptrace(PTRACE_GETREGS, w, 0, &r) == 0)
                                record(&r);
                        ptrace(PTRACE_SYSCALL, w, 0, 0);
                        continue;
                }
                if (WSTOPSIG(status) == SIGTRAP && events == 0) {
                        /* exec stop / initial stop of a TRACEME child */
                        ptrace(PTRACE_SYSCALL, w, 0, 0);
                        continue;
                }
                /* real signal: deliver it unchanged (group-stops included) */
                ptrace(PTRACE_SYSCALL, w, 0,
                       (void *)(long)WSTOPSIG(status));
        }

        /* merge with an existing file for --append (union of workload runs) */
        if (outfile && append) {
                FILE *f = fopen(outfile, "r");
                if (f) {
                        long v;
                        while (fscanf(f, "%ld", &v) == 1)
                                if (v >= 0 && v < MAX_NR)
                                        g_seen[v] = 1;
                        fclose(f);
                }
        }
        {
                FILE *f = stdout;
                long n;
                if (outfile) {
                        f = fopen(outfile, "w");
                        if (!f) {
                                perror("open(out)");
                                return 2;
                        }
                }
                for (n = 0; n < MAX_NR; n++)
                        if (g_seen[n])
                                fprintf(f, "%ld\n", n);
                if (f != stdout)
                        fclose(f);
        }
        return 0;
}
