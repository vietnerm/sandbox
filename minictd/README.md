# minictd — control-plane daemon cho minict (GĐ2 seed)

> **Trạng thái (owner chốt 2026-11-18, D27):** GĐ1 vẫn là giai đoạn hiện tại.
> minictd là "GĐ2 seed gộp sớm": chỉ sửa bảo mật và giữ `make test-minictd` xanh,
> chưa mở tính năng GĐ2 mới cho tới khi DoD GĐ1 xong.

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
./minictd/bin/minictd --listen 127.0.0.1:7601 \
    --minict-bin ./sandbox/minict --base-rootfs ./rootfs \
    --webproxy ./tools/webproxy.py      # minict-exec tự tìm cạnh binary

curl -s -H "Authorization: Bearer $MINICTD_TOKEN" -X POST localhost:7601/v1/run \
  -d '{"name":"s1","image":"base","cmd":["/bin/sh","-c","sleep 300"],"labels":["minict.webport=8125"]}'
curl -s -H "Authorization: Bearer $MINICTD_TOKEN" -X POST localhost:7601/v1/sandboxes/s1/execv2 \
  -d '{"cmd":["hostname"],"timeout_s":5}'
```

Token bắt buộc (`$MINICTD_TOKEN` hoặc `--token-file`; `--insecure-no-auth` chỉ cho dev).
Image: `base`, `dir:<abs>`, `tar:<abs>`, `oci:<ref>` (cần `crane`, **chưa test** — A2).

## Test

`make test-minictd` → `tests/test_minictd.sh`, 20 case (MD0–MD19): parity seccomp
minict.c ↔ minict-exec.c, auth, lifecycle, exec setns, env không rò, timeout, stdin,
NDJSON, preview, path traversal, stop/start, bind-symlink, 404/409, remove sạch.

## Bảo mật — điều luôn đúng

Mọi instance: userns không đặc quyền + mnt/pid/uts/ipc/net ns, pivot_root, drop toàn
bộ caps, NNP, seccomp blocklist, netns chỉ loopback. Exec join đúng các ns đó và cài
lại cùng seccomp. Env của sandbox chỉ đến từ `RunSpec.Env` / `ExecV2Request.Env`.
Trường RunSpec không thực thi được (Memory/CPUs/User/Network/…) được ghi thành Notes
(`GET /v1/sandboxes/{name}/state`), không bao giờ âm thầm.

## Còn mở

CG1 (cgroups: Memory/CPUs/pids), G3 (Traefik + wake probe theo port host), A2 (oci),
S1–S6 từ review tích hợp — chi tiết `docs/GAPS.md`.

## Tài liệu

`docs/API.md` (endpoint), `docs/OPS-MAPPING.md`, `docs/RUNSPEC-MAPPING.md`,
`docs/GAPS.md`; `docs/MERGE-GUIDE.md` + `docs/HANDOFF-REPORT.md` là bản bàn giao gốc
(lịch sử, nối với sandboxd).
