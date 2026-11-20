# D49 — G3 preview routing: Traefik file provider + wake probe (issue #97)

**Ngày**: 2026-11-20 (Phiên 14). **Nhánh**: `feat/97-g3-preview-routing`.

## Vấn đề

minictd unix-socket bridge hoạt động per-port (D16/D26 + webproxy S1) nhưng
preview routing chưa nối: Traefik không biết `s-<id>.<domain>` →
`127.0.0.1:<port>`, wake probe chưa có surface daemon-side (BridgeIP() luôn
rỗng — MERGE-GUIDE delta "wake must switch probe strategy").

## Lựa chọn

1. **File provider do daemon sinh** (chọn): mỗi instance running có webport →
   `sandbox-<name>.yml` trong `--traefik-dir`: router `s-<name>` (1 port) hoặc
   `s-<name>-p<port>` (multi-port D24), `Host(\`s-<name>.<domain>\`)` →
   `http://127.0.0.1:<port>`. Traefik v3.5 (docs + pkg/provider/file/file.go —
   2 nguồn verified, research.md Phiên 14) load mọi `.yml` trong directory +
   fsnotify watch → atomic temp+rename reload đúng 1 lần, temp suffix không
   `.yml` không bao giờ được load.
2. Docker-provider: loại — minictd không có docker daemon, không chạy được.
3. Reverse-proxy tự viết: vi phạm non-goals (không tự viết proxy khi Traefik
   giải quyết được).

## Quyết định chi tiết

- **Hooks** ở 4 lifecycle điểm (sau saveState): Run/Start → +file;
  Stop/Remove → −file. ReconcileAll → SyncAllRoutes (heal cả route dir);
  daemon boot → SyncAllRoutes (reality first). Fail-soft: mọi lỗi route chỉ
  log, không bao giờ fail instance op; `--traefik-dir` rỗng = feature off
  (pattern CG1, zero overhead cho deployment hiện có).
- **Namespace file**: prefix `sandbox-` — file của operator (không prefix)
  trong cùng directory không bao giờ bị đụng (Traefik vẫn load chúng).
- **Domain**: `--traefik-domain` mặc định `localhost` (systemd-resolved resolve
  `*.localhost` native — dev box dùng được không cần DNS).
- **Wake probe** = endpoint `POST /v1/sandboxes/{name}/wake` + CLI
  `sandbox wake`: dial `127.0.0.1:<port>` (dialTimeout 1s, reuse store.go),
  trả per-port TCP-ready; 409 stopped / 404 gone (mapping như exec/stop).
  Đúng contract MERGE-GUIDE Step 5 (BridgeIP()=="" → probe host loopback).
- **Labels**: `traefik.*` giữ nguyên round-trip qua Inspect (domain-staleness
  checks của control plane không đổi); daemon chỉ honor `traefik.enable=false`
  (opt-out). Full docker-label DSL translation là residual — native webport
  routing phủ scope G3; Traefik binary là việc của operator (không fetch/pin
  trong repo này — tránh surface CVE + 40 MB binary cho CI).
- **Deploy**: `--traefik-dir` vào default ExecStart của systemd unit + install.sh
  tạo `~/.config/minictd/traefik` (G3 dùng được ngay trên host thật).

## Test

- Go unit (`routes_test.go`): render exact-YAML (single/multi/default-domain),
  `traefikEnabled` matrix, write/remove/atomic (no .tmp leak), disabled/feature-off,
  SyncAllRoutes lifecycle (alive giữ, dead/stopped/disabled/stale bỏ, file
  operator giữ), Wake (live listener true / port 1 refused false / 409 / 404).
- e2e MD23–MD25 (`tests/test_minictd.sh`, daemon cả 2 lần launch có
  `--traefik-dir`): run → file + content đúng; disabled → không file; stop →
  file biến mất + wake 409; start → file trở lại + wake 200 port-ready;
  remove → wake 404 + file biến mất.
