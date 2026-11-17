#!/usr/bin/env bash
# minict.sh — lifecycle CLI for the minict rootless container.
#
#   ./sandbox/minict.sh up <name>            start instance (bg shell), private rootfs copy
#   ./sandbox/minict.sh web <name> <port>    web instance: busybox httpd serving a
#                                            docroot INSIDE the sandbox, bind on
#                                            127.0.0.1:<port> (no netns yet — loopback
#                                            only, port >= 1024 enforced; see
#                                            docs/decision-log/2026-11-17-web-httpd.md)
#   ./sandbox/minict.sh exec <name> -- cmd   run one-shot cmd in instance rootfs (foreground)
#   ./sandbox/minict.sh shell <name>         interactive-ish shell into instance rootfs
#   ./sandbox/minict.sh down <name>          stop instance, remove its fs copy
#   ./sandbox/minict.sh ps                   list instances
#   ./sandbox/minict.sh bench                run benchmarks/bench.sh
#
# NOTE (PoC day-1 limitation): 'exec' does NOT setns into the running
# instance's namespaces (rootless userns re-entry requires caps we do not
# hold); it starts a second namespace set sharing the same instance rootfs.
# See docs/decision-log/2026-11-17.md.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
INST="$REPO/instances"
BIN="$HERE/minict"

cmd="${1:-help}"
case "$cmd" in
up)
    name="${2:?usage: minict.sh up <name>}"
    [[ -x "$BIN" ]] || { echo "minict binary missing; run: make -C $HERE"; exit 1; }
    [[ -d "$REPO/rootfs" ]] || "$HERE/mkrootfs.sh" "$REPO/rootfs"
    mkdir -p "$INST/$name"
    if [[ -f "$INST/$name/pid" ]] && kill -0 "$(cat "$INST/$name/pid")" 2>/dev/null; then
        echo "instance '$name' already running (pid $(cat "$INST/$name/pid"))"
        exit 1
    fi
    # private rootfs copy per instance: ~2MB, no overlayfs needed
    rm -rf "$INST/$name/rootfs"
    cp -a "$REPO/rootfs" "$INST/$name/rootfs"
    # NOTE: bg `sh` with stdin=/dev/null exits on EOF, so the daemon runs a
    # long sleep as PID 1 instead; `exec`/`shell` still give you a shell in
    # the same instance fs (new ns set per process, PoC limitation — see
    # docs/decision-log/2026-11-17.md)
    nohup "$BIN" "$INST/$name/rootfs" /bin/sh -c 'sleep 31536000' \
        </dev/null >"$INST/$name/log" 2>&1 &
    echo $! > "$INST/$name/pid"
    sleep 0.1
    echo "instance '$name' up (pid $(cat "$INST/$name/pid"), rootfs $INST/$name/rootfs)"
    ;;
web)
    name="${2:?usage: minict.sh web <name> <port> [docdir]}"
    port="${3:?usage: minict.sh web <name> <port> [docdir]}"
    doc="${4:-/www}"
    [[ -x "$BIN" ]] || { echo "minict binary missing; run: make -C $HERE"; exit 1; }
    [[ -d "$REPO/rootfs" ]] || "$HERE/mkrootfs.sh" "$REPO/rootfs"
    # bind must be loopback (no netns yet) and unprivileged (capdrop inside)
    case "$port" in
        ''|*[!0-9]*) echo "port must be numeric, got '$port'"; exit 1 ;;
    esac
    if [[ "$port" -lt 1024 ]]; then
        echo "refusing port < 1024: sandbox has no netns (shared host net) and" \
             "runs capdropped — use an unprivileged port >= 1024"; exit 1
    fi
    if [[ "$doc" != /* ]]; then echo "docdir must be absolute inside rootfs"; exit 1; fi
    mkdir -p "$INST/$name"
    if [[ -f "$INST/$name/pid" ]] && kill -0 "$(cat "$INST/$name/pid")" 2>/dev/null; then
        echo "instance '$name' already running (pid $(cat "$INST/$name/pid"))"
        exit 1
    fi
    # private rootfs copy per instance: ~2MB, no overlayfs needed
    rm -rf "$INST/$name/rootfs"
    cp -a "$REPO/rootfs" "$INST/$name/rootfs"
    # default docroot lives INSIDE the private copy (served by the sandboxed httpd)
    mkdir -p "$INST/$name/rootfs$doc"
    if [[ ! -f "$INST/$name/rootfs$doc/index.html" ]]; then
        cat > "$INST/$name/rootfs$doc/index.html" <<HTML
<html><head><title>minict web</title></head>
<body><h1>minict web</h1>
<p>instance: $name</p>
<p>marker: minict-web</p>
<p>served from inside the sandbox (userns+mountns+pivot_root+capdrop+NNP)</p>
</body></html>
HTML
    fi
    nohup "$BIN" "$INST/$name/rootfs" /bin/sh -c \
        "exec busybox httpd -f -p 127.0.0.1:$port -h $doc" \
        </dev/null >"$INST/$name/log" 2>&1 &
    echo $! > "$INST/$name/pid"
    # wait until the port answers from the host side (max ~2s)
    for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
        if curl -s -m 1 -o /dev/null "http://127.0.0.1:$port/"; then
            echo "web '$name' serving http://127.0.0.1:$port (docroot $doc)"
            exit 0
        fi
        sleep 0.1
    done
    echo "web '$name' failed to listen on 127.0.0.1:$port (log: $INST/$name/log)"
    exit 1
    ;;
exec)
    name="${2:?usage: minict.sh exec <name> -- cmd...}"
    shift 2
    [[ "$1" == "--" ]] && shift
    [[ -d "$INST/$name/rootfs" ]] || { echo "no instance '$name'"; exit 1; }
    exec "$BIN" "$INST/$name/rootfs" "$@"
    ;;
shell)
    name="${2:?usage: minict.sh shell <name>}"
    exec "$BIN" "$INST/$name/rootfs" /bin/sh
    ;;
down)
    name="${2:?usage: minict.sh down <name>}"
    if [[ -f "$INST/$name/pid" ]]; then
        pid="$(cat "$INST/$name/pid")"
        kill "$pid" 2>/dev/null || true
        for _ in 1 2 3 4 5; do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.1
        done
        kill -9 "$pid" 2>/dev/null || true
    fi
    rm -rf "$INST/$name"
    echo "instance '$name' down"
    ;;
ps)
    for d in "$INST"/*/; do
        [[ -d "$d" ]] || continue
        n="$(basename "$d")"
        p="$(cat "$d/pid" 2>/dev/null || echo -)"
        if [[ "$p" != "-" ]] && kill -0 "$p" 2>/dev/null; then
            echo "$n  running  pid=$p  fs=$(du -sh "$d/rootfs" 2>/dev/null | cut -f1)"
        else
            echo "$n  stopped"
        fi
    done
    ;;
bench)
    exec bash "$REPO/benchmarks/bench.sh"
    ;;
help|*)
    sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
    ;;
esac
