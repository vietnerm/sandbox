# Research — 2026-11-20 Phiên 10 (issue #85 — GĐ1 mục 4: stack web thật)

## Nguồn 1: nginx ngx_http_fastcgi_module docs (nginx.org — chính thức)

Link: https://nginx.org/en/docs/http/ngx_http_fastcgi_module.html

Tóm tắt: nginx đứng trước một FastCGI backend bằng `fastcgi_pass`
(unix socket hoặc tcp); tham số script được truyền qua `fastcgi_param`
— hai biến quyết định fcgiwrap chạy script nào là `SCRIPT_FILENAME`
(đường dẫn tuyệt đối tới script) và `PATH_INFO` (sub-path, lấy từ
`$fastcgi_script_name` cho route kiểu `/api/items`). `fastcgi_pass unix:`
trên socket trong cùng fs là cách nhẹ nhất trong sandbox (không cần port
nội bộ thứ hai). Trade-off: nginx không nói chuyện CGI thuần — phải có
FastCGI wrapper (fcgiwrap) làm cầu nối tới CGI shell script.

Áp dụng cho minict: nginx trong instance bind 127.0.0.1:8080 (netns riêng,
loopback-only); `location /api/ { fastcgi_pass unix:/run/fcgi.sock; }` +
SCRIPT_FILENAME hard-coded `/www/cgi-bin/api.cgi` + PATH_INFO để route
sub-path. Config instance-specific được ghi vào rootfs COPY của instance
(không chạm rootfs shared — luật pristine D44).

## Nguồn 2: fcgiwrap README (gnosek/fcgiwrap — upstream)

Link: https://github.com/gnosek/fcgiwrap (README.rst, master)

Tóm tắt: fcgiwrap là FastCGI→CGI wrapper rất nhẹ (~84 KB private memory/
instance), không config; launcher chuẩn là spawn-fcgi (tạo unix socket +
fork daemon, `-s <sock> -M <mode> -P <pidfile>`). Hai mode chạy script:
(1) `SCRIPT_FILENAME` được set → chạy trực tiếp script đó; (2) ngược lại
nối DOCUMENT_ROOT + SCRIPT_NAME rồi tách PATH_INFO. CGI script nhận HTTP
env vars từ FastCGI params + inherit phần còn lại từ môi trường fcgiwrap.

Áp dụng cho minict: payload instance = `spawn-fcgi -s /run/fcgi.sock
-M 0666 -- /usr/bin/fcgiwrap` (socket mode 0666 để nginx worker root ghi
được), sau đó `exec nginx` foreground làm PID 2 — signal lifecycle của
D19 hoạt động không đổi: SIGTERM shim → nginx graceful → shim exit →
kernel SIGKILL toàn bộ pidns còn lại (fcgiwrap orphan không rò rỉ).
Mode (1) SCRIPT_FILENAME dùng làm route chính; PATH_INFO cho sub-path.

## Thêm (kiểm chứng nhanh, không chính 2 nguồn): sqlite3 CLI JSON mode

sqlite3 ≥ 3.33 có `sqlite3 -json <db> "<sql>"` — Alpine 3.22.8 ships
sqlite 3.49.x → dùng được cho CGI trả JSON không cần jq/thư viện ngoài.
CGI input phải sanitize (regex allowlist `[a-zA-Z0-9 _.-]`) vì shell CGI
+ SQL string concat là class injection kinh điển — demo API chỉ nhận
name allowlist, ghi rõ limitation trong code + decision D45.

## Trade-off tổng thể (đã cân nhắc)

- nginx + fcgiwrap + sqlite3 ≈ +4–6 MB rootfs-alpine (8.6 → ~13–15 MB)
  — vẫn ≤ 20 MB ngưỡng disk; perf-guard đo rootfs busybox default
  (2.04 MB) KHÔNG đổi.
- Python3/Node cho API bị loại: +30–40 MB rootfs, phá ngưỡng disk.
- Caddy bị loại: binary Go ~15 MB riêng lẻ, vượt ngân sách.
- CGI shell + sqlite3 là đường nhẹ nhất đạt "nginx + API + SQLite thật"
  trong ngân sách; đây là demo stack GĐ1, không phải target production.
- Pre-install tại BUILD TIME (host) thay vì apk add trong instance: tránh
  đúng class AL7 (chown foreign uid EPERM trong single-uid mapping D20).
