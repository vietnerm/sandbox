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
run_case PID1 "payload is PID 2 behind pidns-1 signal shim" pass \
    ic '[ $$ -eq 2 ]'
run_case PID2 "no host processes visible (ps lists none)" pass \
    ic 'ps | tail -n +2 | grep -q . && exit 1 || exit 0'
# NOTE(#38/D32): PID2 is vacuous w.r.t. pidns (rootfs has no /proc, so `ps`
# lists none regardless). Direct pidns assertion = PID3 below.
run_case PID3 "pidns live: shim is pidns-1, payload pidns-2 (host NSpid)" pass \
    pid3_test
run_case UID1 "uid=0 inside, no foreign gid visible (0/overflow only)" pass uid1_test
run_case UID2 "files created inside owned by real host uid" pass uid2_test

# ---------- privilege state ----------
run_case NNP1 "PR_SET_NO_NEW_PRIVS=1 verified via host /proc" pass nnp1_test
run_case CAPD1 "capdrop real: shim+payload CapEff=0 (host /proc)" pass capd1_test
run_case HOST1 "hostname isolated ('minict')" pass \
    ic 'hostname | grep -qx minict'
run_case SIG1 "SIGKILL wrapper: PDEATHSIG+pidns teardown, no orphan" pass sig1_test
run_case SIG2 "SIGTERM wrapper: tree exits 143 (128+15), no orphan" pass \
    sigfwd_test TERM 143 wrapper
run_case SIG3 "SIGINT wrapper: tree exits 130 (128+2)" pass \
    sigfwd_test INT 130 wrapper trapint
run_case SIG4 "control: TERM to payload (non-init) delivered (143)" pass \
    sigfwd_test TERM 143 payload
run_case SIG5 "orphan grandchildren reaped by shim, no zombies" pass sig5_test

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
