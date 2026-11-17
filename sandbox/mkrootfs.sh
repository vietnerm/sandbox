#!/usr/bin/env bash
# mkrootfs.sh — build a minimal busybox rootfs (~2MB) WITHOUT root.
#
# Source: busybox-static .deb downloaded via apt-get (no install), extracted
# with dpkg-deb -x (pure file ops, works as any user).
#
# Usage: ./mkrootfs.sh [target-dir]   (default: rootfs)
set -euo pipefail

TARGET="${1:-rootfs}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

if [[ -d "$TARGET" && -x "$TARGET/bin/busybox" ]]; then
    echo "rootfs '$TARGET' already exists, skipping (rm -rf it to rebuild)"
    exit 0
fi

echo "==> downloading busybox-static .deb (no root needed)"
(cd "$WORK" && apt-get download busybox-static >/dev/null 2>&1)
DEB=$(ls "$WORK"/busybox-static_*.deb)

echo "==> extracting with dpkg-deb -x"
dpkg-deb -x "$DEB" "$WORK/pkg"

echo "==> assembling rootfs at $TARGET"
mkdir -p "$TARGET"/{bin,dev,etc,proc,tmp,usr/bin,home}
cp "$WORK/pkg/usr/bin/busybox" "$TARGET/bin/busybox"
chmod 0755 "$TARGET/bin/busybox"

# applet symlinks: sh, ls, cat, ps, mount, ...
# NOTE: exclude 'busybox' itself — busybox --list includes it and ln -sf
# would overwrite the real binary with a self-referencing symlink (day-1 bug)
for a in $("$TARGET/bin/busybox" --list | grep -vx busybox); do
    ln -sf /bin/busybox "$TARGET/bin/$a"
done

# /dev placeholders: minict bind-mounts host nodes over these (mknod is
# EPERM inside userns on kernel 5.10.134, see journal/2026-11-17/research.md)
touch "$TARGET"/dev/{null,zero,urandom,random}
chmod 0666 "$TARGET"/dev/{null,zero,urandom,random}

# minimal /etc with a MARKER used by tests/test_isolation.sh to prove the
# sandbox sees OUR passwd, not the host's
cat > "$TARGET/etc/passwd" <<'EOF'
root:x:0:0:root:/root:/bin/sh
minict-sandbox:x:1000:1000:minict sandbox user:/home/minict:/bin/sh
EOF
cat > "$TARGET/etc/group" <<'EOF'
root:x:0:
minict-sandbox:x:1000:
EOF
cat > "$TARGET/etc/hostname" <<'EOF'
minict
EOF
cat > "$TARGET/etc/profile" <<'EOF'
export PATH=/bin:/sbin:/usr/bin
export PS1='minict> '
umask 022
EOF

# writable-by-design note: this tree lives on the host disk owned by the
# calling user; /tmp and /run are covered by tmpfs at runtime
chmod 1777 "$TARGET/tmp"

SIZE=$(du -sh "$TARGET" | cut -f1)
echo "==> done: $TARGET ($SIZE)"
