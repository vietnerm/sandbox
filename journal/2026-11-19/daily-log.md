# Daily Log — 2026-11-19

## Phiên 2 — Orchestrator §11 (ZAI, chu trình tăng tốc theo chỉ thị owner)

> Owner chỉ thị: "đẩy nhanh tiến độ để kịp qua giai đoạn 2, làm nhiều hơn 1
> ngày nhiều hơn 1 xíu" → override luật §1 Bước 3 (1 issue/ngày) — ghi nhận
> như dữ liệu owner-level (tương tự D27). Mọi PR vẫn đi ĐỦ cổng §11 B→C.

### Issue xử lý
- **#44** (p2 chore, from-audit) — status: done (merge #53; issue auto-close
  khi merge — type:chore, §6 không cản).
- **#5 residual** — "chạy lại cả suite trên rootfs distro thật" — status: done
  (merge #54; issue #5 vẫn mở theo §6 chờ owner `verified`).
- **#52** (p0 security, auto cve-daily) — triage cùng ngày: bảng 19 hàng trùng
  khớp watchlist, 0 CVE mới ngoài watchlist, 0 ảnh hưởng sandbox — comment
  chi tiết + **giữ mở** theo §6 (như #34). Mở chore **#56** (cve-daily chỉ
  mở issue khi CVE ngoài watchlist).
- **#55** mới (p2 bug, area:ci) — flake rootfs-alpine thiếu empty-dir 2/4
  build; mitigation trong #54, root-cause cần instrumentation.
- **#57** mới (p1 feature, area:netns) — pasta egress prototype CI-gated
  (GĐ1 mục 3), theo D34.

### Research (D34 — pasta)
- Đo được: máy dev **không có `/dev/net/tun`** → pasta/slirp4netns env-blocked
  dev; chỉ runner CI có (phải probe). Seccomp side OK (AF_INET = SC19).
- Thiết kế đầy đủ trong `decisions/pasta-connectivity.md`: binary pin, flow
  `--netns /proc/<shim>/ns/net`, NET10-12 env-gated, A/B perf tách.
- Quyết định: KHÔNG PR code pasta hôm nay (test chỉ giá trị trên CI → rủi ro
  main đỏ blind); issue #57 cho phiên kế.

### Development (2 merge)
- **PR #53 → `07d016e`** (fix/44): cve-check.py defensive (`parse_nvd` +
  `render_cvss` error-first); suite tool offline 4 case (CV1 repro #44, CV2
  malformed, CV3 V30-fallback, CV4 control cũ-KeyError); mutant caught.
  **LỖI QUY TRÌNH: merge thiếu SCR/SIA** (phiên tăng tốc) — khắc phục post-hoc
  audit sau-merge (PASS, comment PR + label), ghi daily-log + skills.md.
- **PR #54 → `c2b9f0e`** (test/5-escape-alpine): `ISOLATION_ROOTFS` env +
  wrapper + suite thứ 5 (29 case trên alpine); CHROOT1 applet-explicit
  (vacuous-rc127 class mới); flake #55 mitigation (mkdir vô điều kiện +
  self-heal); B1 resync main 07d016e (conflict run-suites union) — sự cố
  conflict-marker sót 1 commit, sửa + `bash -n` checklist. Đủ cổng: SCR ×2
  (98/98 + mutant trên alpine) + SIA PASS (label audit:pass).

### CVE Check
- Sáng (Bước 6 Phiên 1): rc=0, 19 hàng, 0 mới — `cve/deep-escape.md|.json`.
- cve-daily runner (02:50 UTC) mở **#52** — triage: trùng watchlist, giữ mở.
- NVD live OK cả hai lần (không tái hiện crash #44 — fix #53 dùng được).

### Validation "không bị nhiễm"
- `make test` ×2 từ rootfs-alpine sạch (sau mitigation): **98/98**
  (isolation 29 + seccomp 21 + netns 7 + alpine 8 + escape-alpine 29 +
  cve-check 4); SCR clean-room ×2 tại 5d1d0af; `make test-web` 9/9.
- Perf: 1.86 ms / 1160 kB / 2.04 MB — PASS (không đổi runtime).
- Host baseline: unchanged (sha256 khớp); listeners cùng tập nền.

### PR
- #53 — merged `07d016e` — CI xanh — thiếu cổng khi merge → post-hoc audit ✓
- #54 — merged `c2b9f0e` — CI xanh — đủ cổng ✓
- #51 (addendum Phiên 1) — merged trước phiên.

### Kế hoạch ngày mai (GĐ2 gate)
- **#57 pasta prototype** (GĐ1 mục 3) — PR `feat/3-pasta-egress` theo D34:
  probe tun, binary pin, NET10-12, A/B. GĐ1 mục 4 (web stack nginx offline,
  AL7 pre-bake ownership) là ứng viên song song.
- Owner: tick manifest DoD (rootfs thật ✓ escape suite ✓ graceful stop ✓ —
  manifest là G7, agent không tự gộp); review `verified` cho #5/#34/#52.
- #50 MD13 flake + #55 alpine flake: instrumentation khi có dư thời gian.
