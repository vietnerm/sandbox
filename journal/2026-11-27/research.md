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

## Phiên 44 — #219 tên file dạng bytes (base64)

- **Nguồn 1: PEP 383 — Non-decodable Bytes in System Character Interfaces** (peps.python.org/pep-0383, 2009):
  POSIX filename = bytes tùy ý; Python giải bằng `surrogateescape` — byte lỗi
  decode thành surrogate U+DCxx, encode ngược trả byte gốc, `os.listdir` không
  còn bỏ im tên lỗi. **Áp dụng cho minict:** hướng surrogate KHÔNG sống qua wire
  của ta — execv2 stdout là JSON Go, và Go `json.Marshal` thay mọi byte/surrogate
  không phải UTF-8 hợp lệ bằng U+FFFD (đo được: `t\xe1\xbaext.txt` → `t\ufffd\ufffdext.txt`,
  mất thông tin vĩnh viễn). PEP 383 vẫn hợp lệ cho **phần hiển thị**: decode
  strict-fatal, tên lỗi → hiển thị placeholder + hex (nguyên tắc "không bao giờ
  decode-lén-lúc"), còn **wire phải giữ bytes nguyên vẹn** → base64 (ASCII) là
  lựa chọn duy nhất đúng cho execv2.
- **Nguồn 2: git `core.quotepath`** (git-scm.com/docs/git-config; issuebuzz #6309
  làm ví dụ thực tế): mặc định git octal-escape mọi byte ≥ 0x80 khi hiển thị path
  (`\341\272`) — đường "display-escape, giữ bytes gốc trong lõi" giống hệt bài toán
  ta. **Áp dụng:** `name`/`path` trong FsEntry chỉ là display (UTF-8 hợp lệ → text;
  lỗi → `<tên không phải UTF-8>` + `hex: …`), `name_b64`/`path_b64` là nguồn thật
  cho mọi thao tác — parity với triết lý git: bytes là sự thật, chuỗi hiển thị là
  suy diễn có thể mất.
- **Trade-off đã cân nhắc:** (a) toàn bộ wire chuyển sang bytes-framing (như kubectl
  exec binary) — đổi daemon, quá lớn; (b) locale giả lập trong guest — không kiểm
  soát được rootfs user; (c) **base64 chỉ ở argv/stdout của các op files, script
  decode trong sandbox** — 0 byte core (hai phép thử manifest §3), chọn (c).

## Phiên 45 — #218 WebSocket PTY hai chiều (binary frames)

- **Nguồn 1: RFC 6455** (rfc-editor.org/info/rfc6455, Fette & Melnikov 2011):
  §5.3 client→server frames **BẮT BUỘC mask** (chống malicious script kiểm
  soát byte sequence qua cache poisoning — không phải bảo mật nội dung; server
  frames không mask); §1.6 security model = origin-based → CSWSH (Cross-Site
  WebSocket Hijacking) là mối đe dọa chính vì trình duyệt GỬI cookie/token
  ambient cùng origin — đúng lý do issue bắt check Origin=Host; §7 closing
  handshake (close frame + status code 1002 protocol error / 1009 too big /
  4xxx app). **Áp dụng cho minict:** server tự viết stdlib — reject frame
  không mask (1002), reject RSV bits (chưa negotiate extension — permessage-
  deflate phải bị từ chối ở handshake), cap payload 64 KB (1009), close 4401
  (auth) là mã 4xxx hợp lệ cho app-level.
- **Nguồn 2: MDN WebSocket API — binaryType + Writing WebSocket servers**
  (developer.mozilla.org): `binaryType = "arraybuffer"` để nhận byte thô
  (mặc định Blob — decode qua string sẽ phá UTF-8 split giữa chunk); browser
  **tự động trả pong** ở tầng protocol (JS không gửi trực tiếp ping/pong được)
  → server chỉ cần gửi ping định kỳ và đo "không thấy pong/bất kỳ frame nào
  trong 60s" là đủ liveness; `send(Uint8Array)` = binary frame. **Áp dụng:**
  output PTY đi binary nguyên vẹn (không JSON-encode như SSE), xterm
  `term.write(Uint8Array)`; liveness = pong timeout 60s (browser auto-pong
  miễn phí).
- **Trade-off đã cân nhắc:** (a) lib gorilla/websocket/nhooyr — thêm dep,
  vi phạm manifest §3 "mặc định tối thiểu" + go.mod zero-dep hiện có; (b)
  giữ SSE+POST vĩnh viễn — mỗi phím 1 POST, IME/paste dài vẫn nhiều HTTP
  round-trip (đo được trong repro); (c) **RFC 6455 minimal stdlib tự viết
  (~350 dòng, chỉ text/binary/close/ping/pong, không extension)** — chọn (c).

---

# Phiên 45 — GĐ4 1b.a: jail Firecracker không cần root (issue #231)

## Vấn đề
Jailer upstream của Firecracker yêu cầu root: chroot/pivot_root, mknod /dev/kvm +
/dev/net/tun, setuid/setgid, tạo cgroup — tất cả cần CAP_SYS_ADMIN/CHROOT. minict
là runtime rootless (userns+mountns+pivot_root+seccomp, uid ≥ 1000) — câu hỏi:
có thể thay jailer bằng một instance minict làm jail không, và đo được phần nào
không cần KVM?

## Giải pháp đã có / nguồn
- **Nguồn 1: firecracker docs/jailer.md (official, github.com/firecracker-microvm/
  firecracker/blob/main/docs/jailer.md)** — jailer làm đúng 6 việc: chroot +
  pivot_root, tạo device node /dev/kvm + /dev/net/tun, setuid/gid, cgroup v1/v2,
  rlimit (fsize, nofile), --new-pid-ns; **phải chạy root**. Firecracker runtime
  cần: binary + file được stage vào jail, /dev/kvm, /dev/net/tun (khi dùng mạng),
  API unix socket. Không có mục nào cần mknod thật: bind-mount node có sẵn trong
  userns thay thế được (minict đã bind null/zero/urandom/random đúng cách này).
- **Nguồn 2: pandastack "The Firecracker Jailer Explained"** — xác nhận
  "The VMM does not need root privileges at runtime" — jailer chỉ cần root cho
  bước setup (cgroup, chroot, mknod, setuid); sau đó VMM chạy unprivileged với
  đúng bộ file được stage. seccomp-bpf của VMM do firecracker tự cài (seccompiler).
- **Nguồn phụ 1: ZHAW (zhaw.ch, running untrusted code in Firecracker inside
  Docker)** — "Firecracker never runs as root; additional privilege required =
  host kvm group" — thực tế chỉ cần rw /dev/kvm.
- **Nguồn phụ 2: GHSA-36j2-f825-qvgc (security advisory chính thức)** — lỗi
  Arbitrary Host File Overwrite via Symlink trong chính jailer upstream → lập luận
  "dùng jailer = an toàn hơn" không tuyệt đối; một jail tự xây phải có bài test
  riêng (minict đã có suite isolation/escape riêng).

## Đo trên dev (uid 1001, kernel 5.10, KHÔNG /dev/kvm) — bằng chứng /tmp/fc-fetch
- firecracker v1.18.0 (tgz sha256 khớp .sha256.txt chính thức
  1ac8eec8…6623cd; binary b33f46cd…2ba) — static-pie ELF.
- `firecracker --version` → "Firecracker v1.18.0", exit 0.
- `firecracker --api-sock /tmp/…/api.sock` → nghe unix socket; host
  `curl --unix-socket GET /` → **200 OK** + JSON
  `{"id":"anonymous-instance","state":"Not started","vmm_version":"1.18.0"}`.
→ Cả 3 phép đo roadmap (binary khởi động, --version, GET /) không cần KVM trên
host. Việc còn lại: đo lại **bên trong jail minict** (seccomp + pivot_root +
capdrop của minict phải cho phép bộ syscall của VMM).

## Trade-off
- (a) Jailer upstream — buộc root, vi phạm bất biến §0; loại.
- (b) minijail (chromium) — binary ngoài, thêm dep + vẫn cần helper setup;
  minict đã có đủ cơ chế jail được test escape riêng.
- (c) **Jail = instance minict** (chọn): userns+mountns+seccomp+pivot_root có sẵn,
  bind /dev/kvm + thư mục socket API vào qua Volumes, binary stage qua bind ro.
  Lợi: zero-dep, rootless, một code path với container. Cái giá: seccomp mặc định
  (blocklist D11) phải đo được với bộ syscall của firecracker — nếu thiếu,
  bổ sung allowlist riêng (1b.e).
