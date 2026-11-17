# Branch Ledger — SBG (Sandbox Branch Gatekeeper), append-only

> 1 dòng/nhánh theo BRANCH_GATE_PROMPT.md §5. Cập nhật qua PR (không push thẳng main).
> Quyết định: MERGE | DELETE_MERGED | KEEP | DROP. Archive tag chỉ cần cho DROP nhánh
> còn commit chưa gộp; nhánh DELETE_MERGED không cần tag (nội dung đã nằm trọn trong main).

| date | branch | head_sha | decision | reason | pr | issue | archive_tag | restore |
|---|---|---|---|---|---|---|---|---|
| 2026-11-17 | feat/web-busybox-httpd | 6c198e309e9d2493389f19a655f6e38953ca2efb | MERGE → DELETE_MERGED | G1–G8 PASS (CI xanh trên đúng HEAD, mergeable=clean, owner duyệt merge trực tiếp); squash-merge qua API match-head-commit; CI main xanh sau merge (run 44013069212) | #9 | #1, #7, #8(refs) | (none — đã gộp trọn) | commit squash 9b6b83c1 trên main |
| 2026-11-17 | sandbox/daily-2026-11-17 | 919f9215cf934b336f03b5548cb21901425a7e94 | DELETE_MERGED | ancestor của main (`merge-base --is-ancestor` OK, 0 ahead/1 behind), không có PR đính kèm | - | - | (none — đã gộp trọn) | `git checkout 919f9215c` |
