# journal/ — tài liệu theo ngày

Mỗi ngày làm việc có **một thư mục** `journal/YYYY-MM-DD/` (ngày UTC lúc bắt đầu
chu trình). Tên file bên trong **không chứa ngày**. Quy tắc gốc nằm ở `PROMPT.MD §10`.

```
journal/YYYY-MM-DD/
  daily-log.md            nhật ký ngày (template PROMPT.MD §4), mỗi phiên một mục
  research.md             research của ngày
  bench.md                benchmark của ngày (nếu có đo)
  decisions/<slug>.md     quyết định D<n>, slug = slug nhánh/issue
  evidence/<slug>.txt     log test/validation thô
  cve/<slug>.md|.json     kết quả CVE check
  branch/plan.md          kế hoạch gộp/bỏ nhánh của SBG
```

`evidence/` và `cve/` là log thô: không sửa nội dung sau khi commit (kể cả đường dẫn cũ in trong log).

Không theo ngày (tài liệu sống) → `docs/`: `roadmap.md`, `cve-watchlist.md`,
`perf-baseline.md`, `skills.md`, `branch-ledger.md`. Hướng dự án → `idea/manifest.md`.
Prompt agent → `PROMPT.MD` (SDDA) và `prompts/` (SIA, SCR, SBG).

## Tìm nhanh

| Cần tìm | Lệnh |
|---|---|
| Mọi quyết định | `ls journal/*/decisions/` |
| Quyết định D16 | `grep -rn '^## D16' journal/` |
| Evidence netns | `ls journal/*/evidence/netns*` |
| CVE theo ngày | `ls journal/*/cve/` |
| Hôm qua làm gì | `cat journal/$(date -u -d yesterday +%F)/daily-log.md` |

## Mã quyết định

`D<n>` đánh số tiếp nối trong toàn bộ `journal/`. Số tiếp theo = số lớn nhất hiện có + 1
(`grep -rhoE '^## D[0-9]+' journal/ | sort -t D -k2 -n | tail -1`).
Lưu ý lịch sử: ngày 2026-11-17, `decisions/poc.md` và `decisions/ci-truth.md` cùng dùng
D1–D5. Khi trích dẫn các mã này, ghi kèm file (vd. `ci-truth D2`). Mã mới bắt đầu từ D19.

## Ánh xạ từ cấu trúc cũ (trước 2026-11-17)

| Cũ | Mới |
|---|---|
| `docs/daily-log-YYYY-MM-DD.md` | `journal/YYYY-MM-DD/daily-log.md` |
| `research/YYYY-MM-DD.md` | `journal/YYYY-MM-DD/research.md` |
| `benchmarks/YYYY-MM-DD.md` | `journal/YYYY-MM-DD/bench.md` |
| `docs/decision-log/YYYY-MM-DD.md` | `journal/YYYY-MM-DD/decisions/poc.md` |
| `docs/decision-log/YYYY-MM-DD-<slug>.md` | `journal/YYYY-MM-DD/decisions/<slug>.md` |
| `tests/evidence/YYYY-MM-DD-test-output.txt` | `journal/YYYY-MM-DD/evidence/isolation.txt` |
| `tests/evidence/YYYY-MM-DD-web-output.txt` | `journal/YYYY-MM-DD/evidence/web.txt` |
| `tests/evidence/YYYY-MM-DD-<slug>.txt` | `journal/YYYY-MM-DD/evidence/<slug>.txt` |
| `tests/evidence/cve-YYYY-MM-DD-<slug>.md/.json` | `journal/YYYY-MM-DD/cve/<slug>.md/.json` |
| `BRANCH_GATE_PROMPT.md` | `prompts/BRANCH_GATE_PROMPT.md` |
