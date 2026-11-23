#!/usr/bin/env bash
# test_isolation.sh — basic escape/isolation test suite for minict.
# Every case prints PASS / FAIL / PASS(expected-fail) for known gaps.
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
# ISOLATION_ROOTFS: run this same suite against a DISTRO rootfs (alpine,
# roadmap GĐ1 mục 2 residual — many more attack surfaces than busybox);
# default stays the busybox rootfs. test_escape_alpine.sh sets it.
ROOTFS="${ISOLATION_ROOTFS:-$REPO/rootfs}"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

# run a command inside the sandbox
ic() { "$MINICT" "$ROOTFS" /bin/sh -c "$1"; }

# alive <pid>: true iff the process exists and is NOT a zombie (a dead bg
# job stays kill-0-able as a zombie until reaped, which would make signal
# delivery polls report "alive" forever in some harnesses)
alive() {
    [[ -d "/proc/$1" ]] \
        && [[ "$(awk '{print $3}' "/proc/$1/stat" 2>/dev/null)" != Z ]]
}

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
    # SIGTERM from outside the pidns is DELIVERED since the shim (D19), but
    # the no-orphan guarantee must not depend on it: SIGKILL to the wrapper
    # + PR_SET_PDEATHSIG + pidns-1-exit teardown still has to clean up the
    # whole tree (minict -> shim = pidns-1 -> payload) on its own.
    # (Day-1 ENV FACT corrected by D19: the swallowed SIGTERM was pidns-init
    # semantics per pid_namespaces(7), not host seccomp — see
    # journal/2026-11-17/decisions/sigterm.md + SIG4 control below.)
    # sh -c has TWO commands so busybox ash does NOT exec-optimize away the
    # shell: the sh keeps the tag in its cmdline and owns the sleep child.
    # ENV FACT (CI run #20, issue #14): busybox-static on ubuntu-24.04
    # (1.36.1-6ubuntu3.1) runs the `sleep` applet NOFORK — there is NO child
    # process and the sh itself is the sleeper (Debian busybox 1.37 forks).
    # With the shim (D19) the first tag-bearing process found is the SHIM
    # (it forked from minict before the payload exec'd), its child is the
    # payload sh (empty on NOFORK sleep); the guarantee is checked on both.
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
        { alive "$shpid" || alive "$mp"; } || return 0
    done
    kill -9 "$shpid" ${mp:+"$mp"} 2>/dev/null
    return 1
}

# sigfwd_test <sig> <expected-rc> <target: wrapper|payload> <payload-mode>
#   payload-mode=sleep   : exec'd busybox sleep — pure applet, default
#                          dispositions for TERM and INT (native SigIgn=0,
#                          Popen-verified 2026-11-17; the SigIgn=0x6 seen
#                          earlier came from the launcher's POSIX
#                          "asynchronous list" SIG_IGN, fixed in minict by
#                          installing forwarders before the pidns fork and
#                          resetting DFL in the payload pre-exec).
#   payload-mode=trapint : ash that traps INT and exits 130, waiting on a
#                          background sleep (the `wait` builtin is the
#                          interruptible wait). A FOREGROUND sleep child
#                          does not work: non-interactive ash swallows a
#                          direct SIGINT while waiting on it (classic
#                          "sh -c sleep survives INT to the shell" quirk —
#                          would test the shell not minict).
#   target=wrapper : signal to the minict wrapper pid — the real user flow
#                    (`minict.sh down` sends SIGTERM, Ctrl-C sends SIGINT).
#                    The wrapper forwards to the pidns-1 shim, the shim
#                    forwards to the payload (D19); the tree must exit with
#                    128+<sig> (bash convention, mirrored by the shim) and
#                    leave no orphans.
#   target=payload : ROOT-CAUSE CONTROL for issue #6 (D19): a direct host
#                    signal to the payload — a NON-init pidns member since
#                    the shim — is DELIVERED (the pid_namespaces(7) init
#                    rule does not apply to it). This disproves the old D4
#                    hypothesis that the host seccomp swallowed cross-userns
#                    signals: if that were true, this control would fail.
sigfwd_test() {
    local sig="$1" exprc="$2" target="$3" mode="${4:-sleep}"
    local payload_cmd="exec /bin/busybox sleep 61"
    [[ "$mode" == trapint ]] && payload_cmd='trap "exit 130" INT; sleep 61 & wait'
    "$MINICT" "$ROOTFS" /bin/sh -c "$payload_cmd" >/dev/null 2>&1 & local w=$!
    sleep 0.4
    local mp="" p
    if [[ "$(cat "/proc/$w/comm" 2>/dev/null)" == "minict" ]]; then
        mp="$w" # bash fork+exec'd minict directly ($! = minict)
    else
        for p in $(pgrep -P "$w" 2>/dev/null); do
            if [[ "$(cat "/proc/$p/comm" 2>/dev/null)" == "minict" ]]; then
                mp="$p"; break
            fi
        done
    fi
    if [[ -z "$mp" ]]; then kill -9 "$w" 2>/dev/null; return 1; fi
    local shim payload
    shim=$(pgrep -P "$mp" 2>/dev/null | head -1)
    payload=$(pgrep -P "$shim" 2>/dev/null | head -1)
    if [[ ! "$shim" || ! "$payload" ]]; then
        kill -9 "$mp" "$w" 2>/dev/null; return 1
    fi
    if [[ "$target" == wrapper ]]; then
        kill -"$sig" "$mp" 2>/dev/null
    else
        kill -"$sig" "$payload" 2>/dev/null
    fi
    # whole tree (wrapper, shim, payload) gone within ~2.5s, exit 128+sig
    local gone=0
    for _ in 1 2 3 4 5 6 7 8; do
        { alive "$w" || alive "$shim" || alive "$payload"; } \
            || { gone=1; break; }
        sleep 0.3
    done
    if [[ $gone -eq 0 ]]; then
        kill -9 "$mp" "$shim" "$payload" 2>/dev/null
        return 1
    fi
    wait "$w" 2>/dev/null
    local rc=$?
    [[ $rc -eq $exprc ]]
}

# SIG5: orphaned grandchildren re-parented to pidns-1 are REAPED by the
# shim, not left as zombies (SIA audit of PR #21, 2026-11-17, medium).
# Repro of the old bug: `sh -c 'for i in 1..5; do (sleep 0.1 &); done;
# sleep 2'` — each backgrounded sleep re-parents to the shim when its
# subshell exits, then dies. The old shim_wait_loop() only waited for the
# payload, so those re-parented children lingered as permanent Z processes
# under the shim for the whole payload lifetime (pid table leak for
# long-running instances, GĐ2). The fix (tini pattern): a SIGCHLD handler
# reaps every available child (shim_reap, minict.c).
# Host-view detection: a zombie's state is visible in host `ps` regardless
# of pidns; re-parented zombies show PPID = shim's host pid. The payload's
# own unreaped subshell zombies (PPID = payload) are normal shell behavior
# and NOT part of this contract.
sig5_test() {
    "$MINICT" "$ROOTFS" /bin/sh -c \
        'for i in 1 2 3 4 5; do (sleep 0.1 &); done; sleep 2' \
        >/dev/null 2>&1 & local w=$!
    sleep 1.2 # orphans long dead, payload still sleeping
    local mp="" p
    if [[ "$(cat "/proc/$w/comm" 2>/dev/null)" == "minict" ]]; then
        mp="$w"
    else
        for p in $(pgrep -P "$w" 2>/dev/null); do
            if [[ "$(cat "/proc/$p/comm" 2>/dev/null)" == "minict" ]]; then
                mp="$p"; break
            fi
        done
    fi
    if [[ -z "$mp" ]]; then kill -9 "$w" 2>/dev/null; return 1; fi
    local shim z
    shim=$(pgrep -P "$mp" 2>/dev/null | head -1)
    if [[ ! "$shim" ]]; then kill -9 "$mp" "$w" 2>/dev/null; return 1; fi
    z=$(ps -eo ppid=,stat= | awk -v s="$shim" '$1==s && $2 ~ /^Z/' | wc -l)
    wait "$w" 2>/dev/null
    [[ "$z" -eq 0 ]]
}

# ---- direct host-side assertions for capdrop + pidns (issue #38, D32) ----
# FS4/FS5 are vacuous w.r.t. capdrop: the default seccomp blocklist has
# NR_mount, so mount(2) is EPERM with OR without drop_all_caps() — SCR
# mutant M2 (capset removed) slipped the whole suite 21/21. PID2 is
# vacuous w.r.t. pidns: busybox `ps` reads /proc and the rootfs has no
# /proc (AL4), so "ps lists none" holds with OR without CLONE_NEWPID —
# SCR mutant M4 only turned PID1 red. CAPD1 + PID3 below assert the
# MECHANISM directly from the host instead:
#   CAPD1: /proc/<pid>/status CapEff of shim AND payload == 0
#   PID3:  /proc/<pid>/status NSpid nested, shim innermost 1, payload 2
# (pid_namespaces(7): a process has one pid per namespace level it is
# visible in; verified live 2026-11-18: shim "NSpid: <host> 1", payload
# "<host> 2", launcher single-field — launcher keeps its userns caps,
# shim drops for relay posture D16/D19, payload drops before
# install_seccomp(), minict.c drop_all_caps call sites).
_find_sandbox_tree() { # <tag> — classify launcher/shim/payload, host view.
    # All three carry <tag> in their cmdline (shim is a fork of the
    # launcher pre-exec, payload sh gets the -c string). Classify by
    # ancestry: launcher = tag-bearing whose PPid is NOT tag-bearing;
    # shim = its child; payload = child of the shim.
    local tag="$1" p pp pids l="" s="" pl=""
    sleep 0.4
    pids=$(pgrep -f "$tag" 2>/dev/null)
    [[ -z "$pids" ]] && return 1
    for p in $pids; do
        pp=$(awk '/^PPid:/{print $2}' "/proc/$p/status" 2>/dev/null)
        if ! printf '%s\n' "$pids" | grep -qx "$pp"; then l=$p; break; fi
    done
    [[ -z "$l" ]] && return 1
    s=$(pgrep -P "$l" 2>/dev/null | head -1)
    pl=$(pgrep -P "$s" 2>/dev/null | head -1)
    [[ -z "$s" || -z "$pl" ]] && return 1
    SBX_LAUNCH="$l"; SBX_SHIM="$s"; SBX_PAYLOAD="$pl"
    return 0
}
capd1_test() {
    local tag="mk-capdrop-$$" rc=1
    SBX_LAUNCH=""; SBX_SHIM=""; SBX_PAYLOAD=""
    "$MINICT" "$ROOTFS" /bin/sh -c "sleep 61; true # $tag" >/dev/null 2>&1 & local w=$!
    if _find_sandbox_tree "$tag"; then
        [[ "$(awk '/^CapEff:/{print $2}' "/proc/$SBX_SHIM/status" 2>/dev/null)" == "0000000000000000" ]] \
            && [[ "$(awk '/^CapEff:/{print $2}' "/proc/$SBX_PAYLOAD/status" 2>/dev/null)" == "0000000000000000" ]] && rc=0
    fi
    kill -9 "$w" "$SBX_LAUNCH" "$SBX_SHIM" "$SBX_PAYLOAD" 2>/dev/null
    wait "$w" 2>/dev/null
    return $rc
}
pid3_test() {
    local tag="mk-pidns-$$" rc=1 ns_shim ns_pl
    SBX_LAUNCH=""; SBX_SHIM=""; SBX_PAYLOAD=""
    "$MINICT" "$ROOTFS" /bin/sh -c "sleep 61; true # $tag" >/dev/null 2>&1 & local w=$!
    if _find_sandbox_tree "$tag"; then
        ns_shim=$(awk '/^NSpid:/{print NF - 1, $NF}' "/proc/$SBX_SHIM/status" 2>/dev/null)
        ns_pl=$(awk '/^NSpid:/{print NF - 1, $NF}' "/proc/$SBX_PAYLOAD/status" 2>/dev/null)
        # exactly one inner pidns (D19 tree): 2 levels, shim=1, payload=2
        [[ "$ns_shim" == "2 1" ]] && [[ "$ns_pl" == "2 2" ]] && rc=0
    fi
    kill -9 "$w" "$SBX_LAUNCH" "$SBX_SHIM" "$SBX_PAYLOAD" 2>/dev/null
    wait "$w" 2>/dev/null
    return $rc
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

# ---- deep escape PoCs (issue #5, D33) ----
# FD1: an fd the CALLER holds open without FD_CLOEXEC must not survive into
# the payload (minict.c harden_payload_fds closes 3..max via close_range).
# Two stages so the case cannot go vacuous:
#   stage 1 (inheritance control): the SAME fd is visible as
#     /proc/<minict-pid>/fd/9 -> marker — proves bash->minict inheritance is
#     real, so stage 2's EBADF is the FIX working, not bash closing the fd.
#   stage 2: payload `cat <&9` must FAIL (EBADF) and must NOT print the
#     host marker (escape check). Pre-fix behavior: cat prints the marker.
fd1_test() {
    local marker="$REPO/.fd1-marker-$$" mp="" p w inherited=0 rc=1 out
    echo "HOST-SECRET-$$" > "$marker"
    exec 9<"$marker"   # bash redirection: no cloexec — survives fork+exec
    "$MINICT" "$ROOTFS" /bin/sh -c 'sleep 61' >/dev/null 2>&1 & w=$!
    sleep 0.4
    if [[ "$(cat "/proc/$w/comm" 2>/dev/null)" == "minict" ]]; then
        mp="$w"
    else
        for p in $(pgrep -P "$w" 2>/dev/null); do
            if [[ "$(cat "/proc/$p/comm" 2>/dev/null)" == "minict" ]]; then
                mp="$p"; break
            fi
        done
    fi
    if [[ -n "$mp" ]] \
        && [[ "$(readlink "/proc/$mp/fd/9" 2>/dev/null)" == "$marker" ]]; then
        inherited=1
    fi
    kill -9 "$w" "$mp" 2>/dev/null
    wait "$w" 2>/dev/null
    out=$(ic 'cat <&9' 2>&1); rc=$?
    exec 9<&-
    rm -f "$marker"
    [[ $inherited -eq 1 ]] || return 1
    [[ $rc -ne 0 ]] || return 1
    ! printf '%s' "$out" | grep -q 'HOST-SECRET-'
}
# SETUID2: exec a setuid-root-marked binary (busybox chmod 4755 — restored)
# and assert the MECHANISM from the host (D32 style): NoNewPrivs must still
# be 1 and CapEff must still be 0 on the exec'd process. In-ns assertions
# would be vacuous: single-uid mapping makes every setuid bit a no-op
# regardless of NNP (owner uid == executor uid in-ns). The file is owned by
# the REAL host uid (AL7 proved chown foreign-uid EPERM), so a host-root-
# owned suid file — the actual escalation class — cannot exist in rootfs.
setuid2_test() {
    local tag="busybox sleep 61" ok=0
    [[ -f "$ROOTFS/bin/busybox" ]] || return 1
    chmod 4755 "$ROOTFS/bin/busybox" || return 1
    SBX_LAUNCH=""; SBX_SHIM=""; SBX_PAYLOAD=""
    "$MINICT" "$ROOTFS" /bin/busybox sleep 61 >/dev/null 2>&1 & local w=$!
    if _find_sandbox_tree "$tag"; then
        [[ "$(awk '/^NoNewPrivs:/{print $2}' "/proc/$SBX_PAYLOAD/status" 2>/dev/null)" == "1" ]] \
            && [[ "$(awk '/^CapEff:/{print $2}' "/proc/$SBX_PAYLOAD/status" 2>/dev/null)" == "0000000000000000" ]] \
            && ok=1
    fi
    kill -9 "$w" "$SBX_LAUNCH" "$SBX_SHIM" "$SBX_PAYLOAD" 2>/dev/null
    wait "$w" 2>/dev/null
    chmod 755 "$ROOTFS/bin/busybox" 2>/dev/null
    [[ $ok -eq 1 ]]
}
# CORE1: RLIMIT_CORE must be forced to 0 in the payload (harden_payload_fds).
# The harness inherits ITS OWN core limit (often 0) — raise it first, else
# the in-payload `ulimit -c == 0` is vacuous by inheritance, not by the fix.
core1_test() {
    ulimit -c 1048576 2>/dev/null || ulimit -c unlimited 2>/dev/null || return 1
    local rc=0
    ic '[ "$(ulimit -c)" = "0" ]' || rc=1
    ulimit -c 0 2>/dev/null
    return $rc
}
# CORE2: behavior check — a SEGV in the payload must leave NO core file in
# the rootfs root (payload cwd = /) or the repo cwd. NOTE: on hosts with a
# PIPED core_pattern (systemd-coredump) no file would appear even without
# the fix (core(5): RLIMIT_CORE ignored for pipes) — CORE2 is defense in
# depth for file-pattern hosts; CORE1 is the mechanism assertion (D33).
core2_test() {
    "$MINICT" "$ROOTFS" /bin/sh -c 'kill -SEGV $$' >/dev/null 2>&1 || true
    local f
    for f in "$ROOTFS/core" "$ROOTFS/core."* "$REPO/core" "$REPO/core."*; do
        [[ -e "$f" ]] && return 1
    done
    return 0
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
# NOTE(#38/D32): FS4/FS5 alone are vacuous w.r.t. capdrop — seccomp NR_mount
# makes mount EPERM regardless. Direct capdrop assertion = CAPD1 below.
run_case FS4 "mount --bind host / fails (no caps post-drop)" fail \
    ic 'mkdir -p /host && mount --bind / /host'
run_case FS5 "post-capdrop tmpfs mount also fails" fail \
    ic 'mkdir -p /mnt2 && mount -t tmpfs none /mnt2'
run_case FS6 "write to /home lands on-disk in rootfs (host view)" pass fs6_test

# ---------- pid/identity isolation ----------
# issue #31 / D39: minict now mounts a procfs of the instance pidns (before
# detaching the old root). Whether the HOST lets an unprivileged userns
# mount procfs at all is probed independently of minict: a host seccomp
# filter (dev box kernel 5.10) refuses it for every process. On such hosts
# the /proc cases keep the old "absent" assertion (xfail/pass) — never a
# silent PASS for a feature that did not run.
if unshare --user --map-root-user --pid --fork --mount --mount-proc true \
        >/dev/null 2>&1; then
    PROC_ENV=ok
else
    PROC_ENV=blocked
    echo "NOTE: host refuses procfs in an unprivileged userns — /proc cases run in 'absent' mode (D39)"
fi

# PID2 (/proc present): every pid listed is a small sandbox pid — with
# CLONE_NEWPID removed (mutant M4) the procfs would be the HOST pidns and
# list dozens of pids, many > 2 digits.
pid2_test() {
    ic 'n=0; for p in /proc/[0-9]*; do n=$((n+1)); [ "${p#/proc/}" -le 50 ] || exit 1; done; [ $n -ge 2 ] && [ $n -le 8 ]'
}
# PROC1: /proc is THIS pidns: the payload shell sees itself as pid 2 with
# a single NSpid level (a host-pidns procfs would show the host pid).
proc1_test() {
    ic 'awk "/^Pid:/{p=\$2} /^NSpid:/{n=NF} END{exit !(p==2 && n==2)}" /proc/$$/status'
}
# PROC2: /proc/<pid>/root of EVERY visible process is the sandbox root —
# the /proc/self/root escape class (issue #5) stays closed with procfs on.
proc2_test() {
    ic 'for p in /proc/[0-9]*; do [ "$(readlink $p/root)" = / ] || exit 1; done; ls /proc/1/root/ | grep -q "^old-" && exit 1; exit 0'
}
# PROC3: the runtime's procfs is nosuid,nodev,noexec
proc3_test() {
    ic 'grep " /proc proc " /proc/mounts | grep -q nosuid, && grep " /proc proc " /proc/mounts | grep nodev | grep -q noexec'
}

run_case PID1 "payload is PID 2 behind pidns-1 signal shim" pass \
    ic '[ $$ -eq 2 ]'
if [[ $PROC_ENV == ok ]]; then
    run_case PID2 "/proc lists only sandbox pids (no host processes)" pass pid2_test
else
    run_case PID2 "no host processes visible (ps lists none)" pass \
        ic 'ps | tail -n +2 | grep -q . && exit 1 || exit 0'
fi
# NOTE(#38/D32): in 'absent' mode PID2 is vacuous w.r.t. pidns (no /proc).
# Direct pidns assertion from the host side = PID3 below.
run_case PID3 "pidns live: shim is pidns-1, payload pidns-2 (host NSpid)" pass \
    pid3_test
run_case UID1 "uid=0 inside, no foreign gid visible (0/overflow only)" pass uid1_test
run_case UID2 "files created inside owned by real host uid" pass uid2_test

# ---------- bind :ro recursive enforcement (S3, issue #113, D52) ----------
# An unprivileged process cannot mount on the host, but a userns CAN mount
# inside itself; a submount created there is visible to minict run from
# within that userns (minict's own userns is a CHILD of it, and MS_REC
# clones ancestor-owned mounts — measured 2026-11-22). Pre-fix state also
# measured: the rbind clone kept the tmpfs submount WRITABLE while the top
# mount went ro — exactly the S3 gap in GAPS.md. Post-fix the ASSERTION is
# identical on every kernel; the MECHANISM differs and is named in the
# case description: kernel >= 5.12 mount_setattr(AT_RECURSIVE), kernel
# < 5.12 (dev 5.10) per-submount bind-remounts from the host-side
# /proc/self/mountinfo enumeration (the new mountns has no /proc and
# cannot mount one there — EPERM, D39).
S3WORK="$(mktemp -d /tmp/minict-s3.XXXXXX)"
mkdir -p "$S3WORK/plain" "$S3WORK/withsub/sub"
echo ro-marker > "$S3WORK/plain/ro.txt"
echo top-marker > "$S3WORK/withsub/top.txt"
# NB: no marker inside withsub/sub at setup — the tmpfs mounted over it in
# s3_run would hide it; inner-marker is written onto the tmpfs AFTER mount.

S3_ENV=blocked
if unshare --user --map-root-user --mount -- sh -c \
        'mount -t tmpfs none "$1"' s3env "$S3WORK/withsub/sub" 2>/dev/null; then
    S3_ENV=ok
fi
KVER_MM="$(uname -r | awk -F. '{print $1"."$2}')"
if awk -v k="$KVER_MM" 'BEGIN{split(k,a,".");
    exit !(a[1]>5 || (a[1]==5 && a[2]>=12))}'; then
    S3_MECH="mount_setattr"
else
    S3_MECH="mountinfo-fallback"
fi

# s3_run <payload-sh> <host-src> <suffix>: run minict with a spec bind of
# <host-src> to /data<suffix> inside a userns+mountns that carries the
# tmpfs submount at <src>/sub. The submount marker file is written AFTER
# the mount (before it, the mount would hide it). Payloads end with
# `true` so an expected EROFS does not turn into a fake rc failure; only
# a boot refusal (die, rc=126) or harness failure propagates rc.
s3_run() {
    printf 'bind %s:/data%s\n' "$2" "$3" > "$S3WORK/spec"
    MINICT_SPEC="$S3WORK/spec" unshare --user --map-root-user --mount -- sh -c \
        'mount -t tmpfs none "$1/withsub/sub" >/dev/null 2>&1 || {
             echo S3ENVFAIL; exit 9; }
         echo inner-marker > "$1/withsub/sub/inner.txt"
         "$2" "$3" /bin/sh -c "$4"' \
        s3run "$S3WORK" "$MINICT" "$ROOTFS" "$1"
}

bind1_test() {
    local out
    out="$(s3_run 'cat /data/ro.txt; touch /data/w 2>&1; true' \
        "$S3WORK/plain" ':ro')" || return 1
    grep -q ro-marker <<<"$out" \
        && grep -q "Read-only file system" <<<"$out"
}
bind2_test() {
    local out
    out="$(s3_run 'cat /data/top.txt; touch /data/top.txt 2>&1;
                   touch /data/sub/inner.txt 2>&1;
                   cat /data/sub/inner.txt; true' \
        "$S3WORK/withsub" ':ro')" || return 1
    [[ "$(grep -c "Read-only file system" <<<"$out")" == 2 ]] || return 1
    grep -q top-marker <<<"$out" && grep -q inner-marker <<<"$out"
}
bind3_test() {
    local out
    out="$(s3_run 'echo w-top >> /data/top.txt;
                   echo w-sub >> /data/sub/inner.txt;
                   cat /data/top.txt /data/sub/inner.txt; true' \
        "$S3WORK/withsub" '')" || return 1
    grep -q w-top <<<"$out" && grep -q w-sub <<<"$out"
}

# BIND4 (#117/D58): a submount whose src-relative suffix >=256 byte must die
# (fail-closed), not silently truncate to a wrong path (partial-ro config lie).
# Build a 255-byte submount name under a fresh bind src; the suffix "/<255>"
# = 256 byte >= SPEC_SUBMOUNT_PATH → die. Only meaningful on the
# mountinfo-fallback path (kernel <5.12); mount_setattr (>=5.12) needs no
# enumeration, so the die code path is not reached there (SKIP).
bind4_test() {
    local longname sub
    longname="$(printf 'A%.0s' $(seq 1 255))"   # 255-byte single component (NAME_MAX)
    sub="$S3WORK/longsrc/$longname"
    mkdir -p "$sub"
    printf 'bind %s:/data:ro\n' "$S3WORK/longsrc" > "$S3WORK/spec4"
    MINICT_SPEC="$S3WORK/spec4" unshare --user --map-root-user --mount -- sh -c \
        'mount -t tmpfs none "$1" >/dev/null 2>&1 || { echo S3ENVFAIL; exit 9; }
         "$2" "$3" /bin/sh -c "true"' \
        bind4run "$sub" "$MINICT" "$ROOTFS" 2>"$S3WORK/bind4.err"
    local rc=$?
    [[ $rc -ne 0 ]] && grep -q "exceeds.*cap\|refusing truncated" "$S3WORK/bind4.err"
}

run_case BIND1 "bind :ro plain tree: visible, top mount read-only" pass \
    bind1_test
if [[ $S3_ENV == ok ]]; then
    run_case BIND2 "bind :ro tree WITH submount: submount ro too ($S3_MECH)" \
        pass bind2_test
    run_case BIND3 "control: same tree WITHOUT :ro — top+submount writable" \
        pass bind3_test
    if [[ $S3_MECH == mountinfo-fallback ]]; then
        run_case BIND4 "bind :ro submount suffix >256 -> die fail-closed (#117)" \
            pass bind4_test
    else
        echo "SKIP BIND4: mountinfo-fallback path not active (kernel >=5.12 uses mount_setattr, no enumeration)"
    fi
else
    echo "SKIP BIND2/BIND3/BIND4: userns tmpfs submount unavailable on this host"
fi
rm -rf "$S3WORK"

# ---------- privilege state ----------
run_case NNP1 "PR_SET_NO_NEW_PRIVS=1 verified via host /proc" pass nnp1_test
run_case CAPD1 "capdrop real: shim+payload CapEff=0 (host /proc)" pass capd1_test
run_case HOST1 "hostname isolated ('minict')" pass \
    ic 'hostname | grep -qx minict'

# ---------- deep escape PoCs (issue #5, D33) ----------
run_case FD1 "fd-based escape closed: inherited host fd EBADF in payload" pass \
    fd1_test
# /bin/busybox chroot (applet-explicit): the alpine rootfs has the chroot
# applet compiled in but NO /bin/chroot symlink — a bare `chroot` there dies
# rc=127 and the expect=fail case would PASS spuriously (vacuous on alpine).
run_case CHROOT1 "chroot(2) EPERM post-capdrop (jail-escape primitive)" fail \
    ic 'mkdir -p /c && /bin/busybox chroot /c /bin/true'
run_case SETUID2 "suid exec: NNP=1 + CapEff=0 survive (host /proc)" pass \
    setuid2_test
run_case CORE1 "core dump disabled: RLIMIT_CORE=0 forced in payload" pass \
    core1_test
run_case CORE2 "SIGSEGV leaves no core file in rootfs/repo cwd" pass \
    core2_test

run_case SIG1 "SIGKILL wrapper: PDEATHSIG+pidns teardown, no orphan" pass sig1_test
run_case SIG2 "SIGTERM wrapper: tree exits 143 (128+15), no orphan" pass \
    sigfwd_test TERM 143 wrapper
run_case SIG3 "SIGINT wrapper: tree exits 130 (128+2)" pass \
    sigfwd_test INT 130 wrapper trapint
run_case SIG4 "control: TERM to payload (non-init) delivered (143)" pass \
    sigfwd_test TERM 143 payload
run_case SIG5 "orphan grandchildren reaped by shim, no zombies" pass sig5_test

# SIG_STRESS (#30/D62): spawn N instances, send SIGTERM to the minict parent
# right after launch (racing the pidns fork window), verify the payload exits
# 143 (SIGTERM forwarded, not swallowed). With the #30 fix the parent blocks
# SIGTERM/SIGINT around the fork → queued → forwarded after g_child_pid set.
# Best-effort stress (µs window); SIG1-SIG5 are the deterministic regression.
sig_stress_test() {
    local i rc ok=0 spec
    spec="$(mktemp /tmp/minict-sigstress.XXXXXX)"
    : > "$spec"   # empty spec (no directives)
    for i in $(seq 1 20); do
        MINICT_SPEC="$spec" "$MINICT" "$ROOTFS" /bin/sh -c 'sleep 30' &
        local pid=$!
        kill -TERM "$pid" 2>/dev/null
        wait "$pid"; rc=$?
        [[ $rc -eq 143 ]] && ok=$((ok+1))
        # cleanup any surviving sleep (race loser)
        pkill -f "sleep 30" 2>/dev/null || true
    done
    rm -f "$spec"
    # all 20 must forward SIGTERM (exit 143); a swallowed one exits 0/timeout
    [[ $ok -eq 20 ]] || { echo "sig_stress: $ok/20 forwarded (race leak?)"; return 1; }
}
run_case SIG_STRESS "#30 stress: SIGTERM during spawn forwarded (20x)" pass sig_stress_test

# ---------- network isolation (netns ON since issue #3) ----------
run_case NET1 "netns isolated: only lo inside, no host iface" pass \
    ic 'busybox ifconfig -a | grep -q "^lo " && busybox ifconfig -a \
        | grep -Eq "^(eth|ens|enp|eno|wl|ww|bond|veth|dummy)" && exit 1 || exit 0'
run_case NET2 "loopback is UP inside sandbox netns" pass \
    ic 'busybox ifconfig lo | grep -q "UP LOOPBACK RUNNING"'
if [[ $PROC_ENV == ok ]]; then
    run_case PROC1 "/proc is the sandbox pidns view (payload = pid 2)" pass proc1_test
    run_case PROC2 "every /proc/<pid>/root is the sandbox root" pass proc2_test
    run_case PROC3 "runtime procfs mounted nosuid,nodev,noexec" pass proc3_test
else
    # host refuses procfs (D39 'absent' mode): lock the old contract
    run_case PROC1 "/proc absent (host refuses procfs in userns)" xfail \
        ic '[ -e /proc/self ]'
    run_case PROC2 "/proc/self/root unreachable (no procfs mounted)" pass \
        ic '[ ! -e /proc/self/root ]'
    run_case PROC3 "(absent mode) no procfs in /proc/mounts either" pass \
        ic '! grep -q " /proc proc " /proc/mounts 2>/dev/null'
fi
# the PAYLOAD can never mount another procfs (seccomp NR_mount + capdrop)
run_case PROC4 "payload cannot mount a second procfs" fail \
    ic 'mkdir -p /tmp/p && mount -t proc none /tmp/p'
run_case CG1 "no cgroup limits (cgroups v1 only — known gap)" xfail \
    ic '[ -f /sys/fs/cgroup/cgroup.controllers ]'

# ---------- Landlock fs-policy (GĐ2 mục 6, #149/D61 + #154/D63) ----------
# LL1 (CI >=5.13): spec.ReadOnly -> ro-root (write /etc EACCES, /tmp OK).
# LL2 (dev <5.13): spec.ReadOnly -> die 126 (fail-closed, refuse half-protected).
# LL3 (any): default instance -> boots + /tmp write OK (default Landlock, fail-soft dev).
LL_PROBE=0
if python3 -c "
import ctypes, os
libc = ctypes.CDLL(None, use_errno=True)
attr = ctypes.create_string_buffer(8)
r = libc.syscall(444, attr, 8, 0)
e = ctypes.get_errno()
os._exit(0 if (r >= 0 or e != 38) else 1)
" 2>/dev/null; then LL_PROBE=1; fi

LL_SPEC="$(mktemp /tmp/minict-ll-spec.XXXXXX)"; printf 'readonly\n' > "$LL_SPEC"
if [[ $LL_PROBE -eq 1 ]]; then
    # LL1 (CI >=5.13): spec.ReadOnly -> ro-root
    ll1_test() {
        local out
        out=$(MINICT_SPEC="$LL_SPEC" "$MINICT" "$ROOTFS" /bin/sh -c '
            echo x > /tmp/ll-ok 2>/dev/null
            echo y > /etc/ll-deny 2>/dev/null
            test -f /tmp/ll-ok && echo TMP_OK
            test ! -f /etc/ll-deny && echo ETC_DENIED
        ' 2>/dev/null) || return 1
        grep -q TMP_OK <<<"$out" && grep -q ETC_DENIED <<<"$out"
    }
    run_case LL1 "Landlock ro-root: write /etc denied, /tmp allowed (>=5.13)" pass ll1_test
else
    # LL2 (dev <5.13): spec.ReadOnly -> die 126 (fail-closed, #154/D63)
    ll2_test() {
        MINICT_SPEC="$LL_SPEC" "$MINICT" "$ROOTFS" /bin/sh -c 'true' 2>/dev/null
        [[ $? -eq 126 ]]  # die() exit code
    }
    run_case LL2 "Landlock fail-closed: spec.ReadOnly + ENOSYS -> die 126 (<5.13)" pass ll2_test
fi
rm -f "$LL_SPEC"

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
