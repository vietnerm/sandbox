#!/usr/bin/env bash
# sandbox.sh — GĐ1 mục 4: one-command full web stack lifecycle (issue #85, D45).
#
# NOTE: the roadmap's "./sandbox" is impossible on this repo — `sandbox/` is
# an existing DIRECTORY (binary + build scripts), and a file cannot share a
# name with a directory. The one-command UX lives here, as ./sandbox.sh.
#
# Stack inside ONE minict instance (alpine rootfs, full isolation:
# userns+mountns+pidns+netns+capdrop+NNP+seccomp, no Docker, rootless):
#   nginx (bind 127.0.0.1:<port> in the instance's OWN netns)
#     -> FastCGI unix:/run/fcgi.sock
#     -> fcgiwrap (spawned by spawn-fcgi)
#     -> CGI shell script /www/cgi-bin/api.cgi
#     -> SQLite DB /var/lib/minict/app.sqlite (real file on the instance fs)
#   Host reaches it through the D16 unix-socket bridge: webproxy.py
#   127.0.0.1:<port> -> AF_UNIX <rootfs>/.minict-proxy.sock (in-sandbox
#   relay in minict) -> TCP 127.0.0.1:<port> in the instance netns.
#
# Commands:
#   ./sandbox.sh up <name> [--port <host-port>]   start the stack, wait until
#                                                HTTP index + API health both
#                                                answer (print ready ms)
#   ./sandbox.sh down <name>                      teardown (kill tree + fs)
#   ./sandbox.sh ps                               list instances
#   ./sandbox.sh shell <name>                     shell into the instance rootfs
#   ./sandbox.sh bench [N]                        N stack cold starts (spawn ->
#                                                HTTP 200 + API 200), median
#                                                + max, threshold < 5000 ms
#                                                (roadmap GĐ1 mục 4)
#
# API (demo, allowlist-sanitized — see D45 for the injection caveat):
#   GET  /api/health   -> {"status":"ok",...}
#   GET  /api/items    -> JSON array from SQLite
#   POST /api/items?name=X -> INSERT X (allowed chars: [a-zA-Z0-9 _.-])
#
# Prereqs: minict binary (auto-built), rootfs-alpine with stack packages
# (auto-built via mkrootfs-alpine.sh; CDN fail => clear error, no silent
# fallback — fail-closed like the rest of the repo).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
INST="$HERE/instances"
MINICT="$HERE/sandbox/minict"
ALPINE="$HERE/rootfs-alpine"
PROXY_SOCK_IN_ROOT="/.minict-proxy.sock"
STACK_THRESHOLD_MS=5000      # roadmap GĐ1 mục 4: cold start stack < 5 s

die() { echo "sandbox.sh: $*" >&2; exit 1; }

valid_name() { # <name> — instance name must be a safe path component
    # (SIA audit PR #86 low finding — same class as issue #61 on minict.sh:
    #  no '/', no '..' as a path traversal, printable ascii only)
    [[ "${1:-}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]
}

ensure_minict() { [[ -x "$MINICT" ]] || make -C "$HERE/sandbox" >/dev/null; }

ensure_alpine_stack() {
    [[ -d "$ALPINE" ]] || bash "$HERE/sandbox/mkrootfs-alpine.sh" "$ALPINE" >/dev/null
    if [[ ! -x "$ALPINE/usr/sbin/nginx" ]]; then
        # older rootfs build without stack packages — install in place
        MINICT_STACK_AUTOADD=1 bash "$HERE/sandbox/mkrootfs-alpine.sh" "$ALPINE"
    fi
    [[ -x "$ALPINE/usr/sbin/nginx" && -x "$ALPINE/usr/bin/sqlite3" ]] \
        || die "stack packages missing in $ALPINE (CDN/network failed at build?)"
}

pick_port() { # free host port: random + `ss` collision check (issue #75 class)
    local p try=0
    while (( try++ < 20 )); do
        p=$(( 15800 + RANDOM % 1100 ))
        if ! (ss -tln 2>/dev/null || true) | grep -q "127.0.0.1:$p "; then
            echo "$p"; return 0
        fi
    done
    die "no free port found (20 tries)"
}

alive() { # <pidfile> — pid recorded AND still running
    [[ -f "$1" ]] && kill -0 "$(cat "$1")" 2>/dev/null
}

kill_tree() { # <host-pid> — recursive children first, then the pid itself
    local p="$1" c
    for c in $(pgrep -P "$p" 2>/dev/null || true); do
        kill_tree "$c"
    done
    kill "$p" 2>/dev/null || true
}

# ---------- per-instance config files (written into the PRIVATE rootfs copy;
# the shared rootfs-alpine is never touched — pristine rule D44) ----------
write_stack_files() { # <rootfs-copy-dir> <port>
    local R="$1" port="$2"
    mkdir -p "$R/run" "$R/var/log/nginx" "$R/var/lib/minict" "$R/www/cgi-bin" \
             "$R/var/lib/nginx/body" "$R/var/lib/nginx/proxy" "$R/var/lib/nginx/fastcgi" \
             "$R/var/tmp/nginx"

    cat > "$R/etc/nginx/nginx.conf" <<NGINX
daemon off;
# D45: the apk non-root install cannot run package scripts (no chroot), so
# the packaged "nginx" user was never created — and even with it present,
# setuid to an unmapped uid is EPERM after minict's capdrop. Workers run as
# root-IN-NS (uid 0 = calling uid via the single-uid map, D20); everything
# the stack writes is owned by that uid, so this is safe inside the sandbox.
user root;
pid /run/nginx.pid;
worker_processes 1;
error_log /var/log/nginx/error.log warn;
events { worker_connections 64; }
http {
    include /etc/nginx/mime.types;
    access_log /var/log/nginx/access.log;
    client_body_temp_path /var/lib/nginx/body;
    proxy_temp_path /var/lib/nginx/proxy;
    fastcgi_temp_path /var/lib/nginx/fastcgi;
    uwsgi_temp_path /var/tmp/nginx;
    scgi_temp_path /var/tmp/nginx;
    server {
        listen 127.0.0.1:$port;
        server_name minict;
        root /www;
        index index.html;
        location /api/ {
            fastcgi_pass unix:/run/fcgi.sock;
            include /etc/nginx/fastcgi_params;
            fastcgi_param SCRIPT_FILENAME /www/cgi-bin/api.cgi;
            fastcgi_param PATH_INFO \$fastcgi_script_name;
        }
    }
}
NGINX

    cat > "$R/www/index.html" <<HTML
<html><head><title>minict web stack</title></head>
<body><h1>minict web stack</h1>
<p>marker: minict-web-stack</p>
<p>nginx + fcgiwrap + CGI + SQLite inside one rootless sandbox
(userns+mountns+pidns+netns+capdrop+NNP+seccomp)</p>
<p><a href="/api/health">/api/health</a> | <a href="/api/items">/api/items</a></p>
</body></html>
HTML

    cat > "$R/www/cgi-bin/api.cgi" <<'CGI'
#!/bin/sh
# api.cgi — CGI -> SQLite JSON API (demo GĐ1 mục 4; issue #85, D45).
# INPUT SANITIZING: shell CGI + SQL string concat is the classic injection
# class — `name` is filtered to [a-zA-Z0-9 _.-] BEFORE any SQL is built and
# the id path only passes digits. This is a demo API, not a production one
# (real parameter binding comes with a real language runtime in GĐ2+).
DB=/var/lib/minict/app.sqlite
pi="${PATH_INFO:-}"
qs="${QUERY_STRING:-}"

qname() { printf '%s' "$qs" | sed -n 's/.*[?&]\{0,1\}name=\([^&]*\).*/\1/p'; }
sanitize() { printf '%s' "$1" | tr -c 'a-zA-Z0-9 _.-' '_' | tr -d "'\""; }

hdr() {
    printf 'Content-Type: application/json\r\n'
    printf 'Cache-Control: no-store\r\n'
    printf '\r\n'
}
json_esc() { # JSON escaping via busybox ash pattern substitution
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    printf '%s' "$s"
}
notfound() { hdr; printf '{"error":"not found","path":"%s"}\n' "$(json_esc "$pi")"; }

qsel() { # numeric-only id
    printf '%s' "$1" | tr -cd '0-9'
}

case "$REQUEST_METHOD" in
GET)
    case "$pi" in
        /api/health)
            hdr
            printf '{"status":"ok","stack":"minict-web-stack","db":"%s"}\n' "$DB"
            ;;
        /api/items)
            hdr
            out="$(sqlite3 -json "$DB" \
                'SELECT id,name,created FROM items ORDER BY id;' 2>/dev/null || true)"
            printf '%s\n' "${out:-[]}"
            ;;
        /api/items/*)
            id="$(qsel "${pi#/api/items/}")"
            if [[ -n "$id" ]]; then
                hdr
                out="$(sqlite3 -json "$DB" \
                    "SELECT id,name,created FROM items WHERE id=$id;" 2>/dev/null || true)"
                printf '%s\n' "${out:-[]}"
            else
                notfound
            fi
            ;;
        *) notfound ;;
    esac
    ;;
POST)
    name="$(sanitize "$(qname)")"
    if [[ "$pi" == "/api/items" && -n "$name" ]]; then
        if sqlite3 "$DB" "INSERT INTO items(name) VALUES('$name');" 2>/dev/null; then
            hdr
            printf '{"inserted":"%s"}\n' "$name"
        else
            hdr
            printf '{"error":"insert failed"}\n'
        fi
    else
        hdr
        printf '{"error":"bad request"}\n'
    fi
    ;;
*)
    hdr
    printf '{"error":"method not allowed"}\n'
    ;;
esac
CGI
    chmod +x "$R/www/cgi-bin/api.cgi"
}

cmd_up() {
    local name="${1:?usage: sandbox.sh up <name> [--port <host-port>]}"
    valid_name "$name" || die "invalid instance name '$name' (allowed: [A-Za-z0-9][A-Za-z0-9._-]*)"
    shift
    local port=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --port) port="$2"; shift 2 ;;
            *) die "up: unknown arg '$1'" ;;
        esac
    done
    [[ "$port" ]] || port="$(pick_port)"
    [[ "$port" -ge 1024 ]] 2>/dev/null || die "port must be >= 1024 (host-side proxy binds it rootless)"
    ensure_minict
    ensure_alpine_stack
    mkdir -p "$INST/$name"
    if alive "$INST/$name/pid"; then
        die "instance '$name' already running (pid $(cat "$INST/$name/pid"))"
    fi
    # private rootfs copy per instance (~14 MB alpine+stack)
    rm -rf "$INST/$name/rootfs"
    cp -a "$ALPINE" "$INST/$name/rootfs"
    write_stack_files "$INST/$name/rootfs" "$port"

    # payload: fcgiwrap daemon + DB seed + nginx foreground (PID 2).
    # SIGTERM lifecycle = D19 unchanged: shim forwards -> nginx graceful ->
    # shim exit -> kernel SIGKILLs the whole pidns (fcgiwrap orphan dies).
    local payload='spawn-fcgi -s /run/fcgi.sock -M 0666 -- /usr/bin/fcgiwrap || exit 1;
        mkdir -p /var/lib/minict;
        sqlite3 /var/lib/minict/app.sqlite "CREATE TABLE IF NOT EXISTS items(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created TEXT DEFAULT CURRENT_TIMESTAMP);" || exit 1;
        exec /usr/sbin/nginx -c /etc/nginx/nginx.conf'
    local t0 t1 ready_ms=""
    t0=$(date +%s%N)
    nohup env MINICT_WEB_PORT="$port" MINICT_PROXY_SOCK="$PROXY_SOCK_IN_ROOT" \
        "$MINICT" "$INST/$name/rootfs" /bin/sh -c "$payload" \
        </dev/null >"$INST/$name/log" 2>&1 &
    echo $! > "$INST/$name/pid"
    # host-side bridge (S1-hardened resolver, issue #41/D31)
    nohup python3 "$HERE/tools/webproxy.py" \
        --root "$INST/$name/rootfs" --sock "$PROXY_SOCK_IN_ROOT" --port "$port" \
        </dev/null >"$INST/$name/proxy.log" 2>&1 &
    echo $! > "$INST/$name/proxy-pid"

    # readiness: BOTH index and API must answer (stack, not just any listener)
    local ok=0 i
    for i in $(seq 1 60); do
        if curl -fsS -m 1 "http://127.0.0.1:$port/" >/dev/null 2>&1 \
           && curl -fsS -m 1 "http://127.0.0.1:$port/api/health" 2>/dev/null | grep -q '"status":"ok"'; then
            ok=1
            t1=$(date +%s%N)
            ready_ms=$(( (t1 - t0) / 1000000 ))
            break
        fi
        sleep 0.1
    done
    if [[ "$ok" != "1" ]]; then
        echo "stack '$name' FAILED to become ready on 127.0.0.1:$port" >&2
        echo "--- instance log ---" >&2; tail -20 "$INST/$name/log" >&2 || true
        echo "--- proxy log ---" >&2; tail -10 "$INST/$name/proxy.log" >&2 || true
        return 1
    fi
    echo "stack '$name' READY in ${ready_ms} ms — http://127.0.0.1:$port/ (api /api/health, /api/items)"
    echo "READY_MS=$ready_ms"
    echo "$ready_ms" > "$INST/$name/ready-ms"
}

cmd_down() {
    local name="${1:?usage: sandbox.sh down <name>}"
    valid_name "$name" || die "invalid instance name '$name'"
    local d="$INST/$name"
    [[ -d "$d" ]] || { echo "sandbox.sh: no instance '$name'"; return 0; }
    # second teardown layer from HOST side: kill the whole process tree of
    # the wrapper (nginx master is the payload; fcgiwrap is its sibling —
    # belt over the D19 pidns-exit SIGKILL)
    if [[ -f "$d/pid" ]]; then
        kill_tree "$(cat "$d/pid")" || true
    fi
    if [[ -f "$d/proxy-pid" ]]; then
        ppid="$(cat "$d/proxy-pid")"
        kill "$ppid" 2>/dev/null || true
        sleep 0.1
        kill -9 "$ppid" 2>/dev/null || true
    fi
    rm -rf "$d"
    echo "stack '$name' down"
}

cmd_bench() {
    local n="${1:-5}"
    case "$n" in ''|*[!0-9]*) n=5 ;; esac
    ensure_minict
    ensure_alpine_stack
    local times=() i ms bname="stackbench"
    for (( i=1; i<=n; i++ )); do
        cmd_up "$bname" >/tmp/sandbox-bench-up-$i.log 2>&1 || {
            echo "bench run $i FAILED:"; tail -20 /tmp/sandbox-bench-up-$i.log; exit 1; }
        ms="$(cat "$INST/$bname/ready-ms")"
        times+=("$ms")
        echo "run $i: READY_MS=$ms"
        cmd_down "$bname" >/dev/null
    done
    # median + max (python3 always present in the repo toolchain)
    printf '%s\n' "${times[@]}" | python3 -c "
import sys, statistics
ts = [int(x) for x in sys.stdin if x.strip()]
print(f'median={statistics.median(ts):.0f} ms  min={min(ts)} ms  max={max(ts)} ms  N={len(ts)}  threshold=$STACK_THRESHOLD_MS ms')
sys.exit(0 if statistics.median(ts) < $STACK_THRESHOLD_MS else 1)
"
}

case "${1:-help}" in
up)    shift; cmd_up "$@" ;;
down)  shift; cmd_down "$@" ;;
ps)    exec bash "$HERE/sandbox/minict.sh" ps ;;
shell) shift; exec bash "$HERE/sandbox/minict.sh" shell "$@" ;;
bench) shift; cmd_bench "$@" ;;
help|*)
    sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
    ;;
esac
