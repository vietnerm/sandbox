# SBG Branch Plan — 2026-11-19 (Phiên 1)

MODE=plan (Pha A). APPLY sẽ chạy ở Pha C và Pha E.

## Thu thập (nguồn sự thật tại thời điểm plan)
- `git fetch --prune`: remote chỉ còn **`main`** (187bfd2) — không nhánh nào khác.
- PR mở: **0** (REST `GET /pulls?state=open` → `[]`).
- CI `main`: run ci #44273346039 @ 187bfd2 = **success**; unicode-guard = success.
- Issue mở: #44 (chore/area:ci, p2, from-audit — cve-check.py KeyError 'cvss'),
  #34 (security p0, đã triage Phiên 2 2026-11-18 — giữ mở theo §6, 0 CVE ảnh
  hưởng sandbox), #31/#30/#29 (p3), #28/#27/#26 (docs/ci p1–p2), #8/#5 (security
  p2), #4 (feature p2, GĐ2 — luật D27 không chọn).
- Host baseline: sẽ đo ở Bước 7 (chưa có chỉ báo bất thường nào từ journal
  2026-11-18).

## Kế hoạch cho từng nhánh ≠ main
- (không có nhánh nào khác `main` ở local lẫn remote)

| Nhánh | Ahead/Behind | Quyết định plan-mode |
|---|---|---|
| — | — | — |

- DELETE_MERGED: (rỗng)
- MERGE: (rỗng)
- KEEP: (rỗng)
- DROP: (rỗng)

## Kết luận
- Repo đã ở trạng thái mục tiêu §11: chỉ `main`. Không có pha B/C để xử lý ở đầu
  chu trình này.
- §0.6 dừng: không áp dụng (CI main xanh, #34 đã triage, chưa có dấu hiệu host
  baseline đổi).
- Pha D hôm nay: issue **#5** (type:security p2, area:escape-test — mục 2 GĐ1
  trong roadmap: "chroot-escape test sâu hơn"). Nhánh dự kiến
  `sec/5-deep-escape`; PR mới sẽ đi lại Pha B → C trong lần chạy này.

---

# SBG Branch Plan + APPLY — 2026-11-19 (Phiên 3, Orchestrator §11)

MODE=plan ở Pha A → MODE=apply ở Pha C/E (ghi hợp nhất tại đây).

## Thu thập cuối phiên
- Nhánh ≠ main: **feat/3-pasta-egress** @ `47ce5d5` (PR #59, merged squash
  `01899ee` qua API `PUT /pulls/59/merge` match-head-corrected).
- PR mở sau merge: **0**; issue #57 auto-closed bởi merge.
- CI main sau merge @ `01899ee`: **success** (ci + unicode-guard).

## Quyết định theo nhánh
| Nhánh | Ahead/Behind | PR | CI | retest | audit | Quyết định | Lý do |
|---|---|---|---|---|---|---|---|
| feat/3-pasta-egress | 5/0 | #59 | xanh ×5 vòng CI (d3e6e43 đỏ→sửa, 2571734, a80a3a7, 676332b, 47ce5d5 xanh) | SCR PASS ×5 run + mutant M0/M-pin/M-version caught (comment PR) | SIA PASS (label audit:pass @ 47ce5d5) | MERGE → DELETE_MERGED | G1–G8 PASS; không G7; squash `01899ee`; CI main xanh sau merge |

- DELETE_MERGED: feat/3-pasta-egress (đã xoá remote + local sau khi PR MERGED
  khớp head `47ce5d5a`).
- MERGE: (đã thực hiện — #59)
- KEEP: (rỗng) — DROP: (rỗng)
- §0.6 dừng: không áp dụng (CI main xanh; #52/#34 đã triage 0 CVE ảnh hưởng;
  host baseline unchanged — evidence pasta-egress.txt).

## Kết luận
- Repo về trạng thái mục tiêu: chỉ **`main`**.
- Bài học mới ghi skills.md: flip draft→ready bằng REST PATCH no-op với PAT
  fine-grained → dùng GraphQL `markPullRequestReadyForReview` (đối chiếu
  precedent #47→#48 của Phiên 1).

---

# SBG Branch Plan + APPLY — 2026-11-19 (Phiên 5, ZAI — Orchestrator §11)

MODE=plan 06:26Z → MODE=apply 06:44Z (cửa sổ §0.4 30 phút đã qua cho 2 nhánh
đã merge; ghi hợp nhất tại đây).

## Thu thập (06:26Z)
- CI main @ `b5c7e9f`: ci #158 + unicode-guard #43 = success → không dừng vì CI.
- Issue p0 security #52/#34: auto cve-daily, đã triage Phiên 2 (19/19 hàng trùng
  watchlist, 0 CVE mới, 0 ảnh hưởng sandbox), giữ mở chờ owner `verified` (§6)
  → không áp dụng dừng §0.6 (tiền lệ Phiên 1/3).
- Nhánh ≠ main: 3 — chi tiết bảng dưới.

## Quyết định theo nhánh (plan 06:26Z → apply 06:44Z)
| Nhánh | Ahead/Behind | PR | CI | retest | audit | Quyết định | Lý do |
|---|---|---|---|---|---|---|---|
| chore/ledger-2026-11-19-p4 | 1/1 | #65 MERGED, head `dd86968` khớp squash `b5c7e9f` (cherry+=0) | n/a (PR đóng) | n/a | n/a | KEEP@plan (§0.4: commit 06:13Z) → **DELETE_MERGED** @apply | nội dung trọn trong main; xoá 06:44Z, không cần tag |
| fix/31-proc-pidns | 1/2 | #64 MERGED, head `d87a7ac` khớp squash `da429c6` (cherry+=0) | n/a | n/a | n/a | KEEP@plan (§0.4: commit 06:07Z; Phiên 4 ủy quyền apply) → **DELETE_MERGED** @apply | xoá 06:44Z hoàn tất ủy quyền; không cần tag |
| docs/issue-throughput | 1/2→2/3 | #63 OPEN | xanh trên head (`4afae47` run #160: build-test×2/perf/scan) | SCR @ `4afae4` (ZAI mode): PASS-trừ-WEB9-flaky + mutant M0/MG1/MG2 caught | SIA @ `4afae4`: **CONCERNS** (B6/B10/manifest stale; 2 low) | **KEEP** | G7 (workflows/PROMPT/manifest, `needs-human`) + concurrent writer @ `1a39b69` (06:41Z) sửa đủ việc medium → nhường vòng B4; không tự gộp |

## Thực thi apply (06:44Z)
- `git push origin --delete fix/31-proc-pidns` → OK
- `git push origin --delete chore/ledger-2026-11-19-p4` → OK
- Verifed `git ls-remote`: còn `main` + `docs/issue-throughput` (KEEP) ✓
- Không DROP → không archive tag nào cần tạo.

## Kết luận
- Repo về trạng thái mục tiêu §11: `main` + 1 nhánh KEEP (`docs/issue-throughput`,
  chờ owner merge PR #63 sau khi vòng B4 của phiên đồng thời có SCR/CI mới).
- WEB9 flaky (web suite positive-control race) phát hiện trong SCR — tái hiện trên
  main → mở issue `type:bug` riêng, không chặn PR #63.
- Sự kiện phối hợp: phiên Claude ghi nhánh #63 song song (§0.4) — phiên ZAI nhường
  B4, giữ bằng chứng SCR/SIA @ `4afae4` làm audit trail.

---

# SBG Branch Plan + APPLY — 2026-11-19 (Phiên 6, ZAI — tiếp nối chỉ thị owner)

MODE=plan → apply (hợp nhất tại đây). Bối cảnh: owner IM "đọc PROMPT.MD và làm
tiếp thay việc owner" → ủy quyền các việc owner-pending (review+merge #63 G7,
verified #31/#5, đóng #34/#52 — tiền lệ #16/#17).

## Thu thập (07:19Z)
- main `f1612ec` (PR #69 đã gộp bởi phiên Claude lúc 07:15Z, SCR/SIA của họ
  đầy đủ — ZAI không nhảy vào); PR #70 (ledger Claude) đang mở.
- PR #63 @ `e98a379`: đầy đủ G1–G6, G8 + SCR×2/SIA re-verdict PASS của Phiên 5,
  thiếu duy nhất G7 → chủ đề ủy quyền owner.
- CI main xanh mọi mốc; #34/#52/#31/#5 mở chờ owner.

## Quyết định theo nhánh (apply)
| Nhánh | PR | Quyết định | Lý do |
|---|---|---|---|
| docs/issue-throughput | #63 | MERGE → DELETE_MERGED | G7 via ủy quyền owner; B1 resync `eb1dbc2` (delta code #69) → SCR vòng 3 (minictd cục bộ ×3, mutant M-drain caught) + SIA PASS + comment phê duyệt G7 → squash `47023c3`; CI main xanh sau merge |
| fix/50-execstream-drain | #69 | MERGE (bởi Claude) → DELETE_MERGED | ZAI apply xoá nhánh 07:53Z ngoài cửa sổ §0.4 |
| chore/ledger-2026-11-19-claude-p2 | #70 | MERGE (bởi Claude) → DELETE_MERGED | ZAI apply xoá nhánh 07:53Z |
| fix/66-web9-wait-listen | #71 | MERGE → DELETE_MERGED | Pha D của phiên: WEB9 #66 (phát hiện Phiên 5) — D41; SCR/SIA PASS @ `74f501f`; squash `1a12688`; #66 đóng + follow-up #72 |

## Thực thi Pha E (07:53Z)
- Xoá 3 nhánh merged OK; `git ls-remote`: chỉ còn `main` `1a12688` ✓
- Owner-pending: verified + đóng #31, #5, #34, #52 (mỗi issue: bằng chứng
  PR/CI/SCR/SIA + ghi nhận ủy quyền) ✓
- CI main @ `1a12688`: ci + unicode-guard xanh ✓

## Kết luận
- Repo đạt trạng thái mục tiêu §11: chỉ `main`. 0 nhánh KEEP.
- Bài học: 
  - WEB9 flake = 2 cơ chế chồng nhau (slow-start async: đã sửa; burst rc=52
    env-level: đã hardening budget + theo dõi #72).
  - Phối hợp đa-agent: phiên Claude gộp #69/#70 độc lập — ZAI xác minh độc lập
    (M-drain mutant) rồi apply dọn nhánh, không đụng PR của nhau.
