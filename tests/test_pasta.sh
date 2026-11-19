#!/usr/bin/env bash
# test_pasta.sh — pasta connectivity suite for minict (GĐ1 mục 3, issue #57,
# D34 design → D37 implementation).
#
# pasta (passt project) is attached HOST-side to an instance's netns:
#   pasta --foreground --userns /proc/<pid>/ns/user --netns /proc/<pid>/ns/net
#         --config-net
# The instance keeps its full isolation (userns+mountns+pidns+capdrop+NNP+
# seccomp); pasta adds a tap + default route and forwards egress traffic by
# socket splicing on the host. D16 unix-socket bridge stays the loopback-only
# fallback; this suite exercises the REAL egress path.
#
# Cases:
#   PT1    pasta binary acquired via tools/fetch-pasta.sh (pin + sha256 + static
#          build) — runs EVERYWHERE (dev + CI)
#   PT2    pinned version stamp matches `pasta --version` — runs everywhere
#   NET11C CONTROL: WITHOUT pasta, an instance in its own netns CANNOT reach a
#          host 0.0.0.0 listener via the gateway IP — proves NET11's success is
#          caused by pasta, not by leaked host connectivity (runs everywhere)
#   NET10  with pasta: instance sees an Ethernet tap (ifconfig, ioctl-based —
#          /proc is NOT mountable inside, AL4; AF_NETLINK is seccomp-blocked,
#          D15) + default route asserted HOST-side via the netns-scoped
#          /proc/<pid>/net/route (env-gated)
#   NET11  with pasta: egress — one-shot busybox wget to the host listener VIA
#          THE GATEWAY IP (pasta maps gateway → host loopback by default,
#          --map-host-loopback) after the tap is visible (env-gated)
#   NET12  with pasta: host→instance port-forward via MINICT_PASTA_FWD "-t
#          FPORT:8080" → host curl hits httpd inside the sandbox (env-gated)
#   AB1    with pasta: pasta attach latency (up --pasta → route ready + alive),
#          measured HOST-side and reported separately from minict cold_start
#          (env-gated)
#
# Env gate (D34): pasta needs /dev/net/tun. The dev container has NONE
# (measured 2026-11-19, kernel 5.10.134) → NET10/NET11/NET12/AB1 are
# SKIP(env-blocked: no /dev/net/tun), NOT fake-PASS. GitHub ubuntu-24.04
# runners have the device → cases run for real in CI.
#
# Anti-vacuous + anti-race rules (class AL4/FS6, SIA #54):
#   - every expected-failure grep runs BEFORE the instance is torn down
#     (down deletes the instance dir);
#   - in-instance assertions WAIT for the pasta tap (ifconfig poll — ioctl,
#     works without /proc) before printing/asserting;
#   - the suite polls the instance log from the HOST side for markers, so the
#     verdict does not depend on grep timing right after `up` returns;
#   - on FAIL, instance log + pasta.log tails are printed (CI diagnosability).
#
# NOTE: cases run commands INSIDE the pasta-attached instance via the `up --`
# payload override — `exec` starts a NEW ns set (PoC limitation, poc.md) and
# would miss the tap. Do not "fix" the cases to use exec.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs"
PASTA_VERSION="2026_10_02.cba3570"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

NAME_P="pastaci-$$"
HPORT=$(( 19400 + RANDOM % 200 ))    # host listener for NET11C/NET11
FPORT=$(( 19700 + RANDOM % 200 ))    # host port-forward for NET12

PASS=0; SKIP=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|skip> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass) [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        skip) if [[ $rc -eq 125 ]]; then verdict=SKIP
              elif [[ $rc -eq 0 ]]; then verdict=PASS
              else verdict=FAIL; fi ;;
    esac
    printf '%-7s %-56s %s\n' "$id" "$desc" "$verdict"
    case "$verdict" in
        PASS) PASS=$((PASS+1)) ;;
        SKIP) SKIP=$((SKIP+1)); echo "        (env-blocked: no /dev/net/tun on this host — D34/D37)" ;;
        FAIL) FAIL=$((FAIL+1)); echo "        output: $(echo "$out" | head -6 | tr '\n' ' ')" ;;
    esac
    return 0
}

# skip-guard: rc 125 = "env-blocked, skip the case" (never a fake PASS)
skip_no_tun() {
    [[ -c /dev/net/tun ]] || return 125
    return 0
}

# wait_for_log <file> <pattern> [tries] — host-side poll of the instance log
wait_for_log() {
    local f="$1" pat="$2" tries="${3:-30}"
    local i
    for i in $(seq 1 "$tries"); do
        grep -q "$pat" "$f" 2>/dev/null && return 0
        sleep 0.1
    done
    return 1
}

SUFFIXES=" tap eg pf ab"
cleanup() {
    for s in "" $SUFFIXES; do
        bash "$REPO/sandbox/minict.sh" down "$NAME_P$s" >/dev/null 2>&1 || true
    done
    [[ -n "${SRV_PID:-}" ]] && kill "$SRV_PID" 2>/dev/null || true
}
trap cleanup EXIT

# egress target: the HOST's default gateway — pasta maps the gateway address to
# host loopback by default (--map-host-loopback), so a 0.0.0.0 listener on the
# host is reachable from inside WITHOUT any internet/DNS dependency.
GW_IP="$(ip -4 route show default 2>/dev/null | awk '{print $3; exit}')"
if [[ -z "$GW_IP" ]]; then
    echo "FATAL: cannot determine the host default gateway (ip route) — egress cases need it"; exit 1
fi
echo "=== minict pasta tests ($(date '+%F %T')) — host listener $HPORT, fwd $FPORT, gw $GW_IP ==="

# host-side listener used by the NET11C control and the NET11 egress case;
# binds 0.0.0.0 so it catches both the loopback NAT (gateway → 127.0.0.1)
# and any direct host-IP path.
python3 -m http.server "$HPORT" --bind 0.0.0.0 >/dev/null 2>&1 &
SRV_PID=$!
sleep 0.5

dump_logs() { # <suffix> — tails for CI diagnosability
    local d="$REPO/instances/$NAME_P$1"
    echo "        --- instance log ($d/log) ---"
    tail -6 "$d/log" 2>/dev/null | sed 's/^/        /'
    echo "        --- pasta.log ($d/pasta.log) ---"
    tail -6 "$d/pasta.log" 2>/dev/null | sed 's/^/        /'
}

# ---------- acquisition (runs everywhere) ----------
run_case PT1 "pasta binary pinned+verified+static built" pass \
    bash "$REPO/tools/fetch-pasta.sh"

run_case PT2 "pasta --version == pin exactly ($PASTA_VERSION)" pass \
    sh -c "'$REPO/tools/bin/pasta' --version | head -1 | grep -qx 'pasta $PASTA_VERSION'"

# ---------- control: no pasta → gateway unreachable (runs everywhere) ----------
net11c_test() {
    # anti-vacuous (M0 class, same as CHROOT1 #54): assert the sandbox ACTUALLY
    # started (payload prints PROBE_ALIVE) BEFORE trusting the absence of
    # UNREACH_OK — otherwise a dead instance would "pass" this expect-fail case
    bash "$REPO/sandbox/minict.sh" up "$NAME_P" -- \
        /bin/sh -c "busybox echo PROBE_ALIVE && busybox wget -T 2 -q -O /dev/null http://$GW_IP:$HPORT/ && echo UNREACH_OK" \
        >/dev/null 2>&1
    sleep 0.5
    local log="$REPO/instances/$NAME_P/log"
    if ! wait_for_log "$log" "PROBE_ALIVE" 20; then
        dump_logs ""
        return 1
    fi
    if grep -q UNREACH_OK "$log"; then
        dump_logs ""
        return 1
    fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P" >/dev/null 2>&1
    return 0
}
run_case NET11C "control: WITHOUT pasta, gateway unreachable" pass net11c_test

# ---------- env-gated: real pasta attachment (CI / tun present) ----------
net10_test() {
    skip_no_tun || return $?
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-tap" --pasta -- /bin/sh -c \
        'i=0; while [ $i -lt 100 ]; do busybox ifconfig -a | grep -q "encap:Ethernet" && break; busybox sleep 0.1; i=$((i+1)); done; busybox ifconfig -a; busybox sleep 3' \
        >/dev/null 2>&1
    local rc_up=$?
    local log="$REPO/instances/$NAME_P-tap/log"
    local ok_tap=0 ok_route=0
    # tap: wait for the payload to print ifconfig output (poll the log)
    wait_for_log "$log" "encap:Ethernet" 50 && ok_tap=1
    # default route: assert HOST-side via the netns-scoped procfs of the shim
    local pid
    pid="$(cat "$REPO/instances/$NAME_P-tap/pid" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && awk '$2 == "00000000"' "/proc/$pid/net/route" 2>/dev/null | grep -q .; then
        ok_route=1
    fi
    if [[ $ok_tap -eq 0 || $ok_route -eq 0 ]]; then dump_logs "-tap"; fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-tap" >/dev/null 2>&1
    [[ $rc_up -eq 0 && $ok_tap -eq 1 && $ok_route -eq 1 ]]
}
run_case NET10 "with pasta: tap iface + default route inside" skip net10_test

net11_test() {
    skip_no_tun || return $?
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-eg" --pasta -- \
        /bin/sh -c 'i=0; while [ $i -lt 100 ]; do busybox ifconfig -a | grep -q "encap:Ethernet" && break; busybox sleep 0.1; i=$((i+1)); done; busybox wget -T 5 -q -O /dev/null http://'"$GW_IP:$HPORT"'/ && echo EGRESS_OK' \
        >/dev/null 2>&1
    local rc_up=$?
    local log="$REPO/instances/$NAME_P-eg/log"
    local ok=0
    wait_for_log "$log" "EGRESS_OK" 50 && ok=1
    if [[ $ok -eq 0 ]]; then dump_logs "-eg"; fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-eg" >/dev/null 2>&1
    [[ $rc_up -eq 0 && $ok -eq 1 ]]
}
run_case NET11 "with pasta: egress wget sandbox→host via gateway" skip net11_test

net12_test() {
    skip_no_tun || return $?
    # payload is SELF-CONTAINED: it creates the docroot and the marker BEFORE
    # httpd starts (CI round 1: httpd died at startup because the suite wrote
    # /www only after `up` returned — busybox httpd resolves -h at start).
    MINICT_PASTA_FWD="-t $FPORT:8080" \
        bash "$REPO/sandbox/minict.sh" up "$NAME_P-pf" --pasta -- \
        /bin/sh -c 'mkdir -p /www && printf "<p>marker: pasta-port-forward</p>" > /www/index.html && exec busybox httpd -f -p 0.0.0.0:8080 -h /www' \
        >/dev/null 2>&1
    local rc_up=$?
    local ok=0 i
    for i in $(seq 1 12); do
        curl -s -m 2 "http://127.0.0.1:$FPORT/" 2>/dev/null | grep -q pasta-port-forward && { ok=1; break; }
        sleep 0.25
    done
    if [[ $ok -eq 0 ]]; then dump_logs "-pf"; fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-pf" >/dev/null 2>&1
    [[ $rc_up -eq 0 && $ok -eq 1 ]]
}
run_case NET12 "with pasta: host→instance port-forward (-t FPORT:8080)" skip net12_test

ab1_test() {
    skip_no_tun || return $?
    # pasta attach latency, host-side metric (separate from minict cold_start —
    # pasta spawns after `up` returns, out-of-band): time up --pasta end-to-end
    # (rootfs copy + cold start + pasta attach + route wait + aliveness check)
    local t0 t1 ms rc
    t0=$(date +%s%N)
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-ab" --pasta >/dev/null 2>&1
    rc=$?
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-ab" >/dev/null 2>&1
    [[ $rc -eq 0 ]] || return 1
    t1=$(date +%s%N)
    ms=$(( (t1 - t0) / 1000000 ))
    echo "        pasta attach end-to-end (up --pasta incl. route wait): ${ms} ms (host-side, not cold_start)"
    [[ $ms -lt 30000 ]]
}
run_case AB1 "with pasta: attach latency measured (host-side)" skip ab1_test

echo "=== summary: PASS=$PASS SKIP=$SKIP FAIL=$FAIL ==="
echo "(SKIP = pasta env-blocked on this host: no /dev/net/tun — real run happens on CI runners, D34/D37)"
[[ $FAIL -eq 0 ]]
