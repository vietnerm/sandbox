# Manifest — sandbox-lab (minict)

> **Phân cấp nguồn sự thật** (chốt từ 2026-11-17, issue #15):
> 1. File này **đặt hướng** — trả lời "vì sao" và "làm cái gì" theo thứ tự giai đoạn.
> 2. `docs/roadmap.md` là **kế hoạch chi tiết** của từng giai đoạn — trả lời "làm gì tiếp theo".
> 3. `PROMPT.MD` là **quy trình vận hành** (SDDA/SBG/SCR/SIA) — trỏ vào roadmap, không giữ danh sách ưu tiên riêng.
>
> Bất kỳ mâu thuẫn nào giữa 3 nguồn → phân cấp trên là thưa kiện cuối cùng.

## 1. Mục tiêu (Why)

Xây dựng **nền tảng sandbox rootless nhẹ** để chạy dịch vụ web tin cậy trên máy
thường (máy dev, máy không đặc quyền), **không Docker, không root** — chỉ dùng
cơ chế kernel Linux có sẵn (user namespace, mount/pid/uts/ipc/net namespace,
pivot_root, seccomp-bpf, cgroups v2).

Đây KHÔNG phải tái tạo Proxmox. Mỗi giai đoạn chỉ mở đúng một lớp mới trên lớp
đã xong, và lớp cũ phải vẫn xanh test + perf.

## 2. Người dùng (For whom)

- **Dev chạy môi trường web cục bộ**: muốn up/down một stack web cách ly trong
  1 giây, không cần Docker Desktop, không sudo.
- **CI/pipeline không đặc quyền**: runner chỉ có uid thường — sandbox phải chạy
  được rootless, khởi động trong mili-giây (minict: ~2 ms).
- **Ai đóng gói phần mềm thành appliance** (giai đoạn cuối): image OS tối giản
  cài lên máy sạch, vận hành như một thiết bị.

## 3. Nguyên tắc bất biến

- **Rootless bắt buộc**: mọi tính năng phải chạy dưới uid ≥ 1000, không setuid,
  không sudo. Vi phạm = bỏ tính năng, không phải bỏ nguyên tắc.
- **Không Docker** (daemon, CLI, containerd, image build).
- **Nhẹ được đo**: cold start, RSS, disk có ngưỡng perf-guard; vượt ngưỡng phải
  giải trình trong PR.
- **Không merge khi CI đỏ** — và CI phải xanh THẬT (exit code thật, có
  pipefail — bài học run #20, issue #14).

## 4. Phạm vi KHÔNG làm (Non-goals)

- Không tái tạo Proxmox / Kubernetes / Docker-compatible API.
- Không daemon đa tenant đa người dùng (single-user rootless là đủ cho use-case).
- Không hỗ trợ Windows/macOS — chỉ Linux kernel ≥ nền tảng mục tiêu (§5).
- Không tự viết network proxy/thunk khi `pasta`/`slirp4netns` (dự án user-space
  networking chuẩn của podman) giải quyết được — dùng sẵn, chỉ tự làm khi thiếu
  tính năng được chứng minh.
- Không tự viết gVisor/Wasm runtime — nếu cần lớp isolation khác, tích hợp dự án
  có sẵn (Firecracker, Wasmtime).
- Không chạy code untrusted kiểu multi-tenant cloud trên lớp container — mức cách
  ly của userns+seccomp là cho workload TIN CẬY (dev/CI); untrusted thật phải
  dùng lớp microVM (giai đoạn 3).

## 5. Nền tảng mục tiêu (Target platform)

| Hạng mục | Mức mục tiêu | Lý do |
|---|---|---|
| Linux kernel | ≥ 6.1 | Landlock đã ổn định (≥ 5.13), overlayfs + idmapped mounts an toàn (CVE-2023-0386 vá từ 5.11/5.15), cgroups v2 unified mặc định |
| cgroups | **v2** | delegation rootless-friendly (v1 không) |
| KVM | `/dev/kvm` có mặt (cho giai đoạn 3) | runner GitHub thường có — phải KIỂM TRA trước khi lên kế hoạch chi tiết GĐ3 |
| User | uid ≥ 1000, không sudo | nguyên tắc rootless |

**Môi trường dev hiện tại** (máy ZAI: kernel 5.10, cgroups v1, không KVM) chỉ là
nơi chạy test nhanh + benchmark so sánh. Một tính năng bị chặn vì máy dev KHÔNG
được coi là "parked" — được gán `blocked-on-dev-env` và xếp lại khi nền tảng mục
tiêu đáp ứng (runner CI đã chạy kernel 6.x + cgroups v2).

## 6. Bốn giai đoạn (What) — và tiêu chí xong từng giai đoạn

### Giai đoạn 1 — minict: sandbox runtime mạnh (HIỆN TẠI)

Biến minict thành runtime sandbox nhẹ + bảo mật cho web service tin cậy.

**Đã xong** (chi tiết trong docs/roadmap.md): userns+mountns+pidns+utsns+ipcns+
netns, pivot_root, capdrop, NNP, PDEATHSIG+pidns teardown, seccomp-bpf blocklist
(32 syscall + arg-check), busybox httpd trong netns riêng qua unix-socket bridge,
50 test (isolation 17 + seccomp 20 + netns 7 + web 6), perf 1.8–2.0 ms / ~1.1 MB / ~2 MB.

**Definition of Done của giai đoạn 1** — xong khi tất cả đúng:
- [ ] Stack web thật (nginx/caddy + API Node/Python/Go + SQLite hoặc Redis) chạy
      bên trong MỘT instance, host truy cập được — không còn dùng busybox httpd làm đích.
- [ ] `./sandbox up|down|bench` một lệnh cho cả stack; cold start toàn stack < 5s; RAM idle < 300 MB.
- [ ] Escape/isolation suite pass (17 + 20 + 7) + mở rộng chroot-escape sâu (issue #5):
      fd-based, /proc/self/root, setuid trong rootfs (NNP chặn), core dump leak.
- [ ] Connectivity instance ↔ host/Internet qua **pasta hoặc slirp4netns** (đánh giá
      trước; bridge D16 unix-socket chỉ còn là fallback cho loopback-only).
- [ ] Perf vẫn trong ngưỡng perf-guard sau mọi thay đổi trên.

Không nằm trong DoD GĐ1 (chỉ là nền, không đếm): web tĩnh busybox, seccomp
blocklist, netns loopback-only — đã xong, là điều kiện vào chứ không phải đích.

### Giai đoạn 2 — Control plane tối giản

Daemon + API quản lý vòng đời nhiều instance minict (tạo, khởi động, dừng, xoá,
theo dõi tài nguyên). Đây là "bộ não" điều khiển sandbox.

**Điều kiện nghiên cứu TRƯỚC khi bắt đầu** (chốt theo owner 2026-11-17):
- [ ] Research **exec vào instance đang chạy rootless**: `setns()` cần CAP_SYS_ADMIN
      trong userns của instance — process ngoài không đủ caps (README đã ghi "exec
      tạo ns set mới, không setns"). Phương án chuẩn để đo đạc: daemon tạo instance
      là process fork/unshare trực tiếp nên GIỮ full caps trong userns nó tạo →
      setns được từ phía daemon (mô hình RootlessKit/podman exec). Phải có phương
      án được đo đạc trước mới mở code GĐ2.

**Definition of Done của giai đoạn 2**:
- [ ] ≥ 2 instance chạy đồng thời, quản lý được qua API một lệnh (up/down/ps/exec).
- [ ] `exec` vào instance đang chạy nhìn thấy đúng process tree + network của
      instance đó (không còn "mỗi exec một netns riêng").
- [ ] Resource limits (CPU/RAM/pid) áp được qua **cgroups v2 delegation** trên
      nền tảng mục tiêu (kernel ≥ 6.1); trên máy dev cgroups v1 chỉ cảnh báo.
- [ ] Mỗi instance có state rõ ràng (created/running/stopped) và cleanup chắc chắn
      không rò rỉ process/mount/netns sau down.

### Giai đoạn 3 — Đa backend: thêm microVM (Firecracker)

Hai mức cách ly trên cùng control plane: container nhẹ (minict) và microVM mạnh
(Firecracker) — dùng lại mô hình các dự án hiện đại.

**Definition of Done của giai đoạn 3**:
- [ ] Firecracker boot được microVM rootless-quasi (kvm group) trên nền tảng mục tiêu
      — **precondition: kiểm tra `/dev/kvm` trên runner GitHub + máy đích**, nếu
      không có thì GĐ3 giữ trạng thái research-only và ghi rõ trong roadmap.
- [ ] Cùng API một lệnh điều khiển cả hai backend (`sandbox run --backend minict|firecracker`).
- [ ] Startup microVM < 1s cho workload nhỏ; đo và ghi benchmark so với minict.

### Giai đoạn 4 — Appliance OS

Đóng gói toàn bộ stack thành image HĐH tối giản (NixOS hoặc distro nhẹ) — cài đặt
và vận hành như một thiết bị.

**Definition of Done của giai đoạn 4**:
- [ ] Boot được trên máy sạch (live USB/QEMU) vào control plane + runtime sẵn sàng.
- [ ] Update/optimize: image có cơ chế cập nhật tối thiểu và biết tự báo health.
- [ ] Tài liệu triển khai từ zero đến một appliance chạy web stack.

## 7. Thứ tự thực thi

GĐ1 → GĐ2 → GĐ3 → GĐ4. Không nhảy giai đoạn; nếu có lý do buộc phải nhảy
(vd. KVM có sẵn sớm), phải cập nhật lại manifest + roadmap TRƯỚC khi code.
Mọi issue/PR phải gắn đúng một giai đoạn (label `phase:N`) để SDDA chọn việc
theo `docs/roadmap.md`, không chọn theo ý thích.
