# Decision D45 — Stack web thật GĐ1 mục 4: nginx + fcgiwrap CGI + SQLite, `./sandbox.sh` một lệnh

- Ngày: 2026-11-20 (Phiên 10, SDDA theo PROMPT.MD §11)
- Issue: #85 — feat(web-stack): GĐ1 mục 4
- Nhánh: `feat/85-web-stack`

## Bối cảnh

Roadmap GĐ1 mục 4: "Stack web thật trong 1 instance: nginx/caddy + API +
SQLite trên rootfs distro; `./sandbox up|down|bench` một lệnh; cold start
stack < 5 s". Tính đến Phiên 9, web trong sandbox chỉ là busybox httpd
tĩnh (`minict.sh web`) — không có nginx, không API, không DB.

## Quyết định

1. **Ngày ra quyết định: dùng `sandbox.sh` thay vì `./sandbox`.** Lý do vật
   lý: thư mục `sandbox/` (binary + build scripts) đã tồn tại từ PoC —
   Linux không cho phép file và thư mục cùng tên trong một thư mục, nên
   `./sandbox` luôn resolve vào DIR. Roadmap được cập nhật thành
   `./sandbox.sh up|down|bench`. UX mục tiêu "một lệnh" giữ nguyên.
2. **Stack: nginx (front, bind 127.0.0.1:8080 trong netns instance) +
   fcgiwrap/spawn-fcgi (FastCGI→CGI bridge) + CGI shell script gọi
   sqlite3 CLI** (JSON mode ≥ 3.33). Loại python3/node/caddy vì ngân sách
   disk 20 MB (research.md ghi trade-off).
3. **Pre-install tại BUILD TIME** (`mkrootfs-alpine.sh`, host-side
   `apk add --root` như host uid — mô hình fakeroot tự nhiên D20): các
   file cài có owner = calling uid = root-in-ns → nginx/sqlite3 ghi được.
   Tránh đúng class AL7 (apk add trong instance → chown foreign → EPERM).
   CDN fail → rootfs vẫn build được, KHÔNG có gói stack → suite stack
   SKIP(env-blocked) (pattern AL5/audit PR #24) — không đỏ giả CI.
4. **Per-instance config ghi vào rootfs COPY** (nginx.conf, index.html,
   api.cgi, seed DB) — rootfs shared không chạm (luật pristine D44).
5. **Signal lifecycle không đổi (D19)**: payload = `sh -c 'spawn-fcgi ...;
   exec nginx ...'` → nginx foreground là PID 2; SIGTERM shim → nginx
   graceful → shim exit → kernel SIGKILL pidns còn lại → fcgiwrap orphan
   không rò rỉ. `sandbox.sh down` kill_tree wrapper (host pid) làm lớp
   thứ hai cho teardown từ host.
6. **Seccomp blocklist KHÔNG đụng**: blocklist hiện có (D11) không chặn
   syscall nào nginx/fcgiwrap/sqlite cần (bind/listen/accept4/fork/epoll/
   flock/openat đều nằm ngoài 32+NS-flags entry) — minict-exec.c không phải
   sửa, MD0 giữ nguyên.

## API demo (giới hạn khai báo rõ)

- `GET /api/health` → `{"status":"ok",...}`
- `GET /api/items` → JSON array từ SQLite
- `POST /api/items?name=X` → INSERT (name sanitize allowlist
  `[a-zA-Z0-9 _.-]` — shell CGI + SQL concat là class injection kinh điển;
  đây là demo GĐ1, KHÔNG phải API production; test chỉ dùng name hợp lệ).

## Test mới

`tests/test_stack.sh` ST1–ST6 (env-aware), thêm vào `tests/run-suites.sh`
→ `make test` chạy luôn; target riêng `make test-stack`.

## Hậu quả

- rootfs-alpine: +nginx+sqlite+fcgiwrap+spawn-fcgi ≈ 13–15 MB (< 20 MB).
- perf-guard busybox default không đổi; số đo alpine/stack ghi ở
  perf-baseline.md + journal bench.md.
- Residual GĐ1 mục 1/3 (`apk add online` trong instance) KHÔNG được giải
  quyết ở đây — stack cài offline từ build, không phải bằng chứng egress.
