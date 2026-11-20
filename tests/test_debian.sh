#!/usr/bin/env bash
# test_debian.sh — Debian rootfs (bookworm-slim) boots in minict + apt online
# via pasta (issue #88, D46 — GĐ1 mục 1 residual, "Alpine trước, Debian sau").
#
# Rootfs source: debuerreotype/docker-debian-artifacts dist-amd64 branch
# (official `debian:bookworm-slim` Docker rootfs, debuerreotype 0.17,
# epoch 1794787200 = 2026-11-16), pinned + sha256 fail-closed by
# tools/fetch-debian-rootfs.sh (D37 pattern). NOT tracked in git (~83 MB
# unpacked, measured D46) — gitignored, rebuilt when missing, same class as
# rootfs-alpine (D36) and tools/bin/pasta (D34/D37).
#
# Cases (8):
#   DB1    boot: dash payload, /bin/sh executable, NO busybox in rootfs
#          (anti-vacuous: proves we are NOT secretly on busybox)
#   DB2    glibc dynamic linking works in sandbox: dpkg --version
#   DB3    /proc = pidns view (payload pid 2) — env-aware D39
#          (host refuses procfs mount → xfail, PROC1/AL4 pattern)
#   DB4    single-uid ownership (AL6 pattern on glibc): file created inside
#          ns is 0:0 in ns and == real host uid host-side (private copy)
#   DB5    chown foreign uid: EINVAL (unmapped), sandbox alive (AL7 pattern)
#   DB6    CONTROL no-pasta: apt-get update unreachable in lo-only netns
#          (PT4C pattern: PROBE_ALIVE anti-vacuous + payload-process EXIT
#          wait — apt on lo-only can take seconds, absence of the OK marker
#          is only trusted after the payload exited)
#   DB7    with pasta: apt-get update + apt-get install tree ONLINE
#          (PT4 pattern: signature-verified — apt/gpgv is always on, no
#          --allow-untrusted exists; 2-step anti-vacuous #80: dpkg -s tree
#          must FAIL before install; host-side dpkg-DB + binary asserts
#          BEFORE down) (env-gated: /dev/net/tun)
#   DB8    pristine guard on the SHARED rootfs-debian (AL9 pattern, gitignored
#          variant): content manifest unchanged during the suite + no tree in
#          the shared dpkg DB (mutating cases run in PRIVATE instance copies)
#
# Env gates (never fake-PASS, D34/D36/D39):
#   - rootfs-debian missing + fetch fails → rc 125 = SKIP for the whole
#     suite (env-blocked: network to the pinned source), NOT a fake PASS;
#   - /dev/net/tun missing → DB7 SKIP (dev container measured: none;
#     GitHub ubuntu-24.04 runners have it → real run in CI);
#   - /proc mount EPERM (kernel-gated) → DB3 xfail (D39).
#
# NOTE: mutating cases (DB4/DB6/DB7) go through `minict.sh up --rootfs`
# (private copy per instance — wrapper D43). One-shot cases (DB1/DB2/DB3/DB5)
# run on the shared rootfs but are read-only; DB8 proves it.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
DROOTFS="$REPO/rootfs-debian"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }

# ---------- rootfs acquisition (fail-closed, D37) ------------------------
ensure_debian_rootfs() {
    [[ -f "$DROOTFS/.debian-stamp" && -e "$DROOTFS/bin/sh" ]] && return 0
    bash "$REPO/tools/fetch-debian-rootfs.sh" "$DROOTFS" >/dev/null 2>&1 \
        || return 125
    return 0
}
ensure_debian_rootfs || {
    echo "SKIP suite: rootfs-debian fetch failed (network to pinned source)"
    echo "=== summary: PASS=0 FAIL=0 SKIP=1 ==="
    exit 0
}

DEB_SHA="$(cat "$DROOTFS/.debian-stamp")"
DEB_KB="$(du -sk "$DROOTFS" | awk '{print $1}')"

# one-shot command on the SHARED rootfs (read-only cases only)
ic() { "$MINICT" "$DROOTFS" /bin/sh -c "$1"; }

# instance lifecycle helper (private copy, wrapper D43)
up()  { bash "$REPO/sandbox/minict.sh" up "$1" "${@:2}"; }
down() { bash "$REPO/sandbox/minict.sh" down "$1" >/dev/null 2>&1 || true; }

NAME_D="debci-$$"
cleanup() {
    for s in -own -ctl -apt; do down "$NAME_D$s"; done
}
trap cleanup EXIT

# diagnostics on FAIL (pasta pattern): instance log + pasta.log tails — CI
# diagnosability (SIA class: a bare FAIL with no output is not debuggable)
dump_logs() { # <suffix>
    local s="$1"
    echo "        --- instance $NAME_D$s log tail:"
    tail -12 "$REPO/instances/$NAME_D$s/log" 2>/dev/null | sed 's/^/        /' || true
    echo "        --- pasta.log tail:"
    tail -8 "$REPO/instances/$NAME_D$s/pasta.log" 2>/dev/null | sed 's/^/        /' || true
}

# wait_for_log <file> <pattern> [tries] — host-side poll (pasta pattern)
wait_for_log() {
    local f="$1" pat="$2" tries="${3:-30}"
    local i
    for i in $(seq 1 "$tries"); do
        grep -q "$pat" "$f" 2>/dev/null && return 0
        sleep 0.1
    done
    return 1
}

# wait_for_exit <pidfile> [tries] — poll until the payload PROCESS is gone
# (DB6 lesson: a payload that can run for up to 60 s must EXIT before the
# absence of its success marker means anything)
wait_for_exit() {
    local pf="$1" tries="${2:-200}"
    local i pid
    [[ -f "$pf" ]] || return 0
    pid="$(cat "$pf" 2>/dev/null || true)"
    [[ -n "$pid" ]] || return 0
    for i in $(seq 1 "$tries"); do
        kill -0 "$pid" 2>/dev/null || return 0
        sleep 0.25
    done
    return 1
}

# /proc probe (same as test_isolation.sh / test_alpine.sh — D39)
if unshare --user --map-root-user --pid --fork --mount --mount-proc true \
        >/dev/null 2>&1; then
    PROC_ENV=ok
else
    PROC_ENV=blocked
fi
db3_present_test() {
    ic 'awk "/^Pid:/{p=\$2} END{exit !(p==2)}" /proc/$$/status && [ -r /proc/1/status ]'
}
db3_absent_test() {
    ic '[ -e /proc/self ]'
}

# content manifest of the shared rootfs (gitignored dir — no git churn probe,
# so hash file paths + content + symlink targets; AL9 equivalent for D46)
rootfs_manifest() {
    find "$1" \( -type f -o -type l \) -printf '%P\n' 2>/dev/null \
        | LC_ALL=C sort | while IFS= read -r p; do
            f="$1/$p"
            if [[ -L "$f" ]]; then
                printf 'L %s -> %s\n' "$p" "$(readlink "$f")"
            else
                printf 'F %s %s\n' "$p" "$(sha256sum "$f" | awk '{print $1}')"
            fi
        done | sha256sum | awk '{print $1}'
}
MANIFEST_BEFORE="$(rootfs_manifest "$DROOTFS")"

PASS=0; SKIP=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|skip|xfail> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass)  [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        skip)  if [[ $rc -eq 125 ]]; then verdict=SKIP
               elif [[ $rc -eq 0 ]]; then verdict=PASS
               else verdict=FAIL; fi ;;
        xfail) if [[ $rc -ne 0 ]]; then verdict="PASS(expected-fail)"; rc=0;
               else verdict="FAIL(unexpected success)"; fi ;;
    esac
    printf '%-5s %-56s %s\n' "$id" "$desc" "$verdict"
    # accounting matches test_alpine.sh (FAIL* prefix — PASS(expected-fail)
    # counts as PASS, xfail semantics PROC1/AL4); SKIP counted separately
    # (pasta pattern — env-blocked is never a PASS)
    if [[ "$verdict" == SKIP ]]; then
        SKIP=$((SKIP+1)); echo "        (env-blocked: see case — D34/D37)"
    elif [[ "$verdict" == FAIL* ]]; then
        FAIL=$((FAIL+1)); echo "        output: $(echo "$out" | head -6 | tr '\n' ' ')"
    else
        PASS=$((PASS+1))
    fi
    return 0
}

echo "=== minict debian-rootfs tests ($(date '+%F %T')) — bookworm-slim 12.15 amd64 (epoch 1794787200), sha256 ${DEB_SHA:0:12}…, ${DEB_KB} KB ==="

# DB1 — boot + anti-busybox (dash, no busybox in debian rootfs)
run_case DB1 "debian rootfs boots (dash, NO busybox) — #88" pass \
    ic '[ ! -e /bin/busybox ] && [ -x /bin/sh ] && [ -x /bin/dash ] && echo DEBIAN_BOOT && /usr/bin/dpkg-query -S /bin/dash 2>/dev/null | grep -q "^dash: /bin/dash"'

# DB2 — glibc dynamic linking + dpkg work in the sandbox
run_case DB2 "dpkg --version works (glibc dynamic) — #88" pass \
    ic '/usr/bin/dpkg --version | grep -q "Debian .dpkg. package management program"'

# DB3 — /proc inside pidns, env-aware (D39)
if [[ $PROC_ENV == ok ]]; then
    run_case DB3 "/proc = pidns view (payload pid 2) — #31/D39" pass db3_present_test
else
    run_case DB3 "/proc absent: host refuses procfs (env, D39)" xfail db3_absent_test
fi

# DB4 — single-uid ownership on glibc (AL6 pattern, PRIVATE copy).
# Probe path is /var/tmp, NOT /tmp: minict mounts a tmpfs on /tmp (measured,
# minict.c do_mounts) so /tmp files are invisible host-side BY DESIGN.
db4_test() {
    up "$NAME_D-own" --rootfs "$DROOTFS" -- \
        /bin/sh -c 'touch /var/tmp/own-test && echo OWN_TOUCHED' >/dev/null 2>&1
    local rc_up=$? log="$REPO/instances/$NAME_D-own/log" ok=0
    wait_for_log "$log" "OWN_TOUCHED" 100 && ok=1
    local host_uid_in_ns=-1
    if [[ $ok -eq 1 ]]; then
        host_uid_in_ns="$(stat -c %u "$REPO/instances/$NAME_D-own/rootfs/var/tmp/own-test" 2>/dev/null || echo -1)"
    fi
    down "$NAME_D-own"
    [[ $rc_up -eq 0 && $ok -eq 1 && "$host_uid_in_ns" == "$(id -u)" ]]
}
run_case DB4 "single-uid ownership: ns 0:0 = real uid on host" pass db4_test

# DB5 — chown foreign uid: EINVAL (AL7 pattern on GNU coreutils)
db5_test() {
    local out
    out="$(ic 'chown 100:100 /etc/passwd 2>&1; echo RC=$?')"
    echo "$out" | grep -q '^RC=1$' || return 1
    echo "$out" | grep -qi 'invalid argument' || return 1
    ic '[ -e /etc/passwd ]'  # sandbox still alive after the failed chown
}
run_case DB5 "chown foreign uid: EINVAL (unmapped), alive" pass db5_test

# DB6 — CONTROL: no pasta → apt update unreachable (PT4C pattern)
db6_test() {
    up "$NAME_D-ctl" --rootfs "$DROOTFS" -- /bin/sh -c \
        "echo PROBE_ALIVE && timeout 60 apt-get update >/dev/null 2>&1 && echo APT_CONTROL_OK" \
        >/dev/null 2>&1
    local rc_up=$? log="$REPO/instances/$NAME_D-ctl/log" alive=0
    wait_for_log "$log" "PROBE_ALIVE" 50 && alive=1
    if [[ $alive -eq 0 ]]; then return 1; fi
    # payload may run up to ~60 s: wait for EXIT before trusting the absence
    wait_for_exit "$REPO/instances/$NAME_D-ctl/pid" 320 || return 1
    local clean=1
    grep -q APT_CONTROL_OK "$log" 2>/dev/null && clean=0
    down "$NAME_D-ctl"
    [[ $rc_up -eq 0 && $alive -eq 1 && $clean -eq 1 ]]
}
run_case DB6 "control: WITHOUT pasta, apt update unreachable" pass db6_test

# DB7 — with pasta: apt update + install tree, signature-verified (PT4 pattern)
db7_test() {
    [[ -c /dev/net/tun ]] || return 125
    # pasta must be BUILT BEFORE the measured window (CI lesson 2026-11-20:
    # the first build — download + make static — ran inside DB7's 90 s wait
    # and starved the apt attempts; the pasta suite builds it at PT1, but the
    # debian suite runs BEFORE the pasta suite in run-suites order)
    bash "$REPO/tools/fetch-pasta.sh" >/dev/null 2>&1 || return 125
    # payload: wait for DNS readiness via glibc getent (the slim rootfs ships
    # resolv.conf 1.1.1.1 — unreachable pre-attach; glibc DNS timeouts would
    # eat the first apt attempts — measured on CI), 2-step anti-vacuous
    # (dpkg -s tree FAILS first — #80 class), then RETRIES apt update, install
    # --no-install-recommends tree, marker ONLY when dpkg agrees. Signature
    # verification is mandatory in apt (gpgv) — no --allow-untrusted exists.
    up "$NAME_D-apt" --pasta --rootfs "$DROOTFS" -- /bin/sh -c \
        'i=0; while [ $i -lt 240 ]; do getent hosts deb.debian.org >/dev/null 2>&1 && { echo DNS_READY; break; }; sleep 0.25; i=$((i+1)); done; if /usr/bin/dpkg -s tree >/dev/null 2>&1; then echo APT_VACUOUS; exit 1; fi; j=0; while [ $j -lt 15 ]; do timeout 30 apt-get update >/dev/null 2>&1 && { echo APT_UP_OK; break; }; sleep 1; j=$((j+1)); done; timeout 240 apt-get install -y --no-install-recommends tree >/dev/null 2>&1 && /usr/bin/dpkg -s tree >/dev/null 2>&1 && echo APT_ONLINE_OK' \
        >/dev/null 2>&1
    local rc_up=$? log="$REPO/instances/$NAME_D-apt/log"
    local ok=0 ok_dns=0 ok_db=0 ok_bin=0
    wait_for_log "$log" "APT_ONLINE_OK" 2400 && ok=1
    grep -q DNS_READY "$log" 2>/dev/null && ok_dns=1
    # host-side STATE asserts BEFORE down (down deletes the instance dir)
    grep -q "^Package: tree$" \
        "$REPO/instances/$NAME_D-apt/rootfs/var/lib/dpkg/status" 2>/dev/null && ok_db=1
    [[ -x "$REPO/instances/$NAME_D-apt/rootfs/usr/bin/tree" ]] && ok_bin=1
    if [[ $ok -eq 0 || $ok_db -eq 0 || $ok_bin -eq 0 ]]; then dump_logs "-apt"; fi
    down "$NAME_D-apt"
    [[ $rc_up -eq 0 && $ok -eq 1 && $ok_dns -eq 1 && $ok_db -eq 1 && $ok_bin -eq 1 ]]
}
run_case DB7 "with pasta: apt update+install tree ONLINE — #88" skip db7_test

# DB8 — pristine guard on the SHARED rootfs (AL9 pattern, gitignored variant)
db8_test() {
    local bad=0
    if grep -q "^Package: tree$" "$DROOTFS/var/lib/dpkg/status" 2>/dev/null; then
        echo "       (shared dpkg DB contains tree — rootfs nhiễm, class #80)"; bad=1
    fi
    if [[ -e "$DROOTFS/var/lib/apt/lists/partial" ]] \
       && [[ -n "$(ls -A "$DROOTFS/var/lib/apt/lists" 2>/dev/null | grep -v partial)" ]]; then
        echo "       (shared apt lists populated — rootfs nhiễm, class #80)"; bad=1
    fi
    local after
    after="$(rootfs_manifest "$DROOTFS")"
    if [[ "$after" != "$MANIFEST_BEFORE" ]]; then
        echo "       (content manifest of shared rootfs-debian changed in suite run)"
        # diagnostics: NAME the differing files (SIA class — a bare mismatch
        # is not debuggable; CI 2026-11-20 run #1 hit this with no diff shown)
        rootfs_manifest "$DROOTFS" > /tmp/db8-after.txt 2>/dev/null || true
        rootfs_manifest "$DROOTFS" > /tmp/db8-after2.txt 2>/dev/null || true
        diff <(printf '%s\n' "$MANIFEST_BEFORE" | sort) <(sort /tmp/db8-after.txt) | head -12 | sed 's/^/        /'
        bad=1
    fi
    return $bad
}
run_case DB8 "shared rootfs pristine (manifest+dpkg DB) — #80 class" pass db8_test

echo "=== summary: PASS=$PASS SKIP=$SKIP FAIL=$FAIL ==="
echo "(SKIP = env-blocked on this host: no /dev/net/tun — real run on CI runners, D34/D37)"
[[ $FAIL -eq 0 ]]
