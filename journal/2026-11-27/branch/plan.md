# SBG Plan — 2026-11-27 (Phiên 34)

## MODE=plan (Pha A)

### Sync
- main @ `4e17305` (Phiên 33 ledger #170). Pull ff-only OK.
- 0 PR mở. 9 issue mở (toàn p2/p3 from-audit + #8 CVE tracking p2).
  Không p0/p1 security chen ngang → Pha D.
- Kernel dev: 5.10.134 (cgroups v1, không overlay userns, không /dev/net/tun).
  CI runner: ubuntu-24.04 kernel 6.x/7.x (có cgroups v2 delegation + overlay + pasta).

### Kế hoạch nhánh hôm nay
- Tạo: `feat/171-muc8-rss-leak` (GĐ2 mục 8 — daemon RSS ≤ 30 MB + destroy no-leak,
  owner-directed, D69).
- KEEP: 0 nhánh cũ.
- DELETE_MERGED: sẽ dọn sau Pha C.
- Quota: 1 issue/ngày (§1 Bước 3). Phiên 34 = #171.

### Verdict mang sang (Pha C)
- Không áp dụng — đây là PR mới.

### Lưu ý môi trường
- MD53 destroy no-leak: dev 5.10 test busybox rootfs path (no overlay/pasta).
  CI runner kernel 7.x cũng test cùng path (overlay mounts nằm trong shim ns,
  không trong daemon mountinfo → same logic). Không env-gated.
- MD54 daemon RSS ≤ 30 MB: đo trên mọi host có /proc → không env-gated.
  Dev baseline 6372 KB (~6.2 MB), threshold 30 MB.
