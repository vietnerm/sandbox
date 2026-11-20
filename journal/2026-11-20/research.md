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

---

## Phiên 11 — Debian rootfs (issue #88, GĐ1 mục 1 residual)

**Vấn đề:** boot Debian rootfs trong minict + `apt-get install` online qua pasta. Blocker đo được: `minict.c:1343` sanity-check hardcode `/bin/busybox` (debian không có) — boot fail `minict: no busybox`.

**Nguồn 1 — debuerreotype/docker-debian-artifacts (github.com/debuerreotype/docker-debian-artifacts, branch dist-amd64):**
rootfs chính thức của image `debian:bookworm` Docker, build bởi debuerreotype 0.17 từ snapshot.debian.org (epoch 1794787200 = 2026-11-16). OCI layout: layer digest ghi trong `image-manifest.json`; tải `bookworm/slim/oci/blobs/rootfs.tar.gz` (28,247,934 B) — đo sha256 **khớp** digest `sha256:2a7a0929...dfd0` (verified 2026-11-20, tải + sha256sum local). Repo cũng publish provenance (`rootfs.debian_version` = 12.15, `rootfs.tar.xz.sha256`). Provenance mạnh hơn tarball-mirror tùy ý: digest pin fail-closed được.
→ Áp dụng: `tools/fetch-debian-rootfs.sh` pin URL + sha256 + size, fail-closed (pattern D37 pasta).

**Nguồn 2 — deb.debian.org + cấu trúc rootfs slim (đo trực tiếp trên bản tải về):**
`/etc/apt/sources.list.d/debian.sources` (deb822) mặc định **HTTP** `deb.debian.org/debian` + bookworm-security; `Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg` có sẵn ⇒ apt-get update verify signature mặc định BẬT (không thể `--allow-untrusted` kiểu apk — gpgv bắt buộc). **Không có** `/etc/ssl/certs/ca-certificates.crt` ⇒ HTTPS runtime thiếu (giống alpine trước #74) — HTTP mirror đủ cho update + install; HTTPS là residual như alpine (cài ca-certificates sẽ mở — cùng class).
`_apt` uid 42 có trong /etc/passwd nhưng 42 unmapped trong single-uid userns ⇒ setuid(42) EPERM ⇒ apt warning `Download is performed unsandboxed as root` (documented behavior, rc vẫn 0).
→ Áp dụng: test online chạy HTTP; ghi chú warning như hành vi đo được, không phải lỗi.

**Nguồn 3 — images.linuxcontainers.org (lxc distrobuilder):** nguồn thay thế có date-pinned dirs + SHA256SUMS GPG-signed; rootfs ~100MB+, kèm systemd/openrc. Chọn debuerreotype vì nhỏ hơn (slim 28MB nén) + digest OCI pin được + đúng rootfs của image `debian` chính thức.

**Trade-off / quyết định:** (1) rootfs-debian **gitignored** (83MB unpacked) — khác rootfs-alpine (13.3MB tracked) vì quá nặng cho git; fetch CI mỗi run ~28MB ~10-20s. (2) Sanity check binary đổi `/bin/busybox` → `/bin/sh` executable resolve symlink (minict.sh đã có logic chuẩn D43) — busybox/alpine vẫn pass; fail-closed giữ nguyên (không rootfs → không boot). (3) Suite 8 case mirror các class đã có: AL4 (proc env-aware), AL7 (chown EINVAL), AL9 (pristine), PT4/PT4C (online + control anti-vacuous PROBE_ALIVE + 2-step anti-vacuous #80).
