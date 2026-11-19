# Research — 2026-11-19 (issue #5: deep escape PoC)

> Bước 4 của §1. Issue: #5 — fd-based chroot escape, /proc/self/root, setuid vs
> NNP, core dump leak, case seccomp SIGSYS. Nguồn đã mở/đọc thật (không quote
> từ trí nhớ).

## Nguồn 1 — CVE-2018-15664 (Docker `docker cp` symlink race, TOCTOU qua fd)
- https://nvd.nist.gov/vuln/detail/cve-2018-15664 + https://www.openwall.com/lists/oss-security/2019/05/28/2
  (bản NVD trả 503 lúc truy vấn; đối chiếu được nội dung qua RedHat CVE tracker
  https://access.redhat.com/security/cve/cve-2018-15664 và safeguard.sh writeup).
- **Vấn đề**: daemon nắm file descriptor đã mở trước khi kiểm tra symlink;
  attacker đổi đối tượng giữa check và use → daemon đọc/ghi file HOST bằng
  đặc quyền của daemon. Bản chất: **mọi fd mở bên ngoài sandbox và được dùng
  sau khi kiểm tra là bề mặt TOCTOU**.
- **Áp dụng cho minict**: minict launcher mở fd (rootfs dirfd, ns fds ở
  minict-exec) và fork/exec payload; hôm nay **không có close_range/FD_CLOEXEC
  nào** trong payload path (grep minict.c: 0 kết quả) → một fd của caller (vd
  bash `exec 9</etc/passwd`) sống sót fork+exec vào payload, bypass toàn bộ
  pivot_root. Đây chính là vector "fd-based escape" issue #5 nêu. Pattern sữa
  đúng chuẩn industry: **close_range(3, ~0U, 0) ngay trước exec** (tương đương
  những gì Docker/runc/bwrap làm với fd table của process exec'd).

## Nguồn 2 — close_range(2) (man7, kernel 5.9+)
- https://man7.org/linux/man-pages/man2/close_range.2.html + Phoronix 2020
  (https://www.phoronix.com/news/Linux-5.9-Close-Range) xác nhận syscall 436,
  vào kernel từ 5.9.
- **Giải pháp có sẵn**: `close_range(first, last, flags)` đóng dải fd hiệu quả
  (kernel-side, không loop userspace); flags `CLOSE_RANGE_UNSHARE` cho COW fd
  table. Máy dev kernel 5.10 và runner ubuntu-24.04 (6.x) đều ≥ 5.9 → dùng trực
  tiếp được; **fallback**: loop close 3..RLIM (cho kernel cũ), bỏ qua lỗi từng
  fd. Kể cả fd 3+ trong process: chỉ giữ 0/1/2 (stdio).
- **Trade-off**: đóng "quá tay" có thể cắt fd ứng dụng cần — nhưng payload của
  minict là lệnh do operator chỉ định, hợp đồng hiện tại chỉ cam kết stdio
  (minict-exec giữ stdin-passthrough fd 0, cùng quy tắc 0–2). Không hợp đồng
  fd nào khác tồn tại → an toàn.

## Nguồn 3 — PR_SET_NO_NEW_PRIVS (man7 + kernel doc)
- https://man7.org/linux/man-pages/man2/PR_SET_NO_NEW_PRIVS.2const.html +
  https://www.kernel.org/doc/Documentation/prctl/no_new_privs.txt
- **Chốt semantics**: với no_new_privs=1, "execve(2) promises not to grant
  privileges" — setuid/setgid bit + file capabilities bị bỏ qua khi exec; flag
  sống sót qua exec và không thể gỡ (chỉ prctl set, không clear).
- **Áp dụng**: minict đã set NNP cho payload (NNP1 test). Biến thể issue #5 yêu
  cầu: **sau khi exec một binary có setuid bit** flag vẫn phải =1 và CapEff vẫn
  =0 — assert bằng host /proc (kiểu D32) thay vì assert trong-ns (vì single-uid
  mapping làm mọi setuid trong-ns thành no-op nên assert trong-ns sẽ vacuous —
  bài học #38).

## Nguồn 4 — core dump: core(5) + core_pattern container escape
- https://man7.org/linux/man-pages/man5/core.5.html (truy vấn trực tiếp):
  **"RLIMIT_CORE will be ignored if the system is configured to pipe core dumps
  to a program"** — RLIMIT_CORE=0 chặn core FILE, nhưng **không chặn** piped
  core_pattern.
- Elastic rule "Container Escape via Kernel core_pattern"
  (https://www.elastic.co/docs/reference/security/prebuilt-rules/rules/linux/privilege_escalation_container_escape_via_kernel_core_pattern)
  + KubeHound CE_UMH_CORE_PATTERN: nếu container ghi được
  `/proc/sys/kernel/core_pattern` (host view) → crash của tiến trình trong
  container làm HOST chạy handler → escape thực sự. Trường hợp piped: handler
  HOST nhận bộ nhớ của process crash (rò thông tin ra ngoài + noise/DoS).
- **Áp dụng cho minict**: payload không có /proc mount (AL4: mount EPERM cả dev
  lẫn runner) → không thể ghi core_pattern → vector "ghi pattern" đóng. Vector
  còn lại: crash → core file ghi vào cwd (nếu pattern là file) → đầy rootfs/disk
  + file lộ bộ nhớ payload nằm ngoài tmpfs. Mitigation chuẩn: **setrlimit(
  RLIMIT_CORE, {0,0}) trong payload trước exec**. Residual (ghi rõ, không giấu):
  piped pattern không bị RLIMIT chặn — bộ nhớ payload gửi ra handler host; đây
  là một chiều outbound (payload chỉ rò bộ nhớ của chính nó), không phải nâng
  đặc quyền; xử lý triệt để cần pattern-side control (GĐ2, host-side config).

## Nguồn 5 — /proc/self/root & chroot escape classics (bối cảnh, không cần PoC mới)
- Kiến thức nền đã đối chiếu với man7 pivot_root(2)/chroot(2) khi viết test:
  chroot jail escape cổ điển (chroot vào subdir rồi chroot("..") — cần
  CAP_SYS_CHROOT) và escape qua `/proc/self/root/fd` (cần host procfs mount).
- Trạng thái minict: CapEff=0 (CAPD1) → chroot(2) EPERM; /proc không mount
  (PROC1 xfail + AL4) → không có `/proc/self/root` để mổ. Issue yêu cầu
  "xác minh lại sau khi có netns" — netns không đổi mount/proc semantics, nhưng
  test phải **khóa bằng assertion** thay vì lý thuyết: PROC2 (`! -e
  /proc/self/root`) + CHROOT1 (chroot EPERM).

## Case seccomp SIGSYS (đủ mục issue #5)
- Filter minict là blocklist `SECCOMP_RET_ERRNO|EPERM` + wrong-arch
  `SECCOMP_RET_KILL_THREAD` (minict.c:1081-1100). Không dùng RET_TRAP → không
  có SIGSYS catchable từ blocklist hit. Test SC21 khóa contract: syscall bị
  chặn trả **rc=1 (EPERM)** và process sống — nếu ai đổi sang TRAP/KILL thì
  payload chết 159/137, test đỏ. (RET_KILL_THREAD với wrong-arch là chủ ý,
  giữ nguyên.)

## Tổng hợp quyết định cho phiên (D33, xem decisions/deep-escape.md)
1. minict.c payload: `close_range(3, ~0U, 0)` + fallback loop, và
   `RLIMIT_CORE=0` — trước drop_all_caps/install_seccomp.
2. minict-exec.c child: cùng hardening (0–2 giữ, 3+ đóng; CORE=0) — cùng lớp
   bề mặt, giữ đồng bộ hai binary ở mức fd/rlimit (filter không đổi → MD0
   không lệch).
3. Test mới isolation suite: FD1, PROC2, CHROOT1, SETUID2, CORE1, CORE2.
   Seccomp suite: SC21.
4. Residual ghi vào decision + PR: piped core_pattern (GĐ2), không tự đóng
   issue #5 theo §6 (type:security) — chờ owner `verified`.

---

# Phiên 3 — Research pasta (issue #57, GĐ1 mục 3)

> Bổ sung sau D34 (cùng ngày): khi implement đã đọc SOURCE passt 2026_10_02
> thay vì chỉ docs — phát hiện một trap lớn và xác nhận một cơ chế.

## Nguồn 1: passt source 2026_10_02.cba3570 (passt.top cgit snapshot)
- Tarball đã pin (HEAD request + sha256 đo được): https://passt.top/passt/snapshot/passt-2026_10_02.cba3570.tar.gz
- Trang dự án / mô hình phát hành (không có binary tĩnh chính thức): https://passt.top/passt/about
- Trang chính: https://passt.top/passt
- `conf.c conf_pasta_ns()`: `pasta --netns <path>` **không có `--userns`**
  → `netns_only = 1` ngầm định ("Attaching to a netns/PID, with no userns
  given"). Hệ quả: pasta ở lại userns caller, setns vào netns thuộc userns
  instance minict → EPERM. Chống lại giả thuyết D34 ("pasta tự join userns
  sở hữu netns") — must-pass `--userns /proc/<pid>/ns/user` tường minh.
- `conf.c conf_ugid()`: non-root giữ euid/egid (không drop nobody) — với
  `--runas` mặc định chỉ áp dụng khi start as root in init ns.
- `isolation.c isolate_user()`: setgid/setuid(euid) (no-op) → enter_userns
  (euid==owner rule) → uid 0 trong ns (map `0 1001 1`) → giữ CAP_SYS_ADMIN
  cho setns(netns) + unshare trong isolate_prefork. Chain rootless khép kín.
- `fwd.c nat_outbound()`: outbound ns→dst: chỉ MAP host-loopback (mặc định =
  gateway addr) và map-guest-addr bị dịch; còn lại **đi qua nguyên vẹn** →
  wget từ sandbox tới IP host tới listener `0.0.0.0` host — deterministic.
- `pasta --help`: `-t HPORT:GPORT` (host→ns forward), `-f/--foreground`
  (mặc định daemonize — cần cho pid tracking), `--config-net`.

## Nguồn 2: podman rootless pasta invocation
- Oracle lab "Use Pasta Networking with Podman" (ps auxww cho thấy argv thật của pasta rootless, `-t 8080-8080:80-80 --netns /run/user/1001/netns/netns-…`): https://luna.oracle.com/lab/6cc1f2f1-5a2b-44cb-8662-b89aec478449/steps
- eriksjolund/podman-networking-docs (so sánh pasta vs slirp4netns, source-address preserved): https://github.com/eriksjolund/podman-networking-docs
- Podman spawn pasta uid thường: `pasta --config-net -t 8080-8080:80-80 …
  --netns /run/user/1001/netns/netns-XXX` — cùng model "pasta host-side
  process join ns của container", precedent cho flow `up --pasta`.
- Khác: podman chạy pasta TỪ TRONG userns đã join (inherited) nên không cần
  --userns; minict spawn pasta từ host userns → phải --userns (Nguồn 1).
  Đây là điểm thiết kế KHÁC của minict so với podman.

## Áp dụng cho minict (D37, decisions/pasta-egress.md)
1. `tools/fetch-pasta.sh` pin snapshot + sha256 + `make static` (D20 pattern;
   passt không phát binary tĩnh chính thức — build từ source ~19 s).
2. `minict.sh up [--pasta] [-- payload]`: spawn `pasta --foreground
   --userns --netns --config-net` sau khi instance sống; route readiness
   poll host-side `/proc/<pid>/net/route` (00000000).
3. Suite `tests/test_pasta.sh` 7 case: PT1/PT2/NET11C chạy mọi nơi,
   NET10/11/12/AB1 SKIP(env-blocked) khi không tun (máy dev đo được — D34).
   Egress test dùng IP host listener (không internet, không DNS).

## Phiên 4 — procfs trong userns+pidns (issue #31, D39)

Nguồn:
1. Linux `fs/namespace.c` — `mount_too_revealing()` / `mnt_already_visible()`:
   procfs/sysfs chỉ mount được trong userns không phải init khi ns hiện có một bản
   mount cùng loại "fully visible" (không bị mount con che). https://github.com/torvalds/linux/blob/master/fs/namespace.c
2. `pid_namespaces(7)` — procfs phản ánh pidns của **tiến trình thực hiện mount**,
   nên phải mount từ shim (đã trong pidns), không phải launcher.
   https://man7.org/linux/man-pages/man7/pid_namespaces.7.html
3. `mount_namespaces(7)` — mount "locked" khi copy sang userns ít đặc quyền hơn: lý do
   host có overmount trên /proc (container masking) vẫn EPERM.
   https://man7.org/linux/man-pages/man7/mount_namespaces.7.html
4. bubblewrap `bind-mount --proc` làm cùng thứ tự: mount proc trong ns mới trước khi
   bỏ root cũ. https://github.com/containers/bubblewrap

Kết luận: giả thuyết SIA PR #24 đúng; sửa thứ tự là đủ, không cần quyền mới.

## CVE gate (issue #56, D38) — Claude, PR #63

Ghi bổ sung theo SIA PR #63 (B6). Máy phiên Claude không ra được Internet (proxy chặn
NVD/docs) — các nguồn dưới đây là tài liệu chính thức tham chiếu, **không fetch lại
trong phiên này**; nguồn nội bộ đã đọc trực tiếp.

### Nguồn 1 — NVD CVE API 2.0 (https://nvd.nist.gov/developers/vulnerabilities)
- `GET /rest/json/cves/2.0?cveId=…` trả một CVE; tìm theo sản phẩm cần `cpeName=` hoặc
  `keywordSearch=` — đây chính là thứ `cve-check.py` **không** làm (chỉ chấm lại id có
  sẵn). → Ghi giới hạn "không phát hiện CVE mới" vào D38, việc truy vấn theo CPE để
  issue riêng.
- Rate limit không key ~5 request / 30 s; lỗi transient trả HTTP 403/503 → hàng `error`
  trong report (#44). → gate không bao giờ coi hàng lỗi là "mới" (CG4).

### Nguồn 2 — GitHub CLI `gh issue list --search` (https://cli.github.com/manual/gh_issue_list)
  + cú pháp tìm issue (https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)
- `--search` nhận cú pháp search GitHub (`in:title`, `is:open`, `label:`); chuỗi có `"`
  làm query sai nghĩa → dedupe trượt → issue trùng. → CG7: id phải khớp
  `CVE-\d{4}-\d{4,}` trước khi ra khỏi gate (SIA low).
- Search index trễ vài giây–phút so với lúc tạo issue; cron chạy 1 lần/ngày nên không
  đua.

### Nguồn 3 (nội bộ) — `tools/cve-check.py`, `.github/workflows/cve-daily.yml`, issue #34/#52
- #34 (18/11) và #52 (19/11) cùng nội dung: luật cũ "CVSS ≥ 8 → mở p0" lặp mỗi ngày
  cho CVE đã có trong watchlist. Đo được trực tiếp trên repo.

### Áp dụng
Gate = report ∩ (CVSS ≥ 8) − watchlist, fail loud khi input hỏng (CG5/CG7).
