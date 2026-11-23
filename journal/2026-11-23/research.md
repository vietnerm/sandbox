# Research — 2026-11-23 (SDDA Phiên 20, issue #124 / GAPS S6)

Chủ đề: webport collision detection — state-based vs TCP-probe vs dynamic allocation;
docker port allocation precedent; HTTP 409 Conflict semantics.

## Nguồn 1 — docker published-ports (docs.docker.com)
- Link: https://docs.docker.com/config/containers/container-networking/#published-ports
- `docker run -p <host>:<container>` — nếu host port đã chiếm, docker fail sớm với
  `bind: address already in use` (exit 125, không silent). Precedent: detect collision
  up front, fail closed. minict S6 = same posture (409 thay vì exit 125 vì API model).
- Docker dynamic allocation: `-p :<container>` (không host port) → docker chọn port
  ngẫu nhiên free. minict KHÔNG làm (operator-declared, host==container contract).

## Nguồn 2 — net.Listen TCP probe (Go stdlib)
- Link: https://pkg.go.dev/net#Listen
- `net.Listen("tcp", "127.0.0.1:<port>")` bind test: nếu OK, close ngay → port free.
  TOCTOU: giữa close và webproxy bind, process khác chiếm. minict KHÔNG chọn (state-based
  an toàn hơn, không race với external process).

## Nguồn 3 — minict state machine (repo, A3/D47)
- Link: `minictd/internal/minict/store.go` alive(); decision D47 (`journal/2026-11-20/`).
- State machine creating/running/stopped, alive() heals dead-pid running→stopped. Ports
  field trong state.json. Stop/Remove giết ProxyPIDs (webproxies) → port released.
  portsHeldBy reuse alive() + Ports: chính xác cho minict instances.

## Nguồn 4 — HTTP 409 Conflict semantics (RFC 9110 §15.5.10)
- Link: https://www.rfc-editor.org/rfc/rfc9110#section-15.5.10
- 409 Conflict: "request conflicts with the current state of the target resource".
  ErrNotRunning → 409 (instance state conflict). ErrPortInUse cùng class (port held
  by another running instance = state conflict). Parity mapping.

**Trade-off:** state-based detection (scan state.json) thay vì TCP probe — không
TOCTOU với external process, không false-positive trên operator's nginx, cheap.
Trade-off: false positive trên stale state sau daemon crash (reconcile heal).

**Áp dụng cho minict:** `portsHeldBy(except)` state-based + `ErrPortInUse` → 409;
Run/Start check trước spawn; Stop/Remove release (existing). No dynamic allocation
(operator-declared, host==container contract D16/D24).

---

# Research — 2026-11-23 (SDDA Phiên 21, issue #127 / GĐ2 mục 2 template)

Chủ đề: instance template pattern — docker commit/image tag; rootfs snapshot approaches;
list filter conventions.

## Nguồn 1 — docker commit + image tags (docs.docker.com)
- Link: https://docs.docker.com/reference/cli/docker/container/commit/
- `docker commit <container> <image:tag>` — tạo image từ container's filesystem changes
  (có thể running — docker uses overlayfs freeze). minict KHÔNG overlayfs (mục 3) → yêu cầu
  stop trước (consistent snapshot, fail-closed).
- `docker run <image:tag>` — chạy từ image. minict `template:<name>` parity (image form).

## Nguồn 2 — rootfs snapshot: cp -a vs overlayfs vs tar
- Link: https://man7.org/linux/man-pages/man1/cp.1.html (cp -a: preserve symlinks/mode/owner)
- cp -a: full copy (~2 MB/template), simple, parity base/dir. overlayfs: thin-provision
  (share base, COW layer) — nhưng rootless overlayfs + idmapped mounts là mục 3 (riêng).
  tar: archived snapshot (backup/restore, mục 3). Template scope = frozen base tái sử dụng
  → cp -a đủ (không cần thin-provision cho template).

## Nguồn 3 — minict ListByNamePrefix + nameRe (repo)
- Link: `minictd/internal/minict/store.go` nameRe `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`.
- nameRe rejects `_` prefix → `_templates` dir tự skip khi ListByNamePrefix filter.
  Convention: internal dirs dùng `_` prefix (reserved namespace).

## Nguồn 4 — HTTP 409 Conflict cho "op requires stopped" (RFC 9110 §15.5.10)
- Link: https://www.rfc-editor.org/rfc/rfc9110#section-15.5.10
- 409 Conflict: state conflict. ErrInstanceRunning (op requires stopped, instance running)
  = state conflict → 409. Parity ErrNotRunning/ErrPortInUse (cùng class).

**Trade-off:** template = full rootfs copy (cp -a, ~2 MB) thay vì overlayfs (thin-provision)
— đơn giản, không dependency kernel feature, parity base/dir. Trade-off: disk ~2 MB/template
(không share base). Overlayfs thin-provision là mục 3 (storage).

**Áp dụng cho minict:** `template:<name>` image form + `SaveTemplate` (stopped only, fail-closed
409) + `_templates` dir (nameRe-filtered) + `POST /save-template` API. Test MD39: marker ở /
(persistent), không /tmp (tmpfs).

---

# Research — 2026-11-23 (SDDA Phiên 22, issue #130 / GĐ2 mục 3 snapshot)

Chủ đề: rootfs backup/restore — tar.gz trusted vs secureUnpack untrusted; gzip vs zstd;
overlayfs rootless kernel support.

## Nguồn 1 — archive/tar (Go stdlib)
- Link: https://pkg.go.dev/archive/tar
- Go archive/tar: `tar.FileInfoHeader` + TypeSymlink + Linkname (Readlink). Symlink-aware
  round-trip. `filepath.Walk` uses Lstat (không follow symlinks) → preserve TypeSymlink.

## Nguồn 2 — secureUnpack vs trusted extract (repo D50/#109)
- Link: `minictd/internal/minict/unpack.go` secureUnpack; issue #109 (absolute-symlink refusal).
- secureUnpack (D50): untrusted OCI/operator tar, refuse abs symlinks (#109, posture refuse-
  never-sanitize). Snapshot là daemon-created (trusted) — khác trust boundary. `extractGzTar`
  (D57): refuse `..`/abs member PATHS, ALLOW abs symlink TARGETS (busybox applet pattern,
  271 abs symlinks in alpine rootfs).

## Nguồn 3 — gzip vs zstd (Go stdlib)
- Link: https://pkg.go.dev/compress/gzip (stdlib) vs github.com/klauspost/compress/zstd (external).
- Repo stdlib-only (go.mod no deps, D20 no-vendor). gzip stdlib available; zstd needs dep.
  gzip ratio ~2x worse than zstd, nhưng đủ cho backup. zstd deferred (dep decision).

## Nguồn 4 — overlayfs rootless kernel support (man7)
- Link: https://man7.org/linux/man-pages/man2/mount_setattr.2.html + overlayfs userns (kernel 5.11).
- Rootless overlayfs (mount overlayfs trong userns) cần kernel ≥ 5.11 (commit "overlay: allow
  mounting in user namespace"). Dev kernel 5.10 → NOT supported (env-gated). CI ubuntu-24.04
  kernel 6.x → OK. Phần (a) mục 3 (overlayfs) deferred to CI-only phiên.

**Trade-off:** snapshot = full tar.gz (trusted, gzip stdlib) thay overlay layer (thin-provision)
— đơn giản, no kernel feature dep, testable dev. Trade-off: archive full (~2 MB), không thin.
overlayfs thin-provision là phần (a) mục 3 (env-gated).

**Áp dụng cho minict:** `snapshot.go` (tarGzTree + extractGzTar trusted) + `Snapshot` API +
`backup:<name>` image form. Test MD40: marker ở / (persistent), snapshot stopped only.

---

# Research — 2026-11-23 (SDDA Phiên 23, issue #117 / mountinfo truncation)

Chủ đề: snprintf silent truncation; fail-closed posture sibling caps; NAME_MAX/PATH_MAX.

## Nguồn 1 — snprintf(3) truncation semantics (man7)
- Link: https://man7.org/linux/man-pages/man3/snprintf.3.html
- `snprintf(dst, n, "%s", src)`: writes at most n bytes (incl null). If output truncated,
  return value = strlen(src) (would-be length) — nhưng dst chứa truncated string. Silent
  nếu caller không check return. Fix: check strlen(src) >= n trước, hoặc use return value.

## Nguồn 2 — minict.c sibling fail-closed caps (repo D52)
- Link: `sandbox/minict.c` mountinfo enumeration (D52, S3 #113).
- >16 submounts → die (SPEC_SUBMOUNT_MAX cap). mp >1024 → die (mp[1024] cap). Cùng
  posture: refuse incomplete ro bind. Truncation suffix >256 → cùng class (config lie) → die.

## Nguồn 3 — NAME_MAX / PATH_MAX (linux/limits.h, man7)
- Link: https://man7.org/linux/man-pages/man7/limits.7.html (NAME_MAX 255, PATH_MAX 4096).
- Single filename ≤255 byte. A 255-byte submount name → suffix "/<255>" = 256 byte
  >= SPEC_SUBMOUNT_PATH(256) → die. Test BIND4 dùng 255-byte name (NAME_MAX boundary).

## Nguồn 4 — mount_setattr vs mountinfo-fallback (repo D52/D39)
- Link: `sandbox/minict.c` S3 fix (D52); `journal/2026-11-22/decisions/s3-ro-bind-recursive.md`.
- kernel ≥5.12: mount_setattr(AT_RECURSIVE) — one syscall, no enumeration. <5.12:
  per-submount bind-remount from /proc/self/mountinfo. Truncation only in fallback path
  (dev 5.10). BIND4 gated to mountinfo-fallback (SKIP on ≥5.12 CI).

**Trade-off:** die thay warn (issue gợi ý warn low) — die đóng lỗ silent-truncation
outright (consistent sibling caps), nhưng có thể break workload path >255 legitimate
(rare). Partial-ro silent worse → die.

**Áp dụng:** strlen check trước snprintf → die ≥256; memcpy sau check. BIND4 test
255-byte name → suffix 256 → die. Gated mountinfo-fallback (dev 5.10).

## Phiên 24 — seccomp allowlist theo template (issue #139, GĐ2 mục 3a)

Vấn đề: blocklist D11 mặc định ALLOW — mọi syscall mới thêm vào kernel (vd. io_uring
class) tự động Exposure. Allowlist theo template thu hẹp bề mặt cho workload đã biết.

Nguồn 1: OCI runtime-spec + Docker seccomp profile generation — https://github.com/moby/moby/blob/master/profiles/seccomp/README.md
  Docker sinh default.json bằng cách đo (giờ là sysexx/strace-based generate) rồi review
  tay; clone3 được trả ENOSYS để glibc fallback về clone có arg-check (BPF không deref
  được pointer) — cùng trick D11 đã dùng. Bài học: profile phải đo trên workload thật
  của template, KHÔNG chép default của Docker (khác libc, khác phiên bản rootfs).
Nguồn 2: systemd seccomp filter + SystemCallFilter — https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#SystemCallFiltering
  systemd unit dùng SystemCallFilter=~@obsolete (deny-set) hoặc =@system-service
  (allow-set theo nhóm syscall-name, libseccomp). Bài học: cho phép ghép theo "nhóm"
  giúp maintain; nhưng libseccomp là dep ngoài → minict giữ số syscall x86_64 trực tiếp
  (đơn mục tiêu arch đã ghi manifest §5), tên chỉ là comment trong file profile.
Nguồn 3 (tham chiếu thiết kế probe): strace PTRACE_SYSCALL mechanism — https://man7.org/linux/man-pages/man2/ptrace.2.html
  Máy dev không có strace/sudo → viết `tools/syscall-probe.c` (~140 dòng, PTRACE_SETOPTIONS
  TRACESYSGOOD|TRACEFORK|TRACEVFORK|TRACECLONE|TRACEEXEC) đếm tập syscall của toàn bộ
  cây con của `minict` — đúng phạm vi payload, không nhiễm harness.

Áp dụng cho minict:
1. Profile = file text `minictd/internal/minict/profiles/<name>.allow` (go:embed —
   số syscall x86_64 + tên làm comment; go:embed bắt buộc module-relative nên thư mục
   nằm trong minictd/ — số(name) một dòng, # comment).
2. Data-flow: Go load profile → render `seccomp_allow <nr>[,...]` vào MINICT_SPEC
   (payload) và `MINICT_SECCOMP_ALLOW=<nr>[,...]` vào env-file (exec setns) → C parse
   (host-side, trước unshare/setns) → install filter trong child (giống D11).
3. Filter allowlist = blocklist D11 làm SÀN (JEQ→EPERM trước), rồi arg-checks
   clone-NS/socket-AF giữ nguyên, rồi JEQ allowlist→ALLOW, default EPERM. clone3
   luôn ENOSYS (fallback glibc/musl về clone đã arg-check). Không libseccomp.
4. MD0 (so copy verbatim) giữ nguyên vùng so: enum + g_blocklist + install_seccomp —
   hai bản sửa GIỐNG NHAU từng byte trong vùng đó; phần parse directive khác nhau nằm
   ngoài vùng so.

---

# Research — 2026-11-23 (B6 retroactive, PR #142 CVE kernel evaluation, issue #144)

Phiên 25 (SDDA): bổ sung mục research cho PR #142 (from-audit #144 B6 — PR #142 thiếu
research ≥2 nguồn; research nằm rải trong PR body). PR #142 thêm `tools/cve-kernel.py`
đánh giá CVE kernel theo dải NVD CPE `linux_kernel` cho kernel runner + dòng LTS đích.

## Nguồn 1 — NVD API v2.0 rate-limit + CPE matching (NVD docs)
- Link: https://nvd.nist.gov/developers/vulnerabilities (API v2.0; rate limits 5 req/30s anonymous, 50/30s API key)
- NVD API v2.0 `/rest/json/cves/2.0?cpeName=cpe:2.3:o:linux:linux_kernel:*` trả CVE ảnh hưởng CPE đó.
  Rate-limit anonymous 5/30s — #142 dùng control query (CPE 119 ngày, text kernel.org, toàn NVD)
  để verify "0 mới" không rỗng nghĩa (control bắt dạng CPE rút gọn trả 0, run 45113493786).
- Áp dụng: `cve-kernel.py` query CPE linux_kernel cho runner `uname -r` + dòng LTS đích
  (6.1/6.6/6.12), đánh giá "vá từ X / chưa có bản vá / ngoài dải".

## Nguồn 2 — kernel.org CNA flow + lore linux-cve-announce (kernel.org)
- Link: https://www.kernel.org/doc/html/latest/process/cve.html (CNA flow, CVE numbering authority)
- kernel.org là CNA cho linux kernel. CVE kernel công bố qua lore.kernel.org/linux-cve-announce.
  NVD phân tích CVE kernel muộn (≤120 ngày công bố ≤2 ngày NVD sửa) — #142 đo được
  CVE-2026-98432 (TCP 9.8) + CVE-2026-53266 (ebtables 8.8) trên runner azure 7.0.0-1017.
- Áp dụng: `cve-daily.yml` bước mới; CVE HIGH/CRITICAL ngoài watchlist đi qua `cve-gate.py` (≥8.0).

**B3 note** (#144): nhánh `sec/cve-kernel-ranges` thiếu `<N>-<slug>` (PROMPT §1 B5). Không rename
(nhánh owner). Các nhánh sec/ sau dùng đúng `sec/<N>-<slug>` (precedent Phiên 17-23:
sec/113-..., sec/121-..., sec/124-..., fix/117-...).

---

# Research — 2026-11-23 (SDDA Phiên 26, issue #149 / GĐ2 mục 6 Landlock)

Chủ đề: Landlock LSM — ro-root enforcement rootless; dir rule subtree semantics;
Docker --read-only parity; kernel ≥5.13 gate.

## Nguồn 1 — Landlock(7) + landlock_create_ruleset(2) (man7)
- Link: https://man7.org/linux/man-pages/man7/landlock.7.html + https://man7.org/linux/man-pages/man2/landlock_create_ruleset.2.html
- Landlock (kernel 5.13+) — unprivileged fs access control. `landlock_create_ruleset(attr, size, 0)`
  → fd; `landlock_add_rule(fd, LANDLOCK_RULE_PATH_BENEATH, &path_beneath, 0)`; `landlock_restrict_self(fd, 0)`.
- Rule type PATH_BENEATH: a dir rule grants accesses to the dir's SUBTREE (union of ancestor
  rules). Accesses NOT in handled_access_fs are unrestricted.

## Nguồn 2 — LANDLOCK_ACCESS_FS_* flags (linux/landlock.h)
- Link: https://elixir.bootlin.com/linux/latest/source/include/uapi/linux/landlock.h
- Flags (u64): EXECUTE(0), WRITE_FILE(1), READ_FILE(2), READ_DIR(3), REMOVE_DIR(4),
  REMOVE_FILE(5), MAKE_CHAR(6), MAKE_DIR(7), MAKE_REG(8), MAKE_SOCK(9), MAKE_FIFO(10),
  MAKE_BLOCK(11), MAKE_SYM(12). 5.19+: REFER, TRUNCATE, IOCTL_DEV.
- Áp dụng: "/" rule READ_FILE|READ_DIR|EXECUTE (ro); "/tmp"+"/dev/shm" +WRITE|REMOVE|MAKE.

## Nguồn 3 — Docker --read-only + tmpfs parity (docs.docker.com)
- Link: https://docs.docker.com/reference/cli/docker/container/run/#read-only
- `docker run --read-only` + `--tmpfs /tmp` — rootfs ro, tmpfs writable. minict spec.ReadOnly
  via Landlock = same semantics rootless (no overlayfs ro-root, D2 → Landlock instead).

## Nguồn 4 — Landlock rootless + kernel gate (kernel.org)
- Link: https://www.kernel.org/doc/html/latest/userspace-api/landlock.html
- Landlock is unprivileged (no CAP needed), designed for sandboxing. `landlock_restrict_self`
  applies to current thread. Kernel <5.13 → ENOSYS (syscalls 444/445/446 absent). Dev 5.10
  ENOSYS → warn fail-soft; CI ubuntu-24.04 (6.x) → enforce.

**Trade-off:** Landlock ro-root (CI-only) thay overlayfs ro-root (D2, env-gated kernel 5.10
no overlayfs userns). Landlock works rootless on ≥5.13. spec.ReadOnly opt-in (ro-root breaks
write-to-/ workloads — Docker parity).

**Áp dụng:** `landlock_enforce()` in payload child (after seccomp, before exec) — "/" ro +
"/tmp"/"/dev/shm" rw, restrict_self. `readonly` directive. LL1 test CI-gated (probe
create_ruleset).

---

# Research — 2026-11-23 (SDDA Phiên 27, issue #30 / signal race fork→g_child_pid)

Chủ đề: signal mask + fork semantics; parent block propagation; child UNBLOCK vs restore-old.

## Nguồn 1 — signal(7) + sigprocmask(2) (man7)
- Link: https://man7.org/linux/man-pages/man2/sigprocmask.2.html + https://man7.org/linux/man-pages/man7/signal.7.html
- `sigprocmask(SIG_BLOCK, &set, &old)` — adds `set` to blocked, saves previous in `old`.
  `SIG_SETMASK` replaces; `SIG_UNBLOCK` removes `set` from blocked. A signal blocked when
  delivered is QUEUED (not lost) — delivered when unblocked (if handler installed).

## Nguồn 2 — fork(2) + signal mask inheritance (man7)
- Link: https://man7.org/linux/man-pages/man2/fork.2.html
- fork(2): child inherits the parent's signal MASK (blocked set) + dispositions. A parent
  block before fork propagates to the child. The child must explicitly UNBLOCK to reset.

## Nguồn 3 — minict.c shim signal flow (repo D19)
- Link: `sandbox/minict.c` fwd_sig + g_child_pid + sigprocmask (D19 pidns-1 shim).
- Parent installs fwd_sig (g_child_pid=0), forks pidns, sets g_child_pid after. Child
  (shim) BLOCKs fwd around payload fork, restores `old` after. Race: fork→BLOCK window
  + fork→g_child_pid window → fwd_sig no-op → swallowed.

## Nguồn 4 — pthread_sigmask / async-signal-safety (signal-safety(7))
- Link: https://man7.org/linux/man-pages/man7/signal-safety.7.html
- sigprocmask is async-signal-safe (safe in handler/fork window). The parent block around
  fork is safe. UNBLOCK in child is safe.

**Trade-off:** parent block + child UNBLOCK (not restore-`old`) — closes both races (parent
fork→g_child_pid, child fork→BLOCK) without breaking child handlers. `old` captures inherited
block; restore-`old` would keep blocked. UNBLOCK fwd → clean. Effective mask unchanged.

**Áp dụng:** parent sigprocmask(SIG_BLOCK {SIGTERM,SIGINT}) before pidns fork, unblock after
g_child_pid; child shim+payload restore → SIG_UNBLOCK &fwd (not SIG_SETMASK &old).

---

# Research — 2026-11-23 (SDDA Phiên 28, issue #154 / Landlock fail-closed + default policy)

Chủ đề: fail-closed vs fail-soft posture; default Landlock defense-in-depth; operator-requested vs always-on.

## Nguồn 1 — Landlock restrict_self + errno (man7)
- Link: https://man7.org/linux/man-pages/man2/landlock_restrict_self.2.html
- `landlock_create_ruleset` returns -1 ENOSYS (kernel <5.13) or EOPNOTSUPP (Landlock disabled).
- A process restricted via Landlock cannot loosen (rulesets are monotonic — restrict_self only adds).

## Nguồn 2 — fail-closed posture in minict (repo D52/D11)
- Link: `sandbox/minict.c` bind :ro die (D52), seccomp die (D11), mountinfo die (#117/D58).
- Operator-requested feature unavailable → die (refuse half-protected). spec.ReadOnly is
  operator-requested ro-root → Landlock unavailable → die (consistent). Default (always-on
  defense-in-depth) → warn (fail-soft, pivot_root primary).

## Nguồn 3 — Docker --read-only + security-opt no-new-privileges (docs.docker.com)
- Link: https://docs.docker.com/reference/cli/docker/container/run/#read-only
- `--read-only` is operator-requested; if the runtime can't enforce (e.g. no overlayfs),
  docker errors (fail-closed). minict spec.ReadOnly via Landlock = same: unavailable → die.

## Nguồn 4 — Landlock default sandbox (kernel.org samples)
- Link: https://www.kernel.org/doc/html/latest/userspace-api/landlock.html + samples/landlock/sandboxer.c
- The Landlock sandboxer sample restricts a process to a set of paths (default deny all,
  allow listed). minict default: allow "/" (rootfs subtree) all accesses — payload restricted
  to rootfs, host denied (defense-in-depth on pivot_root).

**Trade-off:** fail-closed (spec.ReadOnly + no Landlock → die) vs fail-soft (default + no
Landlock → warn). Operator-requested → die; always-on defense-in-depth → warn.

**Áp dụng:** landlock_enforce() always runs; spec.ReadOnly + ENOSYS → die; default + ENOSYS →
warn; "/" rule RO (spec.ReadOnly) or RW (default). LL2 (dev die) + LL3 (default boots).
