# D81 — Registry template tĩnh: thiết kế `sandbox template push|pull` (Track M M3)

Issue #215 · nhánh `feat/215-template-registry` · ngày 2026-11-27 (Phiên 42)

## Bối cảnh

M1 xong (#197), M2 trọn vẹn xong (#199+#203) → M3 là mục thấp nhất chưa xong của
Track M (GĐ4 research-only vì thiếu /dev/kvm). Roadmap yêu cầu: định dạng file
tĩnh index.json + blob; push từ template local; pull HTTPS, sha256 trước khi giải
nén, giải nén an toàn tái dùng kiểm tra của backup restore; test pull từ web
server tĩnh, sha sai fail-closed, tar độc bị chặn.

## Quyết định

1. **Client-only, zero-dep.** Toàn bộ code ở `minictd/cmd/sandbox/template.go`;
   daemon (`minictd`) không đổi một dòng logic; `go.mod` giữ nguyên (stdlib only).
   Phù hợp luật Track M "không thêm phụ thuộc, tiến trình nền vào minict/minictd".

2. **Định dạng blob: `.tar.gz`, KHÔNG `.tar.zst` như chữ roadmap.** Go stdlib
   không có zstd; thêm lib zstd = dependency mới + phình binary — vi phạm luật
   Track M. Tiền lệ đã có: `snapshot.go` (D57) dùng gzip với ghi chú deviation
   rõ. Roadmap M3a được sửa kèm PR này (chữ `.tar.zst` → `.tar.gz` + ghi chú).

3. **Tái dùng extractor qua import, KHÔNG copy.** Pull giải nén bằng chính
   `secureUnpack` (#104/D50 — fail-closed: reject `..`/absolute, openat
   O_NOFOLLOW, O_EXCL, strip setuid/setgid; D70: symlink target tuyệt đối bị từ
   chối) qua wrapper mới `minict.SecureUnpack`. Copy sang client sẽ lặp lại
   anti-pattern D25 (hai bản seccomp lệch nhau phải thêm test drift). Import
   `internal/minict` từ `cmd/sandbox` hợp lệ (cùng module, không thêm dep).

4. **Verify TRƯỚC khi giải nén.** Blob tải về temp file (hash + size trong lúc
   stream); sha256/size sai → xoá temp, thoát lỗi, KHÔNG chạm đích ("không để
   lại file"). Giải nén diễn ra vào temp dir cùng parent với `--out`, chỉ
   `rename` thành `--out` khi toàn bộ thành công; mọi lỗi giữa chừng → RemoveAll.
   Mô hình descriptor từ OCI distribution spec (index ghi sha256+size).

5. **Threat model hai lớp.** (a) Transport: HTTPS bắt buộc; HTTP chỉ chấp nhận
   khi `--insecure` VÀ host loopback (localhost/127/8/::1) — dùng cho server
   tĩnh cục bộ; redirect hạ cấp https→http bị chặn trong CheckRedirect;
   http.Client luôn có Timeout (bài học M2f/D76). (b) Content: nếu CẢ registry
   bị kiểm soát (index+blob khớp nhau nhưng độc), extractor là tuyến cuối —
   MD69 chứng minh tar có member `../evil.txt` được index xác nhận sha vẫn bị
   từ chối và không file nào được viết ra ngoài.

6. **`--out` không overwrite.** Pull vào dir đã tồn tại → lỗi (fail closed),
   operator xoá tay — tránh hiểu lầm "cập nhật một phần" khi pull lỗi giữa chừng.

7. **Push chỉ nhận symlink RELATIVE.** Target tuyệt đối sẽ bị SecureUnpack từ
   chối lúc pull (D70, owner chốt 2026-11-26), nên push chặn ngay lúc đóng gói
   — template không thể pull thì không được phép vào index. Hệ quả đã ghi vào
   README: layout busybox-applet (symlink `/bin/busybox`) không roundtrip qua
   registry (rootfs alpine/base vẫn ok vì build bằng relative/copy).

8. **Index strict.** `DisallowUnknownFields` cả khi push đọc lẫn pull đọc
   (NO MAGIC, cùng posture stack.json); version schema v1; entry upsert
   (name,version) là khoá; sort ổn định name→version để diff sạch.

## Lựa chọn bị loại

- **Copy `secureUnpack` vào client** — drift rủi ro bảo mật (D25).
- **zstd qua lib ngoài / CGO** — phá zero-dep; chưa có trong stdlib (Go 1.27).
- **pull trực tiếp vào `--out` không qua temp** — lỗi giữa chừng để lại cây
  nửa vời; vi phạm "không để lại file".
- **HTTP cho mọi host khi `--insecure`** — bearer-less download qua mạng không
  tin cậy là MITM kinh điển; chỉ loopback mới đủ an toàn cho `--insecure`.
- **Registry ghi vào daemon (API/template dir)** — M3 là lớp phân phối; daemon
  giữ nguyên hoá đơn trách nhiệm (d51: OCI ở client).

## Hai phép thử (manifest §3)

- **Còn nhẹ không?** Có — client binary +~300 dòng stdlib, không process nền,
  không cache, không daemon-side state; pull = 2 request HTTP.
- **Bỏ đi core còn hữu dụng không?** Có — registry chỉ là công cụ phân phối
  rootfs; runtime/daemon không tham chiếu nó.

## Bằng chứng

- 14 unit test mới (template_test.go): gate/parse/push/pull roundtrip/tamper
  fail-closed/evil tar/symlink/gzip hỏng/overwrite/latest-pick — `go test
  ./cmd/sandbox/` 33/33 PASS (bao gồm test cũ).
- e2e **MD69** trong `tests/test_minictd.sh` (chạy mọi nơi): push → static
  server python → pull roundtrip; tamper size-preserving → fail-closed; evil
  tar → blocked; http không --insecure → refused.
- Số liệu: cold_start/rss/disk không đổi (client-layer; đo lại trong PR).
