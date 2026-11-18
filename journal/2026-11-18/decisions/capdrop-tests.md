# D32 — Assertion trực tiếp capdrop/pidns từ host /proc (issue #38)

- Ngày: 2026-11-18. Nhánh: `test/38-capdrop-pidns`. Issue: #38 (type:chore,
  area:escape-test, from-audit). Suite: `tests/test_isolation.sh` 21 → 23 case.

## Bối cảnh
Negative control của SCR (PR #36 @ 7685b3f) + SIA audit cho thấy:
1. **M2 (bỏ capset trong `drop_all_caps()`) → suite 21/21 xanh** (vacuous).
   Nguyên nhân: FS4/FS5 chỉ đo được "mount EPERM", mà NR_mount nằm trong seccomp
   blocklist mặc định (minict.c:1065) → EPERM đến từ filter bất kể capdrop. Mất
   capdrop sẽ không test nào đỏ.
2. **M4 (bỏ CLONE_NEWPID) → chỉ PID1 đỏ**: PID2 dùng `ps` (đọc /proc, rootfs
   không có /proc — AL4) nên xanh bất kể pidns; SIG1 xanh vì PDEATHSIG độc lập
   pidns.

## Lựa chọn
- **Chọn: assertion host-side qua `/proc/<pid>/status`** (2 test mới):
  - `CAPD1`: tìm cây launcher→shim→payload bằng tag cmdline + phân loại ancestry
    (`_find_sandbox_tree`); assert `CapEff: 0000000000000000` cho shim (relay
    posture, D16/D19) và payload (drop trước install_seccomp, minict.c:1452).
  - `PID3`: assert `NSpid` shim = 2 tầng innermost 1, payload = 2 tầng
    innermost 2 (pid_namespaces(7); probe live xác nhận format).
- Bỏ phương án "chạy không-seccomp để đo capdrop thuần" (đề xuất trong #38):
  cần flag runtime `MINICT_NO_SECCOMP` — mở mặt tấn công, ngược hướng "không
  thêm công tắc tắt an ninh"; CAPD1 đạt cùng giá trị đo mà không đụng runtime.
- Giữ FS4/FS5/PID2/SIG1 nguyên văn + comment chỉ về CAPD1/PID3: chúng vẫn đúng
  như guarantee (mount phải fail; không orphan; ps không thấy gì) — chỉ là
  không phải assertion cơ chế. SIG1 masking được ghi nhận là BY DESIGN: guarantee
  "không orphan" không phụ thuộc pidns (PDEATHSIG tự xử lý), cơ chế pidns bây giờ
  được PID1+PID3 giữ trực tiếp.

## Kết quả negative control (bắt buộc nghiệm thu #38)
- **M2 → CAPD1 FAIL** (PASS=22/23; mọi test khác xanh) — trước patch: 21/21 xanh.
  → đạt yêu cầu "M2 phải làm ≥1 test đỏ" của issue.
- **M4 → PID1 FAIL + PID3 FAIL** (PASS=21/23) — trước patch: chỉ PID1.
- Sau restore: 23/23 xanh.

## Ảnh hưởng
- Test-only (`tests/test_isolation.sh`); không đụng runtime → perf không đổi
  (perf-check 1.89 ms / 1244 kB / 2.04 MB, ngưỡng 100/10240/20).
- Tổng test mới sau gộp: isolation 23 (+2), tổng repo 87 (23+20+7+8+9+20).
- Phát hiện phụ trong phiên: cve-check.py KeyError 'cvss' khi NVD row lỗi →
  issue #44 (không sửa trong PR này — 1 issue/ngày; tool không chặn #38).
