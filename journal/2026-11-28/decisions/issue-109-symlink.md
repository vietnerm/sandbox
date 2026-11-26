# Decision D70 — Issue #109: absolute-symlink refusal policy (keep, document, close)

- **Phiên**: 35 (2026-11-28)
- **Issue**: #109 (oci: refuse absolute-symlink members)
- **Slug**: `issue-109-symlink`

## Bối cảnh
`secureUnpack` (D50, `minictd/internal/minict/unpack.go`) refuse symlink member có target TUYỆT ĐỐI (leading `/`) qua `checkName()` — fail-closed, KHÔNG sanitize. Đo được (Phiên 16): rootfs busybox `--install -s` style có 271 applet symlink target `/bin/busybox` → image đóng từ rootfs này KHÔNG boot được qua `oci:`/`tar:`.

Issue #109 mở câu hỏi cho maintainer: refusal có chặn production use-case? Ba phương án:
- (a) giữ refuse (D50 nguyên trạng) — residual ghi minh bạch; ai dùng `oci:` phải build image với relative links.
- (b) rewrite target absolute → dst-relative lúc extract (WEAKEN D50 — cần threat-model note).
- (c) cho phép absolute target chỉ khi resolve ra inside-dst (grey — vẫn là sanitize).

## Quyết định (owner, 2026-11-28)
**Chọn (a) — giữ từ chối như hiện tại (D50 nguyên trạng).** Lý do:
1. **An toàn nhất**: D50 là fail-closed posture — KHÔNG sanitize, KHÔNG trust symlink target. Mọi phương án (b)/(c) đều weakening: rewrite/sanitize mở attack surface (TOCTOU, path confusion, symlink race). Maintainer principle: "reject, never sanitize" (D50 comment dòng 21).
2. **Threat model giữ nguyên**: `secureUnpack` comment (dòng 22-24) ghi "a symlink planted by the archive can only point at paths INSIDE dst, so it can never reach host files" — absolute target violate invariant này. Cho phép (b)/(c) = phải viết threat-model note mới + test thêm.
3. **Use-case thật**: `oci:`/`tar:` images cho minict là rootfs đóng sẵn (operator build). Busybox `--install -s` tạo absolute symlinks là pattern của DISTRO installer, không phải của image builder có chủ đích. Image builder có thể dùng `busybox --install` (không `-s`, tạo copies) hoặc relative symlinks (`ln -s busybox /path/bin/[` thay vì `ln -s /bin/busybox`).
4. **minict rootfs nội bộ**: `sandbox/mkrootfs.sh` dùng `busybox --install -s` → absolute symlinks, nhưng rootfs này dùng qua `base` image (copy, không qua `secureUnpack`). `oci:`/`tar:` là untrusted path → refusal đúng.

## Hành động
- **Giữ code nguyên trạng**: `checkName()` trong `unpack.go` tiếp tục refuse absolute paths (leading `/`) cho cả member names và link targets.
- **Documentation**: cập nhật README.md "Hạn chế" — rõ yêu cầu relative symlinks cho `oci:`/`tar:` images; gợi ý `busybox --install` (không `-s`) hoặc relative symlink targets cho image builders.
- **docs/roadmap.md**: residual #109 → "resolved D70 (giữ refusal, document)".
- **Đóng #109**: comment giải thích quyết định + link D70, label `wontfix` (không phải bug — design decision) + `area:docs`.

## Phạm vi KHÔNG làm
- (b) rewrite absolute → relative: weakening D50, cần threat-model riêng — KHÔNG làm.
- (c) allow-if-inside-dst: grey sanitize — KHÔNG làm.
- Threat-model file riêng: ghi trong D70 + README + unpack.go comment (đã có).

## Tương lai (nếu cần)
Nếu production use-case thật cần absolute symlinks (vd. pull distro image có `--install -s`), mở issue mới với repro cụ thể → đánh giá lại (b)/(c) với threat-model note. Cho đến thì (a) là posture chính thức.
