# SBG Plan — 2026-11-26 (Phiên 33)

## MODE=plan (Pha A)

### Sync
- main @ `77a2f35` (Phiên 32 ledger #167). Pull ff-only OK.
- 0 PR mở. 9 issue mở (toàn p2/p3 from-audit + #8 CVE tracking p2).
  Không p0/p1 security chen ngang → Pha D.
- Kernel dev: 5.10.134 (cgroups v1, không Landlock native, không /proc userns).
  CI runner: ubuntu-24.04 kernel 6.x/7.x (có cgroups v2 delegation).

### Kế hoạch nhánh hôm nay
- Tạo: `feat/168-observability` (GĐ2 mục 7 — Log JSON + Prometheus metrics +
  /metrics endpoint, owner-directed, D68).
- KEEP: 0 nhánh cũ.
- DELETE_MERGED: sẽ dọn sau Pha C.
- Quota: 1 issue/ngày (§1 Bước 3). Phiên 33 = #168.

### Verdict mang sang (Pha C)
- Không áp dụng — đây là PR mới, không có SHA cũ.

### Lưu ý môi trường
- Mục 7c `/metrics` endpoint trên listener API sẵn có (TCP 127.0.0.1 +
  --unix-socket) — **yêu cầu cùng token** như /v1/*. Fail-closed
  `--metrics-listen 0.0.0.0` → exit ≠ 0 (chỉ loopback).
- MD42 metrics khớp cgroup env-gated như CG0–CG5 (SKIP khi không có cgroups
  v2 delegation — dev 5.10 sẽ SKIP, runner CI kernel 7.x PASS thật).
- Mục 7a log JSON: thay `slog.NewTextHandler` → `slog.NewJSONHandler`,
  giữ `--log-format text|json` mặc định json. KHÔNG ghi token/env-file.
