# SBG Plan — 2026-11-26 (Phiên 40)

## MODE=plan (Pha A) → MODE=apply (trước Pha D, theo chỉ thị owner "hoàn thành SBG trước")

### Sync
- main @ `e36ab77` (Phiên 39 Track M M1+M2 + roadmap M2 residuals d–i, PR #201).
- 0 PR mở. 2 issue mở (#99 p3 chore from-audit, #8 p2 CVE tracking). Không p0/p1 security → không dừng (§0.6).
- CI main: `ci` + `unicode-guard` success @ `e36ab77` → cổng mở.

### Máy chạy SBG
- Máy mới khác máy các phiên trước (kernel xem `uname -r` trong evidence). Host baseline
  không có trên máy này → **thiết lập baseline mới** (sha256 /etc/passwd /etc/shadow
  /bin/bash /usr/bin/sudo + ss -tlnp), không coi là "thay đổi bất thường" vì là
  sandbox môi trường khác, không có báo động đỏ (không phải dấu hiệu xâm nhập:
  toàn bộ file hệ thống chuẩn của image sandbox).

### Thu thập (§2)
| branch | ahead/behind | cherry+ | PR | state | headRefOid == HEAD | commit cuối (UTC) |
|---|---|---|---|---|---|---|
| chore/owner-approve-pty-ci-console | 1/16 | 0 | #184 | MERGED | 895ace7 ✓ | 04:46:25Z |
| docs/core-principles-extension-track | 1/8 | 0 | #195 | MERGED | 7e352b9 ✓ | 06:19:31Z |
| docs/roadmap-m2-residuals | 1/1 | 0 | #201 | MERGED | 06c242e ✓ | 07:59:17Z |
| feat/gd3-console-ui | 1/23 | 0 | #176 | MERGED | 4994c60 ✓ | 03:53:14Z |
| fix/journal-date-2026-11-26 | 1/17 | 0 | #183 | MERGED | 9b1bee2 ✓ | 04:43:10Z |
| fix/low-audit-residuals-61-62-77-93-119 | 1/23 | 0 | #174 | MERGED | 4f88499 ✓ | 03:32:20Z |

Sáu nhánh đều squash-merge (§2: đối chiếu PR MERGED với headRefOid khớp HEAD nhánh → coi đã gộp).

### Quyết định (SBG chạy 08:07:48Z)
| branch | quyết định | lý do |
|---|---|---|
| chore/owner-approve-pty-ci-console | DELETE_MERGED | PR #184 MERGED, HEAD khớp, cherry=0, commit cuối > 3h30' |
| docs/core-principles-extension-track | DELETE_MERGED | PR #195 MERGED, HEAD khớp, cherry=0, commit cuối > 1h45' |
| docs/roadmap-m2-residuals | **KEEP** | PR #201 MERGED, HEAD khớp — nội dung đã vào main; NHƯNG commit cuối 07:59:17Z < 30 phút so với lần chạy SBG (08:07:48Z) và không do lần chạy hiện tại tạo → §0.4 chặn xoá. Xem lại: 08:30Z cùng ngày hoặc đầu phiên sau → DELETE_MERGED |
| feat/gd3-console-ui | DELETE_MERGED | PR #176 MERGED, HEAD khớp, cherry=0, commit cuối > 4h |
| fix/journal-date-2026-11-26 | DELETE_MERGED | PR #183 MERGED, HEAD khớp, cherry=0, commit cuối > 3h20' |
| fix/low-audit-residuals-61-62-77-93-119 | DELETE_MERGED | PR #174 MERGED, HEAD khớp, cherry=0, commit cuối > 4h30' |

- MERGE: 0 (không PR mở). DROP: 0 (không nhánh rác/chưa gộp → không cần archive tag).
- §0.4 các điều kiện khác: 0 PR mở do người tạo; không label keep/do-not-delete; không
  nhánh type:security p0 đang làm dở.

### Thực thi (MODE=apply)
1. Xoá 5 nhánh DELETE_MERGED (`git push origin --delete`), KHÔNG đụng `docs/roadmap-m2-residuals`.
2. Ledger 6 dòng qua PR `chore(branches): ledger 2026-11-26 Phiên 40` (chỉ `journal/**` +
   `docs/branch-ledger.md` → miễn G3/G4/G6, còn G1/G2/G5/G7).
3. Xác minh cuối (§6): `git fetch --prune`; kỳ vọng remote = `main` + `docs/roadmap-m2-residuals`.
