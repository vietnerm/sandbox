#!/usr/bin/env bash
# test_seccomp.sh — seccomp-bpf blocklist suite for minict (issue #2).
#
# With the filter active (default): every probed blocked syscall must
# return EPERM (errno=1) and a plain shell workload must still work.
# Control (MINICT_NO_SECCOMP=1): the discriminating probes must NOT
# return EPERM — proving the EPERM comes from OUR filter, not the host.
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs"
PROBE_SRC="$HERE/seccomp_probe.c"
PROBE_BIN="$ROOTFS/home/scprobe"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

# probe must live inside the rootfs tree that the sandbox sees (/home is
# on-disk visible — proven by FS6 in test_isolation.sh). STATIC: the rootfs
# carries only busybox — no ld.so/libc to satisfy a dynamic binary.
cc -O2 -static -o "$PROBE_BIN" "$PROBE_SRC" || { echo "probe build failed"; exit 1; }
trap 'rm -f "$PROBE_BIN"' EXIT

# run a probe inside the sandbox; print "errno=<N>"
with_filter()    { "$MINICT" "$ROOTFS" /home/scprobe "$1" 2>/dev/null | tail -1; }
without_filter() { MINICT_NO_SECCOMP=1 "$MINICT" "$ROOTFS" /home/scprobe "$1" 2>/dev/null | tail -1; }
expect_eperm()     { [[ "$1" =~ errno=1$ ]]; }        # EPERM == 1
expect_not_eperm() { [[ ! "$1" =~ errno=1$ ]]; }      # not EPERM

PASS=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|fail> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass) [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        fail) [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL ;;
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

# ---------- case functions (function name is what run_case invokes) ----------
sc0() { "$MINICT" "$ROOTFS" /bin/sh -c 'echo ok | grep -q ok && ls / >/dev/null'; }
sc1() { expect_eperm "$(with_filter keyctl)"; }
sc2() { expect_eperm "$(with_filter open_by_handle_at)"; }
sc3() { expect_eperm "$(with_filter bpf)"; }
sc4() { expect_eperm "$(with_filter setns)"; }
sc5() { expect_eperm "$(with_filter unshare)"; }
sc6() { expect_eperm "$(with_filter io_uring)"; }
sc7() { expect_eperm "$(with_filter clone_userns)"; }
sc8() { expect_eperm "$(with_filter ptrace)"; }
sc9() { expect_eperm "$(with_filter alg)"; }
sc10() { [[ "$(with_filter clone3)" == *"errno=38"* ]]; }   # ENOSYS, not EPERM
sc11() { expect_not_eperm "$(without_filter setns)"; }
sc12() { expect_not_eperm "$(without_filter io_uring)"; }
sc13() { expect_eperm "$(with_filter pvm)"; }
sc14() { [[ "$(without_filter keyctl)" == *"errno=0"* ]]; }
sc15() { "$MINICT" "$ROOTFS" /bin/sh -c 'mkdir -p /mnt3 && mount -t tmpfs none /mnt3'; }
sc16() { expect_not_eperm "$(without_filter pvm)"; }
sc17() { expect_eperm "$(with_filter netlink)"; }
sc18() { [[ "$(without_filter netlink)" == *"errno=0"* ]]; }
sc19() { [[ "$(with_filter tcp)" == *"errno=0"* ]]; }

echo "=== minict seccomp tests ($(date '+%F %T')) ==="

# ---------- normal workload must survive the filter ----------
run_case SC0 "shell workload works under filter" pass sc0

# ---------- flat blocklist: blocked syscalls return EPERM ----------
run_case SC1 "keyctl blocked (EPERM)" pass sc1
run_case SC2 "open_by_handle_at blocked (EPERM)" pass sc2
run_case SC3 "bpf blocked (EPERM)" pass sc3
run_case SC4 "setns blocked (EPERM)" pass sc4
run_case SC5 "unshare blocked (EPERM)" pass sc5
run_case SC6 "io_uring_setup blocked (EPERM)" pass sc6
run_case SC7 "clone(CLONE_NEWUSER) blocked (EPERM)" pass sc7
run_case SC8 "ptrace blocked (EPERM)" pass sc8

# ---------- arg-checked rules ----------
run_case SC9 "socket(AF_ALG) blocked (EPERM)" pass sc9
run_case SC10 "clone3 returns ENOSYS (fallback-friendly)" pass sc10

# ---------- A/B control: same probes WITHOUT our filter ----------
run_case SC11 "control: setns NOT EPERM without filter" pass sc11
run_case SC12 "control: io_uring NOT EPERM without filter" pass sc12
run_case SC13 "process_vm_readv blocked (EPERM)" pass sc13
run_case SC14 "control: keyctl works without filter" pass sc14

# ---------- integration: mount blocked by seccomp too (defense in depth) ----------
run_case SC15 "mount inside sandbox fails under filter (EPERM)" fail sc15
run_case SC16 "control: process_vm NOT EPERM without filter" pass sc16

# ---------- netns hardening (D15, issue #3): netlink socket family ----------
run_case SC17 "socket(AF_NETLINK) blocked (EPERM)" pass sc17
run_case SC18 "control: netlink socket works without filter" pass sc18
run_case SC19 "control: socket(AF_INET) still allowed (httpd)" pass sc19

# ---------- SIGSYS semantics (issue #5, D33): the blocklist is ERRNO-mode —
# a blocked syscall returns errno and the process SURVIVES; no RET_TRAP
# (catchable SIGSYS signal) and no KILL on blocklist hits. A mutant flipping
# ERRNO->TRAP (or KILL) makes busybox mount die by signal (rc 128+31=159 /
# 137) instead of returning rc=1 -> this case goes red. Filter-vs-capdrop
# attribution is already covered by the probe A/B controls (SC1 vs SC14);
# this case locks the SURVIVAL contract on the shell path. Wrong-arch
# SECCOMP_RET_KILL_THREAD is intentional (hard fail on non-x86_64) and out
# of scope here.
sc20() { "$MINICT" "$ROOTFS" /bin/sh -c 'mount -t tmpfs none /mnt5 2>/dev/null; test $? -eq 1'; }
run_case SC20 "blocklist hit survives: rc=1 (EPERM), no SIGSYS death" pass \
    sc20

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
