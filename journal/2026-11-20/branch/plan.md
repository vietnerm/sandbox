# SBG Branch Plan — 2026-11-20 (Phiên 10, MODE=plan)

Đầu vào: MODE=plan, SCOPE=all, DATE=2026-11-20.

## Thu thập

- `git fetch --prune origin` — xong.
- Nhánh remote: **chỉ `main`** (`gh api .../branches`: main).
- PR mở: **0** (`gh pr list --state open`: rỗng).
- PR đã merge gần nhất: #83 (squash `282234a`), nhánh đã xoá (DELETE_MERGED, Phiên 9).
- CI `main`: **success** — run `ci` + `unicode-guard` @ `66a4f09` (2026-11-20T02:29Z).

## Bảng quyết định

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| (không có nhánh ≠ main) | — | — | — | — | — | — | remote chỉ còn `main` sau Phiên 9 |

## Điều kiện dừng (§0.6)

- CI `main` đỏ: **không** (success).
- Issue `type:security` `priority:p0` CVE critical chưa vá: **không** (không có issue p0; #8 là p2 track vendor backport).
- Host baseline báo thay đổi trong ngày: **không** (Phiên 9 ghi unchanged; sẽ kiểm lại ở Pha D Bước 7).

→ KHÔNG dừng. Chế độ plan: không gộp/xoá gì. Không có nhánh nào cần xử lý.

---

## Phiên 11 — SBG MODE=plan (Pha A, 2026-11-20)

- `git fetch --prune origin` — xong.
- Nhánh remote: **chỉ `main`**.
- PR mở: **0** (`gh pr list --state open`: rỗng) → Pha B/C không có việc.
- CI `main`: **success** @ `bf409b8` (ledger Phiên 10, run 2026-11-20T03:52Z).

## Bảng quyết định

| branch | ahead/behind | PR | quyết định | lý do |
|---|---|---|---|---|
| (không có nhánh ≠ main) | — | — | — | remote chỉ còn `main` sau Phiên 10 |

## Điều kiện dừng (§0.6)

- CI `main` đỏ: **không** (success).
- Issue `type:security` `priority:p0` mở: **không** (#8 là p2 track backport).
- Host baseline thay đổi: sẽ kiểm lại ở Pha D Bước 7.

→ KHÔNG dừng. Sang Pha D (việc mới). SBG MODE=apply chạy lại ở Pha E.

---

## Phiên 12 — SBG MODE=plan (Pha A, 2026-11-20)

- `git fetch --prune origin` — xong.
- Nhánh remote: **chỉ `main`** (REST `/branches`: `['main']`).
- PR mở: **0** (REST `/pulls?state=open`: rỗng) → Pha B/C không có việc.
- CI `main`: **success** @ `813afb1` (run `ci` + `unicode-guard` 2026-11-20).
- Host baseline đầu phiên: `/etc/passwd 21d5de2d…`, `/bin/bash 223fe856…`,
  `/usr/bin/sudo a59f393b…` — **3/3 khớp journal 2026-11-19 → UNCHANGED**.
- `gh` CLI không có sẵn máy này → REST API curl + PAT (như Phiên 9; PAT qua
  env, không echo vào log/journal).

## Bảng quyết định

| branch | ahead/behind | PR | quyết định | lý do |
|---|---|---|---|---|
| (không có nhánh ≠ main) | — | — | — | remote chỉ còn `main` sau Phiên 11 |

## Điều kiện dừng (§0.6)

- CI `main` đỏ: **không** (success @ 813afb1).
- Issue `type:security` `priority:p0` mở: **không** (#8 là p2 track backport).
- Host baseline thay đổi: **không** (3/3 khớp đầu phiên; kiểm lại Bước 7).

→ KHÔNG dừng. Sang Pha D (việc mới: GĐ2 mục 1 — daemon REST API hoàn thiện).
SBG MODE=apply chạy lại ở Pha C/E.

## Phiên 13 — SBG MODE=plan (Pha A, 2026-11-20)

- `git fetch --prune origin '+refs/heads/*:refs/remotes/origin/*' --tags` — xong.
- Nhánh remote: **chỉ `main`** (`git ls-remote --heads`: `main @ d3b0605`).
- Nhánh local ≠ main: `chore/ledger-12` @ `9c2e4d4`, `feat/91-daemon-service` @
  `b3c44ac` — cả hai PR đã MERGED với headRefOid **khớp HEAD nhánh** (#92 →
  squash `7803730`, #94 → `d3b0605`) → **DELETE_MERGED** (chỉ xoá local ở Pha E;
  remote đã sạch từ Phiên 12).
- PR mở: **0** (`gh pr list --state open`: rỗng) → Pha B/C lượt 1 không có việc.
- CI `main`: **success** @ `d3b0605` (run `ci` #44693206926 + `unicode-guard`
  #44693206785).
- Host baseline đầu phiên: `/etc/passwd 21d5de2d…`, `/bin/bash 223fe856…`,
  `/usr/bin/sudo a59f393b…` — **3/3 khớp evidence Phiên 12
  (`evidence/daemon-service.txt`) → UNCHANGED** (`/etc/shadow` không đọc được
  ở container uid 1001 — như các phiên trước, 3 file là tập đo được).
- Env: kernel 5.10.134, uid 1001; `gh` 2.63.2 đã cài (`/usr/local/bin/gh`,
  PAT qua `gh auth login --with-token`); Go 1.27.4 `~/.local/go` (từ Phiên 12);
  **PID 1 = `tini` (không systemd user manager)**; cgroups **v1** trên máy dev
  → việc cgroups v2 (CG1) sẽ CI-gated trên runner.

## Bảng quyết định

| branch | ahead/behind | PR | quyết định | lý do |
|---|---|---|---|---|
| chore/ledger-12 (local) | — | #94 MERGED | DELETE_MERGED | headRefOid khớp; remote đã xoá |
| feat/91-daemon-service (local) | — | #92 MERGED | DELETE_MERGED | headRefOid khớp; remote đã xoá |

## Điều kiện dừng (§0.6)

- CI `main` đỏ: **không** (success @ d3b0605).
- Issue `type:security` `priority:p0` mở: **không** (#8 là p2 track backport).
- Host baseline thay đổi: **không** (3/3 khớp đầu phiên).

→ KHÔNG dừng. Sang Pha D (việc mới — GĐ2, issue thấp nhất còn mở). SBG
MODE=apply chạy lại ở Pha C/E.
