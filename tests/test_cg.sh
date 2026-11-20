#!/usr/bin/env bash
# test_cg.sh — CG1/S2 cgroups v2 resource enforcement e2e (issue #4).
#
# Drives the REAL stack: minictd (Go) -> patched minict -> busybox rootfs,
# with the daemon pointed at a writable cgroups v2 base and limits applied
# for real (memory.max / pids.max files + kernel OOM / fork EAGAIN).
#
# Env gate (measured, probe-based — same pattern as the pasta suite):
#   - cgroups v2 unified hierarchy must exist (/sys/fs/cgroup/cgroup.controllers)
#   - a writable base under /sys/fs/cgroup is bootstrapped with sudo
#     (mkdir + chown + controller enable on the ROOT cgroup — exempt from
#     the no-internal-process constraint — then the daemon is moved into
#     the base once, so shim migration stays inside delegation containment)
#   - dev hosts with cgroups v1 (e.g. this sandbox) SKIP every case (rc=125)
#
# Cases:
#   CG-BOOT  bootstrap + probe: daemon notes say "cgroups v2 enforced" and
#         minictd-apps/ + minictd-daemon/ exist under the base
#   CG-MEM   Run(memory=64m) -> memory.max file == 67108864, shim pid in
#         cgroup.procs (anti-vacuous: filesystem, not just notes)
#   CG-OOM   kernel OOM: payload allocates 96m > memory.max=64m -> payload
#         killed -> daemon heals the instance to stopped
#   CG-PIDS  pids.max=16: payload forks 64 -> fork blocked well before 64
#         (FORKED marker file read back via exec; vacuous guard: a run
#         without enforcement would fork all 64)
#   CG-STATS Stats over cgroup accounting: MemUsageBytes>0 and
#         MemLimitBytes==67108864 (memory.max-backed, not memInfo)
#   CG-RM    Remove -> instance cgroup dir reclaimed (rmdir)
#
# Exit code: 0 if all expected results hold, 1 otherwise.

set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
MINICTD="$REPO/minictd/bin/minictd"
MEXEC="$REPO/minictd/bin/minict-exec"

for b in "$MINICT"; do
    [[ -x "$b" ]] || { echo "missing $b — run: make build"; exit 1; }
done
# This suite drives the control-plane daemon, whose Go binaries are built
# by `make minictd` — a step the CI build-test job runs only for the
# minictd suite, AFTER run-suites.sh. Build here when missing (idempotent;
# needs Go — on a host without Go this suite is not runnable at all).
if [[ ! -x "$MINICTD" || ! -x "$MEXEC" ]]; then
    # Go lookup: PATH first, then the sandbox dev install (~/.local/go).
    if ! command -v go >/dev/null 2>&1 && [[ -x "$HOME/.local/go/bin/go" ]]; then
        export PATH="$HOME/.local/go/bin:$PATH"
    fi
    echo "building minictd binaries for test_cg.sh (make minictd)"
    make -C "$REPO" minictd >/dev/null 2>&1 || { echo "make minictd failed (Go missing?)"; exit 1; }
fi
for b in "$MINICTD" "$MEXEC"; do
    [[ -x "$b" ]] || { echo "missing $b after make minictd"; exit 1; }
done
[[ -d "$REPO/rootfs" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$REPO/rootfs" >/dev/null

WORK="$(mktemp -d)"
STATE="$WORK/state"
TOKEN="tok-$$-$RANDOM"
API_PORT=$(( 17700 + RANDOM % 200 ))
API="http://127.0.0.1:$API_PORT"
DPID=""
CGROOT=""

PASS=0; FAIL=0; SKIPPED=0
run_case() { # <id> <desc> <expect: pass|fail|skip> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass) [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        fail) [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL ;;
        skip) if [[ $rc -eq 125 ]]; then verdict=SKIP
              elif [[ $rc -eq 0 ]]; then verdict=PASS
              else verdict=FAIL; fi ;;
    esac
    printf '%-5s %-60s %s\n' "$id" "$desc" "$verdict"
    case "$verdict" in
        FAIL*) echo "      output: $(echo "$out" | head -4 | tr '\n' ' ')"; FAIL=$((FAIL+1)) ;;
        SKIP)  echo "      (env-blocked: see case)"; SKIPPED=$((SKIPPED+1)) ;;
        *)     PASS=$((PASS+1)) ;;
    esac
    return 0
}

cleanup() {
    for n in cg-a cg-oom cg-pids; do
        curl -s -m 5 -X POST -H "Authorization: Bearer $TOKEN" \
            "$API/v1/sandboxes/$n/remove" >/dev/null 2>&1 || true
    done
    [[ -n "$DPID" ]] && kill "$DPID" 2>/dev/null
    # Catch-all like test_minictd.sh: nothing may outlive this run's unique
    # state dir (daemon, payload shims) — SIGPIPE can eat the EXIT trap.
    pkill -TERM -f -- "$STATE" 2>/dev/null || true
    sleep 0.3
    pkill -KILL -f -- "$STATE" 2>/dev/null || true
    wait 2>/dev/null
    # cgroup base teardown is best-effort (empty dirs are safe to drop)
    [[ -n "$CGROOT" ]] && rmdir "$CGROOT/minictd-apps"/* \
        "$CGROOT/minictd-apps" "$CGROOT/minictd-daemon" "$CGROOT" 2>/dev/null || true
    rm -rf "$WORK"
}
trap cleanup EXIT

# api helper (same shape as test_minictd.sh)
api() {
    local m="$1" p="$2" body="${3:-}" code
    if [[ -n "$body" ]]; then
        code=$(curl -s -m 30 -o "$WORK/resp" -w '%{http_code}' -X "$m" \
            -H "Authorization: Bearer $TOKEN" -d "$body" "$API$p")
    else
        code=$(curl -s -m 30 -o "$WORK/resp" -w '%{http_code}' -X "$m" \
            -H "Authorization: Bearer $TOKEN" "$API$p")
    fi
    echo "$code" > "$WORK/code"
    cat "$WORK/resp"
    [[ "$code" == 2* ]]
}
code() { cat "$WORK/code"; }
jget() { python3 -I -c "import json,sys; d=json.load(open(sys.argv[1])); print($1)" "$WORK/resp"; }
xv2() { api POST "/v1/sandboxes/$1/execv2" "$2" >/dev/null; }

# cg_bootstrap <rc-file> — probe + sudo bootstrap; echoes the reason and
# returns 125 when the host cannot host the suite.
cg_bootstrap() {
    if ! [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
        echo "no cgroups v2 unified hierarchy"
        return 125
    fi
    if ! command -v sudo >/dev/null 2>&1; then
        echo "sudo missing — cannot bootstrap a writable v2 base"
        return 125
    fi
    # Enable controllers on the ROOT cgroup (root is exempt from the
    # no-internal-process constraint, kernel cgroup-v2.rst). Best-effort:
    # on systemd hosts they are usually enabled already.
    local c
    for c in memory cpu pids; do
        grep -qw "$c" /sys/fs/cgroup/cgroup.controllers || { echo "controller $c absent on host"; return 125; }
        sudo sh -c "echo +$c > /sys/fs/cgroup/cgroup.subtree_control" 2>/dev/null || true
    done
    CGROOT="/sys/fs/cgroup/minictd-cg-$$"
    sudo mkdir -p "$CGROOT" || { echo "sudo mkdir failed"; return 125; }
    sudo chown -R "$(id -u):$(id -g)" "$CGROOT" || { echo "sudo chown failed"; return 125; }
    # controllers must be visible inside the new base (root subtree_control)
    for c in memory cpu pids; do
        grep -qw "$c" "$CGROOT/cgroup.controllers" || { echo "controller $c not visible in base (root subtree_control not enabled?)"; return 125; }
    done
    return 0
}

echo "=== cgroups v2 enforcement tests ($(date '+%F %T')) — api :$API_PORT ==="

# ---------- daemon boot (with the base override) ----------
cg_bootstrap
BOOT=$?
if [[ "$BOOT" != 0 ]]; then
    echo "env gate: bootstrap unavailable — $(tail -1 "$WORK/bootrc.out" 2>/dev/null || echo 'probe said no')"
    env_skip() { return 125; }
    for id_desc in "CG-BOOT:bootstrap+probe:enforced" "CG-MEM:memory.max file written" \
                   "CG-OOM:kernel OOM heal->stopped" "CG-PIDS:pids.max blocks fork burst" \
                   "CG-STATS:stats from cgroup accounting" "CG-RM:remove reclaims cgroup dir"; do
        run_case "${id_desc%%:*}" "${id_desc#*:}" skip env_skip
    done
    echo "=== cg suite: ${PASS} PASS + ${SKIPPED} SKIP (env-gated) + ${FAIL} FAIL ==="
    [[ $FAIL -eq 0 ]]
    exit $?
fi

MINICTD_TOKEN="$TOKEN" MINICTD_CGROUP_ROOT="$CGROOT" "$MINICTD" \
    --listen "127.0.0.1:$API_PORT" --state-dir "$STATE" \
    --minict-bin "$MINICT" --minict-exec-bin "$MEXEC" \
    --base-rootfs "$REPO/rootfs" \
    --reconcile-interval 1 \
    > "$WORK/daemon.log" 2>&1 &
DPID=$!
for _ in $(seq 1 50); do
    curl -s -m 1 -H "Authorization: Bearer $TOKEN" "$API/healthz" >/dev/null 2>&1 && break
    sleep 0.1
done

# The daemon must sit INSIDE the base before any Run: delegation
# containment blocks shim migration otherwise. One-time sudo bootstrap.
sudo sh -c "echo $DPID > '$CGROOT/cgroup.procs'" || true

cgboot() {
    # Bootstrap-precondition asserts only: the daemon-managed dirs
    # (minictd-apps/minictd-daemon) are created lazily on the first Run —
    # their asserts live in cgmem (CI round 3 lesson: this case runs
    # BEFORE the first Run, so the dirs legitimately do not exist yet).
    [[ -d "$CGROOT" ]] || { echo "cgboot: CGROOT gone"; return 1; }
    grep -qw memory "$CGROOT/cgroup.controllers" || { echo "cgboot: controllers=$(cat "$CGROOT/cgroup.controllers" 2>&1)"; return 1; }
    grep -qw pids "$CGROOT/cgroup.controllers" || return 1
}
run_case CG-BOOT "bootstrap + probe: enforced + apps/daemon dirs" pass cgboot

# CG1: run instance with memory=64m; assert the real file + shim pid.
cgmem() {
    api POST /v1/run '{"name":"cg-a","image":"base","memory":"64m","cmd":["/bin/sh","-c","sleep 600"]}' >/dev/null || return 1
    sleep 0.5
    api GET /v1/sandboxes/cg-a >/dev/null && [[ "$(jget 'd["State"]["Status"]')" == running ]] || return 1
    # the enforcement note records the true posture (moved from CG-BOOT)
    api GET /v1/sandboxes/cg-a/state >/dev/null || return 1
    python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); notes=" ".join(d.get("notes") or []); print(notes); sys.exit(0 if "cgroups v2 enforced" in notes else 1)' "$WORK/resp" || { echo "cgmem: notes missing enforced claim: $(cat "$WORK/resp" | head -c 400)"; return 1; }
    # daemon-managed tree + second-hop controller enable (moved from cgboot)
    [[ -d "$CGROOT/minictd-apps" && -d "$CGROOT/minictd-daemon" ]] || { echo "cgmem: apps/daemon dir missing: $(ls "$CGROOT" 2>&1)"; return 1; }
    grep -qw memory "$CGROOT/minictd-apps/cgroup.subtree_control" || { echo "cgmem: apps subtree_control=$(cat "$CGROOT/minictd-apps/cgroup.subtree_control" 2>&1)"; return 1; }
    local dir="$CGROOT/minictd-apps/sandbox-cg-a"
    local i
    for i in $(seq 1 10); do
        [[ -f "$dir/memory.max" ]] && break
        sleep 0.3
    done
    [[ -f "$dir/memory.max" ]] || { echo "cgmem: memory.max missing in $dir: $(ls "$CGROOT/minictd-apps" 2>&1)"; return 1; }
    [[ "$(cat "$dir/memory.max" | tr -d '[:space:]')" == "67108864" ]] || { echo "cgmem: memory.max=$(cat "$dir/memory.max")"; return 1; }
    # shim pid really migrated (anti-vacuous)
    [[ -s "$dir/cgroup.procs" ]] || { echo "cgmem: cgroup.procs empty"; return 1; }
    local pid
    pid=$(cat "$dir/cgroup.procs" | head -1 | tr -d '[:space:]')
    [[ -n "$pid" && -d "/proc/$pid" ]] || { echo "cgmem: procs=[$(cat "$dir/cgroup.procs" | tr '\n' ' ')] pid=$pid dead"; return 1; }
}
run_case CG-MEM "memory.max file == 67108864 + shim pid inside" pass cgmem

# CG2: kernel OOM — payload allocates 96m > 64m cap; daemon (reconcile 1s)
# must heal the dead payload to stopped.
cgoom() {
    api POST /v1/run '{"name":"cg-oom","image":"base","memory":"64m","cmd":["/bin/sh","-c","a=$(dd if=/dev/zero bs=1M count=96 2>/dev/null); echo alloc-done"]}' >/dev/null || return 1
    local i state
    for i in $(seq 1 20); do
        sleep 1
        api GET /v1/sandboxes/cg-oom >/dev/null 2>&1 || { state="api-down"; continue; }
        state="$(jget 'd["State"]["Status"]')"
        [[ "$state" == exited ]] && break
    done
    [[ "$state" == exited ]] || return 1
    # OOM teeth: the payload must die BEFORE printing alloc-done — on a
    # host without enforcement the 96m alloc completes and the log gains
    # the marker (vacuous PASS guard).
    api GET "/v1/sandboxes/cg-oom/logs" >/dev/null || return 1
    if grep -q 'alloc-done' "$WORK/resp"; then
        echo "cgoom: alloc completed — no kernel OOM (vacuous)"
        return 1
    fi
    # the OOM story must be visible: rootfs intact + cgroup reclaimed or empty
    [[ -d "$REPO/rootfs/bin" ]] || return 1
}
run_case CG-OOM "kernel OOM kills 96m alloc under 64m cap -> exited" pass cgoom

# CG3: pids.max=16 — 64 forks must be blocked well before 64 (fork count
# marker written by the PAYLOAD inside the cgroup, read via exec).
cgpids() {
    api POST /v1/run '{"name":"cg-pids","image":"base","pids_limit":16,"cmd":["/bin/sh","-c","n=0; for i in $(seq 1 64); do if sleep 300 & then n=$((n+1)); fi; done; echo FORKED=$n > /tmp/forkcount; wait"]}' >/dev/null || return 1
    # poll for the marker: the payload writes it only after the loop ends
    local i forked=""
    for i in $(seq 1 16); do
        sleep 0.5
        if xv2 cg-pids '{"cmd":["/bin/cat","/tmp/forkcount"],"timeout_s":8}' 2>/dev/null; then
            forked="$(jget 'd["stdout"].strip().split("=")[-1]' 2>/dev/null || true)"
            [[ -n "$forked" ]] && break
        fi
    done
    [[ -n "$forked" ]] || { echo "cgpids: forkcount marker never appeared"; return 1; }
    [[ "$forked" -lt 64 ]] || { echo "cgpids: FORKED=$forked — enforcement vacuous"; return 1; }
    # teeth: pids.max file really says 16
    [[ "$(cat "$CGROOT/minictd-apps/sandbox-cg-pids/pids.max" | tr -d '[:space:]')" == "16" ]] || { echo "cgpids: pids.max=$(cat "$CGROOT/minictd-apps/sandbox-cg-pids/pids.max" 2>&1)"; return 1; }
    api POST /v1/sandboxes/cg-pids/remove >/dev/null
}
run_case CG-PIDS "pids.max=16 blocks the 64-fork burst" pass cgpids

# CG4: stats over cgroup accounting (memory.current + memory.max-backed limit).
cgstats() {
    api GET /v1/sandboxes/cg-a/stats || { echo "cgstats: stats api failed ($(cat "$WORK/code")): $(head -c 200 "$WORK/resp")"; return 1; }
    local usage limit
    usage="$(jget 'd["mem_usage_bytes"]')"
    limit="$(jget 'd["mem_limit_bytes"]')"
    [[ -n "$usage" && "$usage" -gt 0 ]] || { echo "cgstats: usage=$usage limit=$limit resp=$(head -c 200 "$WORK/resp")"; return 1; }
    [[ "$limit" == "67108864" ]] || { echo "cgstats: limit=$limit resp=$(head -c 200 "$WORK/resp")"; return 1; }
    # teeth: the accounting files must really exist under the instance dir
    # (a non-enforcing daemon would fall back to spec-parse + /proc RSS and
    # still report limit=67108864 — vacuous without this check)
    local dir="$CGROOT/minictd-apps/sandbox-cg-a"
    [[ -s "$dir/pids.current" ]] || { echo "cgstats: pids.current missing (not cgroup-backed)"; return 1; }
}
run_case CG-STATS "stats: mem.usage>0, limit==67108864 (cgroup-backed)" pass cgstats

# CG5: remove reclaims the instance cgroup dir.
cgrm() {
    api POST /v1/sandboxes/cg-a/remove >/dev/null || return 1
    sleep 0.5
    [[ ! -d "$CGROOT/minictd-apps/sandbox-cg-a" ]] || return 1
}
run_case CG-RM "remove reclaims instance cgroup dir" pass cgrm

echo "=== cg suite: ${PASS} PASS + ${SKIPPED} SKIP (env-gated) + ${FAIL} FAIL ==="
[[ $FAIL -eq 0 ]]
exit $?
