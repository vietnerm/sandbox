/*
 * minict — minimal rootless container (PoC day 2026-11-17)
 *
 * Stack: user ns + mount ns + pid ns + uts ns + ipc ns, self-written uid_map,
 *        pivot_root + MNT_DETACH of old root, bind-mounted /dev bits,
 *        capability drop + PR_SET_NO_NEW_PRIVS. No Docker, no root required.
 *
 * Usage: minict <rootfs-dir> <cmd> [args...]
 *
 * Verified on kernel 5.10.134 (see research/2026-11-17.md):
 *  - pivot_root works inside unprivileged userns
 *  - mounts over host mountpoints are EPERM (MNT_LOCKED), mounts over plain
 *    dirs inside our rootfs work
 *  - mount(2) of /proc is blocked by the host seccomp filter (warn, continue)
 *  - mknod is EPERM in userns -> bind-mount host /dev/null etc. before pivot
 */
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <linux/capability.h>
#include <sched.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/prctl.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
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

        /* one-shot namespace creation */
        if (unshare(CLONE_NEWUSER | CLONE_NEWNS | CLONE_NEWPID |
                    CLONE_NEWUTS | CLONE_NEWIPC) < 0)
                die("unshare(user|mount|pid|uts|ipc)");

        map_ids(ruid, rgid);

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

                drop_all_caps(); /* after ALL privileged-in-ns work is done */

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
