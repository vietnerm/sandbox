
> MODE=plan ghi lúc 04:55Z; MODE=apply 05:17–05:20Z cùng lần chạy (§11).
> main = 81b5dce2 (run #77 ci: success — không halt §0.6).

## Nhánh remote (plan)

| branch | ahead/behind | PR | CI @ HEAD | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| ci/unicode-guard | 1 / 1 | #36 (open, ready) | scan+build-test×2+perf success @ 0023b97 | SCR @ 7685b3f sau B1 (63/63 ×3) | SIA CONCERNS @ 7685b3f | **KEEP** | G7: diff đụng `.github/workflows/**` + `PROMPT.MD` → không tự gộp; label `needs-human`; B1 sync đã làm (86030d3, fix tab 7685b3f); SCR/SIA comment trên PR |
| feat/minictd-control-plane | 2 / 1 | #35 **MERGED** | n/a | n/a | n/a | **DELETE_MERGED** | squash-match: PR state=closed+merged=true, headRefOid `8287246` == HEAD nhánh (SBG §2) |

## Nhánh local (plan — housekeeping)

| branch | trạng thái | quyết định |
|---|---|---|
| chore/branch-gate-prompt | đã gộp (squash a512824) | DELETE_MERGED (local) |
| chore/daily-log-session5 | đã gộp | DELETE_MERGED (local) |
| feat/3-netns | đã gộp (squash 18cb41e) | DELETE_MERGED (local) |

## Apply (MODE=apply, 05:17–05:20Z)

| branch | quyết định cuối | thực thi | kết quả |
|---|---|---|---|
| feat/minictd-control-plane | DELETE_MERGED | `git push origin --delete feat/minictd-control-plane` | xoá thành công 05:17Z (PR #35 squash-match; commit cuối 04:35:06Z — ngoài cửa sổ 30' §0.4) |
| ci/unicode-guard | KEEP (G7 needs-human) | không gộp, không xoá | PR #36 mở, labels `audit:concerns` + `needs-human`; việc còn lại: owner review |
| chore/branch-gate-prompt, chore/daily-log-session5, feat/3-netns (local) | DELETE_MERGED | `git branch -d` | xoá local thành công |

## Ghi chú

- Issue #34 (CVE p0, auto) đã được đối chiếu với watchlist lúc Pha A — mọi CVE
  trong báo cáo đều có dòng "không với tới"/host-side đã ghi trong `docs/cve-watchlist.md`;
  xử lý triage trong Pha D.
- PR #36 gặp sự cố trong B1: commit sync 86030d3 chuẩn hoá TAB Makefile → CI đỏ
  "missing separator"; tác giả PR fix 7685b3f (byte-identical với fix cục bộ của
  SCR-runner; fix cục bộ chưa push đã bỏ). Bài học vào `docs/skills.md`.
- File `prompts/BRANCH_GATE_PROMPT.md` đã có trên `main` (88af156) theo lệnh owner
  phiên trước — đã xác minh, không cần hành động.

---

# SBG Branch Plan — 2026-11-18 (RUN: orchestrator Phiên 3)

> MODE=plan lúc 05:47Z (trước Pha D); MODE=apply 06:15–06:30Z cùng lần chạy.
> main đầu phiên = 69a94d0 (CI run trên main: success — không halt §0.6);
> main cuối phiên = 9560cfe sau merge PR #42 (CI main: success).

## Nhánh remote (plan → apply)

| branch | ahead/behind (đầu phiên) | PR | CI @ HEAD | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| ci/unicode-guard | 1 / 0 (sau B1: 5/0) | #36 (open, ready) | scan+build-test×2+perf success @ d50bc22 | SCR ×2 @ d50bc22 (56+6+13 PASS; minictd từ CI) | SIA delta PASS @ d50bc22 → label audit:pass | **KEEP** | G7 không đổi (diff gốc đụng `.github/workflows/**` + `PROMPT.MD`) → `needs-human` chờ owner review; B1 resync + SCR/SIA đã làm trong phiên |
| sec/s1-webproxy-symlink | (mới trong phiên — Pha D) | #42 (open, draft→ready) | build-test×2+perf success @ 4104330 | SCR ×2 @ 4104330 (56+9 PASS) | SIA PASS 11/11 @ 4104330 (audit:pass) | **MERGE → DELETE_MERGED** | G1–G8 đầy đủ, không G7; squash match-head-commit 4104330 → main 9560cfe; CI main xanh sau merge |

## Nhánh local (apply)

| branch | quyết định | kết quả |
|---|---|---|
| sec/s1-webproxy-symlink | DELETE_MERGED (squash-match: PR #42 headRefOid 4104330 == HEAD) | xoá local `git branch -D` sau xác minh |
| chore/ledger-2026-11-18 | DELETE_MERGED (squash-match: PR #40 headRefOid da5282a == HEAD) | xoá local |

## Verify cuối (MODE=apply)

- Remote branches: `main` + `ci/unicode-guard` (KEEP) — đúng kỳ vọng §6 SBG.
- `git log main`: 9560cfe (sec #42) trên 69a94d0; CI main: success.
