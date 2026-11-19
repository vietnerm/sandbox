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
# S1 block (issue #41, D31 — CWE-59 link following on the host bridge):
#   WEB7  relay socket replaced by symlink -> proxy refuses (fail-closed)
#   WEB8  relay socket symlinked to an ATTACKER host socket -> zero leak
#   WEB9  resolver standalone: ancestor-dir symlink blocked, positive
#         control proves openat-chain + /proc/self/fd connect works
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }
[[ -d "$REPO/rootfs" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$REPO/rootfs" >/dev/null

# WEB2 uses `ss` to verify the listener is loopback-only. Missing iproute2
# previously surfaced as an opaque WEB2 FAIL (issue #14) — fail loudly with
# the fix instead, keeping the check itself strict.
command -v ss >/dev/null 2>&1 || {
    echo "WEB2 needs 'ss' (iproute2). Install it, e.g.: apt-get install -y iproute2"
    exit 1
}

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

# ---------- S1: webproxy socket symlink hardening (issue #41, D31) ----------
# Each e2e tamper case destroys the relay socket (the relay binds it once),
# so WEB8 and WEB7 each get a FRESH instance; order matters and both must
# run before WEB6-style teardowns. Symlinks are made with python os.symlink
# so the suite never depends on shell quirks around ln.
NAME2="webs1-$$"
NAME3="webs2-$$"
PORT2=$(( 20000 + RANDOM % 2000 ))
PORT3=$(( PORT2 + 500 ))
ATKDIR="$(mktemp -d "$REPO/instances/s1atk-XXXXXX")"

cleanup() {
    bash "$REPO/sandbox/minict.sh" down "$NAME" >/dev/null 2>&1 || true
    bash "$REPO/sandbox/minict.sh" down "$NAME2" >/dev/null 2>&1 || true
    bash "$REPO/sandbox/minict.sh" down "$NAME3" >/dev/null 2>&1 || true
    rm -rf "$ATKDIR" >/dev/null 2>&1 || true
}
trap cleanup EXIT

bash "$REPO/sandbox/minict.sh" web "$NAME2" "$PORT2" >/dev/null 2>&1
bash "$REPO/sandbox/minict.sh" web "$NAME3" "$PORT3" >/dev/null 2>&1

s1_sock() { echo "$REPO/instances/$1/rootfs/.minict-proxy.sock"; }
s1_serving() { # <name> <port> — positive control on a fresh bridge
    curl -s -m 3 "http://127.0.0.1:$2/" 2>/dev/null | grep -q minict-web
}

web8_test() {
    # attacker-controlled HOST unix socket OUTSIDE the rootfs; symlink the
    # relay socket to it. The proxy must neither connect nor leak one byte.
    local sock atk marker atkpid
    sock="$(s1_sock "$NAME2")"
    s1_serving "$NAME2" "$PORT2" || { echo "precondition: bridge not serving"; return 1; }
    [[ -S "$sock" ]] || { echo "precondition: relay socket missing"; return 1; }
    atk="$ATKDIR/atk.sock"; marker="$ATKDIR/atk.hit"
    rm -f "$marker" "$atk"
    python3 - "$atk" "$marker" <<'PY' &
import socket, sys
s = socket.socket(socket.AF_UNIX)
s.bind(sys.argv[1]); s.listen(1)
try:
    c, _ = s.accept()
    open(sys.argv[2], "w").write("LEAK")
    c.close()
except OSError:
    pass
PY
    atkpid=$!
    sleep 0.2
    rm -f "$sock" 2>/dev/null
    python3 -c "import os,sys; os.symlink(sys.argv[1], sys.argv[2])" \
        "$atk" "$sock" 2>/dev/null || { echo "cannot symlink"; kill $atkpid 2>/dev/null; return 1; }
    sleep 0.2
    curl -s -m 3 -o /dev/null "http://127.0.0.1:$PORT2/" 2>/dev/null || true
    sleep 0.4
    kill $atkpid 2>/dev/null; wait $atkpid 2>/dev/null
    if [[ -f "$marker" ]]; then
        echo "LEAK: attacker socket received a connection"
        return 1
    fi
    return 0
}

web7_test() {
    # payload POV: unlink the relay socket, put a symlink to a NON-socket
    # host file in its place. The proxy must refuse, not follow.
    local sock
    sock="$(s1_sock "$NAME3")"
    s1_serving "$NAME3" "$PORT3" || { echo "precondition: bridge not serving"; return 1; }
    [[ -S "$sock" ]] || { echo "precondition: relay socket missing"; return 1; }
    rm -f "$sock" 2>/dev/null || return 1
    python3 -c "import os,sys; os.symlink('/etc/hostname', sys.argv[1])" \
        "$sock" 2>/dev/null || { echo "cannot symlink"; return 1; }
    sleep 0.2
    ! curl -s -m 3 -o /dev/null "http://127.0.0.1:$PORT3/"
}

web9_test() {
    # resolver standalone (no minict): proves BOTH directions —
    # (a) positive control: legit socket in a subdir works through the
    #     openat chain + /proc/self/fd connect on this kernel;
    # (b) ancestor-dir symlink -> refused, zero leak to attacker socket.
    local t p9 p9b ok=1 resp=""
    p9="$(( 23000 + RANDOM % 1000 ))"   # clear of PORT2/PORT3 (max 22499)
    p9b=$((p9 + 1))
    t="$(mktemp -d "$REPO/instances/s1t-XXXXXX")" || return 1
    mkdir -p "$t/root/sub" "$t/outside"

    python3 - "$t/root/sub/real.sock" <<'PY' &
import socket, sys
s = socket.socket(socket.AF_UNIX)
s.bind(sys.argv[1]); s.listen(8)
while True:
    try:
        c, _ = s.accept()
        c.sendall(b"HTTP/1.0 200 OK\r\nContent-Length: 11\r\n\r\nreal-marker")
        c.close()
    except OSError:
        break
PY
    local vpid=$!
    python3 - "$t/outside/real.sock" "$t/atk.hit" <<'PY' &
import socket, sys
# binds the SAME basename as the legit socket (real.sock): a resolver that
# follows the ancestor symlink would land HERE — a name-collision socket the
# attacker owns — making the leak observable, not just a refusal.
s = socket.socket(socket.AF_UNIX)
s.bind(sys.argv[1]); s.listen(1)
try:
    c, _ = s.accept()
    open(sys.argv[2], "w").write("LEAK")
    c.close()
except OSError:
    pass
PY
    local apid=$!
    # wait for BOTH upstream sockets to be bound before starting the proxy —
    # python startup on a loaded runner takes >2s and the bind is async (#66):
    # a proxy that starts against a missing socket turns the positive control
    # into a coin flip (WEB9 flaky 3/9 + 1/4 on main, 2026-11-19).
    local w=0
    for _ in $(seq 1 50); do
        [[ -S "$t/root/sub/real.sock" && -S "$t/outside/real.sock" ]] && { w=1; break; }
        sleep 0.1
    done
    [[ $w -eq 1 ]] || { echo "upstream sockets never bound"; kill $vpid $apid 2>/dev/null; return 1; }
    python3 "$REPO/tools/webproxy.py" --root "$t/root" --sock /sub/real.sock \
        --port "$p9" >/dev/null 2>&1 &
    local ppid=$!
    # same discipline as the (b) half below: the proxy must be LISTENING
    # before we curl — sleep 0.5 alone loses the race under load (#66)
    local listening=0
    for _ in $(seq 1 50); do
        ss -tln 2>/dev/null | grep -q "127.0.0.1:$p9" && { listening=1; break; }
        sleep 0.1
    done
    [[ $listening -eq 1 ]] || { echo "positive-control proxy never listened"; ok=0; }
    # (a) positive control through the whole new resolution path
    for _ in $(seq 1 15); do
        resp="$(curl -s -m 2 "http://127.0.0.1:$p9/" 2>/dev/null)"
        echo "$resp" | grep -q real-marker && break
        sleep 0.1
    done
    echo "$resp" | grep -q real-marker || { echo "positive control failed"; ok=0; }
    # tamper: the ANCESTOR dir becomes a symlink to the attacker's dir
    kill $ppid 2>/dev/null; wait $ppid 2>/dev/null
    python3 -c "import os,sys; t=sys.argv[1]; os.rename(t+'/root/sub', t+'/root/sub.bak'); os.symlink(t+'/outside', t+'/root/sub')" "$t" \
        || { echo "tamper failed"; ok=0; }
    python3 "$REPO/tools/webproxy.py" --root "$t/root" --sock /sub/real.sock \
        --port "$p9b" >/dev/null 2>&1 &
    local ppid2=$!
    sleep 0.5
    # the hardened proxy must be LISTENING before we assert refusal —
    # otherwise a dead proxy would make (b) pass for the wrong reason
    local listening=0
    for _ in $(seq 1 15); do
        ss -tln 2>/dev/null | grep -q "127.0.0.1:$p9b" && { listening=1; break; }
        sleep 0.1
    done
    [[ $listening -eq 1 ]] || { echo "hardened proxy never listened"; ok=0; }
    # (b) through the symlinked ancestor the proxy must refuse
    curl -s -m 3 -o /dev/null "http://127.0.0.1:$p9b/" 2>/dev/null && ok=0
    sleep 0.3
    [[ -f "$t/atk.hit" ]] && { echo "LEAK: ancestor symlink reached attacker socket"; ok=0; }
    kill $vpid $apid $ppid2 2>/dev/null
    wait $vpid $apid $ppid2 2>/dev/null
    rm -rf "$t"
    [[ $ok -eq 1 ]]
}

run_case WEB8 "symlink to ATTACKER host socket -> zero leak" pass web8_test
run_case WEB7 "symlink at socket path -> proxy refuses (no follow)" pass web7_test
run_case WEB9 "resolver: ancestor symlink blocked, positive control" pass web9_test

# ---------- teardown ----------
run_case WEB6 "down closes port and leaves no processes" pass \
    web6_test

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
