# D85 — GĐ4 1b.a: Jail Firecracker = instance minict, không dùng jailer upstream

- **Issue:** #231 (GĐ4 mục 1b.a, roadmap — owner chốt 2026-11-27 GĐ4 ưu tiên số 1)
- **Ngày:** 2026-11-27 (Phiên 45)
- **Trạng thái:** Accepted

## Bối cảnh

Jailer upstream của Firecracker bắt buộc root: chroot/pivot_root, mknod
/dev/kvm + /dev/net/tun, setuid/setgid, tạo cgroup v1/v2 (docs/jailer.md chính
thức). Bất biến §0 PROMPT.MD: rootless (uid ≥ 1000, không setuid). Jail = thành
phần bắt buộc để chạy VMM.

## Quyết định

Chạy `firecracker` **bên trong một instance minict** làm jail:

1. **Cách ly** do minict cấp sẵn: userns (map uid/gid), mountns + pivot_root,
   seccomp (blocklist mặc định D11; allowlist narrow nếu đo thấy thiếu),
   capdrop ALL + no-new-privileges, cgroups v2 delegation (D48 suite).
2. **Stage file qua bind**: thư mục chứa binary `tools/bin` bind `:ro` vào
   `/fc-bin`; thư mục state riêng mỗi jail `<state>/fc/<id>` bind rw vào
   `/fc-state` — API unix socket của VMM nằm trong đó, host curl được qua bind.
3. **Device**: `/dev/kvm` bind rw từ host khi có (minict bind node có sẵn qua
   MS_BIND — mknod EPERM trong userns, không cần). `/dev/net/tun` chỉ khi bật
   mạng microVM (mục 1b.c, sẽ đo riêng).
4. **Bỏ jailer upstream**: không gọi binary `jailer`; spec `JailerArgs` hiện có
   giữ lại làm tài liệu tham khảo + bị test chặn không cho dùng trong đường
   daemon (mời gọi sai → fail trong review).

## Bằng chứng đo được (không KVM)

- Host (uid 1001, kernel 5.10): `--version` exit 0; `GET /` qua unix socket →
  200 `{"state":"Not started","vmm_version":"1.18.0"}` (journal research Phiên 45).
- Trong jail minict: MD72 (e2e daemon thật + minict + seccomp) — binary khởi
  động trong jail, `--version` đúng, `GET /` 200. MD72 chạy được mọi nơi; phần
  KVM là Go test env-gated chạy thật ở CI job `kvm`.

## Phủ định đã cân nhắc

- Jailer upstream — root-only, loại (GHSA-36j2-f825-qvgc cho thấy thêm việc
  "jailer = an toàn mặc định" không tuyệt đối — symlink overwrite trước đây).
- minijail — thêm dep ngoài, trùng cơ chế minict đã có.
- Chỉ chạy firecracker trực tiếp (không jail) — mất cách ly; firecracker tự cài
  seccomp cho chính nó nhưng không có mountns/userns/cgroup.

## Hệ quả

- `internal/firecracker/jail.go`: `JailSpec` → `contract.RunSpec` (binds, cmd,
  tmpfs, seccomp mặc định) + `WithKVM` env-gated.
- fetch-firecracker sửa bug thật: sha256 placeholder + URL thiếu `.tgz` — pin
  lại v1.18.0 với digest đo từ release chính thức (D37 pattern).
- 1b.e sẽ nối tiếp: API client config qua socket bind này; boot thật cần KVM.
