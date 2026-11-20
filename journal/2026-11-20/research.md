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

---

## Phiên 12 — daemon service hoàn thiện (issue #91, GĐ2 mục 1)

**Vấn đề:** seed minictd có REST API nhưng chỉ dùng được qua curl thủ công trong
test; daemon không có cách chạy như service người dùng thường (systemd --user),
không tự sống lại qua reboot, không có proactive reconcile (A3 — chỉ self-heal
khi query).

**Nguồn 1 — systemd.unit (freedesktop.org/man/latest/systemd.unit.html, 200):**
User Unit Search Path xác nhận `~/.config/systemd/user/*` là vị trí chuẩn unit
của user service manager (per-user, không phải system manager). Unit user
`WantedBy=default.target` (không phải multi-user.target — đó là system).
→ Áp dụng: `minictd/deploy/` ship template + `install.sh` render `%h`/`$HOME`
tại install time (quadlet pattern) — không hardcode user; verify bằng
`systemd-analyze verify` (có trên máy dev + runner ubuntu-24.04, đo được exit 0
với stub binary).

**Nguồn 2 — Podman quadlet (docs.podman.io podman-systemd.unit.5.html, 200):**
pattern chuẩn cho runtime rootless chạy như user service: unit được GENERATE
từ template tại install/enable time (không edit unit tay); generated service
defaults `Restart=on-failure` + `WantedBy=default.target`. Rootless runtime
(podman/minict) chỉ cần binary người dùng + state dir `$HOME`-scoped.
→ Áp dụng: `Restart=on-failure`, `RestartSec=2`, `ExecStart` render path
`$HOME/.local/bin/minictd`; autostart instance là việc của DAEMON (flag
`--autostart`) chứ không phải của unit — unit chỉ bảo đảm daemon sống.

**Nguồn 3 — loginctl (freedesktop.org/man/latest/loginctl.html, 200):**
`enable-linger`: "a user manager is spawned for the user at boot and kept
around after logouts. This allows users who are not logged in to run
long-running services." ⇒ điều kiện tiên quyết bắt buộc để autostart qua reboot
hoạt động (không linger → daemon chỉ chạy khi có session login).
→ Áp dụng: `install.sh` hướng dẫn/attempt `loginctl enable-linger` (best-effort,
gần sudo; ghi docs rõ + cảnh báo). Test CI không kiểm linger (runner không có
systemd user instance đầy đủ trong job) — chỉ verify unit cú pháp + cờ daemon.

**Trade-off / quyết định:** (1) CLI `sandbox` tái dùng `contract.NewHTTPClient`
đã implement đủ Runtime interface — không viết client riêng lẻ lặp logic;
(2) autostart policy: chỉ recreate instance đang `state=running` lúc daemon
start (instance `stopped` do operator vẫn stopped — tôn trọng ý đồ stop);
(3) A3 reconcile dùng `--reconcile-interval N` (giây, 0=off) — goroutine ticker
gọi `ReconcileAll()` (quét state dir, `alive()` self-heal đã có — đổi từ lazy
sang proactive); Go unit test không cần spawn payload thật (state.json PID chết
giả → heal); (4) daemon restart không phải restart instance (units/log ghi
"instances keep running" — autostart của reboot riêng biệt).

## Phiên 13 — CG1 cgroups v2 (issue #4, S2)

**Vấn đề:** minictd không enforce CPU/RAM/pid (GAPS CG1/S2 — Notes-only + rlimit
nproc approximation). Rootless resource limits **cần cgroups v2** (cgroups v1
không rootless-friendly). Máy dev: v1 → env-gated; runner ubuntu-24.04: v2
unified → PASS thật được kỳ vọng.

**Nguồn 1 — systemd.io/CGROUP_DELEGATION (200):**
systemd hỗ trợ controllers `cpu, io, memory, pids` trên unified hierarchy v2.
`Delegate=yes` expose controllers cho delegated subtree nhưng **không enable
sẵn** — delegated user phải tự write `cgroup.subtree_control` (`+memory`…).
Model "islands" phù hợp minictd: daemon nằm trong subtree delegated
(`user@<uid>.service` có Delegate mặc định), tạo sub-cgroup cho instance bên
dưới; **bắt buộc rule no-processes-in-inner-nodes** — không enable domain
controller trên cgroup đang chứa process.
→ Áp dụng: tạo subtree `minictd-apps/` (không chứa process) dưới cgroup daemon,
enable `+memory +cpu +pids` trên `minictd-apps/cgroup.subtree_control`, rồi
mỗi instance là con `minictd-apps/sandbox-<name>` — daemon PID không nằm trong
minictd-apps → rule không vi phạm.

**Nguồn 2 — podman rootless.md (200):**
"No support for setting resource limits on systems using cgroups v1" — rootless
resource limits chỉ có trên cgroups v2. Xác nhận: CG1 trên dev v1 là
**env-gated** (không phải parked theo thiết kế — roadmap "blocked-on-dev-env"
vẫn nằm trong kế hoạch).

**Nguồn 3 — kernel.org cgroup-v2.rst (torvalds master, 200):**
- **No Internal Process Constraint**: "only domain cgroups which don't contain
  any processes can have domain controllers enabled in their
  `cgroup.subtree_control`" — củng cố thiết kế subtree apps riêng.
- **Delegation Containment**: delegatee cần write access `cgroup.procs` của
  source + destination + **common ancestor** — move shim PID (nằm trong cùng
  delegated subtree với cgroup instance) vào cgroup con hợp lệ; không thể pull
  process từ ngoài delegated subtree vào (chống injection chéo user).
- Hierarchical: limits không thể escape từ delegated subtree — parent quota
  trội mọi sub-cgroup.
→ Áp dụng: write PID shim vào instance `cgroup.procs`; nếu shim chưa nằm trong
subtree (daemon spawn shim từ PID namespace host của daemon — daemon nằm trong
subtree) → cùng subtree → OK; nếu write fail EACCES → fail-soft + note lý do.

**Trade-off / quyết định:**
1. **Fail-soft, không fail-closed**: thiếu cgroups v2 → giữ hành vi hiện tại
   (Notes + rlimit nproc) + note lý do đo được — instance vẫn boot được trên
   mọi môi trường (không phá baseline 124 case).
2. **Env override** `MINICTD_CGROUP_ROOT`: chỉ cgroup base dir (điểm bắt đầu
   tạo subtree apps) — giúp test unit (temp dir giả lập) + runner (chỉ rõ).
3. **Stats ưu tiên cgroup reads** khi enforced: `memory.current` + `cpu.stat`
   usage_usec + `pids.current`; fallback RSS tree khi không enforced
   (`procstats.go` giữ nguyên).
4. **Cleanup**: destroy/stop → rmdir cgroup instance (chỉ khi rỗng; ignore
   ENOENT) — chết rồi mới dọn, không rò rỉ mount/proc.
5. Test env-gated probe-based (pattern pasta/alpine): SKIP khi không có cgroups
   v2 writable; mutation trên code enforcement (go unit) + shell suite.

## Phiên 14 — G3 preview routing (issue #97)

**Vấn đề**: minictd unix-socket bridge hoạt động per-port nhưng preview routing chưa nối — Traefik chưa biết `s-<id>.<domain>` → `127.0.0.1:<port>`, wake probe chưa có surface daemon-side (BridgeIP() luôn rỗng — MERGE-GUIDE delta).

**Nguồn 1 — Traefik file provider docs (v3.5, verified 200 raw.githubusercontent.com/traefik/traefik/v3.5/docs/content/providers/file.md)**:
- File provider nhận dynamic config qua **1 file hoặc 1 directory** các file `.yml`/`.toml` (`providers.file.directory`).
- Shape: `http.routers.<name>.{rule, entryPoints, service}` + `http.services.<name>.loadBalancer.servers[].url`.
- Router rule dạng `Host(\`s-x.localhost\`)`.

**Nguồn 2 — Traefik provider source (v3.5.0 pkg/provider/file/file.go, verified 200)**:
- Directory mode: `os.ReadDir` → watch từng entry bằng fsnotify (`addWatcher`); event Create/Write/Rename → reload dynamic config.
- Chỉ load `.yml`/`.toml` → **atomic write bằng temp + rename an toàn**: temp có suffix khác `.yml` không bao giờ được load; rename sang `.yml` trigger reload đúng 1 lần.

**Áp dụng cho minict (thiết kế D49)**:
- Daemon sinh **1 file `.yml`/instance** (`sandbox-<name>.yml`) trong `--traefik-dir`: router `s-<name>` (1 port) hoặc `s-<name>-p<port>` (multi-port D24) → `http://127.0.0.1:<port>` (webproxy bridge giữ port host-reachable).
- Hooks sau saveState ở Run/Start (+file), Stop/Remove (−file); ReconcileAll heal cả route dir (syncAllRoutes: remove file stale); daemon boot sync lại từ state dir.
- **Wake endpoint** `POST /v1/sandboxes/{name}/wake`: dial `127.0.0.1:<port>` (dialTimeout sẵn có store.go:602), trả per-port TCP-ready; 409 khi stopped, 404 khi không tồn tại — đúng contract BridgeIP()=="" của MERGE-GUIDE Step 5.
- `traefik.*` labels giữ nguyên round-trip qua Inspect (không dịch DSL docker-label — chỉ honor `traefik.enable=false` để opt-out; residual ghi rõ).
- Fail-soft: `--traefik-dir` rỗng = feature off, zero overhead (pattern CG1).
