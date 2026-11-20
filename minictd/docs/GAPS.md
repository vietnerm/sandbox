# GAPS — what is solved, what is open, what to watch

## Solved in this package (with E2E evidence)

### G1 — exec into a running rootless instance (was the highest risk)

PoC exec (D5) created a NEW namespace set on the same rootfs: execs were
isolated from the payload, had their own netns (no preview access), and no
shared pid view. This package ships the RootlessKit/podman model instead:

- `minictd` runs inside an OUTER userns U0; instances are descendants (U1).
  An ancestor-userns process is capable over U1 → setns into all instance
  namespaces is permitted without any host caps.
- `minict-exec` opens ALL six ns fds before joining (after setns(mnt) the
  instance's pidns-scoped /proc would hide the host pid), joins
  **user→mnt→pid→uts→ipc→net** in order (the full cap set gained at the
  user join covers the rest), re-anchors cwd to the instance root, and
  re-installs minict's seccomp blocklist verbatim (filters are per-THREAD
  and do not cross setns — without this the exec'd cmd would run unfiltered).
- The pid used is the pidns-1 child of the shim (`firstChild` in exec.go),
  so the pid join lands inside the instance.
- Verified: exec sees the instance hostname, env, bind mounts; 2ms round
  trip; stdin passthrough works; `--selftest-seccomp` passes (AF_NETLINK
  → EPERM under the filter).

Fallback posture remains available automatically (binary missing / child
gone → newns mode) — the control plane can treat exec as always-working,
with `state.json`/`GET .../state` recording which mode served.

### G-fixes to minict discovered during E2E (in the patch)

1. `SPEC_MAX_ENTRIES` 16→64 — sandboxd's proxy env + user env can exceed 16.
2. tmpfs size parser off-by-one (`sp+5` → `sp+6`) — `tmpfs /tmp:size=64m`
   previously failed with `bad size '=64m'` (fail-closed, so it was caught
   immediately; fixed and re-verified).

## Open gaps (ordered by integration risk)

### G2 — resource enforcement (CG1)

No cgroups v2 in this build: Memory/CPUs/CPUShares are Notes, PidsLimit is
the rlimit-nproc approximation, Stats uses /proc (RSS tree, 250ms CPU
window) instead of cgroup charges. Plan (minict GĐ2 roadmap): systemd
--user cgroup delegation, then move Stats to cgroup reads and turn the
Notes into native mappings. Interim mitigations for the control plane: the
idle/pressure reapers still work (RSS-based), admission control still
bounds aggregate memory by host MemTotal.

### G3 — preview routing & Traefik

The unix-socket bridge works per-port (verified) but Traefik integration is
not wired: on a minict node, use a Traefik file provider mapping
`s-<id>.<domain>` → `127.0.0.1:<port>`, and teach the wake probe to dial
`127.0.0.1:<webPort>` when `BridgeIP()` is empty (minictd sandboxes have no
bridge IP). `traefik.*` labels are already faithfully round-tripped by
Inspect, so domain-staleness checks keep working unchanged.

### A1 — egress identity

sandboxd injects HTTP(S)_PROXY into the sandbox env (mapped natively here,
and env caps raised to 64 for exactly this). But with loopback-only netns
there is no egress at all yet; when GĐ2 adds slirp/bridge egress, keep the
proxy-env injection and add the per-sandbox Proxy-Authorization token from
the integration plan (HTTPS_PROXY env is daemon-controlled, the payload
cannot override it).

### A2 — OCI images

`oci:<ref>` (crane export | tar --no-same-owner) is implemented but was NOT
exercised in the verification environment (no crane binary). Test before
production use; add a rootfs cache layer (content-addressed unpack) when
images churn.

### A3 — state & telemetry

minictd state machine (creating/running/stopped, self-heal on payload
death, pid-reuse-safe liveness) is in. **2026-11-20 (#91, D47): the periodic
full reconcile loop IS BUILT now** — `--reconcile-interval N` runs a
heal-only `Store.ReconcileAll()` ticker (compare state dir vs /proc; heal
to stopped; never spawns — Start/autostart stay explicit daemon decisions),
plus `--autostart` wake-recreate from spec.json for state=running instances
at boot (MD21). Still not built: the fleet-agent heartbeat (P4).

## Explicit non-goals carried over from minict (unchanged)

No K8s API, no multi-tenant RBAC, no PTY/TTY, no HA/failover of the daemon
itself (instances die with the node — same as docker on a single host; the
control plane's wake-recreate path is the recovery mechanism, and it works
because Run is reproducible from spec.json alone).

## Risk table

| Risk | Impact | Mitigation shipped | Residual |
|---|---|---|---|
| setns exec breaks on other kernels | exec degrades to newns | automatic fallback + mode surfaced in state/version | low: feature is standard on 5.x/6.x |
| host seccomp blocks mounts (tmpfs/shm) | degraded tmpfs (warn, run continues) | warn-not-die matches minict posture; Notes report it | none |
| port collision between instances | second webproxy crashes | TCP-ready probe adds a Note; control plane should allocate ports per node | open (allocation belongs to CP) |
| state dir lost (crash before save) | instance orphaned | Run is reproducible from the DB row (wake-recreate) | none |
| flock lost on NFS | concurrent ops | state dir is node-local by design | none |

---

## Integration review (2026-11-18, vietnerm/sandbox)

The handoff was treated as untrusted code and reviewed before merge. Seven
defects were fixed (R1–R7, details + evidence in
`journal/2026-11-18/decisions/minictd.md` D26): URL-name path traversal,
daemon environment leaking into every sandbox/exec, auth off by default,
half-created instance dirs, unbounded bodies, execstream buffer aliasing +
goroutine leak, and bind targets following payload-planted symlinks on the
host before pivot_root. `tests/test_minictd.sh` (MD0–MD19) guards them; the
same suite is red on the handoff as delivered (negative control).

### Open gaps found during review (not fixed here)

| ID | Gap | Why it matters | Plan |
|---|---|---|---|
| S1 | host `webproxy.py` connects to `<rootfs>/.minict-proxy-<port>.sock`, a path the payload can replace with a symlink to any host unix socket of the same uid (dbus, ssh-agent…) | preview clients could reach host-side sockets | **FIXED 2026-11-18 (issue #41, D31, PR `sec/s1-webproxy-symlink`)**: host bridge resolves the socket symlink-proof — openat chain `O_NOFOLLOW` per component + final `O_PATH`+`S_ISSOCK` + `connect("/proc/self/fd/<n>")` (TOCTOU-free), per connection, fail-closed; WEB7–WEB9 + mutant matrix. Residual: the minictd Go bridge must reuse the same pattern when GĐ2 opens |
| S2 | `ulimit nproc` / `PidsLimit` is per **host uid**, shared by all instances and the user's own processes | small limits break unrelated instances; large ones bound nothing | **FIXED 2026-11-20 (issue #4, D48, PR `feat/4-cgroups-v2`)**: cgroups v2 islands enforcement — daemon self-parks in `<base>/minictd-daemon/`, enables `+memory +cpu +pids` on its own `cgroup.subtree_control`, instances under `<base>/minictd-apps/sandbox-<name>` get `memory.max`/`cpu.max`/`cpu.weight`/`pids.max` + shim-pid migration; Stats read `memory.current`/`pids.current`/`cpu.stat`. Fail-soft: no writable v2 base → pre-CG1 notes + rlimit nproc with a measured reason note. Runner truth: `tests/test_cg.sh` CG0–CG5 (env-gated, CI). Residual: memory.swap.max/io.max not mapped; cpu.weight mapping is linear (not docker's exact formula — D48) |
| S3 | `bind …:ro` remounts only the top mount; submounts of a recursive bind stay rw | ro bind of a tree with mounts inside is partially writable | iterate submounts or `mount_setattr(AT_RECURSIVE)` (kernel ≥ 5.12) |
| S4 | API is plain HTTP; bearer token in clear on anything but loopback | multi-node (VPS cluster goal) needs transport security | unix-socket listener or TLS before any non-loopback use |
| S5 | exec'd processes do not get the spec rlimits (only the payload does) | exec can exceed `nofile` the operator set | apply spec rlimits in minict-exec (spec path via daemon) |
| S6 | webport collisions between instances are not detected up front | second proxy fails, only a Note records it | port allocation in the daemon |
