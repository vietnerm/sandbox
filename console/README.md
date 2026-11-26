# minictd console (GĐ3)

> **Trạng thái (Track M M1, 2026-11-26):** `sẵn sàng` — CSP/traversal/auth e2e
> xanh CI (MD55-60+XSS). Terminal PTY `thử nghiệm` (MD62 env-gated). Biểu đồ
> IO/Network `sẵn sàng` (#185).

Web UI cho `minictd`, dựng từ template Next.js owner giao ngày 2026-11-26
(owner chốt phương án "xuất tĩnh, minictd phục vụ").

- **Không có tiến trình Node lúc chạy.** `make console` chạy `next build` với
  `output: "export"` → thư mục tĩnh `console/out/`. `minictd --ui-dir console/out`
  phục vụ thư mục đó trên **cùng listener** với API (mặc định `127.0.0.1:7601`),
  nên ngân sách RSS ≤ 30 MB và bind localhost giữ nguyên (manifest §7 GĐ3).
- **Xác thực:** trang đăng nhập hỏi token của minictd (`$MINICTD_TOKEN` /
  `--token-file`). Token nằm trong `sessionStorage` của tab và đi kèm mỗi request
  dạng `Authorization: Bearer …`, không dùng cookie → không có credential tự động
  cho CSRF. File tĩnh không cần token; mọi `/v1/*`, `/metrics`, `/healthz` vẫn cần.
- **CSP:** minictd tính sha256 cho từng `<script>` inline trong HTML (Next.js
  export có RSC payload inline) → `script-src 'self' 'sha256-…'`, không
  `unsafe-inline` cho script; `connect-src 'self'`, `frame-ancestors 'none'`.

## Chạy thử

```bash
make build rootfs minictd console
echo "$(head -c 24 /dev/urandom | base64)" > ~/.config/minictd.token
minictd/bin/minictd --token-file ~/.config/minictd.token \
  --minict-bin "$PWD/sandbox/minict" --minict-exec-bin "$PWD/minictd/bin/minict-exec" \
  --base-rootfs "$PWD/rootfs" --webproxy "$PWD/tools/webproxy.py" \
  --ui-dir console/out
# mở http://127.0.0.1:7601/ và dán token
```

Phát triển giao diện không cần daemon: `cd console && npm ci --ignore-scripts && npm run dev`,
mở `http://localhost:3000/?demo=1` (dữ liệu giả trong trình duyệt, chỉ bật khi có `?demo=1`
hoặc build với `NEXT_PUBLIC_CONSOLE_DEMO=1`).

## Ánh xạ API

Các view gốc gọi API kiểu sandboxd; `src/console/minictd.ts` dịch sang minictd:

| Màn hình | minictd |
|---|---|
| Danh sách / chi tiết instance | `GET /v1/sandboxes` + `/{name}` + `/{name}/state` + `/{name}/stats` |
| Tạo instance | `POST /v1/run` (RunSpec: `cpus`, `memory`, `pasta_ports`, `env`, label `minictd.project`) |
| Start / Stop / Restart / Xoá | `POST /{name}/start`, `/stop`, `/remove` |
| Terminal | `POST /{name}/execv2` mỗi dòng (`/bin/sh -c`, timeout 60 s, chưa có PTY) |
| Logs trực tiếp | `GET /{name}/logs?tail=N&follow=1` (fetch stream, có Bearer) |
| Biểu đồ CPU/RAM | poll `GET /{name}/stats` mỗi 2 s, lịch sử giữ trong trình duyệt (GĐ3 mục 3) |
| Templates | `GET /v1/templates`, `GET /v1/backups`, `POST /{name}/save-template`, `POST /{name}/snapshot`; tạo lại bằng image `template:<n>` / `backup:<n>` |

Các trang của sandboxd không có trên minictd (Users, Approvals, Nodes, Network/egress,
Settings, Files) bị ẩn khỏi menu; link sâu tới chúng hiện thông báo "không có trên
minictd" (manifest §4: một người dùng, một máy, không SDN).

## Giới hạn hiện tại

- Terminal là line-mode qua exec; chương trình tương tác (vi, top, REPL) cần endpoint
  PTY (chưa có).
- Chưa xoá được template/backup từ UI (minictd chưa có API xoá).
- Inspect của minictd không trả lại giới hạn CPU/RAM đã đặt nên cột tài nguyên hiện "—";
  RAM thật lấy từ `/stats` (giới hạn cgroup, hoặc RAM máy khi không có cgroup).
- Bundle JS ~1.8 MB (React + Radix + xterm); chỉ tải ở trình duyệt, không tính vào RSS daemon.
