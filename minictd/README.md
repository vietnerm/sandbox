# minictd — control-plane daemon cho minict (GĐ2 seed)

> **Trạng thái (Track M M1, 2026-11-26):** `sẵn sàng` — GĐ2 DoD đạt (Phiên 34),
> GĐ3 DoD đạt (Phiên 37). MD0-MD61 PASS CI. Các tính năng env-gated (overlay,
> pasta, cgroups, PTY) ghi rõ trong bảng README chính.

Daemon rootless (Go, chỉ stdlib) quản lý nhiều instance minict qua HTTP API, theo
đúng interface 14 op + `RunSpec` của control-plane sandboxd (`contract/`), để
control-plane đổi runtime docker → minict ở mức type.

```
client (contract.HTTPClient, bearer token)
   │ HTTP / NDJSON
   ▼
minictd ── state dir ~/.local/state/minictd/<name>/{spec.json,state.json,spec.minict,log,rootfs/,lock}
   ├── sandbox/minict   (MINICT_SPEC, D23/D24) — payload trong user+mnt+pid+uts+ipc+net ns
   ├── bin/minict-exec  (setns + seccomp cài lại, D25) — Exec/ExecV2/ExecV2Stream
   └── tools/webproxy.py — preview port qua unix-socket bridge (D16)
```

## Build + chạy

```bash
make build rootfs minictd          # minict, busybox rootfs, minictd + minict-exec
export MINICTD_TOKEN=$(head -c16 /dev/urandom | od -An -tx1 | tr -d ' \n')
./minictd/bin/minictd --listen 127.0.0.1:7601 --unix-socket ~/.local/state/minictd/minictd.sock \
    --minict-bin ./sandbox/minict --base-rootfs ./rootfs \
    --webproxy ./tools/webproxy.py      # minict-exec tự tìm cạnh binary

curl -s -H "Authorization: Bearer $MINICTD_TOKEN" -X POST localhost:7601/v1/run \
  -d '{"name":"s1","image":"base","cmd":["/bin/sh","-c","sleep 300"],"labels":["minict.webport=8125"]}'
curl -s -H "Authorization: Bearer $MINICTD_TOKEN" -X POST localhost:7601/v1/sandboxes/s1/execv2 \
  -d '{"cmd":["hostname"],"timeout_s":5}'
```

Token bắt buộc (`$MINICTD_TOKEN` hoặc `--token-file`; `--insecure-no-auth` chỉ cho dev).
Image: `base`, `dir:<abs>`, `tar:<abs>`, `oci:<ref>` (cần `crane`, **chưa test** — A2).

## Transport (S4, issue #116)

- `--unix-socket PATH` — listener unix socket harden fail-closed: socket **0600**,
  dir **0700** (tự chmod nếu lỏng, die nếu chmod lỗi), refuse symlink/non-socket
  file, replace stale socket từ run trước. Mô hình như podman rootless service:
  perms filesystem là transport security, không cần PKI.
- `--listen host:port` — TCP truyền thống (mặc định 127.0.0.1:7601); rỗng = tắt
  (unix-only posture).
- Token **bắt buộc trên MỌI transport** (R3 — same-uid process vẫn connect được
  socket; auth là lớp phòng thủ sâu).

## CLI `sandbox` (#91, D47)

Client đầy đủ cùng binary API, không thêm logic (NO MAGIC DEFAULTS):

```bash
sandbox --addr 127.0.0.1:7601 --token "$MINICTD_TOKEN" list
sandbox --socket ~/.local/state/minictd/minictd.sock --token "$MINICTD_TOKEN" list
sandbox --addr 127.0.0.1:7601 --token "$MINICTD_TOKEN" up s1 base \
    --cmd "/bin/sh,-c,sleep 300" --webport 8125
sandbox --addr 127.0.0.1:7601 --token "$MINICTD_TOKEN" exec s1 /bin/hostname
sandbox --addr 127.0.0.1:7601 --token "$MINICTD_TOKEN" stop s1 --sec 10
sandbox --addr 127.0.0.1:7601 --token "$MINICTD_TOKEN" rm s1
```

Token resolve theo thứ tự: `--token` → `$MINICTD_TOKEN` → `$MINICTD_TOKEN_FILE`
→ `~/.config/minictd/token` (install.sh ghi). Transport resolve: `--socket`/
`$MINICTD_SOCKET` (unix) thắng `--addr`/`$MINICTD_ADDR` (TCP); cả hai rỗng →
TCP 127.0.0.1:7601.

## Test

`make test-minictd` → `tests/test_minictd.sh`, 36 case (MD0–MD36): parity seccomp
minict.c ↔ minict-exec.c, auth, lifecycle, exec setns, env không rò, timeout, stdin,
NDJSON, preview, path traversal, stop/start, bind-symlink, 404/409, remove sạch,
CLI round-trip (MD20), autostart wake-recreate (MD21), deploy unit verify (MD22),
G3 routes (MD23–25), unpack hardening (MD26/27), A2 oci e2e offline (MD28–MD32,
MD33 opt-in remote, MD34), S4 unix socket (MD35/36).

## Service người dùng thường (systemd --user, #91 D47)

```bash
bash minictd/deploy/install.sh          # render unit + copy binary + tạo token (0600)
systemctl --user daemon-reload
systemctl --user enable --now minictd
loginctl enable-linger "$USER"          # user manager sống qua boot/logout
```

Daemon flags mới: `--autostart` (recreate instance state=running lúc boot —
instance operator-stopped vẫn stopped) và `--reconcile-interval N` (A3: heal
proactive mỗi N giây; 0 = chỉ heal-lazy khi query). Payload sống qua daemon
restart (Setsid); autostart chỉ phục vụ reboot/daemon-restart.

## Bảo mật — điều luôn đúng

Mọi instance: userns không đặc quyền + mnt/pid/uts/ipc/net ns, pivot_root, drop toàn
bộ caps, NNP, seccomp blocklist, netns chỉ loopback. Exec join đúng các ns đó và cài
lại cùng seccomp. Env của sandbox chỉ đến từ `RunSpec.Env` / `ExecV2Request.Env`.
Trường RunSpec không thực thi được (Memory/CPUs/User/Network/…) được ghi thành Notes
(`GET /v1/sandboxes/{name}/state`), không bao giờ âm thầm.

## Còn mở

A2 (oci test), S3–S6 từ review tích
hợp (S1 FIXED #41, S2/CG1 FIXED #4/D48, G3 FIXED #97/D49) — chi tiết `docs/GAPS.md`.
A3 reconcile: proactive ĐÃ CÓ (#91, `--reconcile-interval`); fleet-agent heartbeat
(P4) vẫn mở.

## Cgroups v2 resource enforcement (CG1/S2, #4, D48)

Khi daemon có một cgroups v2 subtree writable (systemd user delegation, hoặc
`--cgroup-root` / `$MINICTD_CGROUP_ROOT` trỏ vào base đã bootstrap): daemon tự
đậu vào `<base>/minictd-daemon/`, bật `+memory +cpu +pids` trên
`cgroup.subtree_control` của base, và mỗi instance nhận
`<base>/minictd-apps/sandbox-<name>/` với `memory.max` (RunSpec.Memory),
`cpu.max` (RunSpec.CPUs), `cpu.weight` (CPUShares, tuyến tính 1024→100),
`pids.max` (PidsLimit) + shim PID migrate vào. Stats đọc cgroup accounting
trước, /proc fallback sau. KHÔNG có base → fail-soft: hành vi pre-CG1
(Notes + rlimit nproc) kèm note lý do đo được — instance vẫn boot được mọi nơi.

## Tài liệu

`docs/API.md` (endpoint), `docs/OPS-MAPPING.md`, `docs/RUNSPEC-MAPPING.md`,
`docs/GAPS.md`; `docs/MERGE-GUIDE.md` + `docs/HANDOFF-REPORT.md` là bản bàn giao gốc
(lịch sử, nối với sandboxd).
