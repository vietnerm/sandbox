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
#   PT3C   CONTROL: WITHOUT pasta, nslookup inside the netns-only instance
#          FAILS (no reachable resolver in the namespace) — proves PT3's
#          success comes from pasta DNS forwarding, not from anything else
#          (runs everywhere, same class as NET11C)
#   PT3    with pasta: resolve-by-name — spawn_pasta passes --dns-forward <gw>
#          (D42: DNS flows to that address are forwarded to the host resolver,
#          fwd.c fwd_nat_from_tap precedence over map-gw) and writes
#          `nameserver <gw>` into the instance rootfs AFTER tap+route+alive;
#          payload nslookups github.com in a retry loop (resolv.conf lands
#          during the tap wait — anti-race) and asserts DNS_RESOLVED_OK;
#          host-side asserts rootfs/etc/resolv.conf content BEFORE down
#          (env-gated)
#   PT4C   CONTROL (issue #79, D43): WITHOUT pasta, `apk update` inside an
#          ALPINE rootfs instance FAILS (netns lo-only, no resolver — musl
#          falls back to 127.0.0.1:53, refused) — proves PT4's success comes
#          from pasta connectivity, not from anything else. Anti-vacuous:
#          PROBE_ALIVE guard before trusting the absence of APK_CONTROL_OK
#          (runs everywhere; SKIP(env-blocked) only when rootfs-alpine is
#          missing AND the CDN rebuild fails — never a fake PASS)
#   PT4    with pasta: apk add ONLINE (GĐ1 mục 1 residual, issue #79/D43) —
#          alpine instance (--rootfs) attached via pasta: payload waits for
#          the tap, retries `apk update` (APKINDEX over HTTPS via dl-cdn),
#          then `apk add --no-cache tree` (no deps, 30 KB; NOT in the base
#          rootfs apk DB) — signature-verified (NO --allow-untrusted) —
#          asserts `apk info -e tree` + marker APK_ONLINE_OK; HOST-side
#          asserts the apk DB of the instance rootfs records `P:tree` BEFORE
#          down (a log marker alone is not state — PT2 lesson) (env-gated)
#   AB1    with pasta: pasta attach latency (up --pasta → route ready + alive),
#          measured HOST-side and reported separately from minict cold_start
#          (env-gated)
#
# Env gate (D34): pasta needs /dev/net/tun. The dev container has NONE
# (measured 2026-11-19, kernel 5.10.134) → NET10/NET11/NET12/PT3/AB1 are
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

SUFFIXES=" -tap -eg -pf -ab -dns -ndns -apk"
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
# binds 127.0.0.1 ONLY (SIA: a 0.0.0.0 bind on a runner is transiently exposed
# to the internet; pasta maps gateway → 127.0.0.1 by default, so loopback
# binding is sufficient for the egress path).
python3 -m http.server "$HPORT" --bind 127.0.0.1 >/dev/null 2>&1 &
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

# ---------- control: no pasta → no resolver inside the namespace ----------
pt3c_test() {
    # anti-vacuous (M0 class): PROBE_ALIVE first, then expect nslookup to FAIL
    # — rootfs ships no /etc/resolv.conf (mkrootfs.sh, measured) and the netns
    # has only lo (down), so even a stale nameserver entry cannot resolve.
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-ndns" -- \
        /bin/sh -c "busybox echo PROBE_ALIVE && busybox nslookup -timeout=2 github.com && echo DNS_CONTROL_OK" \
        >/dev/null 2>&1
    sleep 0.5
    local log="$REPO/instances/$NAME_P-ndns/log"
    if ! wait_for_log "$log" "PROBE_ALIVE" 20; then
        dump_logs "-ndns"
        return 1
    fi
    if grep -q DNS_CONTROL_OK "$log"; then
        dump_logs "-ndns"
        return 1
    fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-ndns" >/dev/null 2>&1
    return 0
}
run_case PT3C "control: WITHOUT pasta, resolver unavailable" pass pt3c_test

# ---------- control: no pasta → apk cannot fetch its index (issue #79) ------
# shared helper: the alpine rootfs is tracked in git (D36); if it went missing
# locally, rebuild once from the pinned tarball — a CDN failure here is
# env-blocked (rc 125 = SKIP, never a fake PASS; same class as AL5, SIA #24)
ensure_alpine_rootfs() {
    [[ -d "$REPO/rootfs-alpine" ]] && return 0
    bash "$REPO/sandbox/mkrootfs-alpine.sh" "$REPO/rootfs-alpine" >/dev/null 2>&1 \
        || return 125
    return 0
}

pt4c_test() {
    ensure_alpine_rootfs || return $?
    # anti-vacuous (M0 class, same as NET11C): assert the sandbox ACTUALLY
    # started (payload prints PROBE_ALIVE) BEFORE trusting the absence of
    # APK_CONTROL_OK — a dead instance would otherwise "pass" this control
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-apk" \
        --rootfs "$REPO/rootfs-alpine" -- \
        /bin/sh -c "busybox echo PROBE_ALIVE && /sbin/apk update >/dev/null 2>&1 && echo APK_CONTROL_OK" \
        >/dev/null 2>&1
    sleep 0.5
    local log="$REPO/instances/$NAME_P-apk/log"
    if ! wait_for_log "$log" "PROBE_ALIVE" 20; then
        dump_logs "-apk"
        return 1
    fi
    if grep -q APK_CONTROL_OK "$log"; then
        dump_logs "-apk"
        return 1
    fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-apk" >/dev/null 2>&1
    return 0
}
run_case PT4C "control: WITHOUT pasta, apk update unreachable" skip pt4c_test

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

# ---------- env-gated: DNS via pasta (--dns-forward + resolv.conf, D42) ----
pt3_test() {
    skip_no_tun || return $?
    # payload waits for the tap (resolv.conf is written by spawn_pasta right
    # after route-ready + aliveness — roughly when the tap appears), then
    # retries nslookup (marker + retry loop, anti-race pattern NET10/NET11).
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-dns" --pasta -- \
        /bin/sh -c 'i=0; while [ $i -lt 100 ]; do busybox ifconfig -a | grep -q "encap:Ethernet" && break; busybox sleep 0.1; i=$((i+1)); done; j=0; while [ $j -lt 15 ]; do busybox nslookup -timeout=1 github.com && echo DNS_RESOLVED_OK && break; busybox sleep 0.5; j=$((j+1)); done' \
        >/dev/null 2>&1
    local rc_up=$?
    local log="$REPO/instances/$NAME_P-dns/log"
    local ok=0 ok_resolv=0
    wait_for_log "$log" "DNS_RESOLVED_OK" 250 && ok=1
    # (a) resolver file injected host-side, pointing at the --dns-forward
    #     address (= GW_IP). Assert BEFORE down — down deletes the dir.
    grep -qx "nameserver $GW_IP" \
        "$REPO/instances/$NAME_P-dns/rootfs/etc/resolv.conf" 2>/dev/null && ok_resolv=1
    if [[ $ok -eq 0 || $ok_resolv -eq 0 ]]; then dump_logs "-dns"; fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-dns" >/dev/null 2>&1
    [[ $rc_up -eq 0 && $ok -eq 1 && $ok_resolv -eq 1 ]]
}
run_case PT3 "with pasta: resolver injected + nslookup resolves" skip pt3_test

# ---------- env-gated: apk add ONLINE in an alpine instance (issue #79/D43) --
pt4_test() {
    skip_no_tun || return $?
    ensure_alpine_rootfs || return $?
    # payload is SELF-CONTAINED (NET12 lesson): waits for the tap first
    # (resolv.conf is written by spawn_pasta right after route-ready +
    # aliveness — roughly when the tap appears), then RETRIES apk update
    # (musl getaddrinfo reads the single injected nameserver, wiki.musl-libc
    # D43 research), then installs `tree` — signature-verified, NO
    # --allow-untrusted; prints a marker ONLY when `apk info -e tree` agrees.
    bash "$REPO/sandbox/minict.sh" up "$NAME_P-apk" --pasta \
        --rootfs "$REPO/rootfs-alpine" -- \
        /bin/sh -c 'i=0; while [ $i -lt 100 ]; do busybox ifconfig -a | grep -q "encap:Ethernet" && break; busybox sleep 0.1; i=$((i+1)); done; j=0; while [ $j -lt 20 ]; do /sbin/apk update >/dev/null 2>&1 && break; busybox sleep 1; j=$((j+1)); done; /sbin/apk add --no-cache tree >/dev/null 2>&1 && /sbin/apk info -e tree && echo APK_ONLINE_OK' \
        >/dev/null 2>&1
    local rc_up=$?
    local log="$REPO/instances/$NAME_P-apk/log"
    local ok=0 ok_db=0 ok_bin=0
    wait_for_log "$log" "APK_ONLINE_OK" 400 && ok=1
    # host-side STATE asserts, BEFORE down (down deletes the instance dir):
    # (a) the apk DB of the instance rootfs records P:tree — the install went
    #     through the full apk path (index fetch + signature + unpack), not a
    #     stale entry (tree is NOT in the base rootfs DB, measured D43);
    # (b) the binary actually landed in the instance rootfs.
    grep -qx "P:tree" \
        "$REPO/instances/$NAME_P-apk/rootfs/lib/apk/db/installed" 2>/dev/null && ok_db=1
    [[ -x "$REPO/instances/$NAME_P-apk/rootfs/usr/bin/tree" ]] && ok_bin=1
    if [[ $ok -eq 0 || $ok_db -eq 0 || $ok_bin -eq 0 ]]; then dump_logs "-apk"; fi
    bash "$REPO/sandbox/minict.sh" down "$NAME_P-apk" >/dev/null 2>&1
    [[ $rc_up -eq 0 && $ok -eq 1 && $ok_db -eq 1 && $ok_bin -eq 1 ]]
}
run_case PT4 "with pasta: apk add ONLINE (HTTPS+signature) in alpine" skip pt4_test

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
