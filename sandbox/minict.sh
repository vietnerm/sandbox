#!/usr/bin/env bash
# minict.sh — lifecycle CLI for the minict rootless container.
#
#   ./sandbox/minict.sh up <name>            start instance (bg shell), private rootfs copy
#   ./sandbox/minict.sh web <name> <port>    web instance: busybox httpd serving a
#                                            docroot INSIDE the sandbox, in its OWN
#                                            netns (loopback-only, issue #3). The host
#                                            reaches it through a unix-socket bridge:
#                                            webproxy.py listens 127.0.0.1:<port> (host)
#                                            -> AF_UNIX <rootfs>/.minict-proxy.sock
#                                            (in-sandbox relay in minict) -> httpd.
#                                            Port >= 1024 enforced (host proxy binds
#                                            it rootless). See D16 in
#                                            journal/2026-11-17/decisions/netns.md
#   ./sandbox/minict.sh exec <name> -- cmd   run one-shot cmd in instance rootfs (foreground)
#   ./sandbox/minict.sh shell <name>         interactive-ish shell into instance rootfs
#   ./sandbox/minict.sh down <name>          stop instance, remove its fs copy
#   ./sandbox/minict.sh ps                   list instances
#   ./sandbox/minict.sh bench                run benchmarks/bench.sh
#
# NOTE (PoC limitation): 'exec' does NOT setns into the running instance's
# namespaces (rootless userns re-entry requires caps we do not hold); it starts
# a second namespace set sharing the same instance rootfs — with netns ON each
# exec gets its OWN fresh netns (loopback-only), not the web instance's.
# See journal/2026-11-17/decisions/poc.md.
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
    # journal/2026-11-17/decisions/poc.md)
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
    # host-side proxy binds the port rootless; inside the sandbox httpd binds
    # the SAME port number but in its OWN netns (no conflict possible)
    case "$port" in
        ''|*[!0-9]*) echo "port must be numeric, got '$port'"; exit 1 ;;
    esac
    if [[ "$port" -lt 1024 ]]; then
        echo "refusing port < 1024: the host-side proxy binds it rootless" \
             "— use an unprivileged port >= 1024"; exit 1
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
    # default docroot lives INSIDE the private copy (served by the sandboxed
    # httpd). ALWAYS (re)write the per-instance marker page — the base rootfs
    # may carry a stale index.html (rootfs/ is untracked), and the routing
    # test NS6 relies on each instance serving ITS OWN instance name.
    mkdir -p "$INST/$name/rootfs$doc"
    cat > "$INST/$name/rootfs$doc/index.html" <<HTML
<html><head><title>minict web</title></head>
<body><h1>minict web</h1>
<p>instance: $name</p>
<p>marker: minict-web</p>
<p>served from inside the sandbox (userns+netns+mountns+pivot_root+capdrop+NNP+seccomp)</p>
</body></html>
HTML
    # sandbox: httpd binds 127.0.0.1:<port> INSIDE its own netns (loopback-only,
    # issue #3); the in-sandbox relay binds AF_UNIX /.minict-proxy.sock in the
    # new root (filesystem-scoped -> host reachable at rootfs/.minict-proxy.sock)
    PROXY_SOCK_IN_ROOT="/.minict-proxy.sock"
    nohup env MINICT_WEB_PORT="$port" MINICT_PROXY_SOCK="$PROXY_SOCK_IN_ROOT" \
        "$BIN" "$INST/$name/rootfs" /bin/sh -c \
        "exec busybox httpd -f -p 127.0.0.1:$port -h $doc" \
        </dev/null >"$INST/$name/log" 2>&1 &
    echo $! > "$INST/$name/pid"
    # host: webproxy.py relays 127.0.0.1:<port> -> the unix socket above
    # S1 (issue #41): --root/--sock so the proxy resolves the socket
    # symlink-proof (O_NOFOLLOW per component + connect via /proc/self/fd)
    nohup python3 "$REPO/tools/webproxy.py" \
        --root "$INST/$name/rootfs" --sock "$PROXY_SOCK_IN_ROOT" --port "$port" \
        </dev/null >"$INST/$name/proxy.log" 2>&1 &
    echo $! > "$INST/$name/proxy-pid"
    # wait until the bridge answers from the host side (max ~2.5s)
    for _ in $(seq 1 25); do
        if curl -s -m 1 -o /dev/null "http://127.0.0.1:$port/"; then
            echo "web '$name' serving http://127.0.0.1:$port (netns loopback-only, docroot $doc)"
            exit 0
        fi
        sleep 0.1
    done
    echo "web '$name' failed to listen on 127.0.0.1:$port (log: $INST/$name/log, proxy: $INST/$name/proxy.log)"
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
    if [[ -f "$INST/$name/proxy-pid" ]]; then
        ppid="$(cat "$INST/$name/proxy-pid")"
        kill "$ppid" 2>/dev/null || true
        sleep 0.2
        kill -9 "$ppid" 2>/dev/null || true
        rm -f "$INST/$name/proxy-pid"
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
