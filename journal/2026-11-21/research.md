# Research — 2026-11-21 (issue #103 A2 oci: → phát lộ security finding, Pivot sang sec)

## Vấn đề

Đường `oci:<ref>` của minictd (`minictd/internal/minict/rootfs.go`, `unpackOCI`)
đưa nội dung **registry không tin cậy** vào `tar -x -C <dst> --no-same-owner`
chạy host-side, uid operator, TRƯỚC pivot_root/capdrop — ranh giới cách ly nằm
ở bước unpack, không phải ở sandbox runtime. Câu hỏi research: extraction này
fail-closed đo được đến đâu?

## Nguồn mới (≥ 2 yêu cầu PROMPT Bước 4)

1. **Đo trực tiếp GNU tar 1.35 trên máy dev** (kernel 5.10 kangaroo al8; runner
   ubuntu-24.04 cũng ship GNU tar 1.35 — cùng bản upstream) — source chính,
   đúng ethos repo "measured, not assumed". Ma trận đo đầy đủ ở mục dưới.
2. **go-containerregistry (crane) v0.22.1** — verified live qua GitHub API
   (2026-09-04): asset `go-containerregistry_Linux_x86_64.tar.gz`, digest
   `sha256:0ab7a1d6932a213aed964ce97666c3077fe691c8606413674a8b3e0b9ec4cda0`,
   ~16.5 MB → fetch pin được theo pattern D37 (`tools/fetch-pasta.sh`).
   `crane export <ref> -` = flattened rootfs tar của mọi layers (whiteout đã
   áp) → nội dung tar = 100% attacker-controlled nếu ref là image ác ý.
3. **moby `pkg/chrootarchive`** (prior art, cite từ kiến thức — fetch bị chặn
   phiên này): Docker không từng trust host `tar` cho nội dung registry;
   reexec `tar` bên trong chroot jail + whitelist path. Chứng minh industry
   chuẩn là "không tin extractor bên ngoài cho ranh giới bảo mật".

## Ma trận đo (tất cả trên tar GNU 1.35, host-side, không sudo)

| Vector | Tar entry | Kết quả đo | Đánh giá |
|---|---|---|---|
| member name `../escape` | REGTYPE | tar **refuse**: "Member name contains '..'", rc=2 | fail-closed (1.35) |
| member name `/abs/path` | REGTYPE | strip leading `/`, warn, extract trong dst | ok (1.35) |
| symlink target tuyệt đối ra ngoài | SYMTYPE `lnk -> /tmp/x`, rồi `lnk/f` | symlink được TẠO, nhưng open xuyên qua: "Cannot open: Not a directory", rc=2 | chặn write-through (1.35) |
| symlink target tương đối `../..` | SYMTYPE + `lnk/f` | "Cannot open: Not a directory", rc=2 | chặn write-through (1.35) |
| same-name file sau symlink (overwrite) | SYMTYPE rồi REGTYPE cùng tên | bỏ qua, KHÔNG overwrite, rc=0 | ok (1.35) |
| **symlink target tương đối `..` — tồn tại trong rootfs** | SYMTYPE `lnk -> ../victim` | **tạo rc=0**, `cat dst/lnk` đọc ra ngoài rootfs THÀNH CÔNG | **rò rỉ host-side** |

## Kết luận đo được

1. **Extraction-time vectors bị chặn trên GNU tar 1.35** — fail-closed là HÀNH
   VI INCIDENTAL của host tar, không phải của minictd code.
2. **Gap A — phụ thuộc tar host**: busybox tar / GNU tar cũ hơn KHÔNG có các
   protection trên (busybox tar: không refuse `..`, không chặn through-symlink).
   minictd chạy trên "Linux host nhẹ" — không có invariant nào về tar. Trên
   host đó, một image registry ác ý = ghi file tùy ý host-side uid operator =
   **sandbox escape ở ranh giới unpack** (class TOCTOU/symlink CVE-2018-15664
   mà D31/#41 đã vá ở webproxy).
3. **Gap B — dotdot-symlink sống sót trong rootfs**: ngay cả trên 1.35, symlink
   `../` được tạo rc=0 trong rootfs. Runtime trong sandbox: pivot_root + detach
   old root (D39) kẹp `..` tại / ⇒ payload không với tới host — measured-safe.
   Host-side: daemon không walk rootfs sau extract; residual LOW nhưng phải ghi
   GAPS (nếu tương lai có feature walk/snapshot host-side → vector mở lại).
4. `prepareRootfs` đường `tar:<path>` dùng CÙNG `tar -x` — operator-provided
   tar được tin hơn registry, nhưng cùng code path ⇒ harden cùng chỗ.

## Áp dụng cho minict

- Mở issue `type:security priority:p1` (không p0: chưa escape đo được trên env
  chuẩn 1.35 dev+runner; p0 dành cho escape measured theo precedent #41 S1).
  **Pivot issue-hôm-nay** từ #103 (A2 e2e) sang security này theo thứ tự ưu tiên
  §8 ("type:security chen ngang là ưu tiên 1"). #103 giữ mở cho phiên kế.
- Fix: thay `tar -x` bằng **Go stdlib secure extractor** (archive/tar +
  `syscall.Openat` O_NOFOLLOW từng thành phần — pattern D31 đã verified trong
  repo, không thêm dependency): reject tên `..`/absolute, symlink target phải
  relative-không-dotdot, mọi open qua openat O_NOFOLLOW per-component
  (TOCTOU-free), hết phụ thuộc tar host cho cả 2 đường `oci:` và `tar:`.
- e2e negative control: `tar:` image chứa member `..` → Run fail-closed, không
  để lại rootfs dở (case shell MD26+, python3 tarfile dựng hostile tar — python3
  có trên dev lẫn runner).

## Trade-off cân nhắc

- Giữ `tar -x` + pre-scan `tar -tf` trước: 2-pass, vẫn phụ thuộc tar host +
  TOCTOU giữa scan và extract (pipe ở `oci:` không retry được) → bỏ.
- chrootarchive-style reexec: cần exec 自己 binary + mount ns → nặng hơn nhu
  cầu; openat-chain đủ và đã là pattern verified của repo (D31).
- Go extractor chậm hơn tar C trên rootfs lớn: đo được trong PR này (ngưỡng
  perf không đổi — cold path minict không đụng; unpack chỉ chạy lúc Run).
