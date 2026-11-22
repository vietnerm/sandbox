#!/usr/bin/env bash
# install.sh — render + install the minictd systemd --user unit (issue #91, D47).
#
# Pattern: generate-then-install (Podman quadlet style). The .service.in
# template carries @HOME@/@BINDIR@ placeholders; this script renders them,
# copies the binaries, writes the token file if missing, and prints the
# enable + linger steps (not run automatically: enable-linger may need a
# session manager prompt, and --user instances depend on the target host).
#
# Usage: bash minictd/deploy/install.sh [--home DIR] [--bindir REL]
#   --home DIR   target home (default $HOME; must be absolute)
#   --bindir REL path under home holding minictd (default .local/bin)
#
# Rootless: never touches /etc, never runs as root, never uses sudo.
set -euo pipefail

HOME_DIR="${HOME:-}"
BINDIR=".local/bin"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --home)   HOME_DIR="$2"; shift 2 ;;
        --bindir) BINDIR="$2"; shift 2 ;;
        *) echo "install.sh: unknown arg $1" >&2; exit 2 ;;
    esac
done

[[ -n "$HOME_DIR" ]] || { echo "install.sh: no home (use --home DIR)" >&2; exit 2; }
[[ "$HOME_DIR" == /* ]] || { echo "install.sh: --home must be absolute, got $HOME_DIR" >&2; exit 2; }
case "$BINDIR" in /*|*..*) echo "install.sh: --bindir must be relative and without .." >&2; exit 2 ;; esac

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="$HERE/minictd.service.in"
[[ -f "$SRC" ]] || { echo "install.sh: template missing: $SRC" >&2; exit 1; }

# Render with a single sed pass; refuse partial renders (placeholder check).
# SIA PR #92 (low): escape sed-special chars in the replacement — a home
# dir containing & or | would otherwise corrupt the render silently.
esc_sed() { printf '%s' "$1" | sed -e 's/[&|\\]/\\&/g'; }
UNIT="$(sed -e "s|@HOME@|$(esc_sed "${HOME_DIR}")|g" \
              -e "s|@BINDIR@|$(esc_sed "${BINDIR}")|g" "$SRC")"
if printf '%s' "$UNIT" | grep -q '@HOME@\|@BINDIR@'; then
    echo "install.sh: unresolved placeholder after render — abort" >&2
    exit 1
fi

BINDIR_ABS="$HOME_DIR/$BINDIR"
mkdir -p "$BINDIR_ABS" "$HOME_DIR/.config/systemd/user" \
         "$HOME_DIR/.config/minictd" "$HOME_DIR/.config/minictd/traefik" \
         "$HOME_DIR/.local/state/minictd"
# S4 (issue #116): the unix socket lives under the state dir — keep the dir
# 0700 even if it pre-existed looser (daemon re-enforces at boot, fail-closed).
chmod 700 "$HOME_DIR/.local/state/minictd" 2>/dev/null || true

# Binaries: copy from repo bin/ if present (build first: make -C minictd).
for b in minictd minict-exec; do
    if [[ -f "$HERE/../bin/$b" ]]; then
        install -m 0755 "$HERE/../bin/$b" "$BINDIR_ABS/$b"
        echo "installed: $BINDIR_ABS/$b"
    fi
done

# Token: create once (0600, urandom) — never overwrite an existing one.
TOKEN_FILE="$HOME_DIR/.config/minictd/token"
if [[ ! -s "$TOKEN_FILE" ]]; then
    umask 077
    head -c 32 /dev/urandom | base64 | tr -d '\n=' > "$TOKEN_FILE"
    echo "token created: $TOKEN_FILE (first line)"
fi
chmod 600 "$TOKEN_FILE" 2>/dev/null || true

printf '%s\n' "$UNIT" > "$HOME_DIR/.config/systemd/user/minictd.service"
chmod 600 "$HOME_DIR/.config/systemd/user/minictd.service"
echo "unit written: ~/.config/systemd/user/minictd.service"

# Optional: syntax check when systemd-analyze is available (exit code kept
# non-fatal on hosts whose systemd knows not all directives).
if command -v systemd-analyze >/dev/null 2>&1; then
    if systemd-analyze verify "$HOME_DIR/.config/systemd/user/minictd.service" 2>&1; then
        echo "systemd-analyze verify: OK"
    else
        echo "systemd-analyze verify: reported problems above (non-fatal)" >&2
    fi
fi

echo
echo "Next steps (run yourself, not from this script):"
echo "  systemctl --user daemon-reload"
echo "  systemctl --user enable --now minictd"
echo "  loginctl enable-linger \"\$USER\"   # boot-time user manager (no session needed)"
