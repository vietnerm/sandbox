# D79 — #208: Console Files dùng execv2 (phương án 1) — daemon không đổi

- **Ngày:** 2026-11-26 (Phiên 41) · **Issue:** #208 (p2 feature) · **Nhánh:** `feat/208-files-execv2`

## Quyết định (phương án 1 — đúng roadmap: adapter-only, 0 byte core)
- **list**: `FS_LIST_SCRIPT` (constant) — 3 globs dotfiles (`*`, `.[!.]*`, `..?*`,
  bỏ `.`/`..`) + `stat -c "%F\t%s\t%Y\t%a\t%n"`; %n CUỐI → tên chứa TAB sống;
  dòng < 5 cột (tên chứa NEWLINE) bị SKIP (hạn chế v1, ghi trong files-parse.ts).
- **read**: stat một (size guard `FS_MAX_FILE` = 1 MB — 413 quá hạn, KHÔNG âm
  thầm cắt) → `base64 "$1"` → decode best-effort (UTF-8 hợp lệ → text; binary →
  base64 + encoding "base64" cho UI download).
- **write/upload**: stdin qua `execv2(..., stdin)` (contract.Stdin []byte —
  Go json base64 ⇄ raw) → `cat > "$1"` — binary-safe, không giới hạn argv;
  upload validate name (`assertSafeName`: không `/`, NUL, `.`/`..`).
- **create/move/delete**: `mkdir -p` / `touch` / `mv` / `rm -rf --` — path là
  OPERAND argv (`sh -c <CONSTANT> sh <path>`); delete chặn xoá `/`; UI đã
  confirm trước khi gọi.
- **reset**: giữ 501 trung thực (rm -rf toàn workspace qua execv2 không an
  toàn — phương án 2 daemon-side cần owner duyệt).
- **SAFETY (OWASP command injection)**: script là HẰNG SỐ, dữ liệu đi qua
  `$1/$2` quoted positional — shell không re-expand `$()` trong giá trị biến;
  MD67 chứng minh: file tên `a;$(id -u).txt` giữ nguyên (không substitution),
  host sống, content khớp.
- **sftp line cũ** (sandbox-files.tsx:703, sót template) thay bằng ghi chú
  "execv2-backed · paths là argv operand (#208)".

## Test
- 10 unit `console/scripts/files-parse.test.ts` (node --test, TS-strip Node
  ≥ 23.6; `make console-test`): parser busybox (rows thật đo trên rootfs, tên
  TAB, dòng lỗi skip, tên `;` `$()`), assertSafePath/Name, joinPath,
  b64 roundtrip binary+utf8, decodeBest.
- **MD67** e2e daemon thật: operand argv injection-safe + stdin write + list
  4 cột (PASS dev + runner).
- `tsc --noEmit` + `next build` sạch; tsconfig exclude scripts/.
- CI workflow console job chạy `make console` — thêm `make console-test` vào
  workflow là change `.github/**` (G7) → KHÔNG tự gộp; làm PR riêng chờ owner
  (đã ghi trong PR + issue).

## Hai phép thử (manifest §3)
- Còn nhẹ: có — 0 dòng daemon/minict đổi (`git diff main... -- minictd/ sandbox/`
  = trống); RSS daemon +0; console adapter only.
- Bỏ đi core còn hữu dụng: có — xoá handlers, view quay lại 501 trung thực.

## Residual (ghi rõ)
- reset workspace: chờ phương án 2 (owner).
- File > 1 MB: 413 + hướng dẫn phương án 2.
- Tên file chứa NEWLINE: hàng bị skip (hiếm; ghi chú parser).
