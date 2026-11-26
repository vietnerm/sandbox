# Manifest — sandbox-lab (minict)

> **Phân cấp nguồn sự thật** (chốt từ 2026-11-17, issue #15):
> 1. File này **đặt hướng**, trả lời "vì sao" và "làm cái gì" theo thứ tự giai đoạn.
> 2. `docs/roadmap.md` là **kế hoạch chi tiết** của từng giai đoạn, trả lời "làm gì tiếp theo".
> 3. `PROMPT.MD` là **quy trình vận hành** (SDDA/SBG/SCR/SIA), trỏ vào roadmap, không giữ danh sách ưu tiên riêng.
>
> Khi 3 nguồn mâu thuẫn, nguồn đứng trên thắng.

## 1. Mục tiêu (Why)

Xây dựng một **"Proxmox nhẹ" chạy hoàn toàn rootless**: nền tảng quản lý
container và microVM trên một máy Linux, có API, giao diện web, template,
snapshot/backup, giới hạn tài nguyên. Toàn bộ nền tảng (runtime, daemon, UI,
workload) chạy dưới **một user thường**. Không Docker, không root, không setuid.

"Nhẹ hơn rất nhiều lần" phải đo được (so với Proxmox VE: cần cài lại cả OS,
khuyến nghị tối thiểu 2 GB RAM chỉ cho OS + dịch vụ quản lý):

| Chỉ số | Mục tiêu sandbox-lab |
|---|---|
| Cài đặt | trên Linux sẵn có, không reboot, không root (trừ điều kiện tiên quyết §5) |
| RAM control plane lúc rảnh (daemon + API + UI) | ≤ 30 MB |
| Overhead mỗi container rảnh (ngoài app) | ≤ 5 MB |
| Cold start container | ≤ 100 ms (hiện ~2–5 ms) |
| Cold start microVM | < 1 s |
| Kích thước appliance image (GĐ5) | ≤ 300 MB |

## 2. Người dùng (For whom)

- **Một người vận hành một máy** (máy dev, homelab, VPS): muốn tạo/xoá/snapshot
  nhiều môi trường cách ly qua UI hoặc API như Proxmox, nhưng không có/không
  muốn dùng quyền root.
- **CI/pipeline không đặc quyền**: runner chỉ có uid thường, cần sandbox khởi
  động trong mili-giây.
- **Người đóng gói appliance** (giai đoạn cuối): image tối giản cài lên máy sạch.

## 3. Nguyên tắc bất biến

- **Rootless tuyệt đối** (chốt bởi owner 2026-11-17): runtime, daemon, API, UI
  và mọi workload chạy dưới uid ≥ 1000. Không setuid, không sudo, không helper
  đặc quyền. Một tính năng cần root → bỏ hoặc thay bằng phương án rootless,
  **không** nới nguyên tắc. Hệ quả chấp nhận được ghi ở §4 và bảng §6.
- **Không Docker** (daemon, CLI, containerd, image build).
- **Nhẹ được đo**: các chỉ số §1 + ngưỡng perf-guard; vượt ngưỡng phải giải trình trong PR.
- **Không merge khi CI đỏ**, và CI phải xanh thật (exit code thật, pipefail; issue #14).
- **Bảo mật là ưu tiên số 1** (owner chốt 2026-11-23). Instance minict chia chung
  kernel với host: một lỗ hổng kernel có thể đưa tiến trình thoát ra ngoài, và
  namespace/capdrop/seccomp chỉ thu hẹp chứ không xoá được rủi ro đó. Vì vậy,
  **cho tới khi GĐ4 (microVM) xong, minict/minictd chỉ chạy workload tin cậy**:
  code của chính owner hoặc image đã kiểm. Mọi tài liệu, API và UI không được
  quảng cáo minict là ranh giới an toàn cho code lạ. Khi một việc bảo mật và một
  tính năng tranh nhau thứ tự trong cùng giai đoạn, việc bảo mật làm trước.
- **Mặc định tối thiểu** (owner chốt 2026-11-26, D74): mọi tính năng nâng cao
  phải **bật thủ công** (cờ, file cấu hình, công cụ riêng), không tự bật. Một thay
  đổi làm tăng RAM/RSS mặc định của runtime hoặc daemon thêm ≥ 1 MB khi tính năng
  đó **không** được dùng → xem lại thiết kế, ghi giải trình trong PR.
- **Core nhỏ, phần còn lại là lớp ngoài** (D74). "Core" là `minict` (runtime C) +
  `minictd` (daemon một máy) + console tĩnh. Trước khi thêm tính năng, trả lời
  hai câu và ghi vào PR/issue:
  1. *"Thêm cái này, minict/minictd còn nhẹ không?"* Không → làm thành tuỳ chọn
     tắt mặc định, hoặc đưa ra lớp ngoài.
  2. *"Bỏ cái này đi, core còn hữu dụng không?"* Còn → **không** đưa vào core; viết
     thành công cụ riêng, lớp client gọi API, hoặc ví dụ trong `examples/` (§9).
- **Không phát minh lại** (D74): không tự viết DB, đồng thuận phân tán, mạng
  overlay, registry phức tạp hay nền tảng deploy. Tái dùng công cụ có sẵn (SSH,
  rsync, web server tĩnh, Ansible, GitHub Actions, Prometheus...). Nếu một tính
  năng cần etcd/Raft/DB server để chạy thì đó là dấu hiệu sai hướng.
- **Trạng thái tính năng phải ghi rõ** (D74): README và tài liệu từng thành phần
  gắn nhãn cho mỗi tính năng: **sẵn sàng** (có test e2e trên CI xanh), **thử
  nghiệm** (chạy được nhưng thiếu test/env-gated/có residual đã biết), **kế hoạch**
  (chưa có code). Không để trạng thái mơ hồ; nhãn đổi cùng PR làm đổi trạng thái.

## 4. Phạm vi KHÔNG làm (Non-goals)

Lấy cảm hứng từ Proxmox về **trải nghiệm** (UI, template, snapshot, quota),
không sao chép toàn bộ Proxmox:

- Không cluster **trong core** (D74 sửa 2026-11-26). Cluster nhiều VPS là mục tiêu
  của owner, nhưng nằm ở **lớp riêng** gọi API `minictd` của từng máy (§9); `minict`
  và `minictd` vẫn là phần mềm một máy, không biết gì về node khác. Không HA tự
  động, không live migration, không đồng thuận phân tán (etcd/Raft) ở bất kỳ lớp nào.
- Không Ceph/ZFS/LVM, không quản lý block device. Storage là thư mục của user.
- Không bridge/VLAN/SDN, không IP LAN riêng cho từng instance. Mạng chỉ qua
  port-forward user-space (pasta/slirp4netns). Đây là cái giá của rootless tuyệt đối.
- Không RBAC đa người dùng / đa tenant **trong core**. `minictd` một user vận hành,
  xác thực bằng token. Phân quyền nhiều người (nếu làm) chỉ nằm ở lớp cluster §9,
  tuỳ chọn, tắt mặc định.
- Không Kubernetes/Docker-compatible API.
- Không Windows/macOS. Chỉ Linux đạt nền tảng mục tiêu (§5).
- Không tự viết network proxy khi pasta/slirp4netns giải quyết được. Không tự
  viết gVisor/Wasm runtime; nếu cần thì tích hợp dự án có sẵn.
- Lớp container (userns + seccomp) dành cho workload **tin cậy**. Workload không
  tin cậy phải chạy ở lớp microVM (GĐ4).

## 5. Nền tảng mục tiêu (Target platform)

| Hạng mục | Mức mục tiêu | Lý do |
|---|---|---|
| Linux kernel | ≥ 6.1, ở patch level LTS mới (tối thiểu 2026-11-23: 6.1.190 / 6.6.159 / 6.12.113 — `docs/cve-watchlist.md`) | Landlock ổn định, overlayfs rootless + idmapped mounts an toàn (CVE-2023-0386 đã vá), cgroups v2 mặc định |
| cgroups | **v2**, có delegation cho user (systemd user slice) | quota CPU/RAM/pid rootless |
| KVM | `/dev/kvm` đọc/ghi được bởi user (GĐ4) | Firecracker cần, không cần root |
| User-space networking | `pasta` hoặc `slirp4netns` có sẵn | mạng rootless |
| User | uid ≥ 1000, không sudo | nguyên tắc rootless |

**Không yêu cầu kernel đặc biệt** (D74): chạy trên kernel ≥ 6.1 LTS mà distro phổ
biến (Debian, Ubuntu, Alpine, Fedora) cung cấp. Không patch kernel riêng, không
module ngoài cây, không cờ boot đặc biệt. Tính năng cần thứ đó → tuỳ chọn có kiểm
tra lúc chạy và báo rõ khi thiếu, không phải yêu cầu bắt buộc.

**Điều kiện tiên quyết một lần của máy** (do người quản trị máy làm, nằm ngoài
nền tảng, giống việc cài kernel): cài `pasta`/`slirp4netns`, bật cgroup
delegation cho user, cấp quyền `/dev/kvm` (nhóm `kvm`). Nền tảng không tự làm
các việc này và không yêu cầu root lúc chạy.

**Câu hỏi mở cần chốt trước GĐ2:** distro thật (Debian/Alpine) trong instance
thường cần nhiều uid (vd. user `_apt`). Ánh xạ nhiều uid rootless đòi hỏi
`newuidmap`, mà đó là binary setuid của shadow-utils, nên vi phạm §3. Phương án
mặc định: single-uid mapping. Mọi file thuộc "root" trong instance, các thao tác
`chown` sang uid khác được giả lập hoặc bỏ qua. Cần đo xem Alpine/Debian có chạy
được nginx/apt theo cách này không (GĐ1, mục rootfs distro).

**Môi trường dev hiện tại** (máy ZAI: kernel 5.10, cgroups v1, không KVM) chỉ
để test nhanh và benchmark. Tính năng bị chặn vì máy dev được gán
`blocked-on-dev-env`, không phải parked.

## 6. So sánh tính năng với Proxmox

| Tính năng Proxmox | Tương đương trong sandbox-lab | Giới hạn do rootless | GĐ |
|---|---|---|---|
| LXC container | instance minict | single-uid mapping (§5) | 1–2 |
| KVM VM | microVM Firecracker | cần quyền `/dev/kvm` | 4 |
| Template / image | rootfs template (Alpine/Debian minirootfs, tar) | — | 2 |
| Storage ZFS/LVM/Ceph | thư mục trong `$HOME`, overlayfs rootless | không block device, không thin-provision | 2 |
| Snapshot / backup / restore | overlay layer + `tar.zst` | snapshot không "nóng" như ZFS | 2 |
| Bridge, VLAN, firewall | port-forward pasta theo instance; chỉ mở port khai báo | không IP LAN riêng, không VLAN | 2 |
| Quota CPU/RAM/disk | cgroups v2 delegation; disk qua kích thước thư mục | disk quota chỉ cảnh báo | 2 |
| Web UI | UI single-user, bind localhost + token | — | 3 |
| Console (noVNC/xterm.js) | web terminal qua `exec` | — | 3 |
| Biểu đồ tài nguyên | thống kê cgroup theo instance | — | 3 |
| Cluster nhiều node | lớp cluster riêng gọi API `minictd` từng VPS (§9) | không HA, không migration | mở rộng |
| HA / live migration | **không làm** | — | — |
| RBAC nhiều user | chỉ ở lớp cluster, tuỳ chọn (§9) | core vẫn một user + token | mở rộng |

## 7. Năm giai đoạn (What) và tiêu chí xong từng giai đoạn

### Giai đoạn 1 — minict: runtime container mạnh (XONG 2026-11-20)

**Đã xong** (chi tiết trong docs/roadmap.md): userns + mountns + pidns + utsns +
ipcns + netns, pivot_root, capdrop, NNP, PDEATHSIG + pidns teardown, seccomp-bpf
blocklist, busybox httpd trong netns qua unix-socket bridge, rootfs Alpine + Debian,
pasta egress + DNS + port-forward, web stack nginx + CGI + SQLite trong một instance;
`make test` 10 suite (2026-11-23), perf ~2 ms / ~1.2 MB / ~2 MB.

**DoD giai đoạn 1** (tick lại theo thực tế 2026-11-23 — 6/6; owner chốt chuyển GĐ2 ngày 2026-11-23):
- [x] **Rootfs distro thật**: Alpine hoặc Debian minirootfs chạy được trong
      instance (có `/proc` trên nền tảng mục tiêu, init tối thiểu, single-uid
      mapping theo §5), không còn chỉ busybox.
      → Alpine 3.22.8 chạy được + trọn isolation suite trên alpine (#24, #54);
      `/proc` trong pidns đã sửa (#64, D39 — PROC1–4/AL4 xanh trên runner kernel 7.0;
      owner đã gắn `verified` cho #31); Debian bookworm-slim (#88, D46, DB1–DB8).
- [x] Stack web thật (nginx/caddy + API + SQLite hoặc Redis) chạy trong một
      instance, host truy cập được. → nginx + fcgiwrap + CGI + SQLite trên alpine,
      `./sandbox.sh up|down|bench`, cold start median 127 ms, ST1–ST7 (#85, D45).
      Residual (không chặn GĐ2): caddy, API production-grade, đo RAM idle stack.
- [x] Mạng instance ↔ host/Internet qua **pasta hoặc slirp4netns**; bridge D16 chỉ còn là fallback.
      → egress + port-forward qua pasta (#57/#59), DNS `--dns-forward` (#74, D42),
      `apk add` online (#79, D43), `apt` online (#88, D46).
- [x] Escape suite mở rộng (issue #5): fd-based, /proc/self/root, setuid trong rootfs, core dump leak.
      → #48 + #54 (D36); owner đã gắn `verified` cho #5.
- [x] Graceful stop (issue #6), vì control plane GĐ2 cần. → pidns-1 signal shim (#21, D19).
- [x] Perf vẫn trong ngưỡng perf-guard. → perf-check PASS trên runner mỗi PR.

### Giai đoạn 2 — Control plane + storage (lõi "Proxmox nhẹ") (từ 2026-11-23; roadmap Phiên 34 ghi DoD đạt)

**Gate nghiên cứu trước khi viết code:** `exec` vào instance đang chạy rootless
(daemon giữ caps trong userns nó tạo → `setns` từ phía daemon), phải đo được.
→ **Đo được 2026-11-18** (D25, `minict-exec`), cùng lúc owner gộp sớm "seed"
`minictd/` (2026-11-18, D27). Tiến độ thực tế ghi dưới từng ô; ô chỉ tick khi
xong trọn vẹn.

**DoD giai đoạn 2:**
- [ ] Daemon user-level (systemd `--user` unit) + REST API + CLI `sandbox`:
      create/start/stop/destroy/list/exec cho nhiều instance; state machine
      created/running/stopped; **instance tồn tại qua reboot** (autostart tuỳ chọn).
      → CLI `sandbox` + systemd `--user` unit + `--autostart` + reconcile (#92, D47);
      còn thiếu: unit chạy thật dưới user manager (lingering) trên host có systemd.
- [x] Template: tạo instance từ template; lưu instance thành template.
      → `save-template` + `template:<name>` (#128, D56); residual: list/remove template.
- [ ] **Seccomp allowlist theo template** (#29, thêm 2026-11-23, ưu tiên bảo mật):
      mỗi template có profile allowlist riêng, blocklist D11 giữ làm mặc định khi
      template chưa có profile; mỗi profile có test "workload chạy được" + test
      "syscall ngoài danh sách bị chặn".
- [ ] Snapshot/backup/restore từng instance (overlay layer + tar.zst).
- [ ] Port-forward theo instance (pasta), khai báo trong config instance.
- [ ] Quota CPU/RAM/pid qua cgroups v2 delegation; quota disk dạng cảnh báo.
      → CPU/RAM/pid xong (#95, D48, CG0–CG5); còn thiếu cảnh báo quota disk.
- [ ] Observability (thêm 2026-11-23, owner duyệt): log JSON có cấu trúc cho mọi
      sự kiện vòng đời instance + lỗi trong `minictd`; metrics từng instance
      (CPU/RAM/IO/pids đọc từ cgroups v2) qua endpoint `/metrics` dạng Prometheus
      text, **chỉ bind 127.0.0.1/unix socket**, cùng token của API. Chỉ dùng Go
      stdlib (không OpenTelemetry SDK, không thư viện Prometheus client) để giữ
      ngân sách RSS của mục dưới. Chi tiết + test: roadmap GĐ2 mục 7.
- [ ] Control plane idle ≤ 30 MB RSS; destroy không rò rỉ process/mount/netns.

### Giai đoạn 3 — Web UI + console (HIỆN TẠI từ 2026-11-26)

**DoD giai đoạn 3:**
- [ ] Web UI single-user: danh sách instance, tạo từ template, start/stop,
      snapshot/restore, port-forward, quota. Mặc định bind 127.0.0.1 + token.
- [ ] Web terminal vào instance đang chạy (qua `exec` của GĐ2).
- [ ] Biểu đồ CPU/RAM/IO theo instance, **dùng lại `/metrics` của GĐ2** (không thu
      thập riêng lần hai).
- [ ] UI tĩnh (file build sẵn do `minictd` phục vụ, **không tiến trình runtime
      riêng**); framework chỉ được dùng lúc build (owner chốt 2026-11-26: template
      Next.js xuất tĩnh, `console/`); tổng RSS control plane + UI vẫn ≤ 30 MB.

### Giai đoạn 4 — Backend microVM (Firecracker)

Hai mức cách ly trên cùng control plane: container nhẹ cho workload tin cậy,
microVM cho workload không tin cậy.

**DoD giai đoạn 4:**
- [ ] **Precondition:** kiểm `/dev/kvm` trên runner GitHub và máy đích; không có
      thì GĐ4 chỉ ở mức research, ghi rõ trong roadmap.
- [ ] Firecracker chạy dưới user thường (quyền `/dev/kvm`, không root).
- [ ] Cùng API/UI cho cả hai backend (`--backend minict|firecracker`).
- [ ] Startup microVM < 1 s; benchmark so với minict.

### Giai đoạn 5 — Appliance OS

**DoD giai đoạn 5:**
- [ ] Image tối giản (NixOS hoặc distro nhẹ) ≤ 300 MB, boot vào một user thường
      đã có sẵn điều kiện tiên quyết §5. Control plane + UI tự chạy dưới user đó.
- [ ] Health check + cơ chế cập nhật tối thiểu.
- [ ] Tài liệu từ máy trắng đến appliance chạy web stack.

## 8. Thứ tự thực thi

GĐ1 → GĐ2 → GĐ3 → GĐ4 → GĐ5. Không nhảy giai đoạn; nếu buộc phải nhảy (vd. KVM
có sẵn sớm), cập nhật manifest + roadmap **trước** khi code. Mỗi issue/PR gắn
đúng một giai đoạn (label `phase:N`).

**Lịch sử chuyển giai đoạn:**
- 2026-11-18 (D27): code control-plane `minictd/` do owner đặt viết ngoài chu trình
  được gộp sớm như GĐ2 seed; khi đó GĐ1 vẫn là giai đoạn hiện tại, `minictd/` chỉ
  sửa bảo mật và giữ `make test-minictd` xanh.
- 2026-11-23: DoD GĐ1 đạt 6/6; owner chốt **GĐ2 là giai đoạn hiện tại**. Ngoại lệ
  D27 hết hiệu lực, việc GĐ2 trên `minictd/` theo thứ tự trong `docs/roadmap.md`.
  GĐ3 (Web UI) vẫn phải dừng và hỏi owner trước khi mở code.
- 2026-11-26: owner mở **GĐ3** và giao template Next.js cho Web UI; chốt phương án
  xuất tĩnh do `minictd --ui-dir` phục vụ (sửa dòng DoD "không framework nặng").
- 2026-11-26: GĐ3 DoD đạt (Phiên 37); owner chỉ thị mở **GĐ4**. Precondition check
  (Phiên 38 #191): no /dev/kvm on dev + CI standard runner → GĐ4 research-only cho
  đến khi có KVM-capable host.
- 2026-11-26 (D74): owner duyệt đưa góp ý review ngoài vào hướng dự án: nguyên tắc
  "mặc định tối thiểu / core nhỏ / không phát minh lại / nhãn trạng thái" (§3), kernel
  distro không patch (§5), và **track Mở rộng** §9 (stack.yaml, registry template
  tĩnh, lớp cluster nhiều VPS). Non-goal "không cluster" §4 đổi thành "không cluster
  trong core".

## 9. Track Mở rộng — lớp ngoài core (từ 2026-11-26, D74)

Các việc dưới đây **không** phải giai đoạn mới và **không** vào core. Mỗi việc là
lớp client/công cụ riêng gọi API `minictd` đã có, hoặc chỉ là quy ước file tĩnh.
Core không được thêm phụ thuộc hay tiến trình nền vì chúng. Thứ tự chi tiết ở
`docs/roadmap.md` mục "Track M".

**Khi nào làm:** khi mọi mục còn lại của giai đoạn hiện tại bị chặn (môi trường,
vd. GĐ4 thiếu `/dev/kvm`, hoặc chờ owner), SDDA được lấy mục thấp nhất chưa xong
của Track M. Việc bảo mật (§3) vẫn chen trước.

1. **Nhãn trạng thái tài liệu** (§3): README + `minictd/README.md` + console docs.
2. **`stack.yaml`**: định dạng nhiều instance tối giản kiểu compose, **chỉ** gồm cái
   `minictd` làm được (image/template, cmd, env, port pasta, quota, mount thư mục,
   phụ thuộc khởi động + chờ sẵn sàng). CLI `sandbox stack up|down|ps` đọc file và
   gọi API; daemon không parse YAML. File đẩy lên Git để cả nhóm chạy giống nhau.
3. **Registry template tĩnh**: registry = một thư mục trên web server tĩnh bất kỳ
   (HTTPS), chứa `index.json` + `tar.zst` + sha256. `sandbox template pull` tải qua
   HTTPS và **kiểm sha256 trước khi giải nén** (fail-closed); `push` chỉ sinh file
   tĩnh để người dùng tự upload (rsync/scp). Không backend, không DB, tự host trong
   5 phút. Template kéo về vẫn là workload **tin cậy** theo §3.
4. **Lớp cluster nhiều VPS** (mục tiêu owner, như Proxmox): công cụ riêng giữ danh
   sách node + token, gọi API `minictd` từng VPS. `minictd` vẫn chỉ bind 127.0.0.1;
   lớp cluster tới node qua SSH tunnel hoặc WireGuard do người dùng dựng, không mở
   port API ra Internet. Không etcd/Raft, không HA, không migration; state nằm ở
   từng node, cluster chỉ là "nhiều tab" có tổng hợp. Tuỳ chọn về sau: phân quyền 3
   vai (owner/editor/viewer) + audit log (ai/lúc nào/lệnh gì). **Phải có tài liệu
   thiết kế được owner duyệt trước khi viết code.**
5. **Triển khai lên VPS không phải nền tảng riêng**: cung cấp API ổn định + ví dụ
   Ansible playbook / GitHub Action trong `examples/`; người dùng chọn công cụ.

