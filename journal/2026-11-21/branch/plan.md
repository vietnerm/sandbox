# SBG branch plan — 2026-11-21 (MODE=plan, SCOPE=all)

Halted: **no** — main CI xanh (research-log #4, cve-daily #4, unicode-clean-daily #3
đều success @ `ffa3d3a`, 2026-11-21); không có issue `type:security` `priority:p0`
mở; host baseline sẽ kiểm lại ở Pha D trước merge.

## Thu thập

- Remote branches: chỉ `main` @ `ffa3d3a` (sau Phiên 14 remote đã chỉ còn main).
- Local branches: `main` @ `ffa3d3a` (up-to-date), `feat/97-g3-preview-routing` @ `e8ec847`.
- Open PRs: **0** → Pha B/C không có việc.

## Bảng quyết định

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| `feat/97-g3-preview-routing` (local) | 0/0 (cherry: +2 — squash-merge) | #98 MERGED, head_sha `e8ec847` = HEAD nhánh, squash `23cf249` | xanh @ e8ec847 (Phiên 14) | SCR PASS ×2 SHA | SIA PASS | **DELETE_MERGED** | squash-merge đã vào main qua PR #98 (issue #97 auto-close); remote nhánh đã xoá Phiên 14; bản local còn sót — xoá khi MODE=apply, không cần tag (nội dung đã trọn trong main) |

Ghi chú luật SBG §2: squash-merge khiến `git cherry` báo `+2` — đối chiếu PR MERGED
với `headRefOid` = HEAD nhánh (`e8ec847` = `e8ec847`) → coi là đã gộp (khớp).

## remaining_branches sau apply (kỳ vọng)

`main` (remote + local), không KEEP.

## branch-report.json (plan)

```json
{"date": "2026-11-21", "mode": "plan", "halted": false, "halt_reason": "",
 "branches": [{"name": "feat/97-g3-preview-routing", "head_sha": "e8ec847",
   "decision": "DELETE_MERGED", "reason": "squash-merged via PR #98 (23cf249), head sha match",
   "pr": 98, "issue": 97, "gates": {"G1": true, "G2": true, "G5": true},
   "archive_tag": "", "done": false, "error": ""}],
 "remaining_branches": ["main"], "main_ci_after": "success"}
```

---

## MODE=apply — kết quả thực thi (Phiên 15)

| branch | quyết định | kết quả |
|---|---|---|
| `feat/97-g3-preview-routing` (local) | DELETE_MERGED | xoá local (nội dung trọn main qua squash `23cf249`) — không cần tag |
| `sec/104-oci-unpack-traversal` (local+remote) | MERGE → DELETE_MERGED | PR #106 squash **`22831da`** match-head `92b56e0`; #104 auto-close; xoá local + remote (204) |

branch-report.json (apply):
```json
{"date": "2026-11-21", "mode": "apply", "halted": false, "halt_reason": "",
 "branches": [{"name": "feat/97-g3-preview-routing", "head_sha": "e8ec847",
   "decision": "DELETE_MERGED", "reason": "squash-merged Phiên 14 via PR #98", "pr": 98, "issue": 97,
   "gates": {}, "archive_tag": "", "done": true, "error": ""},
  {"name": "sec/104-oci-unpack-traversal", "head_sha": "92b56e0",
   "decision": "MERGE", "reason": "G1-G8 PASS, SCR PASS x2 SHA, SIA PASS, CI xanh",
   "pr": 106, "issue": 104, "gates": {"G1": true, "G2": true, "G3": true, "G4": true, "G5": true, "G6": true, "G7": true, "G8": true},
   "archive_tag": "", "done": true, "error": ""}],
 "remaining_branches": ["main"], "main_ci_after": "success"}
```

```
=== SBG REPORT — 2026-11-21 (apply) ===
Đã gộp     : 1  — #106
Đã xoá     : 2  (đã gộp từ trước: 1 — feat/97 local residue; sau merge: 1 — sec/104)
Giữ lại    : 0
Còn lại    : main
CI main    : xanh (`22831da`)
Dừng?      : không
=============================================
```

## Phiên 16 — SBG MODE=plan (2026-11-21)

| branch | ahead/behind | PR | CI | retest | audit | quyết định | lý do |
|---|---|---|---|---|---|---|---|
| (remote chỉ `main` sau `fetch --prune`) | — | — | success @ d4df1e3 (ci + unicode-guard) | — | — | — | 0 nhánh ngoài main |

- Local residue: `feat/97-g3-preview-routing` đã xoá Phiên 15 (ledger #107); local hiện chỉ `main`.
- 0 PR mở → Pha B/C trống → vào thẳng Pha D (Bước 2–8 §1).
- CI main xanh @ `d4df1e3`; không điều kiện dừng SBG §0.6 (0 issue p0 security mở; host baseline sẽ đo trong Pha D).
