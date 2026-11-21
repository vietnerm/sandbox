# D51 — A2 e2e `oci:` chạy offline qua registry in-memory (issue #103)

## Bối cảnh
Đường `oci:<ref>` (crane export → secureUnpack, D50) chưa bao giờ exercise.
MD27 chỉ pin fail-closed khi KHÔNG có binary crane; không có case nào boot
instance từ image OCI thật.

## Quyết định
1. **Pin crane v0.22.1 + sha256** (`tools/fetch-crane.sh`, D37 pattern):
   `0ab7a1d6932a213aed964ce97666c3077fe691c8606413674a8b3e0b9ec4cda0`
   (go-containerregistry_Linux_x86_64.tar.gz — nguồn checksums.txt chính thức
   của release tag v0.22.1, tarball tải riêng khớp, `crane version` khớp).
   Binary vào `tools/bin/crane` (gitignored, như pasta D34/D37).
2. **Registry in-memory `tools/ocireg/`** (module Go test-harness riêng, dep
   go-containerregistry v0.22.1 = cùng version với binary crane): bind
   127.0.0.1:<port-random>, in-memory (pkg/registry.New()). KHÔNG đụng go.mod
   của minictd; KHÔNG phải production code; KHÔNG bind ngoài loopback.
3. **Suite MD28–MD31** trong `tests/test_minictd.sh` (suite 28→31):
   - MD28 fetch-pin crane (PT1 pattern); môi trường không có mạng → SKIP (125),
     sha mismatch/network-có-mà-fail → FAIL thật.
   - MD29 happy path: layer từ rootfs suite (`$REPO/rootfs`) → `crane append`
     push vào registry local → Run `oci:127.0.0.1:<p>/...` → boot + exec sees
     marker → CRANE thật, HTTP thật, streaming thật — offline deterministic.
   - MD30 negative control: ref không tồn tại (registry local trả 404) →
     Run fail-closed 500 + dir wiped (mở rộng MD27: crane CÓ MẶT).
   - MD31 hostile: layer có member `../` → push thật → crane stream thật →
     secureUnpack refuse mid-stream (crane chết EPIPE) → 500 + R4 wipe +
     assert không file nào escape ngoài rootfs (như MD26 nhưng qua đường
     PIPE oci: — coverage mới).
4. **Remote pull thật (docker.io/ghcr/ttl.sh) = opt-in** qua env
   `MINICTD_OCI_REMOTE_REF` (unset → SKIP). Lý do không mặc định: docker.io
   anonymous rate-limit trên runner IP chia sẻ là flake class đã-documented;
   tiêu chí "CI xanh THẬT" (issue #14) không chấp nhận flake nguồn ngoài.
   Residual ghi minh bạch ở GAPS.md + roadmap.

## Giải pháp bị từ chối
- ttl.sh mặc định cho happy+hostile: phụ thuộc uptime bên thứ ba + đẩy image
  hostile (dù inert) ra infra công khai trong CI mặc định — không cần thiết
  khi có phương án offline.
- Vendor go-containerregistry vào minictd thay shell-out crane: thay đổi
  kiến trúc D50 (binary ngoài, PATH-driven) — ngoài scope issue #103.
- Đọc tarball local cho hostile (không cần registry): đo được crane v0.22.1
  KHÔNG hỗ trợ (`export` luôn parse ref remote) — chỉ `export -` đọc stdin,
  không nối được vào `oci:<ref>` contract.

## Kết quả đo (Phiên 16)
- Suite: **34 PASS + 1 SKIP** (MD33 opt-in unset) — `evidence/oci-crane-e2e.txt`;
  `make test` 10 suites none-failed; web 9/9; perf **2.16 ms / 1212 kB / 2.04 MB PASS**;
  host baseline 3/3 unchanged; CVE check + gate rc=0.
- **Phát lộ phát sinh (ghi đúng luật Bước 4)**:
  - **#108 (type:security p1)**: `unpackOCI` deadlock `crane.Wait()` khi
    secureUnpack refuse giữa stream (pipe đầy → crane block write(2), đo bằng
    goroutine dump) — FIX trong PR này: pure-drain `io.Copy(io.Discard, stdout)`
    trước Wait (không extract, không yếu hoá refusal). MD34 pin.
  - **#109 (type:feature p2)**: secureUnpack refuse absolute symlink targets
    (D50 posture) — rootfs của repo (271 applet links `/bin/busybox`) không boot
    được qua `oci:`; happy layer dùng relative links; phương án refuse/rewrite/
    inside-dst cần maintainer.
  - **2 lớp refuse độc lập đo được cho `../`**: crane upstream tự refuse
    "unsafe tar path" trước khi viết (stderr đo được) + secureUnpack mid-stream.
- **Mutants**: M1 (bỏ drain) → MD34 đỏ; M2b (drop refusal errors) → MD34 đỏ qua
  provenance assert (mượn pattern phiên 15: assert message, không chỉ rc);
  M3 (sha pin mismatch) → fetch-crane exit 1 (binary không bị ghi đè);
  R4-wipe class pin sẵn bởi dir-gone asserts (MD31/32/34) cùng class MD26.
- **Measured lessons**: crane export v0.22.1 không đọc tarball local (mọi arg
  parse thành remote ref); `io.Discard` KHÔNG implement io.Reader (mutant M1
  compile-fail — dùng xoá dòng thay vì noop); harness registry chết giữa
  blob-serve khi process-group bị dọn giữa các tool-call (setsid không cứu —
  pattern này chỉ ảnh hưởng máy dev, không ảnh hưởng suite trong 1 lần chạy);
  suite cleanup leak: RPID set trong subshell của run_case → không tới cleanup
  → pkill theo argv pattern (fix trong PR).
