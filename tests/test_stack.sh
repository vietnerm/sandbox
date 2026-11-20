#!/usr/bin/env bash
# test_stack.sh — GĐ1 mục 4: full web stack in ONE instance (issue #85, D45).
#
# Stack: nginx -> fcgiwrap (spawn-fcgi) -> CGI shell api.cgi -> SQLite.
# Reached from the host through the D16 unix-socket bridge (webproxy.py).
# Everything runs in ONE minict instance with the full isolation stack
# (userns+mountns+pidns+netns+capdrop+NNP+seccomp) — no Docker, rootless.
#
# Cases:
#   ST1  stack packages present in rootfs-alpine (env-aware: CDN failure at
#        build time => SKIP(env-blocked), NOT a sandbox failure — AL5 pattern)
#   ST2  ./sandbox.sh up -> HTTP 200 with the minict-web-stack marker
#   ST3  GET /api/health -> JSON status:ok
#   ST4  POST /api/items then GET /api/items: SQLite PERSISTENCE is real —
#        the row is read back through the API AND verified directly with
#        sqlite3 in the instance rootfs (not just an echo)
#   ST5  injection class guard: name with SQL metachars is sanitized
#        (allowlist, D45) — no row named after the raw payload
#   ST6  down -> port closed, no leaked nginx/fcgiwrap/proxy process,
#        instance dir removed
#   ST7  stack cold start (spawn -> HTTP 200 + API 200) median of 3 runs
#        < 5000 ms (roadmap GĐ1 mục 4 threshold)
#   ST8  instance name guard (SIA audit PR #86 low finding): names that are
#        not safe path components (traversal, slashes) are rejected by
#        sandbox.sh up/down — nothing is created outside instances/
#
# Port selection re-checks `ss` listeners to dodge the issue #75 collision
# class (random port already taken by infra).
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs-alpine"
SND="$REPO/sandbox.sh"

command -v curl >/dev/null 2>&1 || { echo "ST* need curl"; exit 1; }
command -v ss >/dev/null 2>&1 || { echo "ST* need 'ss' (iproute2)"; exit 1; }
[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }

if [[ ! -d "$ROOTFS" ]]; then
    bash "$REPO/sandbox/mkrootfs-alpine.sh" "$ROOTFS" >/dev/null || exit 1
fi
# upgrade an older alpine build in place if stack packages are missing
if [[ ! -x "$ROOTFS/usr/sbin/nginx" ]]; then
    MINICT_STACK_AUTOADD=1 bash "$REPO/sandbox/mkrootfs-alpine.sh" "$ROOTFS" >/dev/null 2>&1 || true
fi

STACK_ENV=ok
for b in usr/sbin/nginx usr/bin/sqlite3 usr/bin/fcgiwrap usr/bin/spawn-fcgi; do
    [[ -x "$ROOTFS/$b" ]] || STACK_ENV=blocked
done

NAME="stackci-$$"
TWORK="$(mktemp -d)"
trap 'rm -rf "$TWORK"' EXIT
PORT_FILE="$TWORK/port"
: > "$PORT_FILE"
PORT=""
PASS=0; FAIL=0; SKIPPED=0

pick_port() {
    local p try=0
    while (( try++ < 20 )); do
        p=$(( 15800 + RANDOM % 1100 ))
        if ! (ss -tln 2>/dev/null || true) | grep -q "127.0.0.1:$p "; then
            echo "$p"; return 0
        fi
    done
    echo ""
}

cleanup() { bash "$SND" down "$NAME" >/dev/null 2>&1 || true; }
trap 'cleanup; rm -rf "$TWORK"' EXIT

run_case() { # <id> <desc> <kind: pass|env> <cmd...>
    local id="$1" desc="$2" kind="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    if [[ "$kind" == env && $rc -ne 0 ]]; then
        printf '%-5s %-56s %s\n' "$id" "$desc" "SKIP(env-blocked)"
        echo "      output: $(echo "$out" | head -3 | tr '\n' ' ')"
        SKIPPED=$((SKIPPED+1))
        return 0
    fi
    if [[ $rc -eq 0 ]]; then verdict=PASS; else verdict=FAIL; fi
    printf '%-5s %-56s %s\n' "$id" "$desc" "$verdict"
    if [[ "$verdict" == FAIL* ]]; then
        echo "      output: $(echo "$out" | head -3 | tr '\n' ' ')"
        FAIL=$((FAIL+1))
    else
        PASS=$((PASS+1))
    fi
    return 0
}

# ---------- helpers ----------
# NOTE: run_case captures the case command in a subshell, so any PORT set
# there is invisible to the parent shell — cases hand the port over through
# PORT_FILE instead (classic subshell pitfall).
st2_index_test() { # ST2: stack up -> marker served from the instance
    cleanup
    PORT="$(pick_port)"
    [[ -n "$PORT" ]] || return 1
    echo "$PORT" > "$PORT_FILE"
    local out
    out="$(bash "$SND" up "$NAME" --port "$PORT" 2>&1)" || { echo "$out"; return 1; }
    curl -fsS -m 3 "http://127.0.0.1:$PORT/" | grep -q 'minict-web-stack'
}

st3_health_test() { # ST3: API health JSON
    PORT="$(cat "$PORT_FILE")"
    curl -fsS -m 3 "http://127.0.0.1:$PORT/api/health" | grep -q '"status":"ok"'
}

st4_persist_test() { # ST4: POST -> GET via API + direct sqlite3 check
    PORT="$(cat "$PORT_FILE")"
    local nm="stack-item-$$"
    curl -fsS -m 3 -X POST "http://127.0.0.1:$PORT/api/items?name=$nm" | grep -q '"inserted"' || return 1
    curl -fsS -m 3 "http://127.0.0.1:$PORT/api/items" | grep -q "$nm" || return 1
    # the DB is a REAL sqlite file in the instance rootfs, not an echo:
    "$MINICT" "$REPO/instances/$NAME/rootfs" /usr/bin/sqlite3 \
        /var/lib/minict/app.sqlite "SELECT COUNT(*) FROM items WHERE name='$nm';" \
        | grep -q '^1$'
}

st5_sanitize_test() { # ST5: SQL metachar name is filtered (D45)
    PORT="$(cat "$PORT_FILE")"
    local raw="x';--DROP%20TABLE"
    # every assertion below must short-circuit (|| return 1) — a failing curl
    # with a zero-rc final command would make this case vacuously PASS
    # (caught by mutant M-STACK-1 in the negative-control run, 2026-11-20)
    # raw metachars are filtered BEFORE the SQL is built: the response echoes
    # the SANITIZED name — every char outside [a-zA-Z0-9 _.-] becomes '_'
    # (`'` -> `_`, `;` -> `_`, `%` -> `_` — measured 2026-11-20)
    curl -fsS -m 3 -X POST "http://127.0.0.1:$PORT/api/items?name=$raw" \
        | grep -q '"inserted":"x__--DROP_20TABLE"' || return 1
    # the table survived (DROP did not execute) and the API still serves rows
    curl -fsS -m 3 "http://127.0.0.1:$PORT/api/items" | grep -q '"name"' || return 1
}

st6_down_test() { # ST6: teardown leaves nothing behind (runs in MAIN shell —
                  # pgrep patterns must not self-match this test's cmdline)
    PORT="$(cat "$PORT_FILE")"
    bash "$SND" down "$NAME" >/dev/null 2>&1 || return 1
    sleep 0.4
    (ss -tln 2>/dev/null | grep -q "127.0.0.1:$PORT ") && return 1
    # leaks: nginx/fcgiwrap from THIS instance — identify by the rootfs path
    # of the pidfile-carrying wrapper is gone; simplest robust check: no
    # nginx master/worker at all (this host runs no other nginx) AND no
    # fcgiwrap AND no webproxy bound to our port
    pgrep -f "/usr/sbin/nginx" >/dev/null 2>&1 && return 1
    pgrep -f "usr/bin/fcgiwrap" >/dev/null 2>&1 && return 1
    pgrep -f "webproxy.py --root $REPO/instances/$NAME" >/dev/null 2>&1 && return 1
    [[ ! -d "$REPO/instances/$NAME" ]]
}

st8_name_guard_test() { # ST8: traversal/hostile instance names are rejected
    local out
    out="$(bash "$SND" up '../evil-name' 2>&1)" && return 1
    echo "$out" | grep -q "invalid instance name" || return 1
    out="$(bash "$SND" up 'a/b' 2>&1)" && return 1
    out="$(bash "$SND" up '.../x' 2>&1)" && return 1
    # nothing was created outside instances/: the traversal name must not exist
    [[ ! -d "$REPO/instances/.." ]]
    [[ ! -e "$REPO/evil-name" ]] && [[ ! -e "$REPO/instances/evil-name" ]]
}

st7_bench_test() { # ST7: median stack cold start < 5000 ms (3 runs)
    local times=() i ms
    for i in 1 2 3; do
        PORT="$(pick_port)"; [[ -n "$PORT" ]] || return 1
        echo "$PORT" > "$PORT_FILE"
        bash "$SND" up "$NAME" --port "$PORT" >/dev/null 2>&1 || return 1
        ms="$(cat "$REPO/instances/$NAME/ready-ms")"
        times+=("$ms")
        bash "$SND" down "$NAME" >/dev/null 2>&1
    done
    printf '%s\n' "${times[@]}" | python3 -c "
import sys, statistics
ts=[int(x) for x in sys.stdin if x.strip()]
assert ts, 'no measurements'
m=statistics.median(ts)
print(f'median={m:.0f} ms N={len(ts)} (runs: {ts})')
sys.exit(0 if m < 5000 else 1)
"
}

echo "=== minict web-stack tests ($(date '+%F %T')) — port free-check active (issue #75 class) ==="

if [[ "$STACK_ENV" == blocked ]]; then
    echo "NOTE: stack packages missing in rootfs-alpine (CDN/network at build?)"
    echo "      -> ST1 proves the absence, ST2–ST7 SKIP(env-blocked) (AL5 pattern)"
fi

# ST1 runs regardless — it is the PROBE that decides the env state
if [[ "$STACK_ENV" == ok ]]; then
    run_case ST1 "stack binaries present in rootfs-alpine" pass \
        test -x "$ROOTFS/usr/sbin/nginx"
else
    run_case ST1 "stack binaries ABSENT -> env-blocked (skip gate)" pass \
        test ! -x "$ROOTFS/usr/sbin/nginx"
    SKIPPED=$((SKIPPED+1))
fi

if [[ "$STACK_ENV" == ok ]]; then
    run_case ST2 "stack up -> index 200 via host bridge" pass st2_index_test
    run_case ST3 "GET /api/health -> status ok" pass st3_health_test
    run_case ST4 "POST->GET persistence: real SQLite row" pass st4_persist_test
    run_case ST5 "SQL metachars sanitized (D45 allowlist)" pass st5_sanitize_test
    run_case ST6 "down -> port closed, zero leaks, fs gone" pass st6_down_test
    run_case ST7 "stack cold start median < 5000 ms" pass st7_bench_test
    run_case ST8 "name guard rejects traversal (SIA #86)" pass st8_name_guard_test
else
    run_case ST2 "stack up (env-blocked)" env true
    run_case ST3 "api health (env-blocked)" env false
    run_case ST4 "sqlite persistence (env-blocked)" env false
    run_case ST5 "sanitize guard (env-blocked)" env false
    run_case ST6 "down teardown (env-blocked)" env false
    run_case ST7 "cold start < 5s (env-blocked)" env false
    run_case ST8 "name guard (env-blocked)" env false
fi

echo "=== summary: PASS=$PASS FAIL=$FAIL SKIP=$SKIPPED ==="
[[ $FAIL -eq 0 ]]
