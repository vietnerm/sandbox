# Branch Ledger — SBG (Sandbox Branch Gatekeeper), append-only

> 1 dòng/nhánh theo BRANCH_GATE_PROMPT.md §5. Cập nhật qua PR (không push thẳng main).
> Quyết định: MERGE | DELETE_MERGED | KEEP | DROP. Archive tag chỉ cần cho DROP nhánh
> còn commit chưa gộp; nhánh DELETE_MERGED không cần tag (nội dung đã nằm trọn trong main).

| date | branch | head_sha | decision | reason | pr | issue | archive_tag | restore |
|---|---|---|---|---|---|---|---|---|
| 2026-11-17 | feat/web-busybox-httpd | 6c198e309e9d2493389f19a655f6e38953ca2efb | MERGE → DELETE_MERGED | G1–G8 PASS (CI xanh trên đúng HEAD, mergeable=clean, owner duyệt merge trực tiếp); squash-merge qua API match-head-commit; CI main xanh sau merge (run 44013069212) | #9 | #1, #7, #8(refs) | (none — đã gộp trọn) | commit squash 9b6b83c1 trên main |
| 2026-11-17 | sandbox/daily-2026-11-17 | 919f9215cf934b336f03b5548cb21901425a7e94 | DELETE_MERGED | ancestor của main (`merge-base --is-ancestor` OK, 0 ahead/1 behind), không có PR đính kèm | - | - | (none — đã gộp trọn) | `git checkout 919f9215c` |
| 2026-11-17 | feat/2-seccomp-bpf | f49916a0 (head trước ledger-commit) | MERGE → DELETE_MERGED | G1–G8: PR #10 ready, CI (ci+perf-guard) xanh trên head f49916a + trên head cuối (ledger commit này); mergeable kiểm trước merge; squash qua API match-head-commit; CI main phải xanh sau merge mới xoá nhánh | #10 | #2 | (none — đã gộp trọn) | commit squash của PR #10 trên main |
| 2026-11-17 | chore/branch-gate-prompt | 23f7491e355f62a920579dcd78cda94c5bcbdbc8 | MERGE → DELETE_MERGED | G1–G8 PASS — docs-only, owner chỉ lệnh trực tiếp "đưa BRANCH_GATE_PROMPT.MD lên repo" (G7 human approval); build-test xanh trên đúng HEAD; squash qua API match-head-commit; CI main xanh sau merge (run 44018921211) | #11 | - | (none — đã gộp trọn) | commit squash a5128241 trên main |
| 2026-11-17 | feat/3-netns | 6268c901b7eddbeca225374aee0b6a3c36c86a0e | MERGE → DELETE_MERGED | G1–G8 PASS — G2 build-test×2+perf xanh trên đúng HEAD; G3 retest 44/44+6/6 cùng SHA (runner); G4 self-audit 0 concerns; G5 mergeable=clean; G6 Closes #3, test mới, D15–D18; G7 diff 19 files, 0 sensitive; G8 perf-guard xanh; squash qua API match-head-commit; CI main xanh sau merge (run 44020968892) | #12 | #3 | (none — đã gộp trọn) | commit squash 18cb41e5 trên main |
