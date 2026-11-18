> **Historical handoff document (kept as delivered, 2026-11-18).** In this repo the
> `minict-spec.patch` is already merged into `sandbox/minict.c`, `patches/` is gone,
> auth is mandatory (`$MINICTD_TOKEN`), and seven review fixes (R1–R7) were applied —
> see `../README.md`, `GAPS.md` and `journal/2026-11-18/decisions/minictd.md`.

# MERGE-GUIDE — integrating minictd into the minict control-plane

Audience: the assistant/engineer merging this package into
`github.com/vietnerm/sandbox` (minict) and wiring it to
`github.com/kungfufanta/4AiCoding` (sandboxd). Read `README.md` first for
the summary and `docs/GAPS.md` for open risks.

Everything in this package is stdlib-only Go 1.23 + two C artifacts. The
contract types are **copied 1:1** from sandboxd's
`control-plane/internal/docker/docker.go` (field names, JSON tags,
doc-confirmed semantics), so the runtime swap is type-level, not
behavioral.

---

## 0. What each artifact does (30-second version)

| Artifact | Role | Source of truth for behavior |
|---|---|---|
| `patches/minict-spec.patch` | Adds `MINICT_SPEC=<file>` to `sandbox/minict.c`: hostname / env / bind[:ro] / webport (N relays) / tmpfs[:size] / shm / ulimit(nofile,nproc). Parsed host-side pre-`unshare`, fail-closed (exit 125), unknown directives warn+skip. | The patch itself (heavily commented, D23/D24) |
| `cmd/minict-exec/minict-exec.c` | setns-based exec into a RUNNING instance. Opens ALL ns fds before joining, joins user→mnt→pid→uts→ipc→net (ancestor-userns caps), re-installs the verbatim minict seccomp blocklist (per-thread filters do not cross setns), PDEATHSIG, SIGTERM/INT forwarding, stdin passthrough. `--selftest-seccomp` built in. | Header comment + minict.c D5/D11/D15/D19 |
| Go module (`contract/`, `internal/`, `cmd/minictd/`) | `minictd` daemon: state dir + flock, RunSpec→MINICT_SPEC renderer, rootfs prep (base/dir/tar/oci), 14 ops, HTTP API + client. | `contract/contract.go` (interface) + `docs/OPS-MAPPING.md` |

## 1. Proven behavior (verified end-to-end before handoff)

On kernel 5.10.134, unprivileged user: full lifecycle of a busybox rootfs
instance (run/inspect/logs/stats/stop→start with rootfs persistence/
remove), setns exec INSIDE the running instance (hostname/env/bind visible,
2ms), NDJSON exec-stream, daemon-side timeout (exit 124 at exactly 2s),
stdin passthrough, and preview `minict.webport` served through the
unix-socket bridge from inside the loopback-only netns.

`make runtime-check` re-proves the patched minict.c compiles; `make vet`
re-proves the Go side.

## 2. Merge steps into `vietnerm/sandbox`

### Step 1 — runtime C

```bash
cd sandbox/
git apply /path/to/minictd/patches/minict-spec.patch
make            # builds ./minict with MINICT_SPEC support
```

Patch is hunk-verified against the repo tip; if it drifts, it is a plain
unified diff against `sandbox/minict.c` (866 lines) — trivial to rebase.

### Step 2 — helpers

```bash
mkdir -p bin
cc -O2 -Wall -o bin/minict-exec minictd/cmd/minict-exec/minict-exec.c
./bin/minict-exec --selftest-seccomp     # must print "seccomp selftest OK"
```

Optionally add `minict-exec` to sandbox's Makefile; it needs nothing but
libc and the same kernel feature set minict already requires.

### Step 3 — the Go module

Two placement options:

- **A (recommended):** copy the whole `minictd/` folder into the repo root.
  Its `go.mod` is `module github.com/vietnerm/sandbox/minictd`, so the
  control-plane (once it has a Go module) imports
  `github.com/vietnerm/sandbox/minictd/contract`.
- **B (monorepo single module):** fold `contract/` into the control-plane's
  module as `internal/runtime/` and keep `cmd/minictd` + `internal/minict` +
  `internal/server` as a second binary in the same module.

No external dependencies; `go build ./... && go vet ./...` must stay clean.

### Step 4 — control-plane seam (sandboxd side)

1. Create `internal/runtime` in the control-plane with the `Runtime`
   interface from `contract/contract.go` (or import it).
2. Alias the docker package's types to the contract types (they are
   identical, so this is a rename + delete):
   ```go
   // internal/docker: delete RunSpec/ContainerJSON/Exec*/Stats/Mount/
   // NetworkEndpointInfo definitions, replace with:
   type RunSpec = contract.RunSpec
   type ContainerJSON = contract.ContainerJSON
   // ... (docker.Client then satisfies contract.Runtime unchanged)
   ```
   OR keep docker untouched and write a 20-line adapter struct — both work.
3. Replace struct fields `Docker *docker.Client` (wake, reconcile, reaper,
   api, observe...) with `Runtime contract.Runtime`.
4. Node → runtime selection (fleet): each fleet node advertises its runtime
   kind; minict nodes get `contract.NewHTTPClient("http://<node>:7601", token)`.
   Single-node OSS deployments: a `--runtime=minictd --minictd-url=...` flag
   on sandboxd is enough.
5. `sandboxspec.Build` (control-plane/internal/sandboxspec/spec.go) needs
   three minict-mode tweaks (all additive):
   - `spec.Labels = append(spec.Labels, "minict.webport="+portsCSV)` — this
     is how preview ports reach minictd (RunSpec has no Ports field by
     design).
   - Image: minictd accepts `base`, `dir:<abs>`, `tar:<abs>`, `oci:<ref>`
     (crane). Map the instance image accordingly; unknown docker-style tags
     are rejected on purpose (NO MAGIC DEFAULTS rule inherited from
     docker.go).
   - Leave Network/Userns/Runtime/ReadOnly/Memory/... as-is: minictd maps
     what it can and records the rest as Notes in state.json (visible via
     `GET /v1/sandboxes/{name}/state`).

### Step 5 — preview path (G3)

minict sandboxes have no bridge IP (`BridgeIP()` returns ""). The wake
readiness probe and Traefik routing need:

- wake: when `cj.BridgeIP() == ""`, probe `127.0.0.1:<webPort>` on the
  minictd node instead (the unix-socket bridge makes the port host-reachable
  when the sandbox is up; TCP-ready within ~2.5s of Run/Start).
- Traefik: either (a) run Traefik on the minict node and use a file provider
  that maps `s-<id>.<domain>` → `127.0.0.1:<webPort>`, or (b) keep the
  docker-provider flow for docker nodes only. The `traefik.*` labels are
  already faithfully carried in `Inspect().Config.Labels`.

### Step 6 — verification checklist after merge

```bash
# runtime
cd sandbox && make && ./minict rootfs /bin/sh -c 'echo ok'   # smoke

# daemon
minictd --listen 127.0.0.1:7601 --minict-bin ./minict \
        --base-rootfs ./rootfs --webproxy ./tools/webproxy.py \
        --minict-exec-bin ./bin/minict-exec --token devtoken

curl -s localhost:7601/healthz
curl -s -X POST localhost:7601/v1/run -d '{"name":"s-t1","image":"base","cmd":["/bin/sh","-c","sleep 300"],"labels":["minict.webport=8125"]}'
curl -s localhost:7601/v1/sandboxes/s-t1 | grep '"running"'
curl -s -X POST localhost:7601/v1/sandboxes/s-t1/execv2 -d '{"cmd":["/bin/sh","-c","hostname"],"timeout_s":5}'
curl -s localhost:7601/v1/sandboxes/s-t1/stats
curl -s -X POST localhost:7601/v1/sandboxes/s-t1/stop -d '{"timeout_sec":1}'
curl -s -X POST localhost:7601/v1/sandboxes/s-t1/remove
```

Then run sandboxd's existing e2e suite pointed at a minict node.

## 3. Semantic deltas vs docker (all documented in code, summarized here)

| Area | docker | minictd | Parity note |
|---|---|---|---|
| Run return | 12-char ID | instance name | control-plane stores it but addresses by name |
| Inspect status | exited/running/... | running/exited | only two states exist |
| BridgeIP | container IP | `""` always | wake must switch probe strategy (G3) |
| ExecV2 timeout | GNU `timeout` inside container | daemon watchdog: TERM group → SIGKILL 2s | same observable result (exit 124, TimedOut) |
| Stats | cgroup charge | /proc RSS tree + 250ms CPU window | approximation, see contract.Stats doc |
| Stop | stop --time=N | TERM shim, N grace, KILL; proxies killed | same contract incl. ErrNotFound |
| Start | docker start | same rootfs, saved spec re-spawned | fs changes persist (verified) |
| Remove | rm -f | idempotent, removes dir | same |
| NetworkSubnets | docker IPAM | defensive stub subnet (flags) | netguard keeps a deny-list entry |

## 4. Security posture (what minict gives you regardless of spec)

Every sandbox, always: unprivileged userns + mount/pid/uts/ipc/net ns +
pivot_root + all-caps drop + PR_SET_NO_NEW_PRIVS + seccomp blocklist (D11 +
AF_NETLINK D15) + PDEATHSIG chain + pidns-1 shim + loopback-only netns (D17,
fail-closed). Spec fields requesting LESS isolation are silently stronger
than asked; fields requesting different isolation (User=, userns host) are
Notes, never enforced. Exec payloads additionally get the same seccomp
filter re-installed inside the joined ns (minict-exec).

minictd itself: state under `~/.local/state/minictd`, flock per instance
(fixes the PoC flat-file TOCTOU), pid-reuse-safe liveness (`/proc/<pid>/stat`
starttime), bearer token auth, no TLS (assume loopback/VPN; add TLS or a
unix-socket listener for remote nodes if needed — the mux is listener-agnostic).

## 5. Known gaps / next steps (details in docs/GAPS.md)

1. **CG1** — cgroups v2: memory/cpu/pids enforcement (systemd --user
   delegation is the designed path in minict's GĐ2 roadmap).
2. **Egress** — loopback-only netns means no outbound net; when GĐ2 adds a
   slirp/bridge, re-point `NetworkSubnets` and the egress-proxy identity
   token (A1) work from the integration plan.
3. **OCI images** — `oci:` path is implemented via `crane export | tar` but
   was not exercised in the verification environment (no crane); test
   before relying on it.
4. **Fleet** — minictd is single-node; node token/lease integration with
   sandboxd's fleet protocol is P4 in the roadmap.
5. **ReadOnly rootfs** — private writable copy for now (overlayfs blocked by
   CVE-2023-0386 research, D2).
