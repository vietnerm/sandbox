# Decision Log — 2026-11-17 — CI truth (pipefail, SIG1 NOFORK, UID1 overflow, run all suites)

Issue: #14. Bối cảnh: CI báo xanh trên main (run #20) nhưng log cho thấy
`UID1 FAIL`, `SIG1 FAIL`, `summary: PASS=15 FAIL=2`, `make: Error 1` — job vẫn
`success` vì exit code của `make test 2>&1 | tee file` là exit code của `tee`.

## D1 — `defaults: run: shell: bash` thay vì `set -o pipefail` từng step

GitHub Actions chạy step không khai `shell:` bằng `bash -e` **không pipefail**.
Khai `shell: bash` ở workflow level → `bash --noprofile --norc -e -o pipefail`,
pipe trả exit code thật của `make`. Áp cho `ci.yml` và `cve-daily.yml`
(cùng bẫy `| tee`). Không sửa từng step vì một default phủ cả các step mới sau này.

## D2 — SIG1: chấp nhận NOFORK sleep, không hạ thấp bảo đảm "no orphan"

Tái hiện bằng chính busybox mà runner tải: Ubuntu 24.04 `busybox-static`
(1.36.1-6ubuntu3.1) chạy applet `sleep` NOFORK → `sh -c "sleep 61; true"` không
có process con; Debian 1.37.0 fork bình thường. Bỏ điều kiện `-z "$mp"` là lỗi
harness (giả định có child), KHÔNG phải lỗi sản phẩm: PDEATHSIG đặt trên sh
(minict fork → child), pidns teardown giết phần còn lại khi PID 1 chết — kiểm
tra chết của shpid + mp (nếu có) vẫn giữ nguyên ý nghĩa. Không đổi sang
applet khác để tránh lệch行为 giữa hai distro busybox.

## D3 — UID1: chấp nhận overflowgid 65534, giữ kiểm gid lạ

`map_ids()` map đúng 1 gid; `setgroups` deny-lock vĩnh viễn trong userns (bắt
buộc trước khi viết gid_map unprivileged) → supplementary groups của host user
(runner có docker/sudo/…) chỉ có thể hiển thị là 65534. Chúng cùng principal
(uid thật) nên không phải break isolation; file-ownership đã có UID2 chứng
minh. Test mới kiểm: uid=0, gid chính=0, mọi gid trong `id -G` ∈ {0, 65534}.
Một gid lạ được MAP thật vào ns vẫn bị bắt.

## D4 — `make test` chạy đủ suite + aggregate exit code

`tests/run-suites.sh` chạy isolation → seccomp → netns kể cả khi suite trước
đỏ, trả rc=1 nếu bất kỳ suite nào đỏ. Lý do: CI cũ dừng ở suite đỏ đầu tiên →
không có bằng chứng seccomp/netns (run #20 chỉ chạy isolation). Tổng hợp lại
vẫn giữ cổng "đỏ là fail" cho merge (G2/PROMPT.MD).

## D5 — `test_web.sh` preflight `ss` thay vì hạ WEB2

WEB2 (loopback-only) là check bảo mật thật — không skip/tolerate. Máy thiếu
`ss` chỉ gặp lỗi rõ ràng kèm hướng dẫn cài iproute2, thay vì FAIL vô nghĩa
(phiên trước bị 5/6 vì đúng trường hợp này).

## Kết quả đo lại (host Debian 13, kernel 5.10, user thường uid 1001)

- make test: 17/17 + 20/20 + 7/7 (rc=0, cả 3 suite luôn chạy)
- make test-web: 6/6; perf-check PASS: cold 1.79 ms / rss 1108 kB / disk 2.02 MB
- Runner-sim (thay busybox Ubuntu 24.04 vào rootfs): isolation 17/17 PASS
