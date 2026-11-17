#!/usr/bin/env bash
# test_isolation.sh — basic escape/isolation test suite for minict.
# Every case prints PASS / FAIL / PASS(expected-fail) for known gaps.
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

# run a command inside the sandbox
ic() { "$MINICT" "$ROOTFS" /bin/sh -c "$1"; }

fs6_test() {
    ic 'echo marker > /home/iso && cat /home/iso | grep -q marker'
    local rc=1
    [[ -f "$ROOTFS/home/iso" ]] && rc=0   # on-disk, NOT tmpfs
    rm -f "$ROOTFS/home/iso"
    return $rc   # rc=0 iff file existed on host side
}
uid2_test() {
    ic 'touch /home/own'
    local rc=1
    [[ "$(stat -c %u "$ROOTFS/home/own" 2>/dev/null)" == "$(id -u)" ]] && rc=0
    rm -f "$ROOTFS/home/own"
    return $rc
}
# NOTE: $! may be an intermediate bash subshell in some harnesses, so we
# target minict itself (first child) instead of assuming $! is minict.
nnp1_test() {
    "$MINICT" "$ROOTFS" /bin/sh -c 'sleep 1' >/dev/null 2>&1 & local w=$!
    sleep 0.3
    local mp
    mp=$(pgrep -P "$w" 2>/dev/null | head -1)
    local rc=1
    if [[ -n "$mp" ]] && grep -q '^NoNewPrivs:[[:space:]]*1$' \
        "/proc/$mp/status" 2>/dev/null; then rc=0; fi
    kill -9 "$w" "$mp" 2>/dev/null
    wait "$w" 2>/dev/null
    return $rc
}
sig1_test() {
    # ENV FACT (see journal/2026-11-17/research.md): SIGTERM sent from outside to a
    # process inside an unprivileged userns is swallowed here (kill() rc=0,
    # no delivery); SIGKILL is delivered. So the no-orphan guarantee is
    # verified via SIGKILL + PR_SET_PDEATHSIG + pidns-1-exit teardown.
    # sh -c has TWO commands so busybox ash does NOT exec-optimize away the
    # shell: the sh keeps the tag in its cmdline and owns the sleep child.
    # ENV FACT (CI run #20, issue #14): busybox-static on ubuntu-24.04
    # (1.36.1-6ubuntu3.1) runs the `sleep` applet NOFORK — there is NO child
    # process and the sh itself is the sleeper (Debian busybox 1.37 forks).
    # The no-orphan guarantee is unchanged (PDEATHSIG targets the sh; pidns
    # teardown covers any other process), so an empty child pid is expected
    # there — only a missing sh is an error.
    local tag="mk-sigtest-$$"
    "$MINICT" "$ROOTFS" /bin/sh -c "sleep 61; true # $tag" >/dev/null 2>&1 & local w=$!
    sleep 0.4
    local shpid mp
    shpid=$(pgrep -f "$tag" | grep -vw "$w" | head -1)   # sandboxed sh
    mp=$(pgrep -P "$shpid" 2>/dev/null | head -1)        # sleep child (empty = NOFORK sleep)
    if [[ -z "$shpid" ]]; then kill -9 "$w" 2>/dev/null; return 1; fi
    kill -KILL "$w" 2>/dev/null
    for _ in 1 2 3 4; do
        sleep 0.3
        { kill -0 "$shpid" 2>/dev/null || kill -0 "$mp" 2>/dev/null; } || return 0
    done
    kill -9 "$shpid" ${mp:+"$mp"} 2>/dev/null
    return 1
}

uid1_test() {
    # issue #14: `id -G` shows supplementary groups of the HOST user as
    # overflowgid 65534 inside the sandbox: map_ids() maps exactly one gid
    # and setgroups is deny-locked in the userns (mandatory before an
    # unprivileged gid_map write), so the host user's extra groups cannot be
    # dropped or mapped. They belong to the SAME principal (uid 1001) and
    # carry no extra host access; UID2 already proves files map back to the
    # host uid. What this test still catches: a FOREIGN gid actually mapped
    # into the namespace (anything other than 0 = our map, 65534 = overflow).
    ic '[ "$(id -u)" = 0 ]' && ic '[ "$(id -g)" = 0 ]' \
        && ! ic 'id -G | tr " " "\n" | grep -qvE "^(0|65534)$"'
}

PASS=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|fail|xfail> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass)  [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        fail)  [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL ;;
        xfail) if [[ $rc -ne 0 ]]; then verdict="PASS(expected-fail)"; rc=0;
               else verdict="FAIL(unexpected success)"; fi ;;
    esac
    printf '%-5s %-56s %s\n' "$id" "$desc" "$verdict"
    if [[ "$verdict" == FAIL* ]]; then
        echo "      output: $(echo "$out" | head -3 | tr '\n' ' ')"
        FAIL=$((FAIL+1))
    else
        PASS=$((PASS+1))
    fi
    return 0
}

echo "=== minict isolation tests ($(date '+%F %T')) ==="

# ---------- filesystem isolation ----------
run_case FS1 "sandbox sees OUR /etc/passwd (marker)" pass \
    ic 'grep -q minict-sandbox /etc/passwd'
run_case FS2 "host home dir /home/z unreachable" pass \
    ic '[ ! -e /home/z ]'
run_case FS3 "no /old-* escape path after pivot_root+umount" pass \
    ic 'ls -d /old-* >/dev/null 2>&1 && exit 1 || exit 0'
run_case FS4 "mount --bind host / fails (no caps post-drop)" fail \
    ic 'mkdir -p /host && mount --bind / /host'
run_case FS5 "post-capdrop tmpfs mount also fails" fail \
    ic 'mkdir -p /mnt2 && mount -t tmpfs none /mnt2'
run_case FS6 "write to /home lands on-disk in rootfs (host view)" pass fs6_test

# ---------- pid/identity isolation ----------
run_case PID1 "shell is PID 1 in its own pid namespace" pass \
    ic '[ $$ -eq 1 ]'
run_case PID2 "no host processes visible (ps lists none)" pass \
    ic 'ps | tail -n +2 | grep -q . && exit 1 || exit 0'
run_case UID1 "uid=0 inside, no foreign gid visible (0/overflow only)" pass uid1_test
run_case UID2 "files created inside owned by real host uid" pass uid2_test

# ---------- privilege state ----------
run_case NNP1 "PR_SET_NO_NEW_PRIVS=1 verified via host /proc" pass nnp1_test
run_case HOST1 "hostname isolated ('minict')" pass \
    ic 'hostname | grep -qx minict'
run_case SIG1 "SIGKILL wrapper: PDEATHSIG+pidns teardown, no orphan" pass sig1_test

# ---------- network isolation (netns ON since issue #3) ----------
run_case NET1 "netns isolated: only lo inside, no host iface" pass \
    ic 'busybox ifconfig -a | grep -q "^lo " && busybox ifconfig -a \
        | grep -Eq "^(eth|ens|enp|eno|wl|ww|bond|veth|dummy)" && exit 1 || exit 0'
run_case NET2 "loopback is UP inside sandbox netns" pass \
    ic 'busybox ifconfig lo | grep -q "UP LOOPBACK RUNNING"'
run_case PROC1 "/proc mount blocked by host seccomp (known gap)" xfail \
    ic 'mount -t proc none /proc'
run_case CG1 "no cgroup limits (cgroups v1 only — known gap)" xfail \
    ic '[ -f /sys/fs/cgroup/cgroup.controllers ]'

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
