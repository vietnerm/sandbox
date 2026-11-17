# Decision Log — 2026-11-17 — Align manifest ↔ roadmap ↔ PROMPT.MD

Issue: #15. Owner chỉ thị trực tiếp: "Fix lại PROMPT.MD để có thể triển khai được
theo hướng manifest" + phân cấp nguồn ưu tiên. PR này đụng `PROMPT.MD` (G7
sensitive) — được duyệt bằng chỉ thị trực tiếp của owner trong session 2026-11-17.

## DA1 — Phân cấp 3 nguồn ưu tiên

Trước đó có 3 nguồn mâu thuẫn: `idea/manifest.md` (4 giai đoạn nhưng viết như lời
khuyên), `docs/roadmap.md` (chỉ GĐ1, đích "full web stack"), `PROMPT.MD §8` (danh
sách ưu tiên riêng, lệch cả hai trên). Chốt: **manifest đặt hướng → roadmap là kế
hoạch → PROMPT.MD trỏ vào roadmap**. §8 thay bằng quy tắc chọn việc: security p0/p1
chen ngang, else mục thấp nhất chưa xong của giai đoạn hiện tại. Lý do: một danh
sách duy nhất (roadmap) tránh drift khi 2 file nói khác nhau; SDDA không có quyền
tự đổi hướng (manifest) mà chỉ làm theo kế hoạch (roadmap).

## DA2 — Manifest viết lại thành manifest thật

Bỏ giọng "Nếu bạn thực sự muốn…". Bổ sung: Mục tiêu (Why), Người dùng, Nguyên tắc
bất biến, **Non-goals** (không tái tạo Proxmox/k8s; không tự làm network proxy khi
pasta/slirp4netns đủ; không tự viết gVisor/Wasm runtime; container layer không cho
untrusted — untrusted là việc của microVM GĐ3), **Nền tảng mục tiêu** và **DoD đo
được từng giai đoạn** (GĐ1–GĐ4). Lý do: DoD phải kiểm chứng được bằng test/benchmark,
không phải ý kiến — mỗi mục GĐ1 đều map vào test suite hoặc ngưỡng perf-guard.

## DA3 — Nền tảng mục tiêu + xếp lại "parked"

Chốt nền tảng mục tiêu: kernel ≥ 6.1, cgroups v2, uid ≥ 1000, /dev/kvm (GĐ3).
Runner CI (ubuntu-24.04) đã đạt mức này. Do đó: Landlock, cgroups v2, overlayfs
không còn là "parked" mà là mục kế hoạch của GĐ2 (`blocked-on-dev-env` chỉ đúng
với máy dev kernel 5.10/cgroups v1). Parked thật còn lại: seccomp allowlist (cần
audit matrix) và gVisor (overhead/model trùng microVM) — lý do thiết kế, không
phải môi trường. Firecracker có điều kiện: phải kiểm `/dev/kvm` trên runner trước.

## DA4 — netns connectivity: pasta/slirp4netns thay vì tự làm proxy

Tự làm proxy (thuần python/webproxy) tốn công bảo trì và khó đúng; pasta/slirp4netns
cài được rootless cả trên CI lẫn máy đích. Roadmap GĐ1 mục 4: đánh giá A/B với
bridge D16 hiện có; D16 giữ làm fallback loopback-only. Lý do: giảm diện tích tự
viện, dùng precedent chuẩn của podman/RootlessKit.

## DA5 — Gate research exec-into-instance trước GĐ2

README ghi đúng: rootless không đủ caps để setns vào instance đang chạy, exec hiện
tạo ns set mới (mỗi exec một netns riêng) — control plane GĐ2 không thể xây trên
hành vi này. Phương án chuẩn để đo: daemon tạo instance là process fork/unshare
trực tiếp nên giữ full caps trong userns nó tạo → setns được từ phía daemon (mô
hình RootlessKit/podman exec). GĐ2 có gate research bắt buộc trước khi viết code.

## DA6 — DoD tổng thể đếm đúng

Danh sách 5 mục DoD (web stack đầy đủ / không Docker-không root / cold<5s+RAM<300MB /
escape pass / một lệnh up|down|bench) — hiện trạng thật: **2/5** (mục 2 và 4).
Con số cũ "5/5 mục phụ đã có nền" đếm cả web tĩnh + seccomp + netns — các mục này
KHÔNG nằm trong danh sách 5 mục; chúng là nền của GĐ1. Đã sửa + ghi chú tránh
đếm lại sai.
