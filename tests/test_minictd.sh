#!/usr/bin/env bash
# test_minictd.sh — end-to-end suite for the minictd control-plane daemon
# (GĐ2 seed, integrated 2026-11-18 from the minictd handoff).
#
# Drives the REAL stack: minictd (Go) -> patched minict (MINICT_SPEC) ->
# minict-exec (setns) -> busybox rootfs, over the HTTP API with a token.
#
# Cases:
#   MD0   seccomp filter in minict-exec.c == minict.c (no drift; filters
#         are per-thread and do not cross setns, so exec re-installs a copy)
#   MD1   /healthz with token -> 200
#   MD2   request without token -> 401
#   MD3   daemon refuses to start with no token (fail-closed auth)
#   MD4   Run + Inspect -> running
#   MD5   exec runs via setns, sees spec hostname
#   MD6   exec shares the payload's mount ns (sees payload /tmp marker)
#   MD7   spec env reaches payload, request env reaches exec
#   MD8   bind :ro visible inside and NOT writable
#   MD9   seccomp re-installed in exec (unshare -U -> EPERM)
#   MD10  daemon env (incl. token) does NOT leak into payload or exec
#   MD11  exec timeout -> exit 124, timed_out=true
#   MD12  exec stdin passthrough
#   MD13  execstream NDJSON: stdout frame + exactly one exit frame
#   MD14  preview label minict.webport: host curl reaches httpd in netns
#   MD15  path-traversal name -> 400, nothing touched outside state dir
#   MD16  Stop -> Start keeps rootfs changes
#   MD17  bind target symlink planted in rootfs -> Start refuses, host untouched
#   MD18  exec on stopped -> 409, unknown instance -> 404
#   MD19  Remove -> dir gone, no instance processes left
#   MD20  CLI sandbox round-trip: list/up/inspect/exec/stop/rm via the CLI
#         binary over the same API (issue #91, D47)
#   MD21  --autostart: daemon killed -> payload killed (reboot semantics,
#         state.json still running) -> fresh daemon --autostart heals +
#         wake-recreates from spec.json, NEW pid, rootfs changes kept
#         (issue #91, D47)
#   MD22  deploy: install.sh renders the --user unit; systemd-analyze
#         verify passes (SKIP without systemd-analyze)
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
MINICTD="$REPO/minictd/bin/minictd"
MEXEC="$REPO/minictd/bin/minict-exec"
SANDBOX="$REPO/minictd/bin/sandbox"

for b in "$MINICT" "$MINICTD" "$MEXEC" "$SANDBOX"; do
    [[ -x "$b" ]] || { echo "missing $b — run: make build minictd"; exit 1; }
done
[[ -d "$REPO/rootfs" ]] || bash "$REPO/sandbox/mkrootfs.sh" "$REPO/rootfs" >/dev/null

WORK="$(mktemp -d)"
STATE="$WORK/state"
TOKEN="tok-$$-$RANDOM"
API_PORT=$(( 17600 + RANDOM % 300 ))
WEB_PORT=$(( 19000 + RANDOM % 900 ))
API="http://127.0.0.1:$API_PORT"
DPID=""

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
    for n in md-a md-b md-c md-auto; do
        curl -s -m 5 -X POST -H "Authorization: Bearer $TOKEN" \
            "$API/v1/sandboxes/$n/remove" >/dev/null 2>&1 || true
    done
    [[ -n "$DPID" ]] && kill "$DPID" 2>/dev/null
    wait 2>/dev/null
    rm -rf "$WORK"
}
trap cleanup EXIT

# api <method> <path> [json-body] -> prints body, returns 0 on 2xx
api() {
    local m="$1" p="$2" body="${3:-}" code
    if [[ -n "$body" ]]; then
        code=$(curl -s -m 20 -o "$WORK/resp" -w '%{http_code}' -X "$m" \
            -H "Authorization: Bearer $TOKEN" -d "$body" "$API$p")
    else
        code=$(curl -s -m 20 -o "$WORK/resp" -w '%{http_code}' -X "$m" \
            -H "Authorization: Bearer $TOKEN" "$API$p")
    fi
    echo "$code" > "$WORK/code"
    cat "$WORK/resp"
    [[ "$code" == 2* ]]
}
code() { cat "$WORK/code"; }
# jget <python-expr over d> reads $WORK/resp
jget() { python3 -I -c "import json,sys; d=json.load(open(sys.argv[1])); print($1)" "$WORK/resp"; }
# xv2 <name> <json> -> runs execv2, leaves response in $WORK/resp
xv2() { api POST "/v1/sandboxes/$1/execv2" "$2" >/dev/null; }

echo "=== minictd e2e tests ($(date '+%F %T')) — api :$API_PORT, web :$WEB_PORT ==="

# ---------- static: seccomp parity ----------
# filter body without comments and without the (differently spelled)
# disable switch; enum + blocklist verbatim
seccomp_src() {
    awk '/^enum \{/,/^\};/' "$1"
    awk '/^static const unsigned short g_blocklist/,/^\};/' "$1"
    awk '/^static void install_seccomp\(void\)/,/^}/' "$1" \
        | sed '/NO_SECCOMP\|g_no_seccomp/,/^        }/d' \
        | grep -vE '^[[:space:]]*(/\*|\*)'
}
md0() {
    diff <(seccomp_src "$REPO/sandbox/minict.c") \
         <(seccomp_src "$REPO/minictd/cmd/minict-exec/minict-exec.c")
}
run_case MD0 "seccomp blocklist identical: minict.c vs minict-exec.c" pass md0

# ---------- auth posture ----------
md3() {
    env -u MINICTD_TOKEN "$MINICTD" --listen "127.0.0.1:$((API_PORT+1))" \
        --state-dir "$WORK/noauth" --minict-bin "$MINICT" >/dev/null 2>&1 &
    local p=$!
    sleep 0.5
    if kill -0 "$p" 2>/dev/null; then kill "$p"; return 1; fi
    return 0
}
run_case MD3 "daemon refuses to start without a token" pass md3

mkdir -p "$WORK/bindsrc" && echo "bind-marker" > "$WORK/bindsrc/hello"
LEAKME="leak-$$" MINICTD_TOKEN="$TOKEN" "$MINICTD" \
    --listen "127.0.0.1:$API_PORT" --state-dir "$STATE" \
    --minict-bin "$MINICT" --minict-exec-bin "$MEXEC" \
    --base-rootfs "$REPO/rootfs" --webproxy "$REPO/tools/webproxy.py" \
    > "$WORK/daemon.log" 2>&1 &
DPID=$!
for _ in $(seq 1 50); do
    curl -s -m 1 -H "Authorization: Bearer $TOKEN" "$API/healthz" >/dev/null 2>&1 && break
    sleep 0.1
done

run_case MD1 "/healthz with token -> 200" pass api GET /healthz
run_case MD2 "request without token -> 401" pass \
    sh -c "[ \"\$(curl -s -o /dev/null -w '%{http_code}' $API/v1/version)\" = 401 ]"

# ---------- lifecycle + exec ----------
RUN_A=$(cat <<EOF
{"name":"md-a","image":"base","hostname":"mdhost",
 "env":["SPECVAR=from-spec"],
 "volumes":["$WORK/bindsrc:/data:ro"],
 "labels":["minict.webport=$WEB_PORT"],
 "cmd":["/bin/sh","-c","echo payload-\$SPECVAR > /tmp/marker; env > /tmp/payload.env; mkdir -p /www && echo minictd-web > /www/index.html; httpd -f -p 127.0.0.1:$WEB_PORT -h /www & sleep 600"]}
EOF
)
md4() {
    api POST /v1/run "$RUN_A" >/dev/null || return 1
    sleep 0.5
    api GET /v1/sandboxes/md-a >/dev/null && [[ "$(jget 'd["State"]["Status"]')" == running ]]
}
run_case MD4 "Run + Inspect -> running" pass md4

md5() {
    xv2 md-a '{"cmd":["/bin/hostname"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["stdout"].strip()')" == mdhost ]] || return 1
    api GET /v1/sandboxes/md-a/state >/dev/null && [[ "$(jget 'd["exec_mode"]')" == setns ]]
}
run_case MD5 "exec via setns sees spec hostname" pass md5

md6() {
    xv2 md-a '{"cmd":["/bin/cat","/tmp/marker"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["stdout"].strip()')" == payload-from-spec ]]
}
run_case MD6 "exec shares payload mount ns (/tmp marker visible)" pass md6

md7() {
    xv2 md-a '{"cmd":["/bin/sh","-c","echo $REQVAR"],"env":["REQVAR=from-req"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["stdout"].strip()')" == from-req ]]
}
run_case MD7 "spec env -> payload, request env -> exec" pass md7

md8() {
    xv2 md-a '{"cmd":["/bin/cat","/data/hello"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["stdout"].strip()')" == bind-marker ]] || return 1
    xv2 md-a '{"cmd":["/bin/sh","-c","echo x > /data/new"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["exit_code"]')" != 0 ]] && [[ ! -e "$WORK/bindsrc/new" ]]
}
run_case MD8 "bind :ro visible and not writable" pass md8

md9() {
    xv2 md-a '{"cmd":["/bin/unshare","-U","/bin/true"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["exit_code"]')" != 0 ]] && jget 'd["stderr"]' | grep -qi "not permitted"
}
run_case MD9 "seccomp in exec: unshare -U -> EPERM" pass md9

md10() {
    xv2 md-a '{"cmd":["/bin/env"],"timeout_s":5}' || return 1
    jget 'd["stdout"]' | grep -qE "LEAKME|MINICTD_TOKEN|$TOKEN" && return 1
    xv2 md-a '{"cmd":["/bin/cat","/tmp/payload.env"],"timeout_s":5}' || return 1
    jget 'd["stdout"]' | grep -q SPECVAR || return 1
    jget 'd["stdout"]' | grep -qE "LEAKME|MINICTD_TOKEN|$TOKEN" && return 1
    return 0
}
run_case MD10 "daemon env/token NOT leaked into payload or exec" pass md10

md11() {
    xv2 md-a '{"cmd":["/bin/sleep","30"],"timeout_s":2}' || return 1
    [[ "$(jget 'd["exit_code"]')" == 124 && "$(jget 'd["timed_out"]')" == True ]]
}
run_case MD11 "exec timeout -> 124 + timed_out" pass md11

md12() {
    local b64; b64=$(printf 'stdin-ok' | base64)
    xv2 md-a "{\"cmd\":[\"/bin/cat\"],\"stdin\":\"$b64\",\"timeout_s\":5}" || return 1
    [[ "$(jget 'd["stdout"]')" == stdin-ok ]]
}
run_case MD12 "exec stdin passthrough" pass md12

md13() {
    api POST /v1/sandboxes/md-a/execstream \
        '{"cmd":["/bin/sh","-c","echo out1; echo err1 >&2; exit 3"],"timeout_s":5}' >/dev/null || return 1
    python3 -I - "$WORK/resp" <<'PY'
import json, sys, base64
fr = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
out = b"".join(base64.b64decode(f["d"]) for f in fr if f["t"] == "stdout")
ex = [f for f in fr if f["t"] == "exit"]
assert out == b"out1\n", out
assert len(ex) == 1 and fr[-1]["t"] == "exit" and ex[0]["exit_code"] == 3, fr
PY
}
run_case MD13 "execstream NDJSON stdout + single exit frame" pass md13

run_case MD14 "preview webport: host curl -> httpd in netns" pass \
    sh -c "for i in 1 2 3 4 5 6 7 8 9 10; do curl -s -m 2 http://127.0.0.1:$WEB_PORT/ | grep -q minictd-web && exit 0; sleep 0.3; done; exit 1"

md15() {
    mkdir -p "$WORK/victim"
    api POST "/v1/sandboxes/..%2Fvictim/remove" >/dev/null && return 1
    [[ "$(code)" == 400 ]] || return 1
    api GET "/v1/sandboxes/..%2F..%2Fetc/logs" >/dev/null && return 1
    [[ "$(code)" == 400 ]] && [[ -z "$(ls -A "$WORK/victim")" ]]
}
run_case MD15 "path-traversal name -> 400, outside untouched" pass md15

md16() {
    xv2 md-a '{"cmd":["/bin/sh","-c","echo persist > /persist.txt"],"timeout_s":5}' || return 1
    api POST /v1/sandboxes/md-a/stop '{"timeout_sec":1}' >/dev/null || return 1
    api GET /v1/sandboxes/md-a >/dev/null && [[ "$(jget 'd["State"]["Status"]')" == exited ]] || return 1
    api POST /v1/sandboxes/md-a/start >/dev/null || return 1
    sleep 0.5
    xv2 md-a '{"cmd":["/bin/cat","/persist.txt"],"timeout_s":5}' || return 1
    [[ "$(jget 'd["stdout"].strip()')" == persist ]]
}
run_case MD16 "Stop -> Start keeps rootfs changes" pass md16

md17() {
    mkdir -p "$WORK/hosttarget"
    api POST /v1/run "{\"name\":\"md-b\",\"image\":\"base\",\"volumes\":[\"$WORK/bindsrc:/evil/x\"],\"cmd\":[\"/bin/sleep\",\"600\"]}" >/dev/null || return 1
    sleep 0.3
    api POST /v1/sandboxes/md-b/stop '{"timeout_sec":1}' >/dev/null || return 1
    # what a payload can do to its own persistent rootfs: swap a parent of
    # the bind target for an absolute symlink pointing at a HOST dir
    rm -rf "$STATE/md-b/rootfs/evil"
    ln -s "$WORK/hosttarget" "$STATE/md-b/rootfs/evil"
    api POST /v1/sandboxes/md-b/start >/dev/null
    sleep 0.5
    api GET /v1/sandboxes/md-b >/dev/null
    [[ "$(jget 'd["State"]["Status"]')" == exited ]] || return 1
    grep -q "symlink component" "$STATE/md-b/log" || return 1
    [[ -z "$(ls -A "$WORK/hosttarget")" ]]
}
run_case MD17 "symlinked bind target -> Start refused, host untouched" pass md17

md18() {
    api POST /v1/sandboxes/md-b/execv2 '{"cmd":["/bin/true"]}' >/dev/null && return 1
    [[ "$(code)" == 409 ]] || return 1
    api GET /v1/sandboxes/no-such >/dev/null && return 1
    [[ "$(code)" == 404 ]]
}
run_case MD18 "exec on stopped -> 409, unknown -> 404" pass md18

md19() {
    api POST /v1/sandboxes/md-a/remove >/dev/null || return 1
    api POST /v1/sandboxes/md-b/remove >/dev/null || return 1
    sleep 0.5
    [[ ! -d "$STATE/md-a" && ! -d "$STATE/md-b" ]] || return 1
    pgrep -f "$STATE/md-" >/dev/null && return 1
    ! curl -s -m 1 "http://127.0.0.1:$WEB_PORT/" >/dev/null 2>&1
}
run_case MD19 "Remove -> dir gone, no instance processes" pass md19

# ---------- CLI + autostart + deploy (issue #91, D47) ----------
sb() { "$SANDBOX" --addr "$API" --token "$TOKEN" "$@"; }

md20() {
    sb list | grep -qv md-c || return 1            # starts empty-ish, md-c absent
    sb up md-c base --cmd "/bin/sh,-c,echo cli-marker > /tmp/cli.txt; sleep 600" >/dev/null || return 1
    sleep 0.5
    sb list | grep -q md-c || return 1
    sb inspect md-c | python3 -I -c \
        "import json,sys; d=json.load(sys.stdin); assert d['State']['Status']=='running'" || return 1
    sb exec md-c /bin/cat /tmp/cli.txt | grep -q cli-marker || return 1
    sb stop md-c --sec 1 || return 1
    sb inspect md-c | python3 -I -c \
        "import json,sys; d=json.load(sys.stdin); assert d['State']['Status']=='exited'" || return 1
    sb rm md-c || return 1
}
run_case MD20 "CLI sandbox round-trip: list/up/inspect/exec/stop/rm" pass md20

md21() {
    sb up md-auto base --cmd "/bin/sh,-c,echo auto-marker > /tmp/auto.txt; sleep 600" >/dev/null || return 1
    sleep 0.5
    local old new st
    old=$(sb inspect md-auto | python3 -I -c "import json,sys; print(json.load(sys.stdin)['State']['Pid'])")
    [[ -n "$old" ]] || return 1
    # reboot semantics: daemon dies first (payload is Setsid -> survives),
    # then the payload shim is killed — state.json still says running.
    kill "$DPID"; wait "$DPID" 2>/dev/null; DPID=""
    kill "$old" 2>/dev/null
    sleep 0.6
    pgrep -f "$STATE/md-auto" >/dev/null 2>&1 && return 1
    # fresh daemon with --autostart (+ A3 reconcile ticker)
    MINICTD_TOKEN="$TOKEN" "$MINICTD" \
        --listen "127.0.0.1:$API_PORT" --state-dir "$STATE" \
        --minict-bin "$MINICT" --minict-exec-bin "$MEXEC" \
        --base-rootfs "$REPO/rootfs" --webproxy "$REPO/tools/webproxy.py" \
        --autostart --reconcile-interval 5 \
        > "$WORK/daemon2.log" 2>&1 &
    DPID=$!
    for _ in $(seq 1 50); do
        curl -s -m 1 -H "Authorization: Bearer $TOKEN" "$API/healthz" >/dev/null 2>&1 && break
        sleep 0.1
    done
    st=none
    for _ in $(seq 1 15); do
        st=$(sb inspect md-auto 2>/dev/null | python3 -I -c "import json,sys; print(json.load(sys.stdin)['State']['Status'])" 2>/dev/null || echo none)
        [[ "$st" == running ]] && break
        sleep 0.3
    done
    [[ "$st" == running ]] || return 1
    new=$(sb inspect md-auto | python3 -I -c "import json,sys; print(json.load(sys.stdin)['State']['Pid'])")
    [[ -n "$new" && "$new" != "$old" ]] || return 1
    sb exec md-auto /bin/cat /tmp/auto.txt | grep -q auto-marker || return 1
    sb rm md-auto >/dev/null || return 1
}
run_case MD21 "--autostart heals + wake-recreates running-after-reboot" pass md21

md22() {
    command -v systemd-analyze >/dev/null 2>&1 || return 125
    mkdir -p "$WORK/home"
    bash "$REPO/minictd/deploy/install.sh" --home "$WORK/home" --bindir bin \
        > "$WORK/install.log" 2>&1 || return 1
    [[ -f "$WORK/home/.config/systemd/user/minictd.service" ]] || return 1
    [[ -x "$WORK/home/bin/minictd" ]] || return 1
    grep -q "ExecStart=$WORK/home/bin/minictd" \
        "$WORK/home/.config/systemd/user/minictd.service" || return 1
    # placeholder leak check (fail-closed render)
    if grep -q '@HOME@\|@BINDIR@' "$WORK/home/.config/systemd/user/minictd.service"; then return 1; fi
    systemd-analyze verify "$WORK/home/.config/systemd/user/minictd.service" || return 1
}
run_case MD22 "deploy: install.sh renders --user unit, systemd-analyze verify" skip md22

echo "=== summary: PASS=$PASS SKIP=$SKIPPED FAIL=$FAIL ==="
if [[ $FAIL -ne 0 ]]; then
    echo "--- daemon log (tail) ---"; tail -20 "$WORK/daemon.log" 2>/dev/null
    echo "--- daemon2 log (tail) ---"; tail -20 "$WORK/daemon2.log" 2>/dev/null
    echo "--- install log (tail) ---"; tail -10 "$WORK/install.log" 2>/dev/null
fi
[[ $FAIL -eq 0 ]]
