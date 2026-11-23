# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 20)

Chu trình: Phiên 20 (SDDA, issue #124 / GAPS S6 — webport collision detection, GĐ2 mục 1).

## Trạng thái đầu chu trình

- Pull main `cdb0e56` (ff, ledger Phiên 19 qua PR #123 squash `cdb0e56`); working
  tree clean; local chỉ `main` (nhánh `sec/121-s5-rlimits-exec` đã xoá cuối Phiên 19).
- PR mở: **0** (REST API `pulls?state=open` rỗng) → Pha B/C đầu chu trình không có
  việc; PR mới của Phiên 20 đi Pha B→C ngay trong lần chạy này.
- Issue mở: 17 — không có `type:security priority:p0/p1` nào chen ngang (#8 p2 track
  vendor backport). Tạo #124 (S6, type:security p1) theo GAPS thứ tự (S1→S6, S6 cuối
  cùng sau S5) + kế hoạch "ngày mai" Phiên 19.

## Kế hoạch DELETE_MERGED / KEEP

- Không nhánh local lẻ; nhánh làm việc `sec/124-s6-webport-alloc` sẽ DELETE_MERGED
  sau squash. Không nhánh KEEP mới.

## Kế hoạch gộp trong ngày

- PR duy nhất hôm nay: `sec/124-s6-webport-alloc` (issue #124) — đi Pha B→C ngay khi
  mở (draft → CI xanh → SCR → SIA → flip ready → squash `--match-head-commit`).
  Ledger cuối ngày qua PR `chore(branches): ledger 2026-11-23 Phiên 20`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 21)

Chu trình: Phiên 21 (SDDA, issue #127 / GĐ2 mục 2 — template).

## Trạng thái đầu chu trình

- Pull main `2d18aeb` (ff, ledger Phiên 20 qua PR #126 squash `2d18aeb`); working tree
  clean; local chỉ `main` (nhánh `sec/124-s6-webport-alloc` đã xoá cuối Phiên 20).
- PR mở: **0** (REST API rỗng) → Pha B/C đầu chu trình không có việc; PR mới đi Pha B→C ngay.
- Issue mở: 17 — không có `type:security priority:p0/p1` chen ngang. GĐ2 mục 1 S-series
  done (Phiên 20); còn systemd-real (env-gated) + #109 (chờ maintainer) — cả hai blocked.
  Move tới mục 2. Tạo #127 (template, type:feature p2) theo §8 "mục thấp nhất chưa xong".

## Kế hoạch DELETE_MERGED / KEEP

- Nhánh `feat/127-template` DELETE_MERGED sau squash. Không KEEP mới.

## Kế hoạch gộp trong ngày

- PR duy nhất: `feat/127-template` (issue #127) — Pha B→C (draft → CI → SCR → SIA →
  flip ready → squash `--match-head-commit`). Ledger qua PR `chore(branches): ledger
  2026-11-23 Phiên 21`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 22)

Chu trình: Phiên 22 (SDDA, issue #130 / GĐ2 mục 3 sub — snapshot/backup/restore).

## Trạng thái đầu chu trình

- Pull main `2634286` (ff, ledger Phiên 21 qua PR #129 squash `2634286`); local chỉ `main`.
- PR mở: **0** (REST API rỗng) → Pha B/C đầu chu trình rỗng → vào thẳng Pha D.
- Issue mở: 17 — không `type:security priority:p0/p1`. GĐ2 mục 1 (S-series done + residuals
  blocked), mục 2 (done Phiên 21). Move tới mục 3. Tạo #130 (snapshot, type:feature p2) —
  mục 3 sub (snapshot/backup/restore); overlayfs (phần a) env-gated kernel 5.10, deferred.

## Kế hoạch DELETE_MERGED / KEEP

- `feat/130-snapshot` DELETE_MERGED sau squash. Không KEEP mới.

## Kế hoạch gộp trong ngày

- PR `feat/130-snapshot` (issue #130) — Pha B→C (draft → CI → SCR → SIA → flip ready →
  squash `--match-head-commit`). Ledger qua PR `chore(branches): ledger 2026-11-23 Phiên 22`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 23)

Chu trình: Phiên 23 (SDDA, issue #117 / mountinfo truncation fail-closed).

## Trạng thái đầu chu trình

- Pull main `a5b83fd` (ff, concurrent activity #132-134 + ledger Phiên 22 qua #135);
  local chỉ `main`.
- PR mở: **0** → Pha B/C rỗng → Pha D.
- Issue mở: 17 — không security p0/p1. GĐ2 mục 1-3 done (snapshot Phiên 22); mục 4-7
  env-gated (overlayfs/port-forward/Landlock/systemd-real all need kernel ≥5.11/Go/pasta).
  #117 (from-audit Phiên 17, C fix in minict.c) — actionable + locally testable → chọn.

## Kế hoạch DELETE_MERGED / KEEP

- `fix/117-mountinfo-truncate` DELETE_MERGED sau squash. Không KEEP mới.

## Kế hoạch gộp trong ngày

- PR `fix/117-mountinfo-truncate` (issue #117) — Pha B→C (draft → CI → SCR → SIA →
  flip ready → squash). Ledger qua PR `chore(branches): ledger 2026-11-23 Phiên 23`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 24)

Chu trình: Phiên 24 (SDDA — Pha B: PR #142, #140; Pha D: issue #139 seccomp allowlist).

## Trạng thái đầu chu trình

- Pull main `6a21582` (ff, +77 commit từ phiên trước); local chỉ `main`.
- PR mở: **2** — #142 `sec/cve-kernel-ranges` (CI xanh @ 1bb5913), #140
  `docs/security-boundary` (CI xanh @ 1704805). Cả hai đụng file G7
  (`.github/workflows/**` / `idea/manifest.md`) → theo luật §11: retest/audit/sửa
  nhưng **không tự gộp** → `needs-human`.
- Nhánh remote đã merge (squash, SHA khớp, chờ DELETE_MERGED ở Pha E):
  `docs/26-27-check-docs` (#136), `docs/gd2-observability` (#137),
  `docs/manifest-phase2` (#132), `fix/55-alpine-empty-dirs` (#134),
  `fix/75-web-free-port` (#133).
- Issue mở: 14; #139 `type:security` `priority:p1` (seccomp allowlist GĐ2 mục 3a,
  owner chốt ưu tiên trước mục 4–8) → chọn cho Pha D.

## Kế hoạch Pha B (thứ tự §11: sec → docs)

1. PR #142: B1 merge main (behind 2) → CI lại → B2 SCR → B3 SIA → B4 verdict.
   G7 → `needs-human`, không gộp.
2. PR #140: B1 merge main (behind 2) → CI lại → B2 SCR (docs-only) → B3 SIA → B4.
   G7 → `needs-human`, không gộp.

## Kế hoạch gộp trong ngày

- Không PR nào tự-gộp được (G7). Sau Pha B: cả hai KEEP `needs-human`.
- Ledger qua PR `chore(branches): ledger 2026-11-23 Phiên 24`.

## Kế hoạch DELETE_MERGED / KEEP

- DELETE_MERGED: 5 nhánh trên (sau khi xác minh lại SHA tại thời điểm apply).
- KEEP: `sec/cve-kernel-ranges`, `docs/security-boundary` (`needs-human`).
- PR mới của Phiên D (seccomp allowlist): qua Pha B→C ngay trong phiên.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 25)

Chu trình: Phiên 25 (SDDA, issue #144 / B6 retroactive research + B3 note).

## Trạng thái đầu chu trình

- Pull main `1f78f2f` (owner concurrent: Phiên 24 #139 seccomp allowlist merged #145);
  local chỉ `main`.
- PR mở: **0** → Pha B/C rỗng → Pha D.
- Issue mở: from-audit #144 (B6 research cho PR #142 owner) — docs, actionable, no env-gate.
  GĐ2 mục 4-7 env-gated (overlayfs/Landlock kernel ≥5.11, port-forward pasta, systemd-real).
  #30 (signal race) hard-to-test. #144 là clean docs fix → chọn.

## Kế hoạch DELETE_MERGED / KEEP

- `chore/144-research-b6` DELETE_MERGED sau squash. Không KEEP mới.

## Kế hoạch gộp trong ngày

- PR `chore/144-research-b6` (docs) — Pha B→C (draft → CI → SIA delta → squash). Ledger gộp
  vào PR này (docs-only, precedent Phiên 17-23 ledger).

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 26)

Chu trình: Phiên 26 (SDDA, issue #149 / GĐ2 mục 6 Landlock ro-root, CI-only).

## Trạng thái đầu chu trình

- Pull main `f48028d` (ff, ledger Phiên 25 qua #148); local chỉ `main`.
- PR mở: **0** → Pha B/C rỗng → Pha D.
- Issue mở: không security p0/p1. GĐ2 mục 4-7 env-gated; owner đang active (#139 seccomp
  allowlist Phiên 24). Tạo #149 (Landlock mục 6, CI-only kernel ≥5.13) theo chỉ thị user.

## Kế hoạch DELETE_MERGED / KEEP

- `feat/149-landlock` DELETE_MERGED sau squash. Không KEEP.

## Kế hoạch gộp trong ngày

- PR `feat/149-landlock` (issue #149) — Pha B→C (draft → CI → SCR → SIA → squash). Ledger
  qua PR `chore(branches): ledger 2026-11-23 Phiên 26`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 27)

Chu trình: Phiên 27 (SDDA, issue #30 / signal race fork→g_child_pid + stress test CI).

## Trạng thái đầu chu trình

- Pull main `9ff9539` (ff, ledger Phiên 26 #151); local chỉ `main`.
- PR mở: **0** → Pha B/C rỗng → Pha D.
- Theo chỉ thị user: #30 (signal race + stress test CI). Assign #30 + in-progress.

## Kế hoạch DELETE_MERGED / KEEP

- `fix/30-signal-race` DELETE_MERGED sau squash. Không KEEP.

## Kế hoạch gộp trong ngày

- PR `fix/30-signal-race` (issue #30) — Pha B→C (draft → CI → SCR → SIA → squash). Ledger
  qua PR `chore(branches): ledger 2026-11-23 Phiên 27`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 28)

Chu trình: Phiên 28 (SDDA, issue #154 / Landlock fail-closed + default policy).

## Trạng thái đầu chu trình

- Pull main `d81df65` (ff, ledger Phiên 27 #153); local chỉ `main`.
- PR mở: **0** → Pha B/C rỗng → Pha D.
- Theo chỉ thị user: Landlock fail-closed (spec.ReadOnly + no Landlock → die) + default
  policy (mọi instance). Tạo #154. Overlayfs/port-forward/RSS = Phiên sau.

## Kế hoạch DELETE_MERGED / KEEP

- `feat/154-landlock-default` DELETE_MERGED sau squash. Không KEEP.

## Kế hoạch gộp trong ngày

- PR `feat/154-landlock-default` (issue #154) — Pha B→C (draft → CI → SCR → SIA → squash).
  Ledger qua PR `chore(branches): ledger 2026-11-23 Phiên 28`.

---

# SBG — Branch Gate plan 2026-11-23 (MODE=plan, Pha A — Phiên 29)

Chu trình: Phiên 29 (SDDA, issue #157 / Landlock ptrace-scope + fail-closed complete).

## Trạng thái đầu chu trình
- Pull main `8127c17` (ledger Phiên 28 #156); local chỉ `main`.
- PR mở: **0** → Pha B/C rỗng → Pha D.
- #157 (p1 security): fail-closed chưa trọn + PROC2 ptrace scope (default revert oan).
  Fix: áp Landlock ở SHIM (shim+payload same domain) + all-die readonly + ABI probe +
  default re-enabled. Assign #157 + in-progress.

## Kế hoạch DELETE_MERGED / KEEP
- `fix/157-landlock-ptrace-scope` DELETE_MERGED sau squash. Không KEEP.

## Kế hoạch gộp
- PR `fix/157-landlock-ptrace-scope` (issue #157) — Pha B→C. Ledger qua PR
  `chore(branches): ledger 2026-11-23 Phiên 29`.
