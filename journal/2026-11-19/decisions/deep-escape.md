# D33 — Deep escape PoC cho issue #5: fd-hardening + core-dump + NNP/chroot/proc assertions

- **Ngày**: 2026-11-19 (Phiên 1)
- **Issue**: #5 (type:security p2, area:escape-test) — mục 2 GĐ1 roadmap
  ("chroot-escape test sâu hơn").
- **Nhánh**: `sec/5-deep-escape`

## Bối cảnh
Issue #5 yêu cầu PoC escape cho 4 lớp: (1) fd-based escape, (2) /proc/self/root
tricks, (3) setuid trong rootfs vs NNP (biến thể của NNP1), (4) core dump leak,
cộng case seccomp SIGSYS. Audit #38 (D32) đã dạy: assertion phải chạm
**cơ chế thật** (host /proc) chứ không phải triệu chứng trong-ns dễ vacuous.

## Phát hiện trước khi vá (grep `close_range|FD_CLOEXEC|RLIMIT` trên minict.c)
- **0** kết quả → payload kế thừa MỌI fd của caller không có FD_CLOEXEC qua
  fork+execvp: một fd trỏ file HOST (vd `exec 9</etc/passwd`) đi xuyên qua
  pivot_root, đọc/ghi host không cần đặc quyền nào. Lớp TOCTOU kiểu
  CVE-2018-15664 (fd mở trước check, dùng sau).
- **0** RLIMIT_CORE → crash của payload ghi core FILE vào rootfs (pattern
  "core" file trên máy dev xác nhận `/proc/sys/kernel/core_pattern` = `core`).
- minict-exec.c: cùng lớp — child kế thừa fd của caller CLI/daemon (fd mở
  TRƯỚC setns vẫn pin open-file-description HOST sau khi exec vào instance).

## Quyết định
1. **`harden_payload_fds()` trong minict.c payload path** (trước
   apply_spec_rlimits/capdrop/seccomp):
   - `setrlimit(RLIMIT_CORE, {0,0})` — die nếu fail (config lie, same rule
     spec ulimit D23).
   - `close_range(3, ~0U, 0)` (Linux 5.9, syscall 436); fallback loop có
     trần 65536 cho kernel cũ, lỗi từng fd bỏ qua. Hợp đồng payload chỉ cam
     kết stdio 0/1/2 — đóng 3+ là thu hẹp thuần túy.
2. **`harden_exec_fds()` trong minict-exec.c child** — cùng 2 động tác, same
   contract (fd 0 = stdin passthrough đã tài liệu hoá). KHÔNG đụng filter
   seccomp → MD0 (so sánh filter hai binary) không lệch.
3. **7 test mới**:
   - isolation: **FD1** (2 giai đoạn: control kế thừa qua /proc/<minict>/fd/9
     để không vacuous, rồi payload `cat <&9` phải EBADF + không lộ marker);
     **CHROOT1** (chroot(2) EPERM post-capdrop — primitive của double-chroot
     escape); **SETUID2** (exec busybox chmod 4755, assert host /proc
     NoNewPrivs=1 + CapEff=0 — D32-style; assertion trong-ns sẽ vacuous vì
     single-uid mapping làm mọi setuid no-op); **CORE1** (ulimit -c == 0
     trong payload; harness chủ động NÂNG limit trước để không kế thừa 0 —
     vacuous-guard); **CORE2** (SEGV không để lại core file — defense-in-depth
     cho host pattern dạng file); **PROC2** (không có /proc/self/root —
     khoá contract "không procfs" bằng assertion, yêu cầu xác minh lại sau
     netns của issue #5).
   - seccomp: **SC20** (blocklist hit sống sót, rc=1 EPERM — không SIGSYS
     death; ERRNO→TRAP/KILL mutant làm case đỏ).
4. **Negative control (mutant) cho từng test mới** — bắt buộc D32/SIA:
   - NC1 bỏ harden_payload_fds → FD1 **FAIL** ✓
   - NC2 bỏ setrlimit CORE → CORE1 **FAIL** ✓ (chạy lại thủ công xác nhận
     sau khi bản chạy script im lặng vì sed/build glitch — kết quả như nhau)
   - NC3 bỏ capdrop payload → CHROOT1 **PASS** giả (unexpected success) ✓ bị
     bắt ở verdict FAIL
   - NC4 ERRNO→TRAP → SC20 **FAIL** ✓
   - NC5 bỏ NNP prctl → SETUID2 **FAIL** ✓ (seccomp tự die vì cần NNP —
     đúng logic: NNP là điều kiện cần của filter)
   - PROC2/CORE2 không mutant được trong môi trường này (PROC2 phụ thuộc
     mount-block của môi trường ngoài; CORE2 phụ thuộc core_pattern host) —
     ghi rõ, không claim mutant-coverage cho 2 case này.

## Residual (không giấu)
- **Piped core_pattern**: core(5) — "RLIMIT_CORE will be ignored if the system
  is configured to pipe core dumps to a program" → handler host vẫn nhận bộ
  nhớ payload khi crash. Một chiều outbound (bộ nhớ của chính payload), không
  phải nâng đặc quyền; core_pattern không với tới được từ payload (không
  /proc, AL4). Xử lý triệt để là host-side pattern policy → **GĐ2**.
- **PROC2 phụ thuộc môi trường**: nếu kernel/env cho mount proc trong pidns
  (shim đã thử), /proc/self/root của payload là proc của pidns (không phải
  host proc) → trick vẫn không thoát được, nhưng assertion `[ ! -e ]` sẽ đỏ
  trên kernel như vậy → khi đó phải đổi assertion sang kiểm namespace-scope
  của proc (ghi vào follow-up, không phải hôm nay).
- close_range fallback: kernel < 5.9 dùng loop có trần — tốt đủ cho fd table
  thực tế (≤ vài trăm).

## Số đo
- Test: isolation 23 → **29** (+6 run_case: FD1, CHROOT1, SETUID2, CORE1,
  CORE2, PROC2), seccomp 20 → **21** (SC0–SC19 có sẵn; thêm SC20 — kiểm tra
  `grep -oE 'run_case SC[0-9]+'` xác nhận không trùng id). Tổng suite repo
  87 → **94** (29 isolation + 21 seccomp + 7 netns + 8 alpine + 9 web +
  20 minictd).
- Perf: host-side, cold path thêm 1 syscall (close_range) + 1 setrlimit → đo
  lại trong `tools/perf-check.sh` (kết quả trong evidence + PR).

## Bài học quá trình (mở rộng docs/skills.md)
- Mutant script KHÔNG ĐƯỢC `git checkout` file chưa commit ngoài file đang
  mutant: lần chạy đầu đã xoá WIP minict.c + 2 suite (khôi phục từ lịch sử
  Edit; NC1 của lần đó vẫn hợp lệ vì chạy trước khi wipe). WIP phải commit
  trước khi chạy mutant.
