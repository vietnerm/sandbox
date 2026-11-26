# Dọn 5 issue from-audit mức low (#61, #62, #77, #93, #119) — 2026-11-26

## Ngữ cảnh
Owner (2026-11-26): rà soát issue mở, cái nào giải quyết nhanh được thì sửa và đóng
trước khi sang GĐ3. Năm issue dưới đây đều là low từ SIA, đề xuất sửa đã rõ trong issue.

## Quyết định
1. **#119 (S4 listener TOCTOU)**: socket được TẠO 0600 bằng `umask 0177` quanh `bind`
   (umask process-wide an toàn vì `main` mở listener trước mọi goroutine nền), bỏ
   `os.Chmod` theo path (hàm này follow symlink: là cửa sổ thật duy nhất còn lại).
   `bind(2)` không follow/không ghi đè path có sẵn (EADDRINUSE) nên swap trước bind
   → fail-closed. Lstat sau bind kiểm tra: socket, không symlink, 0600. Đóng listener khi
   lỗi verify thì KHÔNG unlink theo tên (path có thể đã là file của người khác).
   Test mới: `TestOpenUnixSocketUmaskIndependent` (umask 0 → vẫn 0600, umask được trả lại;
   mutant bỏ Umask bị bắt) + `TestListenUnixRefusesPlantedSymlink`.
2. **#61 (minict.sh)**: `valid_name` cùng regex với minictd `validName`
   (`[A-Za-z0-9][A-Za-z0-9_.-]{0,63}`) cho up/exec/shell/down, fail-closed exit 2;
   `MINICT_PASTA_FWD` tách bằng `read -r -a` (word-split có chủ ý, không glob).
3. **#77 (pasta chết sau attach)**: chọn phương án 2 + 3 của issue: `down` cảnh báo kèm
   tail pasta.log khi pasta đã chết trước instance; `ps` hiện `pasta=DEAD`. Không thêm
   monitor loop nền (thêm process, tốn RSS); residual: chỉ phát hiện lúc down/ps.
4. **#62 (AB1)**: số ms ghi qua file tạm (run_case chạy trong subshell) và in dòng
   `AB1: <ms> ms` ở cấp suite khi PASS. Mục 2 (mutant M-userns) chấp nhận positive leg
   NET10 trên CI làm bằng chứng hành vi, như issue đã đề xuất.
5. **#93 (MD21 pid reuse)**: so `start_ticks` trong state.json với field 22 của
   `/proc/<pid>/stat` (parse sau dấu `)` cuối) trước khi kill, kiểm tra lại ngay trước kill.

## Đo
- `go test ./cmd/minictd/` PASS (8 case listener); mutant bỏ Umask → FAIL.
- `bash -n` 3 script; `valid_name` thử 8 tên (2 hợp lệ, 6 bị từ chối); `-t*` không bị glob.
- `make check-docs`, `make unicode-check` PASS. Suite pasta/minictd e2e cần uid ≥ 1000 +
  /dev/net/tun: nguồn sự thật là CI.
