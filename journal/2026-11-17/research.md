# Research 2026-11-17 — User namespaces + mount ns + pivot_root: mini-container không runtime

## Phạm vi
Chủ đề backlog #1: tự viết mini-container rootless chỉ bằng cơ chế kernel có sẵn,
không Docker/bwrap/nsjail. Kèm khảo sát khả năng thực tế của môi trường dev hiện có.

## 1. Cơ chế hoạt động (tổng hợp từ nguồn chính thống)

### user_namespaces(7)
- `unshare(CLONE_NEWUSER)` đưa caller vào userns mới **ngay lập tức**; `getuid()` trả 65534
  (nobody) cho tới khi `/proc/self/uid_map` được ghi.
- Ghi map không cần CAP_SETUID ở parent nếu chỉ map **effective uid của chính mình**
  (1 entry) — đây là nền tảng của mọi rootless container (LXC rootless, Podman, RootlessKit).
- Bắt buộc ghi `/proc/self/setgroups = "deny"` **trước** `gid_map` (kernel ≥3.19) khi
  không có CAP_SETGID ở parent. **Hệ quả ít biết:** sau deny, syscall `setgroups(2)`
  bị EPERM vĩnh viễn trong userns đó (đã dính khi PoC — phải bỏ khỏi capdrop).
- Capabilities: process có **full caps trong userns mới**, nhưng chỉ trên các object
  được sở hữu bởi userns đó (mount tạo sau unshare, pidns của chính nó...).
- Lịch sử bảo mật: userns từng là bề mặt CVE lớn. Ví dụ đã xác minh trên NVD:
  **CVE-2023-0386** — OverlayFS copy-up cho phép thực thi setuid file trái phép
  khi mang file có capability từ mount `nosuid` sang mount khác trong userns.
  → quyết định hôm nay: **không dùng overlayfs trong userns trên kernel 5.10**.

### mount_namespaces(7) + pivot_root(2)
- Sau `unshare(CLONE_NEWNS)`, mọi mount cũ trở nên **MNT_LOCKED**; mount mới đè lên
  host mountpoint bị chặn theo check `mount_too_revealing` (đã đo: EPERM khi mount
  tmpfs lên host `/tmp`, `/proc`, `/sys`, `/dev`).
- **Mount vào thư mục thường bên trong rootfs của mình thì OK** — đã đo và xác minh.
- `pivot_root(new_root, put_old)`: yêu cầu new_root là mount point (→ bind rootfs
  lên chính nó trước), put_old nằm dưới new_root, cùng mount. Sau pivot:
  `umount2(put_old, MNT_DETACH)` để host root hoàn toàn tàng hình. **Fail-closed**:
  nếu umount old root lỗi phải abort, không được tiếp tục.
- Trước pivot: `mount(NULL, "/", MS_REC|MS_PRIVATE)` để cắt propagation
  (shared subtree) — nếu không, mount event có thể leak hai chiều với host.

### pid namespace
- `unshare(CLONE_NEWPID)` **không** đưa caller vào pidns mới — chỉ con sau `fork()`.
  Mẫu chuẩn: unshare → setup mount → fork → child (PID 1) exec.
- Khi PID 1 của pidns chết, kernel kill toàn bộ process còn lại trong ns đó —
  đây là cơ chế "teardown tự động" tận dụng cho no-orphan guarantee.
- Mount `/proc` bên trong pidns mới cho view process đúng ns; ở môi trường hôm nay
  bị chặn (mục 3).

### Signal & process group
- `PR_SET_PDEATHSIG` bị **clear khi fork** → phải set lại trong child.
- Busybox ash: (a) **exec-optimize** lệnh cuối của `sh -c` (sh biến thành lệnh đó,
  mất cmdline gốc); (b) **defer fatal signal khi đang wait con** (WCE semantics).
  → wrapper phải setpgid con + kill cả process group khi dọn dẹp.

## 2. So sánh nhanh các lựa chọn thay thế (đánh giá định tính + số liệu công khai)

| Công nghệ | Isolation | Startup | RAM overhead | Độ phức tạp | Ghi chú |
|---|---|---|---|---|---|
| **Tự viết (hôm nay)** | ns x5 + capdrop + NNP | ~1ms (đo) | ~1MB (đo) | ~260 dòng C, toàn quyền kiểm soát | không overlayfs, không netns |
| bubblewrap | ns x5, battle-tested | ~10-50ms (cite) | nhỏ | tool ngoài, phải cài | không có trong env này |
| Podman rootless | ns + overlayfs + netns (slirp4netns) | ~500ms (cite) | ~100MB (cite) | cao, cần nhiều daemon/deps | nặng hơn nhu cầu dev/CI |
| gVisor runsc | syscall interception (mạnh nhất) | ~200-500ms (cite) | ~50-100MB (cite) | rất cao | chống kernel-exploit, overkill cho dev |
| Firecracker microVM | VM hardware | ~125ms VM boot (cite) | ~50MB/cũng guest | rất cao | cần KVM — env này không có |
| Wasmtime/WASI | capability-based | ~5-20ms (cite) | rất thấp | model khác (không POSIX đầy đủ) | backend WASM là chủ đề ngày sau |

(cite) = số liệu công khai, không đo được trong env này; dùng để so sánh tương đối.

## 3. Kết quả đo thực nghiệm trên môi trường ngày hôm nay

Kernel `5.10.134-013.15.kangaroo.al8.x86_64`, user `z` (uid 1001), Debian 13 userspace,
host có seccomp filter active (Seccomp:2, 1 filter) và cgroups **v1** (không có v2 unified).

| # | Cơ chế | Kết quả |
|---|---|---|
| 1 | unprivileged userns (uid_map 0→1001) | OK |
| 2 | mount tmpfs trong mountns (dir thường) | OK |
| 3 | mount đè lên host mountpoint | **EPERM (MNT_LOCKED)** |
| 4 | pivot_root + umount2(MNT_DETACH) old root | OK |
| 5 | pidns qua unshare+fork (PID 1 trong ns) | OK |
| 6 | utsns sethostname | OK |
| 7 | ipcns | OK |
| 8 | mount /proc (kể cả trong rootfs mới) | **EPERM — host seccomp chặn** |
| 9 | mknod device node trong userns | **EPERM** → fallback bind-mount /dev/null từ host trước pivot |
| 10 | cgroups v2 / Landlock (≥5.13) | Không khả dụng trên kernel này |
| 11 | **SIGTERM từ host vào process trong userns** | **rc=0 nhưng KHÔNG delivery** — đo bằng A/B/C matrix; SIGKILL được delivery bình thường |

Số 11 là phát hiện bất ngờ nhất của ngày: `kill -TERM <pid-trong-userns>` trả về
thành công nhưng tín hiệu không tới (SigPnd=0 mãi). SIGKILL được delivery.
Giả thuyết: host seccomp/LSM của sandbox product chặn cross-userns signal với
fake-success (bảo vệ supervisor). Hệ quả thiết kế: cleanup phải dựa
SIGKILL + `PR_SET_PDEATHSIG` + teardown-tự-động-của-pidns, không dựa TERM.

## Nguồn
- man7.org: user_namespaces(7), mount_namespaces(7), pivot_root(2), namespaces(7),
  capabilities(7) — https://man7.org/linux/man-pages/
- Kernel docs: sharedsubtree (mount propagation),
  https://www.kernel.org/doc/Documentation/filesystems/sharedsubtree.txt
- NVD CVE-2023-0386 (đã query trực tiếp NVD API 2.0, xác minh mô tả) — overlayfs+userns
- rootless patterns: https://github.com/rootless-containers/rootlesskit (patterns),
  https://github.com/containers/bubblewrap (reference implementation)
- Landlock (cần ≥5.13, không dùng được hôm nay):
  https://docs.kernel.org/userspace-api/landlock.html
- util-linux unshare(1) — tham chiếu flow uid_map/setgroups

---

# Phiên 3 (SDDA cycle) — Web server trong sandbox + netns feasibility cho web stack

## Vấn đề
DoD mục 1 (PROMPT.MD §8): web server thật chạy trong minict. Câu hỏi: (a) httpd nào
đủ nhẹ và đã có sẵn; (b) bind ở đâu khi chưa có netns; (c) khi có netns thì rootless
kết nối ra host bằng gì.

## Nguồn mới hôm nay (đã đọc thật)

1. **busybox `networking/httpd.c`** (mirror/busybox trên GitHub, đúng source của applet
   v1.37.0 đang trong rootfs):
   - Usage: `httpd [-ifv[v]] [-c CONFFILE] [-p [IP:]PORT] [-u USER[:GRP]] [-r REALM] [-h HOME]`
     — `-p [IP:]PORT` cho bind chọn địa chỉ (default *:80), `-f` foreground, `-h HOME` docroot.
   - `-u USER[:GRP]` privilege drop SAU khi bind (FEATURE_HTTPD_SETUID) — không cần vì
     sandbox đã capdrop; Deny/Allow IP logic có sẵn (D:* deny-all precedence) — hữu ích
     làm ACL lớp 2 khi chưa có netns.
   - CGI bật theo build config; static + index.html là đủ cho mốc đầu tiên.
   - Đối chiếu thực tế: probe live cho HTTP 200 + `Content-type` đúng, không có header
     `Server:` (giảm fingerprint — ok cho lab).

2. **"Linux User Namespaces: 262% More Kernel Attack Surface" (edera.dev)**:
   - userns cấp CAP_NET_ADMIN trong user+net ns → mở mặt nf_tables/nftables cho process
     unprivileged; "40 CVE trong 5 năm, 18 riêng nf_tables, chỉ tới được khi userns bật".
   - **Kết luận cho minict**: netns (issue #3) PHẢI đi cùng seccomp (issue #2) — chặn
     netlink/nftables từ payload; capdrop hiện tại là mitigation (đo được mount EPERM).

3. **Rootless networking patterns (podman/slirp4netns/pasta docs tổng hợp)**:
   - Pattern chuẩn: container có netns riêng + usermode TCP/IP translation (slirp4netns
     hoặc pasta — default mới của podman); port publishing qua userspace.
   - Port <1024 rootless cần `net.ipv4.ip_unprivileged_port_start` hoặc redirect.
   - Env này KHÔNG có slirp4netns/pasta → pattern thay thế ngày đầu: **bind loopback
     trong host netns (share)** — web chỉ reach từ host local, không lộ interface ngoài.

## Thực nghiệm trên env hôm nay (đo thật, không suy đoán)

| Thử | Kết quả |
|---|---|
| `unshare --user --net` với uid 1001 | OK — userns+netns mới được tạo |
| `ip link set lo up` trong netns mới | OK — CAP_NET_ADMIN hiệu lực trong ns |
| loopback ping trong netns | OK — 0.081 ms |
| `ip link add dummy0 type dummy` | OK |
| `ip link add veth0 type veth peer veth_a` | OK (cú pháp busybox: `peer <name>`) |
| Nối 1 đầu veth ra host netns | **không thể rootless** (cần CAP_NET_ADMIN host ns = root) |
| busybox httpd -f -p 127.0.0.1:18099 -h /www trong minict | OK — curl 200 từ host |

## Trade-off + áp dụng cho minict

- **Hôm nay (issue #1)**: busybox httpd + bind 127.0.0.1 trong shared host netns.
  Ưu: 0 dependency, +0MB disk, <1s ready. Nhược: không isolate network (gap đã biết
  NET1), chỉ an toàn nhờ bind loopback + capdrop + NNP.
- **Ngày tới (issue #3)**: thêm `CLONE_NEWNET` vào minict (setup trước capdrop):
  loopback-only là đủ cho web test; veth trong ns tạo được nhưng KHÔNG nối ra host được
  rootless → cần usermode proxy (tự viết unix-socket bridge hoặc port ssh-style
  forwarding) — đây là phần lớn nhất của netns day.
- **Cặp bài trương**: netns + seccomp chặn netlink/nftables (bài học edera.dev) —
  nếu không, netns mới còn MỞ THÊM mặt tấn công kernel từ sandbox.

---

# Phiên 4 (SDDA cycle) — seccomp-bpf blocklist cho minict (issue #2)

## Vấn đề
Sau capdrop+NNP, sandbox vẫn gọi được TẤT CẢ syscall — trong đó có cả lớp
syscall "đắt đỏ" về mặt attack surface (bpf, io_uring, open_by_handle_at,
keyctl, setns, module loading...). Cần 1 lớp filter cuối (depth 4:
pivot_root → capdrop → NNP → seccomp) mà không phá busybox httpd/sh.

## Nguồn mới hôm nay (đã đọc thật)

1. **Kernel docs — seccomp_filter.rst (docs.kernel.org, bản 5.x/6.x)**:
   - `prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &prog)` yêu cầu
     NO_NEW_PRIVS (hoặc CAP_SYS_ADMIN) — minict đã set NNP trước unshare,
     filter cài trong child sau capdrop là hợp lệ.
   - Return actions: `SECCOMP_RET_ERRNO` trả errno không thực thi syscall —
     đúng semantic cần cho "blocked nhưng fail visible"; `RET_KILL_THREAD`
     dùng cho arch-mismatch; filter cài xong là không gỡ được (chỉ gỡ khi
     process chết) — phù hợp "last lock".
2. **man7 seccomp(2)**: mục cảnh báo ptrace-based tracer + seccomp (<4.8) →
   chốt đưa ptrace vào blocklist luôn (phòng cả hướng process_vm_readv/writev
   đọc bộ nhớ process khác).
3. **"io_uring and Sandbox Security" (pandastack, 08/2026)**: io_uring thực thi
   opcodes TRONG kernel ring không qua syscall-per-op → seccomp filter nhìn
   không thấy I/O của ring; các operator lớn đi qua io_uring_setup một lần.
   → blocklist io_uring_setup/enter/register là cách chuẩn "disable io_uring"
   khi không dùng (busybox không dùng).
4. **Docker default seccomp profile** (tổng hợp qua SUSE/mrcloudbook):
   Docker cũng chọn blocklist ~50 syscall, default ALLOW — precedent cho
   blocklist thay vì allowlist trong container runtime (chi tiết D11).
5. **gVisor blog "Optimizing seccomp usage" (2024)**: chi phí evaluate cBPF
   đo bằng nanoseconds/syscall; overhead tổng thấy được chỉ khi syscall
   rate cực cao. Đối chiếu đo của mình: +0.2ms/cold-start (~vài nghìn
   syscall lúc spawn) — nhất quán.

## Áp dụng cho minict + kết quả đo

- Blocklist 32 syscall + arg-check clone(CLONE_NEW*) + socket(AF_ALG),
  clone3→ENOSYS (fallback-friendly), default ALLOW, arch-guard
  KILL_THREAD khi không phải AUDIT_ARCH_X86_64.
- Overhead A/B N=40: p50 1.38ms (ON) vs 1.18ms (OFF) = **+0.2ms**, p95 ~0 —
  cách ngưỡng 100ms của PROMPT 70x.
- Test: 17 case mới (tests/test_seccomp.sh) gồm 4 control A/B chứng minh
  EPERM đến từ filter mình, không phải host. Bài học số syscall: D14.


---

# Phiên 5 (SDDA cycle) — netns: CLONE_NEWNET + unix-socket bridge (issue #3)

> Kế hoạch ngày mai của phiên 1-2 + phiên 3/4: netns cho minict, web vẫn phải
> reachable, kèm seccomp chặn netlink (D10). Bằng chứng: journal/2026-11-17/evidence/netns.txt.

## Nguồn mới (≥2, đã đọc thật)

1. **user_namespaces(7) (man7.org)** — "Privileged operations on resources
   governed by the nonuser namespace require that the process has the
   necessary capabilities in the user namespace that owns the nonuser
   namespace" → CAP_NET_ADMIN trong userns tự tạo là ĐỦ để bring-up `lo` của
   netns do chính userns đó sở hữu. Đối chiếu thực tế: đúng, NHƯNG có bẫy —
   uid_map rỗng (unshare --user không map) = unmapped user = KHÔNG có cap
   nào (đo: `ip link set lo up` EPERM trước khi ghi map, OK sau đó) → D18.
2. **network_namespaces(7) (man7.org)** — "A physical network device can
   live in exactly one network namespace" + netns cô lập devices/stacks →
   netns mới chỉ có `lo` (down), không có eth0 → test NET1 assert "no host
   iface inside" bằng ifconfig -a (ioctl path, không cần netlink).
3. **RootlessKit README + docs/port.md (github.com/rootless-containers/
   rootlesskit)** — `--port-driver=builtin`, publish `-p 127.0.0.1:8080:80/tcp`
   (parent proxy port cho child netns; slirp4netns là 1 driver riêng cần
   binary). Precedent kiến trúc cho bridge web của minict (D16): parent
   (host) giữ TCP listener, relaying qua kênh IPC đến child netns.

## Vấn đề / giải pháp đã có / trade-off / áp dụng cho minict

- **Vấn đề:** rootless netns = loopback-only, không có đường ra host
  (veth cần CAP_NET_ADMIN host netns — đã đo phiên 3; slirp4netns/pasta
  không có trong env).
- **Giải pháp đã có:** (a) slirp4netns/pasta — usermode TCP/IP stack,
  nặng, không có sẵn; (b) RootlessKit builtin port driver — parent proxy +
  kênh IPC; (c) veth trong userns — tạo được trong ns mới nhưng không nối
  được ra host netns rootless.
- **Trade-off:** usermode stack = đầy đủ nhất nhưng thêm tầng parse IP;
  unix-socket bridge = chỉ cần AF_UNIX (filesystem-scoped), byte-pump
  đơn giản, nhưng chỉ serve 1 service/1 port-known (đủ cho web httpd).
- **Áp dụng (D16):** cầu 2 chặng qua socket file nằm trên rootfs disk:
  host webproxy.py (TCP 127.0.0.1:port) → unix rootfs/.minict-proxy.sock →
  relay trong minict → httpd 127.0.0.1:port trong netns. Relay fork trước
  capdrop/seccomp, tự capdrop, không exec. Host proxy python asyncio.

## Đo thực nghiệm trong phiên (bổ sung env-facts)

- `unshare --user --net` (util-linux, không --map-root-user): lo-up EPERM
  (uid_map rỗng = không caps) — bẫy D18.
- AF_UNIX connect qua path GẶC NHIÊN hoạt động xuyên netns (socket file trên
  rootfs disk, host connect được) — nền của D16, đo E2E 200 OK.
- socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE) tạo được unprivileged trên
  host env này (khác AF_ALG bị host seccomp chặn sẵn) → dùng được làm A/B
  control cho SC17/SC18.
- GNU Make 4.4.1 (env này) từ chối recipe 8-spaces; runner (make 4.3) thì
  chấp nhận → fix tabs của phiên 4 đã tuột khỏi main, ghim lại trong PR này.

---

# Phiên 8 — Alpine minirootfs làm rootfs distro thật (issue #23, GĐ1 mục 2)

## Vấn đề
busybox rootfs 2 MB là PoC; "Proxmox-lite" cần rootfs distro thật (cài gói,
init chuẩn, /proc đầy đủ). Ràng buộc rootless tuyệt đối: không Docker, không
newuidmap setuid (manifest §5 — chỉ single-uid mapping `0 → uid-thật`).

## Nguồn mới trong phiên

1. **Alpine CDN — v3.22 releases** (dl-cdn.alpinelinux.org/alpine/v3.22/releases/x86_64/):
   `alpine-minirootfs-3.22.8-x86_64.tar.gz` là bản mới nhất (05-Nov-2026), CDN
   phát hành kèm `.sha256` cho từng file → pin version + verify checksum bắt buộc
   (không pin = test không tái lập được; không verify = download bị tin mù quáng).
2. **pid_namespaces(7) (man7)**: "After creating a new PID namespace, it is
   useful for the child to change its root directory and mount a new procfs
   instance at /proc so that tools such as ps(1) operate correctly" — quy tắc
   quyền: mount procfs yêu cầu CAP_SYS_ADMIN **trong userns sở hữu pidns**;
   minict unshare userns+pidns cùng lúc rồi mới mount → sở hữu khớp → hợp lệ
   trên kernel thường. Máy dev kernel 5.10 bị host seccomp chặn mount(2)
   (known gap PROC1); runner ubuntu-24.04 (kernel 6.x) không có filter đó.
3. **"Bootstrapping Alpine Linux without root" (blog.brixit.nl, 03/2024)**:
   bootstrap alpine-base bằng `apk.static --root <dir> --initdb add alpine-base`
   trong userns; vấn đề tương đương minict: mknod EPERM → bind-mount dev nodes,
   /proc qua `unshare --mount-proc`. Khác mấu chốt: bài dùng newuidmap + subuid
   (`--map-users=10000,0,10000`) — **không áp dụng được cho minict** vì manifest
   §5 cấm newuidmap setuid → bài học: cần chiến lược ownership riêng (below).
4. **Apptainer fakeroot docs (apptainer.org/docs/user/main/fakeroot.html)**:
   pattern `FAKEROOTDONTTRYCHOWN=1` — giả lập root bằng cách **không cố chown**;
   cho thấy industry xử lý "unprivileged extract khuyến khích ownership root"
   bằng bỏ qua chown. Với single-uid mapping của minict, host-uid-thật ≡ ns-uid 0
   (root) → file extract với uid thường **hiện là root:root bên trong sandbox**
   = fakeroot tự nhiên, không cần tool thêm (giả lập, trade-off: mọi file "root",
   không phân biệt gói/user nào cài — chấp nhận cho PoC, ghi lại).

## Áp dụng cho minict (D20)

- Pin `ALPINE_VERSION=3.22.8` + verify `.sha256` trước extract; GNU tar non-root
  mặc định `--no-same-owner` → toàn bộ tree thuộc uid 1001 ↔ root trong ns (khớp
  pattern Apptainer, không cần fakeroot).
- `/proc` trong pidns: code minict đã mount proc pre-capdrop (đo phiên 1: host
  seccomp chặn, warn-continue) — test mới phải **env-aware** (dev-host: assert
  fail-soft không crash; runner: assert `/proc/self/status` Pid:1 thật).
- apk offline: netns loopback-only → không `apk add` online trong instance
  (chờ pasta mục 4); đo offline bằng local repo pre-seed 1 gói nhỏ để kiểm
  hành vi chown/extraction trong sandbox thật.
- init: minirootfs có sẵn busybox init; mô hình payload-as-PID1 + shim D19
  đủ cho one-shot/web; init thật (reap vòng đời dài) để GĐ2 exec/daemon — ghi
  rõ để không tự ý mở scope.
