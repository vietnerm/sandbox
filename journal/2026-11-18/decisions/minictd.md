# Decisions — minictd control-plane (nhánh `feat/minictd-control-plane`)

Nguồn: gói bàn giao `minictd-handoff.zip` do một agent khác viết theo yêu cầu của
owner (Doan). Gói được coi là **code chưa tin cậy**: đọc MERGE-GUIDE/README chỉ để
định hướng, mọi claim được kiểm lại bằng review code + chạy thật.

## D23 — `MINICT_SPEC=<file>`: cấu hình instance dạng file dòng (từ gói bàn giao)

- Chỉ thị: `hostname`, `env K=V`, `bind src:dst[:ro]`, `webport <port> <sock>`,
  `tmpfs path[:size=N]`, `shm <size>`, `ulimit nofile|nproc=soft[:hard]`.
- Parse **phía host, trước `unshare`** (DAC theo uid thật), lỗi → exit 125
  (fail-closed); chỉ thị lạ → warn + bỏ qua (daemon mới nói được với binary cũ).
- Giới hạn cứng: 64 mục/danh sách, 1024 B/dòng, 64 KB/file.
- bind lỗi → die (container sai cấu hình là nói dối); tmpfs/shm lỗi → warn (dịch vụ
  thêm, host seccomp có thể chặn mount — cùng tư thế `/tmp` hiện có).
- `setrlimit` trước capdrop. **Lưu ý đo được khi review:** `RLIMIT_NPROC` đếm theo
  **uid thật trên host** (single-uid mapping), nên `PidsLimit → nproc` là xấp xỉ
  dùng chung cho mọi instance + tiến trình của user — không phải pids.max (CG1).
- Không có spec → hành vi cũ y nguyên (regression 56/56 + web 6/6, cold start +0.1 ms
  trong nhiễu — `evidence/minictd-regress.txt`).

## D24 — nhiều relay webport mỗi instance (từ gói bàn giao)

Mỗi `webport` một relay con (PDEATHSIG + teardown như D16). Cặp env
`MINICT_WEB_PORT/MINICT_PROXY_SOCK` vẫn giữ. Daemon cấp port qua label
`minict.webport=3000,5173` (RunSpec không có trường Ports).

## D25 — exec vào instance đang chạy bằng `setns` (`minict-exec`) — GATE GĐ2 đo được

- Mô hình RootlessKit/podman: tiến trình cùng euid với owner của userns instance
  có đủ caps trong userns đó → `setns` user → mnt → pid → uts → ipc → net,
  **mở mọi ns fd trước khi join** (sau setns(mnt) `/proc` là view của instance).
- **seccomp không đi qua setns** (filter theo thread) → `minict-exec` cài lại
  bản sao **nguyên văn** blocklist D11/D15. Chống trôi: test MD0 so sánh enum +
  blocklist + thân `install_seccomp()` giữa hai file (mutant bỏ 1 syscall → đỏ).
- Đo được (kernel 6.18, uid 1001): exec thấy hostname spec, thấy marker `/tmp`
  của payload (cùng mount ns), bind `:ro` không ghi được, `unshare -U` → EPERM,
  timeout → 124, stdin, NDJSON stream (MD5–MD13). Đây là bằng chứng cho mục
  "Gate nghiên cứu" của GĐ2 trong manifest §7 — CI runner sẽ xác nhận lần 2.
- Fallback `newns` (tư thế PoC D5) chỉ dùng khi thiếu binary.

## D26 — review lúc tích hợp: 7 lỗi sửa trước khi gộp

| ID | Lỗi trong gói bàn giao | Sửa | Test |
|---|---|---|---|
| R1 | `{name}` từ URL (đã unescape `%2F`) đi thẳng vào `filepath.Join(Root,name)` cho mọi op trừ Run → tạo file `lock` ở thư mục bất kỳ, `RemoveAll` thư mục ngoài state dir nếu có `state.json` | `validName` trong `loadState`/`lock`/`lockCreate`, HTTP 400 | MD15 |
| R2 | daemon truyền `os.Environ()` cho minict/minict-exec, minict-exec không `clearenv` → **toàn bộ env của daemon lọt vào sandbox** (token, proxy, creds; đo được); `MINICT_NO_SECCOMP`/`MINICT_NO_NETNS` trong env daemon sẽ âm thầm tắt seccomp/netns của mọi instance | env con tối giản (`PATH`), `clearenv()` trong minict-exec | MD10 |
| R3 | không token = không auth (mặc định); so sánh token không constant-time; `--token` lộ qua `/proc/<pid>/cmdline` | bắt buộc token (`$MINICTD_TOKEN`/`--token-file`), `--insecure-no-auth` cho dev, `subtle.ConstantTimeCompare` | MD2, MD3 |
| R4 | Run lỗi giữa chừng để lại thư mục không `state.json` → Run sau `cp -a` VÀO rootfs cũ, List thấy "ma" | xoá thư mục khi Run lỗi | (review) |
| R5 | body HTTP không giới hạn | `MaxBytesReader` 16 MB | (review) |
| R6 | execstream: frame giữ slice của buffer đọc bị tái dùng → output hỏng khi frame còn trong channel; client ngắt → goroutine kẹt + zombie | copy chunk, select ctx, vẫn `wait()` khi ngắt | MD13 |
| R7 | bind dst: `mkdir -p` + `open(O_CREAT)` + `mount` đi theo symlink **trước pivot_root**; rootfs ghi được bởi payload và tồn tại qua stop/start → payload trồng `ln -s /host/dir /evil` thì lần Start sau tạo thư mục/file **trên host** (đo được: `x` xuất hiện ngoài rootfs) | `spec_make_target`: `lstat` từng thành phần, symlink → exit 125, tạo file/dir theo loại src, `O_NOFOLLOW|O_EXCL` | MD17 |

Negative control: cùng suite chạy trên gói như bàn giao → 6 case đỏ (MD2, MD3, MD10,
MD15, MD17, MD18) — `evidence/minictd-ab.txt`.

## D27 — vị trí của minictd trong kế hoạch (owner chọn trên decision card, 2026-11-18)

**GĐ1 vẫn là giai đoạn hiện tại.** minictd vào repo như "GĐ2 seed gộp sớm" (ngoại
lệ manifest §8, ghi trước khi gộp): chỉ sửa bảo mật + giữ `make test-minictd` xanh;
không mở tính năng GĐ2 mới (CG1, G3, systemd unit, CLI `sandbox`, template…) cho tới
khi DoD GĐ1 xong. Phương án bị loại: chạy song song GĐ1+GĐ2; chuyển hẳn sang GĐ2.
