# RUNSPEC-MAPPING — 23 fields of sandboxd's RunSpec on minict

sandboxd fills RunSpec in `internal/sandboxspec/spec.go Build()` (create +
wake-recreate share it). Exact values the control plane sends, and what
minictd does with each.

Legend: **native** = enforced by minict/MINICT_SPEC · **adapted** = mapped
to the closest minict mechanism · **note** = recorded in state.json Notes,
never silently dropped.

| # | Field | Typical value from sandboxd | Status | minictd mapping |
|---|---|---|---|---|
| 1 | Name | `s-<id>` | **native** | instance dir name + returned handle; validated `[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}` |
| 2 | Hostname | name | **native** | `hostname <v>` directive (sethostname inside uts ns; verified) |
| 3 | Network | `sandboxd_net` | note | loopback-only netns is stronger than any docker net (D17) — no external net either way |
| 4 | Userns | `host` | note | minict is ALWAYS unprivileged userns (cannot opt out — by design) |
| 5 | Runtime | `""` / `runsc` | note | minict is its own runtime; gVisor/runc not applicable |
| 6 | ReadOnly | `true` | note | private writable rootfs copy for now (overlayfs D2 blocked by CVE-2023-0386); workspace bind carries durable state anyway |
| 7 | CapDrop | `["ALL"]` | **native** | minict always drops ALL caps in its own userns; other entries recorded as moot |
| 8 | SecurityOpt | `["no-new-privileges"]` | **native** | `PR_SET_NO_NEW_PRIVS` always set; other values noted |
| 9 | CPUShares | `100` | note | no cgroups in this build (CG1) |
| 10 | CPUs | `""` / `"1.5"` | note | no cgroups (CG1) |
| 11 | Memory | `"10240m"` | note | no cgroups (CG1); value still used for Stats.MemLimitBytes |
| 12 | MemorySwap | `"10240m"` | note | same |
| 13 | PidsLimit | `1024` | **adapted** | `ulimit nproc=1024:1024` (setrlimit pre-capdrop; per-uid approximation of cgroups pids.max, noted) |
| 14 | Ulimits | `["nofile=65536:65536"]` | **native** | `ulimit nofile=…` lines; only nofile/nproc understood, others noted+skipped |
| 15 | Tmpfs | `["/tmp:size=512m","/var/tmp:size=128m"]` | **native** | `tmpfs <path>[:size=N]` directives (MS_NOSUID\|NODEV; warn-not-die where host seccomp blocks mount) |
| 16 | Env | spec env + proxy env (sorted) | **native** | `env K=V` lines; keys validated `[A-Za-z_][A-Za-z0-9_]*`, cap 64 entries (proxy env from sandboxd fits) |
| 17 | Volumes | `<workspace>:/home/sandbox`, resolv.conf `:ro` | **native** | `bind <src>:<dst>[:ro]`; host src must be absolute, ct-dst checked for `..` (fail-closed); named volumes rejected |
| 18 | Labels | `traefik.http.routers.*`, visibility | **native** | kept verbatim in state.json → surfaced in `Inspect().Config.Labels` (wake domain-staleness checks keep working); `minict.webport` label consumed for relays |
| 19 | User | `1000:1000` | note | payload runs as the userns-mapped root uid; no per-spec uid mapping yet |
| 20 | ShmSize | `""` / `"1g"` | **native** | `shm <v>` directive → tmpfs on `/dev/shm` (size grammar k/m/g shared with C parser) |
| 21 | Restart | `""` | note | no auto-restart (reaper/wake ownership stays with the control plane) |
| 22 | Image | instance image tag | **adapted** | `base` (—base-rootfs), `dir:<abs>`, `tar:<abs>`, `oci:<ref>` (crane export pipe tar). Unknown tags rejected with hint — no magic fallback |
| 23 | Cmd | sandbox command argv | **native** | payload argv after rootfs; empty → `/bin/sh -c "sleep 31536000"` (documented default, same posture as minict.sh up) |

## Reserved label

```
labels: minict.webport = "3000" | "3000,5173"
```

Each port P: minictd adds `webport P /.minict-proxy-P.sock` to the spec
(in-sandbox unix relay, D24 multi-relay) and runs
`python3 tools/webproxy.py --root <rootfs> --sock /.minict-proxy-P.sock --port P`
on the host (S1/D31: the bridge resolves the relay socket symlink-proof —
openat O_NOFOLLOW chain + connect via /proc/self/fd) → `curl 127.0.0.1:P`
reaches the sandbox's loopback-only httpd (verified E2E). Ports must be
1024..65535 (host proxy binds unprivileged).

## Why some fields are notes instead of errors

The control plane sends ONE RunSpec shape for both docker and minict nodes.
Erroring on `Memory=` would break the shared builder; silently ignoring
would be a lie. The note list (persisted in state.json, visible via
`GET /v1/sandboxes/{name}/state`, logged at Run) is the middle path: the
operator sees exactly which protections are docker-only until CG1/GĐ2
cgroups land.
