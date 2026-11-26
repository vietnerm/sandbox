# Research — GĐ4 precondition: /dev/kvm + Firecracker rootless + --backend design

## Ngày: 2026-11-26 | Phiên: 38 | Issue: #191

## Nguồn 1 — Firecracker design + KVM requirement
- URL: https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md + getting-started.md
- Tóm tắt:
  - Firecracker là VMM (virtual machine monitor) nhẹ, tạo microVM với vCPU (≤64) + memory tùy ý.
  - **Yêu cầu HARD: `/dev/kvm` read/write** — KVM kernel module (kvm_intel hoặc kvm_amd) phải loaded.
  - AWS EC2: chỉ `.metal` instances (bare metal) có KVM — virtualized instances KHÔNG có nested KVM.
  - Throughput: 5 microVMs/host core/s (180/s trên 36 cores) — rất nhẹ.
  - Mỗi microVM có HTTP API server (unix socket) để configure + control.
- Áp dụng minict: **/dev/kvm là blocker** — dev machine (cloud container, no vmx/svm) + GitHub runner (ubuntu-24.04 standard, no nested KVM) đều KHÔNG có /dev/kvm. GĐ4 ở mức **research-only** cho đến khi có máy KVM-capable.

## Nguồn 2 — Firecracker jailer (rootless path)
- URL: https://github.com/firecracker-microvm/firecracker/blob/main/docs/jailer.md
- Tóm tắt:
  - Jailer: chương trình isolate Firecracker process — `--uid <uid> --gid <gid>` (drop privileges), `--chroot-base-dir`, `--cgroup-version`, `--netns`, `--new-pid-ns`.
  - Chạy Firecracker dưới user thường: jailer sets uid/gid → Firecracker process runs as that user.
  - /dev/kvm phải accessible cho user đó (chmod 0666 hoặc group kvm).
  - Network: jailer tạo netns → TAP device trong netns → Firecracker connects TAP.
- Áp dụng minict: rootless Firecracker = jailer + /dev/kvm 0666 + TAP trong netns. Tương tự pattern minict (userns + netns + pasta). Nhưng TAP cần CAP_NET_ADMIN → cần userns với netns hoặc pasta-style slirp.

## Nguồn 3 — Firecracker rootfs + kernel requirements
- URL: https://github.com/firecracker-microvm/firecracker/blob/main/docs/getting-started.md
- Tóm tắt:
  - **Kernel**: uncompressed vmlinux binary (x86_64 hoặc aarch64). AWS cung cấp CI artifacts (S3).
  - **Rootfs**: ext4 filesystem image (raw file, không directory). Phải chứa `/sbin/init` hoặc tương tự.
  - **Drives**: rootfs as read-only hoặc read-write drive (PUT /drives/rootfs).
  - **Boot source**: PUT /boot-source với `kernel_image_path` + `boot_args` (console=ttyS0).
  - MicroVM boot: PUT /actions with `action_type: "InstanceStart"`.
- Áp dụng minict: rootfs hiện tại (busybox directory) không dùng trực tiếp được — cần convert sang ext4 image. Hoặc build riêng: `mkfs.ext4` + copy rootfs vào. Kernel: fetch vmlinux từ S3 (pinned+sha256, pattern D37).

## Nguồn 4 — Firecracker API (unix socket HTTP)
- URL: https://github.com/firecracker-microvm/firecracker/blob/main/docs/api_requests.md + SPECIFICATION.md
- Tóm tắt:
  - API: unix socket, HTTP PUT/GET.
  - `PUT /boot-source` — kernel path + boot args.
  - `PUT /drives/{drive_id}` — path + read_only + root_device.
  - `PUT /machine-config` — vcpu_count + mem_size_mib.
  - `PUT /network-interfaces/{iface_id}` — host_dev_name (TAP) + guest_mac.
  - `PUT /actions` — InstanceStart, InstanceHalt, CtrlAltDel.
  - `PUT /logger` — unix socket log path.
  - `PUT /mmds` — metadata service.
- Áp dụng minict: daemon quản lý Firecracker API socket per microVM. Go stdlib net/httpunix có thể gọi PUT qua unix socket. Same RunSpec → map: spec.CPUs → vcpu_count, spec.Memory → mem_size_mib, spec.Image → rootfs drive.

## Nguồn 5 — /dev/kvm trên GitHub Actions runner
- URL: https://github.com/actions/runner-images (ubuntu-24.04 image spec)
- Tóm tắt:
  - Standard GitHub-hosted ubuntu-24.04 runner: runs inside Hyper-V VM → **NO nested KVM** (no /dev/kvm).
  - GitHub "larger runner" (cores=16+) with nested virtualization enabled: có /dev/kvm.
  - Self-hosted runner trên bare metal: có /dev/kvm (nếu CPU hỗ trợ + kvm module loaded).
  - GitHub Actions không officially support /dev/kvm trên standard runners.
- Áp dụng minict: CI cho GĐ4 cần self-hosted runner hoặc GitHub larger runner with nested virt. Standard ubuntu-24.04 → SKIP Firecracker tests (env-gated như overlay/pasta).

## --backend design (minictd)

### Flag
`--backend minict|firecracker` (default: minict). Store.Backend field.

### RunSpec (contract, additive)
```go
type RunSpec struct {
    ...existing fields...
    Backend string `json:"backend,omitempty"` // "minict" (default) | "firecracker"
}
```
Console adapter maps `backend` from the create form.

### minict backend (existing)
- userns + pidns + mountns + seccomp + capdrop + minict shim.
- Rootfs: directory (copyTree / overlay).
- Terminal: PTY via minict-exec setns.

### firecracker backend (new, GĐ4)
- **Prerequisites**: /dev/kvm accessible (0666 or group kvm), Firecracker binary (fetched pinned+sha256), vmlinux kernel (fetched), rootfs ext4 image (built from existing rootfs).
- **Run/Start**:
  1. Prepare rootfs: `mkfs.ext4` + copy rootfs tree vào image (hoặc overlay ext4).
  2. Create TAP device in netns (or pasta slirp for egress).
  3. Spawn Firecracker (via jailer for rootless: uid/gid/chroot/netns).
  4. Configure via API socket: boot-source (kernel), drives (rootfs), machine-config (vcpu/mem), network-interfaces (TAP).
  5. InstanceStart.
- **Stop**: CtrlAltDel or InstanceHalt via API → kill process.
- **Remove**: kill + cleanup TAP + rootfs image + API socket.
- **Exec**: Firecracker không có exec (it's a VM) → serial console (PUT /logger + ttyS0) hoặc SSH vào guest.
- **Terminal**: serial console over unix socket (xterm reads serial output, writes serial input).
- **Stats**: Firecracker API GET /machine-config + memory metrics (balloon device).

### Architecture
```
minictd (--backend firecracker)
  ├─ firecracker binary (tools/fetch-firecracker.sh, pinned+sha256)
  ├─ jailer (rootless: uid/gid/chroot)
  ├─ vmlinux kernel (tools/fetch-vmlinux.sh, pinned+sha256)
  ├─ rootfs ext4 image (from existing rootfs: mkfs.ext4 + copy)
  ├─ TAP device (in netns, or pasta for egress)
  └─ API socket (unix, per microVM)
```

### Quota
- Daemon RSS: +~5 MB (Firecracker process management code, no heavy deps).
- Per-microVM: Firecracker process ~10-20 MB RSS (VMM itself) + guest memory (128 MB+).
- microVM startup: <125 ms (Firecracker measured, vs minict ~2 ms — khác biệt lớn).

## Kết luận
- **GĐ4 là research-only** trên infra hiện tại (no /dev/kvm dev + CI).
- **Design hoàn chỉnh**: --backend flag, RunSpec.Backend, Firecracker rootless via jailer, ext4 rootfs, TAP network, API socket management, serial console terminal.
- **Code sẽ implement khi có KVM-capable host**: self-hosted runner hoặc bare metal.
- **Roadmap chi tiết**: mục 0 (precondition ✓ research) → mục 1 (firecracker backend) → mục 2 (benchmark).
