# OPS-MAPPING — the 14 operations, sandboxd → minictd

Source contract: `4AiCoding/control-plane/internal/docker/docker.go`.
Implementation: `internal/minict/runtime.go`, `exec.go`, `store.go`.
"E2E" column = verified live before handoff (kernel 5.10.134, unprivileged
user, busybox rootfs from mkrootfs.sh).

| # | Op | sandboxd call sites | minictd implementation | Parity notes | E2E |
|---|---|---|---|---|---|
| 1 | Run | api create, wake recreate | render `MINICT_SPEC` → prepare rootfs (`base`/`dir:`/`tar:`/`oci:`) → spawn `minict <rootfs> <cmd…>` (Setsid, log file) → spawn webproxy.py per `minict.webport` label → TCP-ready probe | returns name-as-ID; existing instance (any state) errors like `docker run --name X` | yes |
| 2 | Inspect | wake compare (image/user/domain), api create post-run (PID), observe | state.json + `/proc` liveness (zombie + starttime check) → `ContainerJSON` | status running/exited; labels/mounts/cmd from saved spec.json; BridgeIP always "" | yes |
| 3 | Remove | reconcile sweep, purge | lock → kill shim+proxies → `rm -rf` instance dir | idempotent nil on missing (`rm -f` parity) | yes |
| 4 | Start | wake path | re-spawn payload from saved spec.json on the SAME rootfs; bind sources re-checked | no-op when running; fs changes persist (verified with a file written via exec) | yes |
| 5 | Stop | idle reaper, pressure reaper | TERM shim → grace `timeoutSec` → KILL; proxies killed; state→stopped | ErrNotFound on missing; no-op on stopped; `--time=0` = immediate KILL path | yes |
| 6 | Exec | api exec v1 | ExecV2 without extras | buffered stdout/stderr/exit-code | yes |
| 7 | ExecV2 | api exec v2 (`?stream=false`) | `minict-exec --pid <pidns-1-child> --cwd --env-file -- cmd` (setns) or `minict <rootfs> cmd` (newns fallback, env via spec file, cwd via POSIX sh wrapper) · daemon watchdog TERM→KILL 2s · head/tail bounded buffers | exit 124 + TimedOut parity; Truncated marker byte-identical to docker.go | yes (setns + timeout + stdin + truncation) |
| 8 | ExecV2Stream | api exec v2 (`?stream=ndjson`) | same spawn, raw pipes; server muxes into NDJSON frames | MaxOutputBytes ignored (streaming = escape hatch) — docker parity | yes |
| 9 | Logs | api v1 logs (`?tail&follow`) | instance `log` file: tail-N or streaming follow (stops ~2s after payload death or on ctx/Close) | merged stdout+stderr order = file order | yes (tail; follow code-path exercised via server stream) |
| 10 | ListByNamePrefix | reconcile orphans | ReadDir instances root, prefix filter, sorted | any state (ps -a parity) | yes |
| 11 | Stats | v1 observe, pressure reaper | RSS tree sum (`/proc/*/status`) + 250ms CPU window (`/proc/*/stat`, USER_HZ=100) + descendant count; MemLimit from spec.Memory or MemTotal; Net/Block = 0 | documented approximation (CG1); zeros when stopped | yes |
| 12 | ServerVersion | telemetry | opaque string `minictd/0.1.0 (minict rootless, exec=setns|newns)` | docker `version --format {{.Server.Version}}` parity (opaque string) | yes |
| 13 | Info | readyz | same string as ServerVersion | docker `info --format {{.ServerVersion}}` parity | yes |
| 14 | NetworkSubnets | netguard deny-list | returns `--sandbox-subnet`/`--sandbox-gateway` (default 10.200.0.0/24 · 10.200.0.1) | defensive stub: loopback-only netns (D17) means sandboxes cannot source traffic; entry kept for belt-and-braces + future slirp | code-path only |

## Exec mode resolution

```
MinictExecBin set + file exists
  └── resolve pidns-1 child of shim (firstChild)
        ├── child found  → setns mode (preferred; runs INSIDE the instance)
        └── no child     → newns fallback (payload just died or 1-process tree)
binary missing → newns fallback (PoC parity, D5)
```

`newns` fallback delivers req.Env through a MINICT_SPEC file with `env`
lines only; `work_dir` through `cd "$1" && shift && exec "$@"`; it does NOT
re-apply binds/tmpfs/webports (documented PoC limitation).

## Contract type fidelity

`contract/contract.go` copies RunSpec (23 fields), ContainerJSON (+BridgeIP),
NetworkEndpointInfo, Mount, ExecResult, ExecV2Request/Result, Stats,
ExecTimeoutExit and ErrNotFound **byte-for-byte in shape** from docker.go
(JSON tags added only where the type crosses HTTP: RunSpec, Exec*, Stats
already had them). The 14-op interface signature is identical, so
`docker.Client` and `contract.HTTPClient` are interchangeable behind one
`contract.Runtime` value.
