# D57 — GĐ2 mục 3 sub: snapshot/backup/restore (tar.gz trusted) (issue #130, Phiên 22, 2026-11-23)

## Ngữ cảnh

Manifest GĐ2 mục 3 DoD: "snapshot/backup/restore (overlay layer + tar.zst)".
mục 3 có 2 phần: (a) overlayfs rootless + idmapped mounts; (b) snapshot/backup/restore.
Phần (a) env-gated (kernel 5.10 dev không hỗ trợ rootless overlayfs trong userns, cần ≥5.11).
Phiên này làm phần (b): snapshot/backup/restore tar.gz.

## Quyết định

Chọn **trusted tar.gz path riêng**, không reuse secureUnpack:

1. **`snapshot.go` mới** (`tarGzTree` + `extractGzTar`): Go archive/tar + compress/gzip
   (stdlib). Symlink-aware (Lstat, TypeSymlink + Linkname qua Readlink).
2. **Trusted path**: refuse `..`/absolute member PATHS (path traversal defense), ALLOW
   absolute symlink TARGETS. Lý do: rootfs có 271 absolute symlinks (busybox applet
   `/bin/ash -> /bin/busybox`), intentional — resolve inside sandbox sau pivot_root.
   secureUnpack (D50) refuse abs symlinks cho untrusted OCI #109; snapshot là daemon-created
   (trusted), khác trust boundary.
3. **gzip thay tar.zst**: Go stdlib không có zstd; repo stdlib-only (go.mod no deps, D20
   no-vendor). gzip là deviation có tài liệu; zstd cần dep decision (deferred).
4. **`Snapshot(name, backupName)`**: yêu cầu STOPPED (fail-closed `ErrInstanceRunning` → 409),
   tarGzTree rootfs → `<Root>/_backups/<backupName>.tar.gz`.
5. **`backup:<name>` image form**: extractGzTar archive → instance rootfs.
6. **API**: `POST /v1/sandboxes/{name}/snapshot` body `{"backup":"<name>"}` → 200.

## Phủ nhận / residual

- **overlayfs rootless + idmapped mounts** (phần (a)): env-gated kernel 5.10 dev. CI
  ubuntu-24.04 (kernel 6.x) hỗ trợ. Separate phiên. Hiện mỗi instance vẫn copy rootfs
  (~2 MB/instance).
- **zstd compression**: gzip tạm (stdlib); zstd cần dep (deferred). tar.gz lớn hơn tar.zst.
- **overlay layer**: snapshot hiện là full tar.gz (không thin-provision). Thin-provision cần
  overlayfs (phần (a)).
- **list/remove-backup API**: chưa có. Cleanup qua state dir.
- **Trusted trust boundary**: snapshot archive tạo bởi daemon (trusted). Nếu operator tampers
  archive (file trong `<Root>/_backups/`), restore vẫn safe (refuse `..`/abs member paths),
  nhưng symlink targets không validate (trusted assumption). Risk: low (operator là
  single-user, manifest §2).

## Tests

- **MD40**: run src (marker `/snap-marker`), snapshot RUNNING → 409, stop, snapshot OK, run dst
  `backup:md-snap` → exec sees marker. Marker ở `/` (persistent), không `/tmp` (tmpfs).
- Mutant (SCR): skip stopped check → running snapshot succeeds → MD40 409 assertion red;
  skip `..` path check → path traversal tar refused (extract returns error → run dst fails).

## Bài học đo được

- **Trust boundary distinction**: secureUnpack (untrusted OCI, refuse abs symlinks #109) vs
  snapshotExtract (trusted daemon-created, allow abs symlinks). Cùng rootfs, khác trust model.
- **271 abs symlinks**: alpine rootfs busybox applet pattern — đo được khi check rootfs. Đây
  là lý do snapshot KHÔNG thể reuse secureUnpack.
- **stdlib gzip**: no zstd dep → gzip. Trade-off: archive lớn hơn (~2x), nhưng no vendor (D20).
