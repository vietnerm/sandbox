#!/usr/bin/env bash
# test_web.sh — web service inside minict: busybox httpd serving from the
# instance rootfs, reached from the host via the shared host netns.
#
# Cases:
#   WEB1  web up -> HTTP 200 with the minict-web marker
#   WEB2  listener is loopback-only (127.0.0.1:<port>, NOT 0.0.0.0)
#   WEB3  path traversal cannot read the HOST /etc/passwd
#   WEB4  content is served from the instance PRIVATE rootfs copy
#         (probe file written into the instance fs is visible, host fs is not)
#   WEB5  HTTP error path answers (404 for missing file), not a crash
#   WEB6  down -> port closed, no leaked processes, pid file cleaned
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$REPO/rootfs" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$REPO/rootfs" >/dev/null

NAME="webci-$$"
PORT=$(( 18000 + RANDOM % 2000 ))
HOST_USER="$(id -un)"
DOCROOT_HOST="$REPO/instances/$NAME/rootfs/www"

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

cleanup() { bash "$REPO/sandbox/minict.sh" down "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# runs in the MAIN shell so pgrep -f cannot self-match this test's own
# sh -c cmdline (same pitfall as sig1_test in test_isolation.sh)
web6_test() {
    bash "$REPO/sandbox/minict.sh" down "$NAME" >/dev/null 2>&1
    sleep 0.3
    ss -tln | grep -q "127.0.0.1:$PORT" && return 1
    pgrep -f "httpd -f -p 127.0.0.1:$PORT" >/dev/null 2>&1 && return 1
    [[ -d "$REPO/instances/$NAME" ]] && return 1
    return 0
}

echo "=== minict web tests ($(date '+%F %T')) — port $PORT ==="

bash "$REPO/sandbox/minict.sh" web "$NAME" "$PORT" >/dev/null 2>&1

# ---------- serving ----------
run_case WEB1 "httpd inside sandbox answers 200 with marker" pass \
    sh -c "curl -s -m 5 http://127.0.0.1:$PORT/ | grep -q minict-web"

run_case WEB2 "listener is loopback-only (no 0.0.0.0 bind)" pass \
    sh -c "ss -tln | grep -q '127.0.0.1:$PORT' && ! ss -tln | grep -q '0.0.0.0:$PORT'"

# ---------- security ----------
run_case WEB3 "path traversal cannot read host /etc/passwd" pass \
    sh -c "curl -s -m 5 --path-as-is 'http://127.0.0.1:$PORT/../../../etc/passwd' \
             | grep -q \"${HOST_USER}:x:\" && exit 1 || exit 0"

# ---------- private rootfs proof ----------
mkdir -p "$DOCROOT_HOST" 2>/dev/null
echo "probe-$NAME" > "$DOCROOT_HOST/probe.txt" 2>/dev/null || true
run_case WEB4 "probe in instance rootfs served; host cwd is not" pass \
    sh -c "curl -s -m 5 http://127.0.0.1:$PORT/probe.txt | grep -q probe-$NAME"

run_case WEB5 "missing file -> 404, server stays alive" pass \
    sh -c "[ \"\$(curl -s -m 5 -o /dev/null -w '%{http_code}' \
             http://127.0.0.1:$PORT/no-such-file.html)\" = 404 ] \
             && curl -s -m 5 http://127.0.0.1:$PORT/ | grep -q minict-web"

# ---------- teardown ----------
run_case WEB6 "down closes port and leaves no processes" pass \
    web6_test

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
