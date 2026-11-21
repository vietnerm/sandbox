# Research — 2026-11-21 (issue #103 A2 oci: → phát lộ security finding, Pivot sang sec)

## Vấn đề

Đường `oci:<ref>` của minictd (`minictd/internal/minict/rootfs.go`, `unpackOCI`)
đưa nội dung **registry không tin cậy** vào `tar -x -C <dst> --no-same-owner`
chạy host-side, uid operator, TRƯỚC pivot_root/capdrop — ranh giới cách ly nằm
ở bước unpack, không phải ở sandbox runtime. Câu hỏi research: extraction này
fail-closed đo được đến đâu?

## Nguồn mới (≥ 2 yêu cầu PROMPT Bước 4 — links khả kiểm)

1. **Đo trực tiếp GNU tar 1.35 trên máy dev** (kernel 5.10 kangaroo al8; runner
   ubuntu-24.04 cũng ship GNU tar 1.35 — cùng bản upstream) — source chính,
   đúng ethos repo "measured, not assumed". Ma trận đo đầy đủ ở mục dưới.
2. **go-containerregistry `pkg/crane/export.go`** — verified live qua GitHub
   contents API (2026-11-21): doc comment "Export writes the filesystem
   contents (as a tarball) of img to w" — multi-layer đi qua `mutate.Extract`
   (flatten). Link:
   https://github.com/google/go-containerregistry/blob/main/pkg/crane/export.go
   ⇒ nội dung tar = 100% attacker-controlled nếu ref là image ác ý; chuỗi
   `crane export | tar -x` không có bước validate nào ở giữa.
3. **go-containerregistry release v0.22.1** (2026-09-04) — verified live qua
   releases API: asset `go-containerregistry_Linux_x86_64.tar.gz`, digest
   `sha256:0ab7a1d6932a213aed964ce97666c3077fe691c8606413674a8b3e0b9ec4cda0`,
   ~16.5 MB → fetch pin được theo pattern D37 (`tools/fetch-pasta.sh`) khi #103
   làm e2e. Link: https://github.com/google/go-containerregistry/releases/tag/v0.22.1
4. **moby `pkg/chrootarchive`** — prior art (industry chuẩn: reexec tar trong
   chroot jail + whitelist path, KHÔNG trust host tar cho nội dung registry).
   **Cite từ kiến thức — không fetch được phiên này** (api.github.com tree
   trả rỗng với moby/moby; raw.githubusercontent chặn) — flag unverified,
   không dùng làm chứng cứ, chỉ làm định hướng thiết kế.

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

## Phiên 16 — research #103 (A2: e2e `oci:` với crane thật)

**Nguồn (SIA B6, link thật — mở kiểm chứng được):**
- crane v0.22.1 release + checksums: https://github.com/google/go-containerregistry/releases/tag/v0.22.1
  (checksums.txt: https://github.com/google/go-containerregistry/releases/download/v0.22.1/checksums.txt —
  sha256 `0ab7a1d6…` trích từ đây, cross-check tarball tải riêng + `crane version`)
- go-containerregistry in-memory registry (harness): https://pkg.go.dev/github.com/google/go-containerregistry/pkg/registry
- crane CLI export/append semantics (docs + source `cmd/crane`): https://github.com/google/go-containerregistry/tree/v0.22.1/cmd/crane
- docker.io anonymous pull rate-limit (flake class cơ sở cho quyết định offline):
  https://docs.docker.com/docker-hub/usage/ (đo cục bộ: UNAUTHORIZED/429 surface khi
  ref sai được parse thành docker.io/library/... — log trong research chính)
- unverified-fetch note: moby/moby chrootarchive (tham chiếu extractor pattern,
  không fetch — flagged per D31/D37 discipline)

**Vấn đề**: đường `oci:<ref>` (`unpackOCI`: `crane export <ref> - | secureUnpack`,
D50) đã code nhưng chưa bao giờ exercise — không binary `crane`, không image thật.
MD27 chỉ pin fail-closed khi thiếu binary.

**Đo cục bộ (2026-11-21, crane v0.22.1 linux/amd64, sha256 verified từ
checksums.txt chính thức của release)**:

1. `crane export` v0.22.1 KHÔNG đọc tarball từ path local — mọi arg đều được
   parse thành remote registry ref (`img.oci.tar` → docker.io/library/img.oci.tar,
   UNAUTHORIZED đo được). Không có scheme prefix `tarball:`/`oci-layout:`.
   `crane append -o` xuất docker-archive; chỉ `crane export -` đọc image từ stdin.
2. **Registry in-memory trên 127.0.0.1 chạy được plain-HTTP KHÔNG cần `--insecure`**:
   server 20 dòng với `go-containerregistry/pkg/registry.New()` (in-memory,
   stdlib http.ListenAndServe), `crane push img.oci.tar 127.0.0.1:15100/mini:dev`
   + `crane export 127.0.0.1:15100/mini:dev -` đo được thành công. Nguyên nhân:
   go-containerregistry thử https rồi fallback http khi connection-refused
   (error message đo được liệt kê cả hai scheme).
3. `crane append -f <layer.tar> -t <full-ref>` push trực tiếp vào registry,
   không cần file trung gian.

**Giải pháp đã có ở dự án tương tự**: crane CLI dùng `pkg/registry` in-memory
chính nó làm fixture trong unit test của go-containerregistry; các e2e container
khác (podman CI) chạy registry thật trong container — không khả thi rootless
không Docker. ttl.sh là registry công khai ephemeral — phụ thuộc uptime bên thứ
ba + rate/flake (chọn làm fallback opt-in, không mặc định).

**Trade-off**: registry in-memory = thêm 1 module Go test-harness
(`tools/ocireg/`, dep go-containerregistry v0.22.1 — build standalone, KHÔNG
đụng go.mod của minictd) + binary thứ hai trong `tools/bin/` (gitignored).
Đổi lại: e2e `oci:` **hoàn toàn offline, deterministic** (no docker.io rate
limit — flake CI đã-documented; no ttl.sh uptime), hostile-image probe đẩy
thật qua registry thật bằng crane thật → secureUnpack phải refuse mid-stream
(EPIPE) — coverage mà MD26 (file `tar:`) không có.

**Áp dụng cho minict**:
- `tools/fetch-crane.sh` pin v0.22.1 + sha256 `0ab7a1d6…` (D37 pattern,
  nguồn: checksums.txt của release + tarball khớp + `crane version` khớp).
- `tools/ocireg/` harness (bind 127.0.0.1, port random, KHÔNG phải production
  code; binary vào `tools/bin/` gitignored).
- Suite `test_minictd.sh` thêm MD28–MD31: fetch-pin, happy-path boot+exec,
  bad-ref fail-closed (crane có mặt — mở rộng MD27), hostile-layer mid-stream
  refuse + R4 wipe + assert không escape ngoài rootfs.
- Real-world remote pull (TLS/auth/redirect docker.io) = opt-in qua
  `MINICTD_OCI_REMOTE_REF` (SKIP khi unset) — ghi residual minh bạch trong
  GAPS.md; không mặc định vì docker.io anonymous rate-limit trên runner IP
  chia sẻ là flake class đã-documented.
