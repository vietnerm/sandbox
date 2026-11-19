#!/usr/bin/env bash
# minict.sh — lifecycle CLI for the minict rootless container.
#
#   ./sandbox/minict.sh up <name> [--pasta] [--rootfs <dir>] [-- payload...]
#                                        start instance (bg shell), private
#                                        rootfs copy. Payload override: give a
#                                        command after `--` to run it instead
#                                        of the default sleep (one-shot probes,
#                                        in-instance servers — the command runs
#                                        in THIS instance's ns set, unlike
#                                        `exec` which starts a NEW ns set).
#                                        --rootfs <dir> boots a DIFFERENT rootfs
#                                        (e.g. rootfs-alpine from mkrootfs-alpine.sh,
#                                        issue #79/D43); default stays the busybox
#                                        rootfs. Fail-closed: missing dir or no
#                                        /bin/sh in it is an error, NO silent
#                                        fallback to the default rootfs.
#
#   Connectivity (GĐ1 mục 3, issue #57, D37):
#   `--pasta` / MINICT_PASTA=1 attaches pasta (passt project, pinned by
#   tools/fetch-pasta.sh) to the instance's netns AFTER it is up:
#     pasta --foreground --userns /proc/<pid>/ns/user --netns /proc/<pid>/ns/net
#           --config-net [$MINICT_PASTA_FWD]
#   NOTE the explicit --userns: with `--netns` alone pasta implies
#   --netns-only (stays in ITS userns) and setns into OUR netns would EPERM
#   (conf.c conf_pasta_ns, passt 2026_10_02). Joining the instance userns
#   works rootless: pasta's euid == userns owner (single-uid map 0<-1001,
#   D20) → uid 0 + caps inside → tap + default route. MINICT_PASTA_FWD is
#   appended verbatim (e.g. "-t 18080:80" = host 18080 → instance 80).
#   Fail-closed: no /dev/net/tun (dev container, D34) or MINICT_NO_NETNS=1
#   → error, no silent skip. Host-side metrics: pasta spawn is measured
#   SEPARATELY from minict cold_start (A/B in tests/test_pasta.sh AB1).
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

down_pasta() { # <name> — kill the pasta process of an instance if any
    local name="$1"
    if [[ -f "$INST/$name/pasta-pid" ]]; then
        local ppid="$(cat "$INST/$name/pasta-pid")"
        kill "$ppid" 2>/dev/null || true
        sleep 0.1
        kill -9 "$ppid" 2>/dev/null || true
        rm -f "$INST/$name/pasta-pid"
    fi
}

spawn_pasta() { # <name> <shim-pid> — attach pasta to the instance netns (D37)
    local name="$1" pid="$2"
    if [[ ! -e "/proc/$pid/ns/net" ]]; then
        echo "pasta attach failed: /proc/$pid/ns/net gone (instance died?)"
        exit 1
    fi
    if [[ ! -c /dev/net/tun ]]; then
        echo "pasta attach failed: /dev/net/tun missing on this host (D34: "\
             "dev container env-blocked; GitHub runners have it)" >&2
        # fail-closed AND clean: kill the instance we just started, no orphan
        kill "$pid" 2>/dev/null || true; sleep 0.2; kill -9 "$pid" 2>/dev/null || true
        rm -rf "$INST/$name"
        exit 1
    fi
    bash "$REPO/tools/fetch-pasta.sh" >/dev/null
    local PASTA_BIN="$REPO/tools/bin/pasta"
    # D42 (issue #74): DNS. pasta forwards DNS flows (UDP/TCP :53) addressed to
    # the --dns-forward address to the host's first resolver (passt.1; conf.c
    # add_dns_resolv4 reads it host-side at startup). Without --dns-forward a
    # query to the gateway would fall into map-gw → 127.0.0.1:53, which only
    # works when the host resolver listens there (this host uses public
    # 100.100.2.x) — so --dns-forward is mandatory for determinism. Address =
    # the host default gateway, i.e. the same GW_IP `--config-net` copies as
    # guest_gw and that tests/test_pasta.sh NET11 uses as egress target.
    # No default route on host → skip DNS config entirely (fail-soft; same
    # behavior as pre-D42 — resolver stays unconfigured in the instance).
    local DNS_ARGS=()
    local GW
    GW="$(ip -4 route show default 2>/dev/null | awk '{print $3; exit}')"
    if [[ -n "$GW" ]]; then
        DNS_ARGS=(--dns-forward "$GW")
    else
        echo "pasta DNS: no host default route — resolver not configured (D42 fail-soft)" >&2
    fi
    # Explicit --userns is REQUIRED: `--netns` alone implies --netns-only
    # (passt conf.c conf_pasta_ns) and pasta could not setns into a netns
    # owned by the instance's userns. euid==owner rule makes this rootless.
    nohup "$PASTA_BIN" --foreground \
        --userns "/proc/$pid/ns/user" --netns "/proc/$pid/ns/net" \
        --config-net ${DNS_ARGS[@]+"${DNS_ARGS[@]}"} ${MINICT_PASTA_FWD:-} \
        </dev/null >"$INST/$name/pasta.log" 2>&1 &
    local ppid=$!
    echo "$ppid" > "$INST/$name/pasta-pid"
    # route readiness (host-side view, netns-scoped): /proc/<pid>/net/route
    # shows the shim's netns table; default row = Destination 00000000.
    local t0 t1 ok=0
    t0=$(date +%s%N)
    for _ in $(seq 1 100); do
        if awk '$2 == "00000000"' "/proc/$pid/net/route" 2>/dev/null | grep -q .; then
            ok=1; break
        fi
        kill -0 "$ppid" 2>/dev/null || break
        sleep 0.05
    done
    t1=$(date +%s%N)
    if [[ "$ok" == "1" ]]; then
        # pasta may die right after configuring the tap (e.g. forward-spec
        # problems) — route seen once is NOT enough; require it alive briefly
        sleep 0.3
        if ! kill -0 "$ppid" 2>/dev/null; then
            echo "pasta attached '$name' then DIED — pasta.log:" >&2
            tail -20 "$INST/$name/pasta.log" >&2 || true
            kill "$pid" 2>/dev/null || true; sleep 0.2; kill -9 "$pid" 2>/dev/null || true
            rm -rf "$INST/$name"
            exit 1
        fi
        echo "pasta attached to '$name' in $(( (t1 - t0) / 1000000 )) ms "\
             "(tap + default route + alive; log $INST/$name/pasta.log)"
        # D42: instance resolver — write `nameserver <GW>` into the instance's
        # private rootfs ONLY after tap + route + aliveness are confirmed
        # (a dead pasta must not leave a resolver pointing nowhere). pasta
        # intercepts DNS flows to this address (see --dns-forward above).
        if [[ -n "$GW" ]]; then
            if mkdir -p "$INST/$name/rootfs/etc" \
               && printf 'nameserver %s\n' "$GW" \
                    > "$INST/$name/rootfs/etc/resolv.conf"; then
                echo "pasta DNS: nameserver $GW written to instance rootfs (--dns-forward)"
            else
                echo "pasta DNS: could not write resolv.conf in $INST/$name/rootfs "\
                     "— resolve-by-name will not work" >&2
            fi
        fi
    else
        echo "pasta attach to '$name' FAILED (no default route in 5s) — pasta.log:" >&2
        tail -20 "$INST/$name/pasta.log" >&2 || true
        kill "$ppid" 2>/dev/null || true; sleep 0.1; kill -9 "$ppid" 2>/dev/null || true
        kill "$pid" 2>/dev/null || true; sleep 0.2; kill -9 "$pid" 2>/dev/null || true
        rm -rf "$INST/$name"
        exit 1
    fi
}

cmd="${1:-help}"
case "$cmd" in
up)
    name="${2:?usage: minict.sh up <name> [--pasta] [--rootfs <dir>] [-- payload...]}"
    shift 2 2>/dev/null || true
    PASTA_ON=0
    SRC_ROOTFS="$REPO/rootfs"
    # D43 (issue #79): flags before `--` in ANY order: --pasta (D37) and
    # optional --rootfs <dir> (e.g. rootfs-alpine). The default busybox
    # rootfs is auto-built when missing; a custom --rootfs must already
    # exist AND contain /bin/sh (fail-closed, no silent fallback).
    while [[ "${1:-}" == "--pasta" || "${1:-}" == "--rootfs" ]]; do
        if [[ "$1" == "--pasta" ]]; then
            PASTA_ON=1; shift
        else
            SRC_ROOTFS="${2:?usage: --rootfs <dir>}"; shift 2
        fi
    done
    if [[ "${MINICT_PASTA:-0}" == "1" ]]; then PASTA_ON=1; fi
    PAYLOAD=(/bin/sh -c 'sleep 31536000')
    if [[ "${1:-}" == "--" ]]; then shift; PAYLOAD=("$@"); fi
    [[ -x "$BIN" ]] || { echo "minict binary missing; run: make -C $HERE"; exit 1; }
    if [[ "$SRC_ROOTFS" == "$REPO/rootfs" ]]; then
        [[ -d "$REPO/rootfs" ]] || "$HERE/mkrootfs.sh" "$REPO/rootfs"
    else
        [[ -d "$SRC_ROOTFS" ]] || { echo "--rootfs: directory not found: $SRC_ROOTFS (build it first, e.g. make rootfs-alpine)"; exit 1; }
        # /bin/sh may be a symlink (alpine: /bin/sh -> /bin/busybox, ABSOLUTE
        # target — valid only INSIDE the rootfs, dangling on the host). Resolve
        # it relative to the rootfs root before the executability check.
        _sh="$SRC_ROOTFS/bin/sh"
        if [[ -L "$_sh" ]]; then
            _ltgt="$(readlink "$_sh")"
            case "$_ltgt" in
                /*) _sh="$SRC_ROOTFS$_ltgt" ;;
                *)  _sh="$(dirname "$_sh")/$_ltgt" ;;
            esac
        fi
        [[ -e "$_sh" ]] || { echo "--rootfs: /bin/sh missing (after symlink resolve: $_sh) — not a usable minict rootfs"; exit 1; }
        [[ -x "$_sh" ]] || { echo "--rootfs: no executable /bin/sh in $SRC_ROOTFS — not a usable minict rootfs"; exit 1; }
    fi
    if [[ "$PASTA_ON" == "1" ]] && [[ "${MINICT_NO_NETNS:-0}" == "1" ]]; then
        echo "--pasta requires the instance's private netns; unset MINICT_NO_NETNS"; exit 1
    fi
    mkdir -p "$INST/$name"
    if [[ -f "$INST/$name/pid" ]] && kill -0 "$(cat "$INST/$name/pid")" 2>/dev/null; then
        echo "instance '$name' already running (pid $(cat "$INST/$name/pid"))"
        exit 1
    fi
    # private rootfs copy per instance: ~2MB (busybox) / ~10MB (alpine),
    # no overlayfs needed
    rm -rf "$INST/$name/rootfs"
    cp -a "$SRC_ROOTFS" "$INST/$name/rootfs"
    # NOTE: bg `sh` with stdin=/dev/null exits on EOF, so the daemon runs a
    # long sleep as PID 1 instead; `exec`/`shell` still give you a shell in
    # the same instance fs (new ns set per process, PoC limitation — see
    # journal/2026-11-17/decisions/poc.md). Payload override (--) runs the
    # given command in THIS instance's ns set instead of the sleep.
    nohup "$BIN" "$INST/$name/rootfs" "${PAYLOAD[@]}" \
        </dev/null >"$INST/$name/log" 2>&1 &
    echo $! > "$INST/$name/pid"
    sleep 0.1
    echo "instance '$name' up (pid $(cat "$INST/$name/pid"), rootfs $INST/$name/rootfs)"
    if [[ "$PASTA_ON" == "1" ]]; then
        spawn_pasta "$name" "$(cat "$INST/$name/pid")"
    fi
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
    down_pasta "$name"
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
        pasta=""
        if [[ -f "$d/pasta-pid" ]] && kill -0 "$(cat "$d/pasta-pid")" 2>/dev/null; then
            pasta="  pasta=yes"
        fi
        if [[ "$p" != "-" ]] && kill -0 "$p" 2>/dev/null; then
            echo "$n  running  pid=$p  fs=$(du -sh "$d/rootfs" 2>/dev/null | cut -f1)$pasta"
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
