# Daily Log — 2026-11-17

> Phiên 1 (sáng): PoC minict rootless mini-container.
> Phiên 2 (chiều): push GitHub + bổ sung Giai đoạn C (CVE Check) & D (No-Compromise Validation)
> theo quy trình mới. Mọi số liệu dưới đây là đo thực tế, không suy đoán.

## 1. Research Summary

- **Kỹ thuật đã khảo sát (phiên 1):** user namespaces + mount ns + pivot_root —
  tự viết mini-container không runtime (chi tiết: `journal/2026-11-17/research.md`).
- **Kỹ thuật đã khảo sát (phiên 2 — engines cho Giai đoạn A tương lai):**
  - Landlock + seccomp (kiểu landrun/sandlock): cần kernel ≥5.13 — **kernel này 5.10.134
    không có Landlock** → chỉ research lý thuyết cho env đích; ưu tiên seccomp-bpf thuần
    cho các ngày tới.
  - bubblewrap: không có sẵn trong env; là reference implementation của đúng stack
    ns mà minict đang tự viết — dùng để đối chiếu hành vi khi cần.
  - gVisor (runsc) / Firecracker: không khả thi trong env này (chưa cài được runsc;
    Firecracker cần KVM — env không có). Theo dõi advisories qua GitHub Security
    Advisories thay vì CVE ID (bài học: CVE-2022-0316 trên NVD là của theme WordPress,
    KHÔNG phải Firecracker).
- **Nguồn tham khảo:** man7.org (user_namespaces.7, mount_namespaces.7, pivot_root.2),
  kernel.org sharedsubtree, NVD API 2.0 (6 CVE query), rootlesskit/bubblewrap repos.
- **Đánh giá ưu/nhược:** tự viết = toàn quyền kiểm soát + footprint nhỏ nhất nhưng
  tự chịu edge case (đã gặp 4+ ngay ngày 1); bubblewrap/gVisor = battle-tested nhưng
  nặng/cần cài; Firecracker = isolation mạnh nhất nhưng cần KVM.

## 2. Development Progress

- **Kỹ thuật đang dùng:** minict v0.1 — `unshare(CLONE_NEWUSER|NEWNS|NEWPID|NEWUTS|NEWIPC)`
  + self uid_map/gid_map + MS_REC|MS_PRIVATE + pivot_root + umount2(MNT_DETACH) fail-closed
  + bind /dev bits + tmpfs /tmp + capset(0) + PR_SET_NO_NEW_PRIVS + PR_SET_PDEATHSIG
  + process-group kill.
- **Cấu hình sandbox:** rootfs busybox 2MB (private copy per instance, 25-30ms);
  `/tmp` tmpfs MS_NOSUID|MS_NODEV; `/dev` = bind 4 node (null/zero/urandom/random);
  không netns (known gap), không cgroups v2 (env chỉ có v1), không /proc trong sandbox
  (host seccomp chặn mount proc — known gap có chủ đích).
- **Web service đã chạy thử:** chưa (ngày 1 tập trung core isolation). Mốc DoD: 2/5
  (rootless ✅, escape tests ✅). Plan: nginx/SQLite bên trong minict sau seccomp+netns.
- **Hiệu năng đo được (N=20):**
  - cold start (spawn→exit): **p50 = 1.2ms, p95 = 1.4ms** (baseline bare sh: 0.4ms)
  - RAM overhead idle: **~1.1 MB/instance** (wrapper 1100kB + sandbox process)
  - disk base: **2.0 MB**; fs copy per instance: 25-30ms
  - Target check: <2s ✅ (dư 1600x), <30MB ✅, <50MB ✅
- **Phát hiện env quan trọng:** SIGTERM từ host vào process trong userns bị
  **nuốt** (kill() rc=0, không delivery — A/B/C matrix với plain sleep làm control);
  SIGKILL được delivery. → cleanup = SIGKILL + PDEATHSIG + pidns-1-exit teardown.

## 3. CVE Check Results

- **Kernel version:** `5.10.134-013.15.kangaroo.al8.x86_64`, build 2026-07-30
  (vendor kernel — Alibaba backport riêng, số phiên bản không đủ kết luận patch status).
- **CVE áp dụng (đã query NVD API 2.0 — bảng đầy đủ: `docs/cve-watchlist.md`):**

| CVE | CVSS | Fixed-in (stable) | Status | Hành động |
|---|---|---|---|---|
| CVE-2022-0847 Dirty Pipe | 7.8 | 5.10.102 | inferred patched (.134 > .102) | monitor |
| CVE-2022-0185 fs context | 8.4 | 5.10.87 | inferred patched | monitor; sandbox không mount fs lạ |
| CVE-2023-0386 overlayfs | 7.8 | 5.10.179 | **possibly unpatched** (.134 < .179) | **mitigated by design D2** (không overlayfs trong userns) |
| CVE-2024-1086 nf_tables | 7.8 | 6.1.75/6.6.14 | unverified trên 5.10 | **không với tới — đo được**: exploit cần CAP_NET_ADMIN, sandbox capdrop toàn bộ, mount rc=1 (evidence) |
| CVE-2021-22555 netfilter | 7.8 | 5.10.42 | inferred patched | monitor |
| CVE-2024-21626 runc | 8.6 | runc 1.1.12 | **N/A** — không dùng OCI runtime | tham chiếu cho fd hygiene của minict |

- **Trạng thái vá:** 3/6 inferred-patched theo stable line; 2/6 unverified nhưng
  mitigated-by-design; 1/6 N/A. **Không phát hiện CVE critical chưa-vá ảnh hưởng trực
  tiếp đến minict đang chạy** → tiếp tục thử nghiệm trong phạm vi kiểm soát.
- **Hành động khắc phục:** giữ D2 (no overlayfs), capdrop + NNP là mitigation chính,
  cve-watchlist + `tools/cve-check.py` chạy hằng ngày, đọc vendor advisory (al8)
  khi cần kết luận backport chính xác.

## 4. No-Compromise Validation

- **Kiểm tra toàn vẹn (đo sau mỗi lần chạy test):**
  - Leaked sandbox processes sau test suite: **0** (pgrep trống — evidence file)
  - Host /etc/passwd hash: `21d5de2d...d412bd` — không thay đổi trong toàn bộ phiên
  - instances/ dọn sạch sau `down`
- **Kết quả thử nghiệm escape (tests/test_isolation.sh, 16/16):**
  - FS: host /etc/passwd, /home/z, /old-* escape path — không visible; mount bind /
    và mount sau capdrop — EPERM; write on-disk đúng rootfs (6 PASS)
  - PID: PID 1 riêng, không thấy host processes (2 PASS)
  - UID: uid=0 chỉ trong ns; file on-disk thuộc real uid 1001 (2 PASS)
  - PRIV: NoNewPrivs=1 (verify qua /proc host); hostname isolated; SIGKILL → no
    orphan (PDEATHSIG + pidns teardown) (3 PASS)
  - Known gaps (expected-fail, có chủ đích): netns chưa isolate, /proc bị host
    seccomp chặn, không cgroup v2 (3)
- **Bằng chứng:**
  - `journal/2026-11-17/evidence/isolation.txt` — full test output + integrity check
  - `journal/2026-11-17/bench.md` — số liệu hiệu năng
  - `journal/2026-11-17/decisions/poc.md` — D1-D5
- **Kết luận:** sandbox không bị escape, host không bị ảnh hưởng. Không chạy mã
  không tin cậy bên ngoài sandbox; mọi thí nghiệm trong scope kiểm soát.

## 5. Kế hoạch ngày mai

1. **Giai đoạn A/B:** seccomp-bpf filter tối thiểu cho minict (chặn open_by_handle_at,
   keyctl, bpf, setns ngoài; đo syscall overhead) — kernel 5.10 hỗ trợ đầy đủ.
2. **Giai đoạn C:** chạy `tools/cve-check.py` + kiểm tra advisory al8 mới nếu có.
3. **Giai đoạn D:** thêm escape test sâu (fd-based chroot escape, core_dump leak,
   setuid binary trong rootfs bị NNP chặn).
4. Nghiên cứu netns loopback + unix-socket proxy (slirp4netns/pasta không có trong env).
5. Mục tiêu lớn: web server (busybox httpd hoặc caddy static) chạy trong sandbox,
   phục vụ HTTP qua port host — first step của DoD.

---

# Phiên 3 (SDDA cycle — ClonePullPrompt job) — web server trong sandbox + CI scaffolding

> Vòng lặp SDDA 8 bước theo PROMPT.MD: Sync → Triage (7 issues + 24 labels) → chọn
> issue #1 (area:web) → Research → Develop → CVE check → Validation → PR. Bằng chứng
> đầy đủ trong `journal/2026-11-17/evidence/web.txt`.

## 0. Issue xử lý (SDDA)

- **#1** — feat(web): busybox httpd trong minict — status: **done** (PR chờ review)
- #7 — chore(ci): 5 workflow bắt buộc + PR template + root Makefile — status: **done** trong cùng PR
- #2..#6 — backlog seeds (seccomp, netns, cgroups, escape sâu, SIGTERM) — status: open
- #8 — sec(cve): track backport CVE-2026-31431 / CVE-2026-98432 — status: open (p2)
- Triage: repo trước phiên này có 0 issue / 0 PR / chỉ default labels → tạo 24 labels
  + 8 issues từ known gaps (daily-log phiên 1-2 + roadmap + PROMPT §8).

## 1. Research Summary (phiên 3 — `journal/2026-11-17/research.md` mục Phiên 3)

- **busybox `httpd.c` (source, đúng bản 1.37.0 trong rootfs):** `-p [IP:]PORT`,
  `-u` privilege drop, Deny/Allow IP logic, CGI theo build config. Đối chiếu live:
  HTTP 200, không header `Server:`.
- **edera.dev "userns = 262% more kernel attack surface":** CAP_NET_ADMIN trong
  userns mở mặt nf_tables (18 CVE/5 năm) → **netns phải đi cùng seccomp (D10)**.
- **podman/slirp4netns/pasta pattern:** usermode networking là chuẩn rootless;
  env không có tool đó → hôm nay dùng bind-loopback trong shared netns (D7).
- **Thực nghiệm env (đo thật):** `unshare --user --net` OK; lo up + dummy + veth
  OK trong netns mới; nối veth ra host netns rootless = không thể (cần CAP_NET_ADMIN
  host) → usermode proxy là phần lớn của ngày netns (issue #3).

## 2. Development Progress (phiên 3)

- **Nhánh:** `feat/web-busybox-httpd`
- **Thay đổi:**
  - `sandbox/minict.sh` + subcommand `web <name> <port> [docdir]` (D6: busybox httpd
    như PID 1 của instance; D7: bind bắt buộc 127.0.0.1, port ≥ 1024, docroot nằm
    trong private rootfs copy)
  - `tests/test_web.sh` — 6 case mới (WEB1-6): 200+marker, loopback-only, traversal
    không leak host passwd, docroot private copy, 404+server sống, down sạch
  - Root `Makefile`: build/rootfs/test/test-escape/test-web/perf-check/cve-check/bench
  - `tools/perf-check.sh` — perf-guard gate (cold/RSS/disk vs ngưỡng PROMPT)
  - `tools/cve-check.py` — CLI `--kernel/--watchlist/--format md/--json` (D9: chỉ
    parse CVE ở cột đầu của dòng bảng; bài học false positive)
  - `.github/workflows/`: ci.yml, cve-daily.yml, perf-guard.yml, issue-triage.yml,
    research-log.yml + `.github/PULL_REQUEST_TEMPLATE.md` (PROMPT §2-3)
  - `docs/perf-baseline.md` (ngưỡng + số đo), `journal/2026-11-17/decisions/web-httpd.md`
    (D6-D10), `benchmarks/bench-web.sh` + benchmark phiên 3
- **Perf:** cold_start p50 = **1.03-1.41 ms** | rss = **1028-1148 kB** |
  disk = **2.01-2.02 MB** (perf-check PASS) | web-ready p50 = **18 ms**,
  RSS web instance ~2.0 MB, disk 2.1 MB

## 3. CVE Check Results (phiên 3 — evidence `journal/2026-11-17/cve/web.md`)

- **Kernel:** `5.10.134-013.15.kangaroo.al8.x86_64` (cùng env phiên trước).
- **Watchlist 16 CVE (NVD verify thật):** 6 kernel/engine cũ (CVE-2021-22555 update
  7.8 → **8.3** theo NVD hiện tại) + **5 CVE kernel mới 2026** + **5 CVE busybox
  httpd mới** (do httpd vừa vào sandbox):
  - CVE-2026-98432 TCP IPv4 UAF — **9.8 CRITICAL** — host-side; sandbox không là
    ingress (WEB2 loopback-only đo được) → issue #8 track backport
  - CVE-2026-31431 "Copy Fail" algif_aead 7.8 — range chứa 5.10.134, NHƯNG đo được
    **socket(AF_ALG) → EPERM cho uid thường** (host seccomp) → không với tới (evidence)
  - CVE-2025-39964 af_alg race 7.8 (KEV) — cùng surface AF_ALG → cùng mitigation
  - CVE-2026-53266 ebtables 8.8 (KEV) — cần CAP_NET_ADMIN host → không với tới
  - CVE-2025-39682 kTLS 9.8 (KEV) — host-side, sandbox không thiết lập kTLS
  - busybox httpd: CVE-2026-124739 7.5 / CVE-2026-88829 7.2 / 6.5 / 5.3 / 3.7 —
    **không với tới**: static-only, không httpd.conf/auth/proxy/deny-rules
- **False positive bắt được:** quét full-text watchlist bắt nhầm CVE-2022-0316 9.8
  (WordPress theme) → sửa parser chỉ nhận dòng bảng (D9); evidence sạch.
- **Không có CVE critical CHƯA-VÁ ẢNH HƯỞNG trực tiếp minict** → tiếp tục thử nghiệm
  trong phạm vi kiểm soát; không cần dừng dev (giữ điều kiện PROMPT §6).

## 4. No-Compromise Validation (phiên 3 — evidence `journal/2026-11-17/evidence/web.txt`)

- `make test` (isolation/escape): **16/16 PASS**
- `make test-web`: **6/6 PASS** (suite mới)
- `make perf-check`: PASS (1.41 ms / 1028 kB / 2.02 MB)
- Host baseline: `/etc/passwd` sha256 `21d5de2d...d412bd` — **giống hệt phiên trước**
  (unchanged); `/etc/shadow` unreadable (rootless — đúng thiết kế); `/bin/bash`
  `223fe856...`, `/usr/bin/sudo` `a59f393b...` (lần đầu ghi — sẽ so chéo các ngày sau)
- Listeners: 6 listener env-owned (12600/12700/81/19001/19005/19006 — có trước,
  không thuộc minict); 0 process leak sau test (`pgrep minict|httpd` trống)

## 5. Kế hoạch ngày mai

1. **Issue #3 (netns)**: `CLONE_NEWNET` trong minict + loopback-only trong ns riêng;
   unix-socket proxy để web vẫn reachable; **bắt buộc kèm seccomp chặn netlink/nftables
   (D10 + issue #2)**. Đo perf lại (target giữ ≤ 5 ms cold start).
2. Re-check PR #9 CI; merge khi xanh; chuẩn bị issue #2 (seccomp allowlist).
3. CVE: theo dõi advisory al8 cho CVE-2026-31431/98432 (issue #8).

## 6. PR (phiên 3)

- **#9** — `feat(web): busybox httpd inside minict + make test-web; chore(ci): required
  workflows + perf gate` — **ready for review** (đã chuyển từ draft sau khi CI xanh)
- CI commit f78fc7d: `build-test` success ×2 (push + pull_request), `perf` (perf-guard)
  success — kiểm tra qua API check-runs
- Merge: **không** — chờ maintainer review (PROMPT §6: không tự merge)
- Self-audit cuối phiên: `make test` 16/16 + `make test-escape` 16/16 +
  `make test-web` 6/6 PASS (evidence `journal/2026-11-17/evidence/self-audit.txt`)
- Định nghĩa Done của ngày: triage toàn bộ (0 issue mới ngoài vòng — 8 issue seeds
  tạo từ known gaps) ✓ | 1 PR, CI xanh ✓ | CVE check + watchlist cập nhật ✓ |
  escape test + host baseline + evidence ✓ | daily-log commit ✓ | 0 issue security
  p0 bỏ quên (chỉ có #8 p2 với evidence không-with-tới) ✓

## Trạng thái ngày: DAY PASS

---

# Phiên 4 (SDDA cycle) — SBG branch gate + seccomp-bpf (issue #2)

> Hai nhiệm vụ theo chỉ lệnh maintainer: (1) SBG — kiểm tra và gộp nhánh theo
> prompts/BRANCH_GATE_PROMPT.md; (2) đọc lại PROMPT.MD và thực hiện tiếp kế hoạch trong
> daily-log = mục 1 "Kế hoạch ngày mai" của phiên 1-2 (seccomp-bpf, issue #2).

## 0. SBG — Branch Gate (chạy trước, MODE=apply theo chỉ lệnh trực tiếp của owner)

- Điều kiện dừng §0.6: **không kích hoạt** — 0 issue `type:security priority:p0`
  (#8 là p2, đo được "không với tới"), host baseline unchanged, CI main chưa đỏ.
- Thu thập: 2 nhánh ≠ main. Đánh giá cổng G1–G8 cho PR #9 (`feat/web-busybox-httpd`
  @6c198e309): PR open ready, author=owner, mergeable=clean; ci + perf-guard success
  trên đúng HEAD; retest evidence 16/16+6/6; audit self-audit PASS không concerns;
  behind=0; PR đúng PROMPT (test mới + decision-log D6-D10 + perf đo; bundling #1+#7
  giải trình theo PROMPT §1 B8); `.github/workflows/**` thuộc đúng phạm vi issue #7
  (G7 "ngoài phạm vi issue" không áp dụng) + owner duyệt merge trong phiên.
- Thực thi: squash-merge PR #9 qua API `match-head-commit` → `9b6b83c1`; **chờ CI main
  xanh** (run 44013069212, lần đầu `ci.yml` chạy trên main); DELETE_MERGED
  `feat/web-busybox-httpd` + `sandbox/daily-2026-11-17` (ancestor, không cần tag).
- Kết quả cuối: **chỉ còn `main`**; ledger `docs/branch-ledger.md` tạo mới (append-only).
- Báo cáo SBG: `branch-plan.md` + `branch-report.json` (artifact phiên chạy).

## 1. Issue xử lý (phiên 4)

- **#2** — feat(seccomp): minimal seccomp-bpf blocklist — status: **done** (PR phiên 4)
- Không làm thêm issue khác (giữ 1 issue/phiên dev; SBG là nhiệm vụ vận hành riêng).

## 2. Research Summary (phiên 4 — `journal/2026-11-17/research.md` mục Phiên 4)

- **Kernel docs seccomp_filter.rst + man7 seccomp(2):** PR_SET_SECCOMP cần NNP (đã có);
  RET_ERRNO = fail visible; filter không gỡ được → phù hợp "last lock".
- **pandastack io_uring 08/2026:** ring-op thực thi trong kernel, seccomp nhìn không
  thấy I/O → blocklist io_uring_setup/enter/register là chuẩn "disable io_uring".
- **Docker default profile = blocklist ~50 syscall** (SUSE/mrcloudbook tổng hợp) +
  **gVisor 2024: cBPF ~ns/syscall** → precedent thiết kế + kỳ vọng overhead.

## 3. Development Progress (phiên 4)

- **Nhánh:** `feat/2-seccomp-bpf`
- **Thay đổi:**
  - `sandbox/minict.c`: `install_seccomp()` — cBPF blocklist 32 syscall + arg-check
    `clone()` CLONE_NEW* (chặn ns lồng nhau) + `socket(AF_ALG)` (refs #8) +
    `clone3`→ENOSYS (fallback-friendly, D13); arch-guard KILL_THREAD; default ALLOW;
    cài trong child sau capdrop, trước exec; `MINICT_NO_SECCOMP=1` để tắt (A/B).
  - `tests/seccomp_probe.c` + `tests/test_seccomp.sh`: 17 case (SC0-SC16) — 11 assert
    EPERM/ENOSYS với filter + 4 control A/B (chứng minh EPERM từ filter mình) +
    SC0 workload sống sót + SC15 mount fail. Probe build `-static` (rootfs không libc).
  - Root `Makefile`: `test` chạy cả 2 suite; target `test-seccomp`; **fix nền sẵn có:
    recipe 8-spaces → tab** (GNU Make ≥4.4 từ chối spaces — "missing separator";
    CI cũ pass vì runner còn make 4.3).
  - Docs: decision-log D11-D14 (D14: bắt 5 số syscall sai nhờ control test — setns
    là 308 không phải 306), perf-baseline (row phiên 4), roadmap (seccomp done,
    DoD 4/5), README, skills.md (5 pitfalls seccomp), cve-watchlist (defense-in-depth).
- **Perf:** perf-check PASS — cold_start **1.15 ms** (N=20) | rss **1060 kB** |
  disk **2.02 MB** | A/B overhead seccomp **+0.2 ms p50** (1.38 ON vs 1.18 OFF, N=40
  interleaved) — cách ngưỡng 100 ms 70x.

## 4. CVE Check Results (phiên 4 — evidence `journal/2026-11-17/cve/seccomp.md`)

- Kernel `5.10.134-013.15.kangaroo.al8.x86_64`; watchlist 16 CVE + 1 header row,
  **0 lỗi parse**. Không CVE critical chưa-vá ảnh hưởng trực tiếp minict.
- Seccomp mới chuyển các mitigation "chỉ host-side" thành "chặn cả trong sandbox":
  AF_ALG (CVE-2026-31431 class), io_uring (class bypass), process_vm_*, keyring,
  open_by_handle_at, setns/unshare — defense-in-depth ghi vào cve-watchlist.

## 5. No-Compromise Validation (phiên 4 — evidence `journal/2026-11-17/evidence/seccomp.txt`)

- `make test` (isolation 16 + seccomp 17): **33/33 PASS**
- `make test-web`: **6/6 PASS** — httpd chạy bình thường DƯỚI filter
- `make perf-check`: PASS (1.15 ms / 1060 kB / 2.02 MB)
- Host baseline: `/etc/passwd` `21d5de2d...d412bd` — **khớp chính xác phiên trước**
  (unchanged); leaked processes: **0**; listeners: chỉ 6 listener env-owned có trước.

## 6. PR (phiên 4)

- **#10** — `feat(seccomp): syscall blocklist after capdrop + 17-test suite (Closes #2)` (từ `feat/2-seccomp-bpf`)
  — ready khi CI xanh; SBG gộp squash khi đủ cổng, xoá nhánh sau xác minh, ledger cập nhật.
- Định nghĩa Done của phiên: SBG gộp+dọn nhánh xong ✓ | research ≥2 nguồn mới ✓ |
  test mới (17) chạy được qua `make test` ✓ | decision-log D11-D14 ✓ | perf đo +
  baseline cập nhật ✓ | CVE check + evidence ✓ | host baseline unchanged ✓ | daily-log ✓

## Trạng thái phiên: DAY PASS


---

# Phiên 5 (SDDA cycle + SBG apply) — netns isolation + unix-socket web bridge (issue #3)

> Chỉ lệnh maintainer: (1) kiểm tra và gộp nhánh; (2) đọc lại PROMPT.MD và thực hiện
> tiếp kế hoạch trong daily-log (mục "Kế hoạch ngày mai" — netns, issue #3);
> (3) đưa BRANCH_GATE_PROMPT.MD lên repo. Kết quả: 2 PR gộp (#11, #12), repo về
> trạng thái chỉ còn `main`, CI main xanh sau từng lần gộp.

## 0. SBG — Branch Gate (MODE=apply, chạy theo chỉ lệnh trực tiếp của owner)

- **Kiểm tra nhánh (đầu phiên):** fetch --prune + API: remote chỉ còn `main`
  (nhánh phiên trước đã gộp + xoá sạch); PR #9/#10 = MERGED; không có nhánh nào
  chờ gộp → không còn việc gộp nào trước phiên dev. Điều kiện dừng §0.6 không
  kích hoạt: 0 issue `type:security priority:p0` (#8 là p2), host baseline
  unchanged (`21d5de2d…d412bd`), CI main xanh (run 44016693725).
- **PR #11** — `chore(branches): add prompts/BRANCH_GATE_PROMPT.md (SBG system prompt)`
  (@23f7491e): docs-only, owner chỉ lệnh trực tiếp ("đưa file BRANCH_GATE_PROMPT.MD
  lên repo luôn nhé") = G7 human approval; build-test xanh trên đúng HEAD;
  squash qua API match-head-commit → `a5128241`; CI main xanh (run 44018921211);
  DELETE_MERGED (HTTP 204). File `branch-gate.yml` (draft) KHÔNG đưa lên —
  `run: ${AGENT_CMD:?set AGENT_CMD}` chưa cấu hình, lên repo sẽ fail cron hằng ngày.
- **PR #12** — feat(netns) (@6268c901, chi tiết mục 2): gate G1–G8 PASS
  (G2 build-test×2 + perf xanh trên đúng HEAD; G3 retest 44/44+6/6 cùng SHA trên
  runner; G4 self-audit 0 concerns; G5 mergeable=clean; G6 Closes #3, 1 issue/PR,
  test mới, decision-log D15–D18; G7 diff 19 files — 0 file nhạy cảm; G8
  perf-guard xanh) → squash match-head-commit → `18cb41e5`; CI main xanh
  (run 44020968892); DELETE_MERGED (HTTP 204).
- **Kết quả cuối:** remaining_branches = `["main"]`; ledger thêm 2 row (PR #11,
  PR #12). Báo cáo SBG: `branch-plan.md` + `branch-report.json` (artifact phiên).

## 1. Issue xử lý (phiên 5)

- **#3** — feat(netns): network namespace isolation — status: **done** (PR #12,
  `Closes #3` tự đóng).
- Không làm thêm issue khác (1 issue/phiên dev; SBG + upload là nhiệm vụ vận hành).

## 2. Development Progress (phiên 5)

- **Nhánh:** `feat/3-netns`
- **Thay đổi:**
  - `sandbox/minict.c`: CLONE_NEWNET mặc định BẬT fail-closed (D17,
    `MINICT_NO_NETNS=1` opt-out); `bring_lo_up()` sau khi ghi uid_map, trước
    capdrop (D18: uid_map rỗng = 0 caps — đo được); in-sandbox unix-socket
    relay cho web (D16: fork trước capdrop/seccomp, tự capdrop, không exec,
    fork-per-conn, SIGCHLD auto-reap, teardown 3 lớp PDEATHSIG + pidns-1-exit
    + group-kill); seccomp thêm `socket(AF_NETLINK)` → EPERM (D15).
  - `tools/webproxy.py` (mới): host-side TCP 127.0.0.1:<port> → unix bridge
    (asyncio, half-close propagate, bind loopback-only); `minict.sh web` khởi
    động + lưu `proxy-pid`; `down` kill cả hai.
  - `tests/test_netns.sh` (mới, 7 case NS1–NS7; NS2 là control
    `MINICT_NO_NETNS=1` chứng minh NS1 đến từ netns mình); `test_isolation.sh`
    NET1 xfail → pass thật (NET1: chỉ còn lo, NET2: lo UP); `test_seccomp.sh`
    +SC17/SC18 (netlink block + control A/B) +SC19 (AF_INET vẫn OK); probe
    thêm `netlink`/`tcp`.
  - Root `Makefile`: target `test-netns`; `test` chạy 3 suite; **ghim lại fix
    tabs** (GNU Make 4.4.1 từ chối recipe 8-spaces — fix phiên 4 đã tuột khỏi
    main sau squash; runner make 4.3 nên CI không bắt được).
  - Fix nền: `minict.sh web` LUÔN ghi đè index.html theo instance (base rootfs
    untracked còn sót file test cũ làm NS6 fail giả).
  - Docs: decision-log D15–D18; research Phiên 5 (user_namespaces(7),
    network_namespaces(7), RootlessKit --port-driver); perf-baseline (+row);
    roadmap (netns done — DoD 5 nền tảng đủ); cve-watchlist (defense-in-depth
    netns); skills.md (7 pitfalls netns); README v0.2.
- **Perf:** perf-check PASS — cold_start **1.90 ms** (N=20) | rss **1032 kB** |
  disk **2.03 MB**; A/B netns N=40 interleaved: **+0.28 ms** (ON 1.80 vs OFF
  1.52) — tổng cộng dồn từ v0.1 ≈ 1.7–1.9 ms, dưới target ≤ 5 ms.

## 3. CVE Check Results (phiên 5 — evidence `journal/2026-11-17/cve/netns.{md,json}`)

- Kernel `5.10.134-013.15.kangaroo.al8.x86_64`; watchlist 16 CVE (NVD) —
  **0 CVE mới**. Không CVE critical chưa-vá ảnh hưởng trực tiếp minict.
- Netns + netlink-block chuyển mitigation của CVE-2024-1086 (nf_tables) và
  CVE-2026-53266 (ebtables KEV) từ "thiếu CAP_NET_ADMIN" thành "mất cả surface
  netlink từ trong sandbox" (SC17 đo được EPERM) — defense-in-depth ghi vào
  cve-watchlist.

## 4. No-Compromise Validation (phiên 5 — evidence `journal/2026-11-17/evidence/netns.txt`)

- `make test` (isolation 17 + seccomp 20 + netns 7): **44/44 PASS**
- `make test-web`: **6/6 PASS** — qua bridge unix-socket mới
- `make perf-check`: PASS (1.90 ms / 1032 kB / 2.03 MB)
- Host baseline: `/etc/passwd` `21d5de2d…d412bd` — khớp chính xác phiên trước
  (unchanged); `/bin/bash`/`/usr/bin/sudo` khớp lần ghi phiên 4; leaked
  processes: 0; listeners: chỉ 6 listener env-owned có trước.

## 5. Kế hoạch ngày mai

1. **Full web stack** (DoD tổng thể): nginx/caddy + SQLite trong 1 instance;
   mở rộng bridge D16 cho multi-service (port map per service); đo cold start
   toàn stack < 5s.
2. Issue #5 (escape sâu): thêm case cho mặt netns (netlink đã chặn — SC17 là
   evidence), fd-based chroot escape, setuid vs NNP, core dump leak.
3. Issue #8: re-check advisory al8 cho CVE-2026-31431/98432.
4. `cve-daily.yml` cron chạy 02:00 UTC — kiểm run đầu tiên trên main (workflow
   đã có từ phiên 3, chưa từng chạy schedule).

## 6. PR (phiên 5)

- **#11** — `chore(branches): add prompts/BRANCH_GATE_PROMPT.md (SBG system prompt)` —
  merged `a5128241`; CI main xanh (run 44018921211).
- **#12** — `feat(netns): CLONE_NEWNET + loopback-only + unix-socket web bridge
  (Closes #3)` — merged `18cb41e5`; CI main xanh (run 44020968892).
- Định nghĩa Done của phiên: SBG kiểm tra nhánh + gộp mọi PR đủ cổng ✓ |
  research ≥2 nguồn mới ✓ | test mới (netns 7 + SC17–19) qua `make test` ✓ |
  decision-log D15–D18 ✓ | perf đo + baseline cập nhật ✓ | CVE check + evidence ✓ |
  host baseline unchanged ✓ | daily-log + ledger commit qua PR ✓ |
  BRANCH_GATE_PROMPT.MD lên repo theo lệnh owner ✓

## Trạng thái phiên: DAY PASS

---

## 0. SBG — Branch Gate (chạy trước phiên, MODE=plan)

- Thu thập (SBG §2): origin chỉ có `main` @ 5dc367b; 0 PR mở; branch-plan + branch-report
  sinh và lưu tại docs/branch-plan-2026-11-17.md / docs/branch-report-2026-11-17.json.
- **Phát hiện halt-equivalent: CI main xanh GIẢ** — run #20 (44021359091) báo `success`
  nhưng log chứa `UID1 FAIL`, `SIG1 FAIL`, `summary: PASS=15 FAIL=2`, `make: Error 1`
  (exit code của `tee` vì không có pipefail); seccomp + netns không hề chạy trên CI.
- Owner chốt hướng xử lý (session 2026-11-17): sửa pipefail + SIG1 TRƯỚC netns/feature;
  align manifest ↔ roadmap ↔ PROMPT.MD theo hướng manifest.

## 1. Issue xử lý (phiên 6)

- **#14** (type:bug, area:ci, p0) — CI xanh giả: pipefail + SIG1/UID1 đỏ thật trên
  runner + seccomp/netns không chạy khi isolation đỏ → DONE qua PR #16.
- **#15** (type:chore, area:docs, p1) — align manifest/roadmap/PROMPT.MD → DONE qua PR #17.

## 2. Research Summary (phiên 6 — nguồn chính, ghi thêm `docs/skills.md` "CI truth")

- **GitHub Actions default shell**: step không khai `shell:` chạy `bash -e` KHÔNG
  pipefail → `cmd | tee` trả rc của `tee`. Khai `shell: bash` = `bash --noprofile
  --norc -e -o pipefail {0}` (docs.github.com workflow-syntax#defaultsrunshell).
  Nguồn 2: chính log run #20 in rõ `shell: /usr/bin/bash -e {0}`.
- **busybox applet NOFORK**: busybox-static Ubuntu 24.04 (1.36.1-6ubuntu3.1 — đúng
  bản runner `apt-get download`) chạy applet `sleep` NOFORK → `sh -c "sleep N"` KHÔNG
  có process con; Debian 1.37.0 fork bình thường. Tái hiện bằng cách tải đúng .deb
  Ubuntu, thay binary vào rootfs, probe `pgrep -P`. Nguồn 2: user_namespaces(7) —
  setgroups deny-lock (bắt buộc trước gid_map unprivileged) làm supplementary groups
  của host user chỉ hiển thị qua overflowgid 65534 → `id -G | grep -qx 0` fail trên
  runner (user runner có docker/sudo groups).

## 3. Development Progress (phiên 6)

- **PR #16 (`fix/14-ci-truth`)**: ci.yml + cve-daily.yml `defaults: run: shell: bash`;
  sig1_test NOFORK-aware (chấp nhận mp rỗng, vẫn kiểm shpid chết); uid1_test chấp nhận
  {0, 65534}, chặn gid lạ; `tests/run-suites.sh` chạy đủ 3 suite + exit code tổng hợp;
  test_web.sh preflight `ss`. Decision-log D1–D5.
- **PR #17 (`docs/15-…`)**: manifest viết lại (mục tiêu/users/non-goals/target
  platform/4 giai đoạn + DoD đo được); roadmap 4 giai đoạn + parked tách
  env/thật + pasta/slirp4netns thay tự-làm-proxy + gate research setns cho GĐ2;
  PROMPT.MD phân cấp nguồn sự thật, §8 trỏ roadmap, baseline 50 test. Decision-log DA1–DA6.
- **Perf (local, N=20)**: cold_start **1.79 ms** | rss **1108 kB** | disk **2.02 MB**
  — PASS mọi ngưỡng; docs-only/runtime-changed: không có thay đổi runtime.

## 4. CVE Check Results (phiên 6 — evidence `journal/2026-11-17/evidence/ci-truth.txt` §6)

- NVD API TIMEOUT từ sandbox phiên này (2 lần: 120s, 45s) — không có kết quả check mới.
- Watchlist 16 CVE giữ nguyên từ phiên 5 (0 CVE mới ảnh hưởng tại lần check thành công
  cuối). cve-daily cron trên runner là nguồn check độc lập; từ phiên này exit code
  của cve-daily cũng thật (pipefail).

## 5. No-Compromise Validation (phiên 6 — evidence `journal/2026-11-17/evidence/ci-truth.txt`)

- `make test` local: **44/44 PASS** (isolation 17 + seccomp 20 + netns 7, rc=0,
  đủ 3 suite luôn chạy) + runner-sim busybox Ubuntu: **17/17**.
- `make test-web`: **6/6 PASS**; `make perf-check`: PASS (1.79ms/1108kB/2.02MB).
- Host baseline: `/etc/passwd` `21d5de2dc39f2ceb…` khớp chính xác phiên 5; bash/sudo
  ghi-recall; 6 listeners cùng bộ env-owned — **unchanged**.
- **CI trên runner (ubuntu-24.04): 50/50 PASS với pipefail thật** — run #22 (PR #16)
  là lần đầu tiên CI chạy đủ 4 suite với exit code thật.

## 6. PR (phiên 6)

- **#16** — `fix(ci): real exit codes (pipefail) + runner-true SIG1/UID1 + run all
  suites (Closes #14)` — merged `2de34b4d`; CI PR xanh thật (run #22) + CI main xanh
  thật (run #23, 44023210555). Nhánh đã xoá (DELETE_MERGED, ledger có dòng).
- **#17** — `docs: align manifest/roadmap/PROMPT.MD — hierarchy, honest DoD, target
  platform (Closes #15)` — merged `83e2c8379`; CI main xanh thật (run #26, 44023790802).
  PR đụng PROMPT.MD (G7) — owner chỉ thị trực tiếp "Fix lại PROMPT MD … theo hướng
  manifest"; nhánh đã xoá (DELETE_MERGED, ledger có dòng).
- Định nghĩa Done của phiên: SBG plan trước ✓ | CI xanh THẬT trên main (pipefail +
  đủ suite) ✓ | SIG1/UID1 đúng môi trường runner (tái hiện + fix + runner-sim) ✓ |
  align 3 nguồn docs ✓ | decision-log (D1–D5, DA1–DA6) ✓ | perf đo + PASS ✓ |
  host baseline unchanged ✓ | evidence + daily-log + ledger qua PR ✓

## Kế hoạch ngày mai

1. GĐ1 roadmap mục 1 (mới): SIGTERM swallowed (#6) — lifecycle graceful stop ở tầng
   runtime trước khi control plane (GĐ2) cần nó.
2. GĐ1 mục 4: đánh giá pasta/slirp4netns A/B với bridge D16 (cài rootless trên runner).
3. CVE check lại với NVD (phiên này timeout) + kiểm run cve-daily cron đầu tiên.
4. Kiểm `/dev/kvm` trên runner GitHub (probe workflow) — precondition GĐ3.

---

# Phiên 8 — Alpine minirootfs (issue #23, GĐ1 mục 2)

## Issue xử lý
- #23 — feat(rootfs): Alpine minirootfs — status: done (PR chờ merge)
- Triage: #6 +label `status:in-progress` (PR #21 đang xử lý); #4/#5/#8 backlog
  p2 giữ nguyên; tạo label `area:rootfs`

## Research
- Alpine CDN v3.22 (dl-cdn.alpinelinux.org): pin `alpine-minirootfs-3.22.8-x86_64`
  (05-Nov-2026), CDN có `.sha256` per-file → verify bắt buộc (D20-1)
- pid_namespaces(7) (man7): mount procfs hợp lệ khi userns sở hữu pidns — nền
  cho AL4 env-aware
- "Bootstrapping Alpine Linux without root" (blog.brixit.nl 03/2024): apk.static
  --root --initdb + bind-mount dev + `--mount-proc`; dùng newuidmap+subuid —
  không áp dụng cho minict (manifest §5 cấm) → chiến lược ownership riêng (D20-2)
- Apptainer fakeroot docs: `FAKEROOTDONTTRYCHOWN=1` pattern — precedent cho
  "unprivileged extract không cố chown"; single-uid map = fakeroot tự nhiên

## Development
- Nhánh: `feat/23-alpine-rootfs`
- Thay đổi: `sandbox/mkrootfs-alpine.sh` (mới), `tests/test_alpine.sh` (mới,
  AL1–AL8), `run-suites.sh` +4th suite, Makefile (`rootfs-alpine`/`test-alpine`),
  `.gitignore`, docs (perf-baseline, cve-watchlist, roadmap), decision D20
- Đo nổi bật: apk add offline signature-verified rc=0 (AL5); ownership
  `0:0` ns ↔ `1001:1001` host (AL6); chown foreign-uid EPERM post-capdrop (AL7)
- Perf: cold_start=1.83 ms (busybox 1.88 cùng lúc), rss=1087 KB, disk=8.6 MB —
  PASS mọi ngưỡng; perf-check busybox baseline vẫn PASS (1.84/1140/2.02)

## CVE Check
- Kernel: 5.10.134-013.15.kangaroo.al8.x86_64
- CVE mới: 0 (16 CVE watchlist đối chiếu NVD — file `cve/alpine-rootfs.{md,json}`)
- Component mới apk-tools 2.14.13: 2 CVE cũ đã vá (2021-30139, 2021-36159),
  0 CVE mở; CVE-2026-39973 = Apktool (Android) KHÔNG phải apk-tools (D9 lần nữa)
- Ảnh hưởng sandbox: không (apk offline; online chờ mục 4)
- Hành động: watchlist thêm mục apk-tools, re-check khi bật egress

## Validation "không bị nhiễm"
- `make test`: PASS 52/52 (isolation 17 + seccomp 20 + netns 7 + **alpine 8**)
- `make test-web`: PASS 6/6
- Host baseline: passwd sha256 `21d5de2d…` khớp ngày 1 = unchanged; bash/sudo
  lần đầu ghi (không có record cũ để so — xem evidence); shadow EACCES như mọi phiên
- Bằng chứng: `journal/2026-11-17/evidence/alpine-rootfs.txt`, `alpine-perf.txt`,
  `host-baseline.txt`; script bench `bench-alpine-rss.py` dời sang `benchmarks/` (B10 audit)

## PR
- #24 — `feat(rootfs): alpine minirootfs 3.22.8 — rootless distro rootfs + apk offline (Closes #23)` — ready (không draft), CI xanh 3/3 trên HEAD 4d460cb (run 44040642447): ci + perf-guard PASS
- Merge: để SBG phiên sau xét cổng G1–G8 (giữ nguyên cơ chế branch-gate, không tự merge)

## Kế hoạch ngày mai
- GĐ1 mục 3 (issue #5): escape test sâu trên rootfs distro thật (`/proc` thật,
  nhiều mặt tấn công hơn busybox) — Alpine giờ có sẵn làm mồi
- SBG: gộp PR #21 (fix #6) nếu không còn commit < 2h + có retest/audit artifact

### ENV FACT bổ sung (sau lần chạy CI đầu của PR #24)
- Runner ubuntu-24.04 kernel `7.0.0-1017-azure` cũng **EPERM khi mount /proc
  trong pidns** (run 44039937935) — giả thuyết "runner 6.x/7.x cho phép" SAI.
  AL4 env-aware vẫn PASS (nhánh fail-soft). Root-cause runner (apparmor/lockdown/
  seccomp của azure?) → follow-up. Đã sửa D20-4 + roadmap cho khớp đo thật.

# Phiên 8b (review-fix theo SIA audit) — PR #24: pin sha256 + honest AL4/AL7

## Việc đã sửa (commit `fix(review): … (#24)`)
1. **[medium supply-chain]** pin sha256 3.22.8 trong script (3 nguồn độc lập
   chéo xác minh; CDN .sha256 giữ làm lớp transport); mutant bad-pin → rc=1.
2. **[test rỗng]** AL4 → xfail known-gap PROC1-pattern (D22-2), hết vacuous.
3. **[low]** AL7 assert EINVAL cụ thể (đo được "Invalid argument"); URL derive
   từ `ALPINE_VERSION`; `bench-alpine-rss.py` → `benchmarks/`; AL5 env-blocked
   không còn làm CI đỏ giả khi CDN outage.

## Kết quả chạy (clean clone f6e41e0, uid 1001)
- `make test` ×3: 17+20+7+8 PASS; web 6/6 ×3; perf 1.89/1032/2.02 PASS.
- alpine RSS median 1103 kB (busybox 1130, N=5) — không regression.
- sha256 pin: đo trùng pin trên 3 mirrors; bad-pin mutant ABORT rc=1.

## Còn lại
- Root-cause mount /proc (D20-4) — đề xuất thử mount proc trước umount old
  root; theo dõi bằng case AL4 tự-đỏ khi sửa. Ghi ở decision D22-2.
