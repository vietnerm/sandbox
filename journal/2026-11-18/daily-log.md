# Daily Log — 2026-11-18

## Phiên 1 — tích hợp control-plane `minictd` (owner bàn giao)

> Không phải chu trình SDDA thường: owner (Doan) giao gói `minictd-handoff.zip` do
> một agent khác viết và yêu cầu đưa vào repo + cập nhật prompt/tài liệu cho đúng
> tiến độ. Gói được xử lý như **code chưa tin cậy**: review tay, build, chạy thật,
> A/B với bản gốc.

### Issue xử lý
- Không có issue GitHub riêng (việc do owner giao trực tiếp). Nhánh
  `feat/minictd-control-plane`, nhãn đề xuất `phase:2` + `needs-human` (đụng
  `PROMPT.MD`, `prompts/**`, `.github/workflows/**`, `idea/manifest.md` — G7).

### Quyết định
- D23 `MINICT_SPEC`, D24 multi-webport, D25 exec setns (gate GĐ2 đo được),
  D26 review 7 lỗi R1–R7, D27 **GĐ1 vẫn là giai đoạn hiện tại** (owner chọn trên
  decision card) → `journal/2026-11-18/decisions/minictd.md`.
- Manifest §8 ghi ngoại lệ "GĐ2 seed gộp sớm" **trước** khi gộp; roadmap GĐ2 có luật
  D27 để SDDA không tự mở việc GĐ2.

### Development
- `sandbox/minict.c`: gộp patch MINICT_SPEC + sửa R7 (bind target không đi theo
  symlink; tạo file hay dir theo loại src — gói gốc luôn tạo dir nên bind file hỏng).
- `minictd/`: daemon Go (stdlib), `cmd/minict-exec` (C); sửa R1–R6 (xem D26).
- `tests/test_minictd.sh` 20 case, `make minictd`, `make test-minictd`, job CI.
- Lỗi của gói gốc được ghi nhận khi review: pump execstream alias buffer; Run lỗi để
  lại thư mục "ma"; daemon không auth mặc định; `os.Environ()` lọt vào sandbox (đo
  được cả env phiên làm việc lọt vào `execv2 env`).

### CVE Check
- Không chạy `tools/cve-check.py` trong phiên này (phiên tích hợp, không phải chu
  trình ngày) — **chu trình SDDA kế tiếp phải chạy bù**.
- Bề mặt mới: Go stdlib (`net/http`, `os/exec`). `go.mod` ghi tối thiểu 1.23 (đã hết
  hỗ trợ upstream) → CI dùng `setup-go` `go-version: stable` để luôn có stdlib vá mới
  nhất. Chưa chạy `govulncheck` (follow-up).

### Validation "không bị nhiễm"
- Máy: container kernel 6.18.44, chạy test dưới uid 1001 (không root).
- `make test`: isolation 21/21 + seccomp 20/20 + netns 7/7 + alpine 8/8 (sau khi cài
  `iproute2` — thiếu `ss` làm NS5 đỏ, lỗi môi trường); `make test-web` 6/6;
  `make perf-check` PASS (4.01 ms / 3104 kB / 2.15 MB trên máy này).
- Cold start A/B có/không patch: +0.10 ms median (nhiễu) → `evidence/minictd-regress.txt`.
- `make test-minictd` 20/20 → `evidence/minictd-e2e.txt`; cùng suite trên gói gốc
  14/20 (6 đỏ đúng các case R-fix) → `evidence/minictd-ab.txt`.
- Lưu ý SIA: evidence này do chính phiên tích hợp sinh ra — chỉ log CI/SCR mới tính.

### PR
- PR `feat(minictd): integrate control-plane daemon + setns exec (GĐ2 seed)` — để
  owner review (G7).

### Kế hoạch ngày mai
- SDDA quay lại GĐ1 (D27): mục thấp nhất chưa xong của roadmap GĐ1.
- Ứng viên `type:security` được phép chen ngang: **S1** (webproxy theo symlink
  socket trong rootfs, có từ D16).
- Chạy bù CVE check; xem kết quả job CI `test-minictd` trên runner (xác nhận D25 lần 2).
