# Decision Log — 2026-11-17 (netns, issue #3)

## D15: netns phải đi kèm block socket(AF_NETLINK) — CHẤP NHẬN

**Bối cảnh:** thêm CLONE_NEWNET mở thêm mặt tấn công: netlink là kênh cấu hình
cho nftables/ebtables/audit — đúng lớp CVE đang track trong watchlist
(CVE-2024-1086 nf_tables UAF, CVE-2026-53266 ebtables SNAT OOB KEV, cả hai cần
CAP_NET_ADMIN để khai thác, nhưng defense-in-depth yêu cầu chặn cả surface,
không chỉ cap).

**Quyết định:** seccomp blocklist thêm arg-check `socket(AF_NETLINK)` → EPERM
(cùng pattern với socket(AF_ALG) của phiên 4). Sandbox giữ nguyên khả năng
socket(AF_INET/AF_INET6/AF_UNIX) — httpd/wget không đổi (SC19 kiểm control).
Đo được (SC17/SC18): với filter → EPERM; không filter → errno=0 (netlink route
socket tạo được unprivileged trên cả env này lẫn runner) → EPERM chứng minh
đến từ filter mình, không phải host.

**Trade-off:** payload trong sandbox không đọc được route/neigh qua netlink
(busybox `ip` applet chết) — chấp nhận: sandbox không cần nhìn cấu hình mạng,
chỉ cần loopback HTTP.

## D16: bridge web qua unix-socket relay trên rootfs path — CHẤP NHẬN

**Bối cảnh:** với netns riêng, host KHÔNG thể mở TCP vào sandbox (veth nối ra
host netns rootless = không thể — cần CAP_NET_ADMIN host netns, đo phiên 3;
slirp4netns/pasta không có trong env). Precedent đúng bài toán: RootlessKit
`--port-driver` (builtin) — parent proxy port cho child netns; slirp4netns thì
cần binary riêng.

**Quyết định — cầu 2 chặng qua AF_UNIX (filesystem-scoped, không phải
netns-scoped):**
```
host:   tools/webproxy.py  TCP 127.0.0.1:<port>  -->  unix <rootfs>/.minict-proxy.sock
sandbox: relay trong minict  unix <sock>  -->  TCP 127.0.0.1:<port>  (httpd trong netns)
```
- Relay là process fork từ container child SAU pivot_root (socket file nằm
  trên disk của instance rootfs → host thấy qua `instances/<name>/rootfs/`)
  và TRƯỚC capdrop/seccomp → relay tự capdrop, không exec, không có filter
  (không cần — chỉ là byte-pump accept/connect/poll, fork-per-connection,
  SIGCHLD=SIG_IGN auto-reap). Attack surface của relay: đọc/ghi byte giữa 2
  socket, không có caps, không có fs ngoài socket path.
- Teardown 3 lớp: PDEATHSIG (parent = pidns-1), pidns-1-exit teardown,
  cùng process group với payload (group-kill của wrapper).
- Host side: `tools/webproxy.py` (python asyncio, bind cứng `127.0.0.1`,
  fork-free, half-close propagate bằng `write_eof`) — stdin/stdout không dùng;
  `minict.sh web` khởi động + lưu `proxy-pid`; `down` kill cả hai.

**Kết quả đo:** E2E curl host → 200 + marker (NS3); 2 instance cùng lúc mỗi
proxy route đúng instance của nó (NS6); host bind loopback-only (NS5); down
sạch (NS7). Đã khẳng định E2E bằng tay trước khi viết test (chain OK ở
04:40:xx, evidence `journal/2026-11-17/evidence/netns.txt`).

**Trade-off:** proxy python trên host thêm ~1 process/instance web (RSS nhỏ,
không thuộc ngưỡng sandbox vì là tooling host-side, đo trong perf-check vẫn
PASS); HTTP/1.1 keep-alive qua 2 chặng pump vẫn hoạt động (curl + httpd test
đủ ma trận 404/probe/traversal).

## D17: netns là DEFAULT-ON và fail-closed — CHẤP NHẬN

`unshare()` thêm CLONE_NEWNET cho MỌI run (up/web/exec/shell). Nếu host từ chối
tạo netns → `die()` (exit 126) — KHÔNG fallback câm về shared host net (giữ
nguyên tắc fail-closed của repo). Opt-out tường minh: `MINICT_NO_NETNS=1`
(dùng cho A/B đo overhead + host không hỗ trợ). Isolation giảm dần theo thứ tự
đúng: ai cần shared net phải tự nhận rủi ro bằng cờ, không phải mặc định.

## D18 (bài học đo được): `lo` phải bring-up SAU khi ghi uid_map

`lo` trong netns mới ở trạng thái DOWN và cần CAP_NET_ADMIN trong userns
SỞ HỮU netns đó (user_namespaces(7): "Privileged operations on resources
governed by the nonuser namespace require that the process has the necessary
capabilities in the user namespace that owns the nonuser namespace").
Bẫy đo được hôm nay: `unshare --user --net` của util-linux (không map uid)
giữ uid_map RỖNG → process là unmapped user → KHÔNG có cap nào trong userns
mới → `ip link set lo up` = EPERM ("RTNETLINK answers: Operation not
permitted"). Ghi uid_map (0 → ruid) xong thì bring-up OK ngay. Vì vậy trong
minict: unshare → map_ids → bring_lo_up → … → capdrop. Nếu để lo-up sau
capdrop sẽ chết vĩnh viễn (không còn cap), nếu để trước uid_map cũng chết
(chưa có cap).

## Bẫy khác bắt được trong phiên (đề xuất vào skills.md)

1. **rootfs là thư mục untracked sống xuyên session** — base rootfs còn sót
   `www/index.html` của bài test cũ ("minict-web-marker-20261117") khiến
   `[[ ! -f index.html ]]` của `minict.sh web` skip viết marker theo instance
   → NS6 (routing 2 instance) fail giả. Sửa: LUÔN ghi đè index.html khi `web`
   (docroot là private copy, ghi đè an toàn). Bài học: state ngoài git phải
   được script ghi đè chủ động, không tin "đã có sẵn".
2. **`[[ ]]` là bashism** — NS4 chạy qua `sh -c` (dash) → `[: not found`.
   Dùng `[ -S ... ]` POSIX.
3. **GNU Make 4.4.1 từ chối recipe 8-spaces** — fix "tabs" của phiên 4 KHÔNG
   còn trong main sau squash (runner vẫn make 4.3 nên CI không bắt). Gọi lại
   fix trong PR này; thêm mục tiêu `test-netns`.

## Overhead đo được (A/B interleaved, benchmarks/bench-netns-ab.sh, N=40)

| Mode | cold start p50 | p95 | ghi chú |
|---|---|---|---|
| netns ON (default) | 1.80 ms | 2.15 ms | + CLONE_NEWNET + lo up |
| netns OFF (control) | 1.52 ms | 1.69 ms | MINICT_NO_NETNS=1 |
| **delta netns** | **+0.28 ms** | +0.46 ms | ioctl SIOCGIF/SIFFLAGS + unshare netns |
| perf-check (N=20, default) | 1.90 ms | — | ngưỡng 100 ms → dư ~52x |

Cộng dồn từ baseline v0.1 (1.2 ms): +0.2 ms (seccomp) + 0.28 ms (netns) ≈
1.7–1.9 ms p50 — vẫn dưới target roadmap ≤ 5 ms sau netns+seccomp.

## Kẹt >30 phút? Có, 1 lần

NS6 fail giả do stale index.html (bẫy #1) — mất ~20 phút để phân biệt
"proxy hỏng" vs "nội dung sai" (curl thấy 200 nhưng marker cũ), 5 phút sửa.
