#!/usr/bin/env bash
# seccomp-profile-gen.sh — measure a template workload and write the seccomp
# allowlist profile (GĐ2 mục 3a, issue #139, D59).
#
# Usage:
#   tools/seccomp-profile-gen.sh <name> [<name>...]
#
# Requires: cc (to build tools/syscall-probe if missing), rootfs of the
# template present (rootfs/, rootfs-alpine/, rootfs-debian/).
#
# How it works:
#   - each workload script (tools/seccomp-workloads/<name>.sh) wraps EVERY
#     `sandbox/minict` invocation with:
#       "$PROBE" --after-exec 2 --append "$RAW" -- sandbox/minict ...
#     exec 1 = minict itself, exec 2 = the payload — so the sandbox SETUP
#     phase (unshare/pivot_root/mount/capdrop, all between the two execs)
#     never enters the profile; those syscalls stay floor-denied forever.
#   - the D11 blocklist is ACTIVE during measurement (no MINICT_NO_SECCOMP):
#     syscalls the floor denies can never be satisfied by the allowlist.
#   - profile = sorted "<nr> <__NR_name>" lines, x86_64, annotated from the
#     kernel UAPI header; written to minictd/internal/minict/profiles/.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
PROBE="$HERE/syscall-probe"
UNISTD="/usr/include/x86_64-linux-gnu/asm/unistd_64.h"
OUT="$REPO/minictd/internal/minict/profiles"

mkdir -p "$OUT"
[[ -x "$PROBE" ]] || {
    cc -O2 -Wall -Wextra -std=gnu11 -o "$PROBE" "$HERE/syscall-probe.c"
}

gen() {
    local name="$1" raw="/tmp/probe-$1.txt"
    rm -f "$raw"
    RAW="$raw" PROBE="$PROBE" bash "$HERE/seccomp-workloads/$name.sh" \
        > "/tmp/probe-$name-run.log" 2>&1
    if [[ ! -s "$raw" ]]; then
        echo "$name: EMPTY probe output — workload failed?" >&2
        tail -5 "/tmp/probe-$name-run.log" >&2 || true
        return 1
    fi
    : > "$OUT/$name.allow"
    {
        echo "# seccomp allowlist profile '$name' (x86_64) — D59, issue #139."
        echo "# Measured: tools/seccomp-workloads/$name.sh via tools/syscall-probe"
        echo "# (--after-exec 2: payload tree only, D11 blocklist active)."
        echo "# Enforced as: D11 blocklist floor + allowlist + default EPERM;"
        echo "# blocklist entries stay denied even if listed here."
        echo "# Regenerate: tools/seccomp-profile-gen.sh $name"
        while read -r nr; do
            nm="$(grep -hE "^#define __NR_[a-zA-Z0-9_]+ ${nr}\$" "$UNISTD" 2>/dev/null \
                | head -1 | sed -E 's/^#define (__NR_[a-zA-Z0-9_]+).*/\1/' || true)"
            printf '%s %s\n' "$nr" "${nm:-unknown}"
        done < "$raw" | sort -n
    } > "$OUT/$name.allow"
    echo "$name: $(grep -cv '^#' "$OUT/$name.allow") syscalls"
}

for n in "$@"; do
    gen "$n"
done
