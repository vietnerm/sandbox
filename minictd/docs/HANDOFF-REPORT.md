> **Historical handoff document (kept as delivered, 2026-11-18).** In this repo the
> `minict-spec.patch` is already merged into `sandbox/minict.c`, `patches/` is gone,
> auth is mandatory (`$MINICTD_TOKEN`), and seven review fixes (R1–R7) were applied —
> see `../README.md`, `GAPS.md` and `journal/2026-11-18/decisions/minictd.md`.

# minictd — handoff package

> **Báo cáo tóm tắt (tiếng Việt).** Package này hiện thực hóa đúng yêu cầu:
> phía minict có đủ **14 operation** (`Run, Inspect, Remove, Start, Stop,
> Exec, ExecV2, ExecV2Stream, Logs, ListByNamePrefix, Stats, ServerVersion,
> Info, NetworkSubnets`) + **1 struct RunSpec** của sandboxd
> (`control-plane/internal/docker/docker.go`, 23 trường — đếm chính xác,
> không phải 24), dưới dạng một **daemon `minictd`** nói được cùng "ngôn
> ngữ" với control-plane của 4AiCoding/sandboxd.
>
> **Đã build sạch + chạy E2E thật trong môi trường thử nghiệm**: run/inspect/
> stop/start/remove, exec setns vào instance đang chạy, exec-stream NDJSON,
> timeout watchdog (exit 124), stdin passthrough, preview qua unix-socket
> bridge, logs (tail + follow), stats từ /proc, persistence rootfs qua
> stop/start. Chi tiết phần "Đã verify".

---

## 1. Ba mảnh ghép trong zip

| Thành phần | File | Làm gì |
|---|---|---|
| **1. Patch runtime C** | `patches/minict-spec.patch` | Vá `sandbox/minict.c` thêm `MINICT_SPEC=<file>` (D23/D24): hostname, env, bind (ro), webport (multi-relay), tmpfs (size), shm, ulimit (nofile/nproc). Parse host-side trước `unshare`, fail-closed (exit 125), directive lạ warn+skip (forward-compatible). |
| **2. Exec helper C** | `cmd/minict-exec/minict-exec.c` | `exec` vào instance **đang chạy** bằng `setns` theo thứ tự user→mnt→pid→uts→ipc→net (mô hình RootlessKit/podman, chốt G1/D5). Kèm seccomp blocklist copy nguyên văn từ minict.c (filter per-thread KHÔNG đi qua setns), PDEATHSIG, forward tín hiệu, stdin passthrough, exit-code parity. |
| **3. Daemon Go** | `contract/` + `internal/` + `cmd/minictd/` | `minictd` — daemon rootless hiện thực **đúng interface 14 ops** của sandboxd: state machine (creating/running/stopped), flock chống TOCTOU, HTTP API mirror contract, HTTP client dùng lại được ngay trong control-plane. Chỉ dùng stdlib Go 1.23, không phụ thuộc ngoài. |

`patches/minict-spec.c` chỉ là bản gốc `minict.c` (tham chiếu diff, có thể bỏ).

## 2. Kiến trúc khi đã gộp

```
sandboxd control-plane
  └── internal/runtime.Runtime  = contract.Runtime (14 ops)
        ├── dockerRuntime  = docker.Client            (node docker, nguyên trạng)
        └── minictRuntime  = contract.NewHTTPClient() (node minict)
              │  HTTP/NDJSON, bearer token
              ▼
        minictd (mỗi node minict một daemon)
          ├── MINICT_SPEC → minict (user+mnt+pid+uts+ipc+net ns, pivot_root,
          │   capdrop, NNP, seccomp) ← KHÔNG đổi runtime C trừ patch
          ├── minict-exec (setns) cho Exec/ExecV2/ExecV2Stream
          ├── webproxy.py + relay unix-socket cho preview ports
          └── state dir ~/.local/state/minictd/instances/<name>/
              {spec.json, state.json, spec.minict, log, rootfs/, lock}
```

## 3. Đã verify (E2E thật, kernel 5.10.134)

| Flow | Kết quả |
|---|---|
| `Run` rootfs `base` (busybox 2MB) | instance chạy, pid ghi nhận, pid-reuse-safe |
| `Inspect` | Status running/exited đúng, Labels/Mounts/Cmd faithful |
| `ExecV2` **setns** | chạy TRONG instance: hostname riêng, env đúng, bind thấy; 2ms |
| `ExecV2Stream` | NDJSON `{t:stdout/stderr/exit}` theo đúng wire format |
| Timeout 2s vs `sleep 30` | exit 124, `timed_out=true`, đúng 2000ms |
| Stdin | passthrough qua minict-exec vào `/bin/cat` |
| Preview `minict.webport=8124` | `curl 127.0.0.1:8124` trả HTML từ httpd trong netns loopback-only |
| `Logs` tail | đọc log payload đúng |
| `Stats` | RSS cây tiến trình ~3.1MB, PIDs=5, MemLimit từ spec |
| Stop → file → Start | **file vẫn còn** (rootfs giữ nguyên qua stop/start) |
| Remove (idempotent) | 204, danh sách rỗng |
| `minict-exec --selftest-seccomp` | PASS (blocklist chặn AF_NETLINK EPERM) |
| 404/409 mapping | `ErrNotFound`→404, exec-when-stopped→409 |

Không verify được trong môi trường này: `oci:` (cần `crane`), cgroup limits
(môi trường không có cgroups v2 writable — đúng như CG1 của minict), và
kịch bản fleet đa node.

## 4. Merge nhanh (xem chi tiết `MERGE-GUIDE.md` cho Claude)

```bash
# 1. vá runtime C
cd sandbox/ && git apply ../minictd/patches/minict-spec.patch && make

# 2. build exec helper
cc -O2 -o bin/minict-exec minictd/cmd/minict-exec/minict-exec.c

# 3. build daemon
cd minictd/ && go build -o bin/minictd ./cmd/minictd

# 4. chạy thử
./bin/minictd --listen 127.0.0.1:7601 --minict-bin ../sandbox/minict \
  --base-rootfs ../sandbox/rootfs --webproxy ../sandbox/tools/webproxy.py \
  --minict-exec-bin ./bin/minict-exec
```

Control-plane: đổi `docker.RunSpec` → `contract.RunSpec` (types đã copy 1:1),
inject `contract.HTTPClient` cho node minict — phần còn lại của sandboxd
(wake, reconcile, reaper, observe) không phải đổi logic, chỉ đổi kiểu.

## 5. Giới hạn đã ghi nhận (trung thực, có Note trong state.json)

- **CG1**: Memory/CPUs/CPUShares không enforce (không cgroups v2); PidsLimit
  map tạm sang `rlimit nproc`; Stats = RSS cây tiến trình, Net/Block = 0.
- **Rootfs luôn writable private copy** — `read_only` chưa hỗ trợ (D2
  overlayfs bị CVE-2023-0386 giữ lại).
- **Netns loopback-only** (D17): không egress ra ngoài; preview đi cầu
  unix-socket theo port.
- Exec mode `newns` (fallback PoC) thiếu bind/tmpfs — chỉ khi không có
  binary minict-exec; mặc định đã là **setns**.
- `User`, `Restart`, `Userns`, `Runtime`, `Network`: ghi Note, không enforce
  (minict đã rootless-builtin; không auto-restart).
- Env cap 64 dòng, line 1024B (đã raise từ 16; đủ cho proxy env của sandboxd).
- Không PTY/TTY (đúng như docker.Client của sandboxd).

## 6. Bố trí file

```
minictd/
├── README.md                      ← file này
├── MERGE-GUIDE.md                 ← hướng dẫn gộp cho Claude (tiếng Anh)
├── Makefile                       (build / vet / runtime-check)
├── go.mod                         (module github.com/vietnerm/sandbox/minictd)
├── patches/
│   ├── minict-spec.patch          (vá sandbox/minict.c — MINICT_SPEC D23/D24)
│   └── minict-spec.c              (bản gốc minict.c — tham chiếu, có thể xóa)
├── cmd/
│   ├── minict-exec/minict-exec.c  (setns exec helper — G1)
│   └── minictd/main.go            (daemon entry, flags, graceful shutdown)
├── contract/
│   ├── contract.go                (RunSpec 23 trường + types + interface 14 ops)
│   ├── httpclient.go              (HTTPClient — thỏa Runtime, cho control-plane)
│   └── stream.go                  (wire format NDJSON exec-stream)
├── internal/
│   ├── minict/
│   │   ├── store.go               (state dir, flock, render MINICT_SPEC, spawn)
│   │   ├── rootfs.go              (image forms: base/dir/tar/oci)
│   │   ├── runtime.go             (Run/Inspect/Remove/Start/Stop/Logs/List/
│   │   │                           Stats/ServerVersion/Info/NetworkSubnets)
│   │   ├── exec.go                (Exec/ExecV2/ExecV2Stream + head/tail writer)
│   │   └── procstats.go           (/proc: stat, RSS, descendants, meminfo)
│   └── server/server.go           (HTTP API = contract map 1:1)
└── docs/
    ├── API.md                     (endpoint + curl + status code)
    ├── OPS-MAPPING.md             (bảng 14 ops: sandboxd → minictd → verify)
    ├── RUNSPEC-MAPPING.md         (bảng 23 trường → cơ chế minict)
    └── GAPS.md                    (G1–G3, A1–A3, rủi ro, việc còn lại)
```
