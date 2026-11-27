# Research — 2026-11-27 (Phiên 42) — issue #215: registry template tĩnh (M3)

## Nguồn 1 — Snyk: Zip Slip (directory traversal khi giải nén archive)
https://security.snyk.io/research/zip-slip-vulnerability (Snyk Security, 2018;
bảng ảnh hưởng kèm CVE theo ecosystem)

Tóm tắt: Zip Slip là lớp lỗi "arbitrary file overwrite" khi extractor nối tên
member (`../../evil.sh`) vào thư mục đích mà không validate. Affects tar/zip/
apk/… trên mọi ecosystem; pattern lỗi phổ biến nhất là code copy-paste kiểu
`new File(destDir, entry.getName())` + copy. Bài học cốt lõi: **reject tuyệt
đối, không sanitize** — sanitize (`..` → `_`) đã từng sinh lỗi n-goát khác.

Áp dụng cho minict: repo ĐÃ có lời giải chuẩn hơn Zip-Slip-fix thông thường —
`secureUnpack` (minictd/internal/minict/unpack.go, issue #104/D50): reject tên
absolute/`..`, symlink target chỉ trong dst, openat(O_NOFOLLOW) từng component
(TOCTOU-free), O_EXCL chặn duplicate, strip setuid/setgid. M3 KHÔNG viết extractor
mới cho pull — **tái dùng nguyên khối** qua export wrapper, tránh anti-pattern
copy-drift (bài học D25: hai bản seccomp lệch nhau → test đỏ).

## Nguồn 2 — OCI image/distribution spec: index + digest (sha256) làm bản đồ nội dung
https://opencontainers.org/posts/blog/2024-03-13-image-and-distribution-1-1
(OCI 1.1, image index) + https://blog.quarkslab.com/digging-into-the-oci-image-specification.html
(Quarkslab, 2022 — cấu trúc image index → manifest → layer, digest = sha256 nội dung)

Tóm tắt: OCI tách "catalog" (index JSON liệt kê descriptor: mediaType, digest,
size) khỏi "blob" nội dung; mọi đối tượng được định danh bằng sha256 nội dung,
nên fetch-first-verify-later là an toàn theo thiết kế. Model này dùng được trực
tiếp cho registry tĩnh không server động: `index.json` chứa (name, version,
sha256, size_bytes, created), blob là `<name>-<ver>.tar.gz`.

Áp dụng cho minict: `template pull` tải index → tìm entry → tải blob → **hash
trước, giải nén sau**; sha256 sai → fail-closed, xoá temp, không chạm đích.
Lưu ý threat model: sha256 chỉ chứng minh blob == index mô tả; nếu CẢ registry
bị kiểm soát (index + blob cùng bị thay) thì lớp chặn cuối là secureUnpack
(tar độc `../`/symlink vẫn phải bị từ chối) — test MD69 phủ cả hai nhánh.

## Nguồn 3 (nội bộ, đo được) — snapshot.go deviation gzip↔zstd + unpack.go baseline
minictd/internal/minict/snapshot.go:24-26 ("stdlib has no zstd … gzip is a
deliberate, documented deviation") và unpack.go (GNU tar 1.35 vs busybox tar
measurement, 2026-11-21).

Trade-off định dạng nén: zstd nhỏ hơn/nhanh hơn nhưng cần dependency ngoài
(CGO/binary) — vi phạm Track M "không thêm phụ thuộc" và làm phình binary;
giữ `.tar.gz` (stdlib compress/gzip) theo đúng tiền lệ snapshot, ghi deviation
vào decision D81 + README. Registry format ghi `tar.gz` (KHÔNG ghi `.tar.zst`
như roadmap chữ — decision sửa cho khớp thực tế, roadmap cập nhật khi M3 xong).

## Kết luận thiết kế (đem vào PR)
1. Client-only: toàn bộ code nằm ở `minictd/cmd/sandbox/template.go`; daemon
   không đổi; zero-dependency mới (go.mod giữ nguyên).
2. Extractor dùng chung: export `SecureUnpack` wrapper từ internal/minict
   (import cùng module — không copy, không drift).
3. Gate transport: HTTPS bắt buộc; HTTP chỉ chấp nhận khi `--insecure` VÀ host
   loopback (localhost/127.0.0.0/8/::1). http.Client có Timeout (bài học D76-M2f).
4. Fail-closed full-cycle: tải vào temp file → hash → extract vào temp dir kế
   bên đích → rename; MỌI lỗi giữa chừng xoá temp (không để lại file).
5. Template registry chỉ dùng symlink RELATIVE (D70: absolute target bị từ chối
   khi giải nén — busybox applet pattern không roundtrip qua registry, ghi rõ).
