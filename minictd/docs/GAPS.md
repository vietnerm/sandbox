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

### G3 — preview routing & Traefik — **SHIPPED 2026-11-20 (issue #97, D49)**

The unix-socket bridge works per-port (verified) and G3 wires the routing +
probe now: the daemon generates a **Traefik file provider** route per
running instance when `--traefik-dir` is set — `sandbox-<name>.yml` maps
`s-<name>.<domain>` → `127.0.0.1:<port>` (multi-port D24 →
`s-<name>-p<port>.<domain>`), written atomically (temp+rename — Traefik v3.5
loads every .yml in the directory and fsnotify-watches it), removed on
Stop/Remove, rebuilt at daemon boot and reconcile (SyncAllRoutes; heal paths
drop routes of dead instances). **Wake probe**: `POST /v1/sandboxes/{name}/wake`
dials `127.0.0.1:<webPort>` when `BridgeIP()` is empty (always, for minictd) —
per-port TCP-ready map, 409 when stopped, 404 when gone. `traefik.*` labels are
still faithfully round-tripped by Inspect so domain-staleness checks keep
working unchanged; the daemon honors `traefik.enable=false` as opt-out.
**Residual**: full docker-label DSL translation (traefik.http.routers.* rules,
middlewares) is NOT implemented — native webport routing covers the G3 scope;
Traefik itself is an operator-deployed binary (not pinned/fetched by this repo).
Deploy: `--traefik-dir` default in the systemd unit + install.sh mkdir.

### A1 — egress identity

sandboxd injects HTTP(S)_PROXY into the sandbox env (mapped natively here,
and env caps raised to 64 for exactly this). But with loopback-only netns
there is no egress at all yet; when GĐ2 adds slirp/bridge egress, keep the
proxy-env injection and add the per-sandbox Proxy-Authorization token from
the integration plan (HTTPS_PROXY env is daemon-controlled, the payload
cannot override it).

### A2 — OCI images — **E2E TESTED 2026-11-21 (issue #103, D51, Phiên 16)**

`oci:<ref>` (crane export → **secureUnpack**, Go stdlib fail-closed extractor —
2026-11-21, issue #104/D50: was `tar -x --no-same-owner`, which leaned on
incidental GNU tar 1.35 protections; busybox/old tar would extract `..`
members and follow write-through symlinks — measured matrix in
`journal/2026-11-21/research.md`) is exercised END-TO-END with the real
crane v0.22.1 (pin + sha256, `tools/fetch-crane.sh`, D37 pattern) against an
in-memory registry on 127.0.0.1 (`tools/ocireg`, offline — no docker.io
rate-limit flake class): happy-path boot + exec (MD30), nonexistent ref →
fail-closed + wiped (MD31), hostile `../` image → refused + wiped + no
escape (MD32), `>`64 KB mid-stream refusal → 500 FAST (MD34 — pins the
unpackOCI pure-drain fix, **issue #108**: a mid-stream refusal used to
deadlock `crane.Wait()` until request-context cancel, measured with a
goroutine dump). Two INDEPENDENT refusal layers measured for the `../`
class: crane upstream itself refuses unsafe tar paths before writing, and
secureUnpack refuses them mid-stream (MD32 provenance assert accepts both).
Rootfs-cache layer (content-addressed unpack) when images churn: still open
(out of #103 scope).

**Residuals (measured, issue #103/#109):**
- **Absolute symlink targets are REFUSED** (D50 posture: reject, never
  sanitize) — busybox `--install -s`-style rootfs (including THIS repo's
  `sandbox/mkrootfs.sh` output: 271 applet links → `/bin/busybox`) cannot
  boot via `oci:`; MD34 pins the refusal. Resolution options (refuse /
  rewrite / inside-dst-only) need a maintainer decision — **issue #109**.
- Real-world remote pull (TLS/auth/redirect surfaces: docker.io/ghcr/ttl.sh)
  is opt-in via `MINICTD_OCI_REMOTE_REF` (MD33, SKIP unset) — docker.io
  anonymous rate limits on shared runner IPs are a documented flake class
  and conflict with the "CI xanh thật" bar (issue #14).
- crane export v0.22.1 cannot read local tarballs (every arg is parsed as a
  remote ref — measured); the harness registry is the offline workaround.


### Unpack boundary hardening — issue #104 (D50, 2026-11-21)

`prepareRootfs` fed untrusted content (oci: registry refs, operator tars)
into host `tar -x` host-side, uid operator, BEFORE pivot_root — the
sandbox's trust boundary was incidental host-tar behavior. Now: stdlib-only
Go extractor (`internal/minict/unpack.go`) with openat O_NOFOLLOW
per-component discipline (D31 pattern, issue #41 webproxy precedent):
member `..`/absolute refused, link targets validated, duplicates refused
(O_EXCL), setuid/setgid/sticky stripped (pure `stripSpecialBits` — unit-
verifiable on hosts whose LSM strips bits at create), FIFOs via mknodat
(rootless-safe), char/block refused (mknod EPERM rootless). No host tar in
the untrusted path anymore. **Residual (SIA #106, 2026-11-21)**: a tar entry
with mode 000 extracts as a 0000 file (parity with docker/tar — no floor
applied; standard images do not ship mode-000 files).

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
| S3 | `bind …:ro` remounts only the top mount; submounts of a recursive bind stay rw | ro bind of a tree with mounts inside is partially writable | **FIXED 2026-11-22 (issue #113, D52, PR `sec/113-s3-ro-bind-recursive`)**: recursive ro enforced — `mount_setattr(AT_RECURSIVE, MOUNT_ATTR_RDONLY)` on kernel ≥ 5.12; ENOSYS (kernel < 5.12) → per-submount bind-remounts from host-side `/proc/self/mountinfo` enumeration (pre-unshare — the only /proc window, D39); mountinfo unreadable or > 16 submounts → fail-closed die. Measured pre-fix repro (tmpfs submount stayed rw) + post-fix submount EROFS on dev 5.10 + BIND1–BIND3 + mutants M-A/M-B/M-C caught. Residual: mount_setattr path e2e-measured on CI runner only; same-sb inner binds are exact on ≥ 5.12 only |
| S4 | API is plain HTTP; bearer token in clear on anything but loopback | multi-node (VPS cluster goal) needs transport security | **FIXED 2026-11-22 (issue #116, D53, PR `sec/116-s4-unix-socket`)**: `--unix-socket PATH` — pathname unix socket, filesystem perms ARE the transport security (podman rootless service model): socket 0600 + dir 0700 enforced fail-closed at boot (chmod loose dir, chmod failure aborts), symlink/non-socket files refused (D31 posture), stale socket replaced, socket unlinked on clean shutdown; `--listen ""` disables TCP (unix-only posture); token auth stays REQUIRED on every transport (R3 — same-uid processes can still connect). Client: `contract.NewUnixHTTPClient` + CLI `--socket`/`$MINICTD_SOCKET`. Unit: `--unix-socket` in ExecStart. Tests: unit `listener_test.go` (6 cases: perms, stale, symlink, regular file, file-as-dir, empty path) + MD35/MD36 e2e (both transports, runtime perms, unix-only posture via ss pid check, 401 no-token, CLI round-trip). Residual: TLS for real cross-host multi-node (unix socket is local-only by design) |
| S5 | exec'd processes do not get the spec rlimits (only the payload does) | exec can exceed `nofile` the operator set | **FIXED 2026-11-22 (issue #121, D54, PR `sec/121-s5-rlimits-exec`)**: the daemon appends `MINICT_RLIMIT_<NAME>=soft[:hard]` directives to the exec env-file (parsed from `spec.Ulimits`, nofile/nproc only — parity buildSpecMinict); `minict-exec.c` pulls them into a SEPARATE array (NOT exported to the cmd env, R2 parity) and `apply_spec_rlimits()` runs them in the child AFTER `harden_exec_fds` (RLIMIT_CORE=0 stays untouched) and BEFORE `drop_all_caps` (D23: raising the hard limit needs CAP_SYS_RESOURCE held in the joined userns until capdrop). Test MD37: `ulimit -n` inside exec == spec nofile. PidsLimit NOT mapped for exec: cgroups v2 already bounds the exec'd process via the instance cgroup (pids.max); without cgroups the per-uid nproc approximation carries the S2 shared-limit hazard and stays payload-only. Residual: none (nofile/nproc parity with payload; other resources are cgroups-managed) |
| S6 | webport collisions between instances are not detected up front | second proxy fails, only a Note records it | **FIXED 2026-11-23 (issue #124, D55, PR `sec/124-s6-webport-alloc`)**: the daemon detects webport collisions BEFORE spawn — `portsHeldBy(except)` scans all instances via `ListByNamePrefix`, filters `alive(st).State == running`, collects `port -> name` (except the instance being Run/Start'd); Run and Start check each declared port against the held set and return `ErrPortInUse` (mapped to HTTP 409 Conflict, parity `ErrNotRunning`) instead of spawning a webproxy that would EADDRINUSE. Stop/Remove kill the webproxies (existing) → port released → a subsequent Run/Start with the same port succeeds. Test MD38: two instances same `minict.webport` → second Run 409; stop first → second Run succeeds. Residual: external (non-minict) processes holding the port still cause the webproxy to fail with a Note (out of S6 scope — ops/host issue, not a daemon-allocatable collision); no dynamic port allocation (operator-declared ports, not daemon-assigned) |
