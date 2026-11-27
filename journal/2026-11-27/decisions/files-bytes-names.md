# D83 — files bytes-wire: tên file dạng bytes (base64) + recovery (#219, phiên 44, 2026-11-27)

## Bối cảnh
Files (#208/D79) truyền tên/path dưới dạng chuỗi JSON qua execv2. Go `json.Marshal`
thay byte không phải UTF-8 bằng U+FFFD → đo được trên daemon thật: file
`t<0xE1><0xBA>ext.txt` list về UI thành `t\ufffd\ufffdext.txt` — không mở/đổi tên/xoá
được từ UI. Tên chứa newline còn làm vỡ hàng list (hạn chế v1 đã ghi trong
files-parse.ts). Issue #219 chốt sẵn phương án 1: chỉ adapter, daemon không đổi.

## Quyết định
Mọi path/tên đi qua execv2 dưới dạng **base64-of-raw-bytes**, hai chiều:

1. **List** — `FS_LIST_SCRIPT_B64` in mỗi entry một dòng
   `type<TAB>size<TAB>mtime<TAB>mode<TAB>path_b64<TAB>name_b64`:
   - basename lấy bằng shell suffix expansion `${f##*/}` (raw bytes, không applet);
   - base64 không chứa tab/newline → tên chứa newline/tab không còn vỡ hàng;
   - **bẫy đo được:** busybox `stat` KHÔNG interpret `\t` trong format (khác
     printf/tr) → format stat phải chứa REAL TAB. MD71 bắt lỗi này trước khi land.
2. **Path vào** — operand argv là path_b64; script decode trong sandbox:
   `p=$(printf %s "$1" | base64 -d; echo x); p=${p%x}; ... "$p"` (trick `echo x`
   giữ trailing newline mà command substitution strips). Áp cho read/write/
   create/move/delete/upload/stat. Script vẫn là hằng số; `"$p"` luôn quoted.
3. **Wire** — `FsEntry` thêm `name_b64`/`path_b64` (nguồn thật); `name`/`path` chỉ
   display: decode strict-fatal, lỗi → `<tên không phải UTF-8>` + `hex: …`
   (git `core.quotepath` philosophy — bytes là sự thật). API nhận `path_b64`
   (list/read/content/create/move/upload/delete).
4. **Recovery** — entry Invalid UTF-8 hiện badge + nút "Đổi tên an toàn":
   `moveFsEntry(path_b64_gốc → joinPathB64(dir, tên_mới))`, tên mới qua
   `assertSafeName`. Không bao giờ dùng chuỗi đã decode chứa U+FFFD làm path.
5. **An toàn** — `assertSafePathB64` kiểm trên BYTES sau decode (bắt đầu `/`,
   không NUL, ≤4096); delete chặn root trên decoded bytes (`length <= 1`).

## Phương án đã loại
- Bytes-framing toàn bộ execv2 (kiểu kubectl exec binary): phải đổi daemon —
  vi phạm "adapter-only, daemon không đổi" của issue.
- Surrogateescape kiểu PEP 383: Go `json.Marshal` vẫn thay surrogate bằng
  U+FFFD trên wire — không sống qua execv2 stdout.
- Hiển thị octal-escape như git: chỉ giải quyết display, không giải quyết ops
  (vẫn không mở/đổi tên/xoá được).

## Hai phép thử manifest §3
- 0 byte core: chỉ sửa `console/` (files-parse.ts, minictd.ts, api.ts,
  sandbox-files.tsx, demo-mode.ts) + tests (files-parse.test.ts, MD71) + docs.
- Kèm fix syntax hỏng sẵn trên main `6883e08` (`sandbox-files.tsx`
  `const ome, setHome]` → `const [home, setHome]`) — console build gãy trước đó.

## Bằng chứng
- MD71 (chạy mọi nơi): PASS — list 6 cột × 4 tên byte-chính xác (invalid byte,
  newline+TAB, tiếng 3-byte, emoji 4-byte), read/move/delete qua decode-prefix,
  không injection ra host (`make test-minictd`: 65 PASS + 9 SKIP + 0 FAIL).
- Unit: `make console-test` 15 pass / 0 fail (parseStatEntries 6 cột, b64
  round-trip byte lỗi, assertSafePathB64 trên bytes, joinPathB64/parentB64/
  crumbsFromB64 byte-level, decodeFsName placeholder+hex).
- `tsc --noEmit` + `next build` sạch. Perf: không đụng runtime hot path.
