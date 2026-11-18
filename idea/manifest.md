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

## 4. Phạm vi KHÔNG làm (Non-goals)

Lấy cảm hứng từ Proxmox về **trải nghiệm** (UI, template, snapshot, quota),
không sao chép toàn bộ Proxmox:

- Không cluster nhiều node, không HA, không live migration.
- Không Ceph/ZFS/LVM, không quản lý block device. Storage là thư mục của user.
- Không bridge/VLAN/SDN, không IP LAN riêng cho từng instance. Mạng chỉ qua
  port-forward user-space (pasta/slirp4netns). Đây là cái giá của rootless tuyệt đối.
- Không RBAC đa người dùng / đa tenant. Một user vận hành, xác thực bằng token.
- Không Kubernetes/Docker-compatible API.
- Không Windows/macOS. Chỉ Linux đạt nền tảng mục tiêu (§5).
- Không tự viết network proxy khi pasta/slirp4netns giải quyết được. Không tự
  viết gVisor/Wasm runtime; nếu cần thì tích hợp dự án có sẵn.
- Lớp container (userns + seccomp) dành cho workload **tin cậy**. Workload không
  tin cậy phải chạy ở lớp microVM (GĐ4).

## 5. Nền tảng mục tiêu (Target platform)

| Hạng mục | Mức mục tiêu | Lý do |
|---|---|---|
| Linux kernel | ≥ 6.1 | Landlock ổn định, overlayfs rootless + idmapped mounts an toàn (CVE-2023-0386 đã vá), cgroups v2 mặc định |
| cgroups | **v2**, có delegation cho user (systemd user slice) | quota CPU/RAM/pid rootless |
| KVM | `/dev/kvm` đọc/ghi được bởi user (GĐ4) | Firecracker cần, không cần root |
| User-space networking | `pasta` hoặc `slirp4netns` có sẵn | mạng rootless |
| User | uid ≥ 1000, không sudo | nguyên tắc rootless |

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
| Cluster / HA / migration | **không làm** | — | — |
| RBAC nhiều user | **không làm** | — | — |

## 7. Năm giai đoạn (What) và tiêu chí xong từng giai đoạn

### Giai đoạn 1 — minict: runtime container mạnh (HIỆN TẠI)

**Đã xong** (chi tiết trong docs/roadmap.md): userns + mountns + pidns + utsns +
ipcns + netns, pivot_root, capdrop, NNP, PDEATHSIG + pidns teardown, seccomp-bpf
blocklist, busybox httpd trong netns qua unix-socket bridge, 50 test, perf ~2 ms / ~1.1 MB / ~2 MB.

**DoD giai đoạn 1** (xong khi tất cả đúng):
- [ ] **Rootfs distro thật**: Alpine hoặc Debian minirootfs chạy được trong
      instance (có `/proc` trên nền tảng mục tiêu, init tối thiểu, single-uid
      mapping theo §5), không còn chỉ busybox.
- [ ] Stack web thật (nginx/caddy + API + SQLite hoặc Redis) chạy trong một
      instance, host truy cập được.
- [ ] Mạng instance ↔ host/Internet qua **pasta hoặc slirp4netns**; bridge D16 chỉ còn là fallback.
- [ ] Escape suite mở rộng (issue #5): fd-based, /proc/self/root, setuid trong rootfs, core dump leak.
- [ ] Graceful stop (issue #6), vì control plane GĐ2 cần.
- [ ] Perf vẫn trong ngưỡng perf-guard.

### Giai đoạn 2 — Control plane + storage (lõi "Proxmox nhẹ")

**Gate nghiên cứu trước khi viết code:** `exec` vào instance đang chạy rootless
(daemon giữ caps trong userns nó tạo → `setns` từ phía daemon), phải đo được.
→ **Đo được 2026-11-18** (D25, `minict-exec`), cùng lúc owner gộp sớm "seed"
`minictd/` (ngoại lệ §8 bên dưới). Các ô DoD dưới đây vẫn chưa tick.

**DoD giai đoạn 2:**
- [ ] Daemon user-level (systemd `--user` unit) + REST API + CLI `sandbox`:
      create/start/stop/destroy/list/exec cho nhiều instance; state machine
      created/running/stopped; **instance tồn tại qua reboot** (autostart tuỳ chọn).
- [ ] Template: tạo instance từ template; lưu instance thành template.
- [ ] Snapshot/backup/restore từng instance (overlay layer + tar.zst).
- [ ] Port-forward theo instance (pasta), khai báo trong config instance.
- [ ] Quota CPU/RAM/pid qua cgroups v2 delegation; quota disk dạng cảnh báo.
- [ ] Control plane idle ≤ 30 MB RSS; destroy không rò rỉ process/mount/netns.

### Giai đoạn 3 — Web UI + console

**DoD giai đoạn 3:**
- [ ] Web UI single-user: danh sách instance, tạo từ template, start/stop,
      snapshot/restore, port-forward, quota. Mặc định bind 127.0.0.1 + token.
- [ ] Web terminal vào instance đang chạy (qua `exec` của GĐ2).
- [ ] Biểu đồ CPU/RAM/IO theo instance từ thống kê cgroup.
- [ ] UI tĩnh, không framework nặng; tổng RSS control plane + UI vẫn ≤ 30 MB.

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

**Ngoại lệ đã ghi (2026-11-18, owner chốt trên decision card — D27):** code
control-plane `minictd/` (daemon + `minict-exec` + `MINICT_SPEC`) do owner đặt
viết ngoài chu trình được gộp sớm như **GĐ2 seed**. Giai đoạn hiện tại **vẫn là
GĐ1**; với `minictd/` chỉ sửa bảo mật và giữ `make test-minictd` xanh, không mở
việc GĐ2 mới trước khi DoD GĐ1 xong. PR của seed gắn `phase:2`.
