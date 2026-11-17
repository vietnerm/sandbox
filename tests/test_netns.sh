#!/usr/bin/env bash
# test_netns.sh — network namespace isolation suite for minict (issue #3).
#
# The sandbox runs in its OWN netns (loopback-only, `lo` brought up by
# minict while it still holds caps — D18). The host reaches a web instance
# through the unix-socket bridge (D16):
#   host webproxy.py (127.0.0.1:PORT) -> AF_UNIX rootfs/.minict-proxy.sock
#   -> in-sandbox relay -> 127.0.0.1:PORT (httpd inside the netns)
#
# Cases:
#   NS1  sandbox CANNOT reach a host loopback listener (own empty loopback)
#   NS2  control: MINICT_NO_NETNS=1 CAN reach the same listener — proves the
#        NS1 failure comes from OUR netns, not from "nothing listening"
#   NS3  web E2E through the unix-socket bridge (200 + instance marker)
#   NS4  in-sandbox relay socket exists on the instance rootfs (host-visible)
#   NS5  host-side proxy binds loopback-only (no 0.0.0.0)
#   NS6  two instances: each proxy routes to ITS OWN sandbox (no mixing)
#   NS7  down: proxy killed, port freed, instance dir removed
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs"
INST="$REPO/instances"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$ROOTFS" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$ROOTFS" >/dev/null

NAME="netnsci-$$"
NAME2="netnsci2-$$"
PORT=$(( 18400 + RANDOM % 100 ))
PORT2=$(( 18500 + RANDOM % 100 ))
SRV_PORT=$(( 18600 + RANDOM % 100 ))   # host-side listener for NS1/NS2

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

cleanup() {
    bash "$REPO/sandbox/minict.sh" down "$NAME"  >/dev/null 2>&1 || true
    bash "$REPO/sandbox/minict.sh" down "$NAME2" >/dev/null 2>&1 || true
    [[ -n "${SRV_PID:-}" ]] && kill "$SRV_PID" 2>/dev/null || true
}
trap cleanup EXIT

ic() { "$MINICT" "$ROOTFS" /bin/sh -c "$1"; }

echo "=== minict netns tests ($(date '+%F %T')) — ports $PORT/$PORT2, host srv $SRV_PORT ==="

# host-side loopback listener used to discriminate our netns from shared net
python3 -m http.server "$SRV_PORT" --bind 127.0.0.1 >/dev/null 2>&1 &
SRV_PID=$!
sleep 0.5

# ---------- isolation ----------
run_case NS1 "sandbox netns CANNOT reach host loopback listener" fail \
    ic "busybox wget -T 2 -q -O /dev/null http://127.0.0.1:$SRV_PORT/"

run_case NS2 "control: MINICT_NO_NETNS=1 reaches the same listener" pass \
    sh -c "MINICT_NO_NETNS=1 '$MINICT' '$ROOTFS' /bin/sh -c \
           'busybox wget -T 2 -q -O /dev/null http://127.0.0.1:$SRV_PORT/'"

# ---------- web bridge E2E ----------
bash "$REPO/sandbox/minict.sh" web "$NAME" "$PORT" >/dev/null 2>&1

run_case NS3 "host curl -> unix bridge -> httpd in netns (200+marker)" pass \
    sh -c "curl -s -m 5 http://127.0.0.1:$PORT/ | grep -q minict-web"

run_case NS4 "relay unix socket exists on instance rootfs" pass \
    sh -c "[ -S '$INST/$NAME/rootfs/.minict-proxy.sock' ]"

run_case NS5 "host proxy binds loopback-only (no 0.0.0.0)" pass \
    sh -c "ss -tln | grep -q '127.0.0.1:$PORT' && ! ss -tln | grep -q '0.0.0.0:$PORT'"

# ---------- routing correctness: two instances, no cross-mixing ----------
bash "$REPO/sandbox/minict.sh" web "$NAME2" "$PORT2" >/dev/null 2>&1

run_case NS6 "two instances: each proxy serves its OWN sandbox" pass \
    sh -c "curl -s -m 5 http://127.0.0.1:$PORT/  | grep -q \"instance: $NAME\" \
           && curl -s -m 5 http://127.0.0.1:$PORT2/ | grep -q \"instance: $NAME2\" \
           && ! curl -s -m 5 http://127.0.0.1:$PORT/  | grep -q \"instance: $NAME2\""

# ---------- teardown ----------
ns7_test() {
    bash "$REPO/sandbox/minict.sh" down "$NAME" >/dev/null 2>&1
    sleep 0.3
    local rc=0
    ss -tln | grep -q "127.0.0.1:$PORT" && rc=1
    [[ -f "$INST/$NAME/proxy-pid" ]] && rc=1
    [[ -d "$INST/$NAME" ]] && rc=1
    return $rc
}
run_case NS7 "down kills proxy, frees port, removes instance" pass ns7_test

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
