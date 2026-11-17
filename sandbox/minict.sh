#!/usr/bin/env bash
# minict.sh — lifecycle CLI for the minict rootless container.
#
#   ./sandbox/minict.sh up <name>            start instance (bg shell), private rootfs copy
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
    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
    ;;
esac
