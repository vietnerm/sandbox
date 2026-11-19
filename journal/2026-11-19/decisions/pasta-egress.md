# D37 — Pasta egress + port-forward prototype (GĐ1 mục 3): CI-gated suite,
# pin-acquired binary, --userns trap avoided

- **Ngày**: 2026-11-19 (Phiên 3 — theo thiết kế D34, issue #57)
- **Nhánh**: `feat/3-pasta-egress`
- **Trạng thái**: IMPLEMENTED (prototype); NET10–12 + AB1 chỉ chạy thật trên
  runner CI (máy dev không có `/dev/net/tun` — đo được, D34).

## Quyết định chính

1. **Acquisition = pin tarball snapshot cgit + `make static`** (pattern D20,
   không vendor binary vào repo):
   - `tools/fetch-pasta.sh`: pin `passt-2026_10_02.cba3570.tar.gz`, sha256
     `7b81892b…ac484b6` (đo tại pin-time 2026-11-19 từ passt.top cgit — nguồn
     canonical duy nhất vì passt không phát hành binary tĩnh chính thức; rủi ro
     residual: single-source pin, mitigated bằng fail-closed digest check +
     re-download cùng digest; nâng cấp pin phải đổi bảng pin_lookup).
   - Build static mất ~19 s máy dev (gcc 14), binary ~1.4 MB + `.avx2` sibling
     (install cả hai để dispatcher không warn vào `pasta.log`).
2. **`--userns` tường minh là BẮT BUỘC** (kiến thức mới từ source passt
   2026_10_02, `conf.c conf_pasta_ns()`): `pasta --netns <path>` mà không có
   `--userns` → ngầm `netns_only=1` → pasta KHÔNG join userns của instance →
   setns vào netns thuộc userns minict sẽ EPERM. Với `--userns
   /proc/<pid>/ns/user`: join được rootless vì euid pasta (1001) == owner
   userns (creator = minict uid 1001) → trong ns là uid 0 (map `0 1001 1`,
   D20) → đủ caps cho setns(netns) + TUNSETIFF. Đã verify từng bước trong
   `isolation.c isolate_user()` (không drop-to-nobody vì conf_ugid giữ euid
   khi non-root).
3. **Lệnh spawn** (trong `minict.sh spawn_pasta`): `pasta --foreground
   --userns … --netns … --config-net [$MINICT_PASTA_FWD]` — `--foreground`
   để giữ pid cho `pasta-pid` (mặc định daemonize); route readiness poll
   host-side qua `/proc/<pid>/net/route` (netns-scoped, row `00000000`).
4. **Payload override `up <name> [--pasta] [-- payload…]`**: NET10–12 cần chạy
   lệnh TRONG ns set của instance đã gắn pasta — `exec` mở ns set MỚI (PoC
   limitation) nên không dùng được; payload override là lối ra nhỏ nhất, không
   đụng `minict.c` (seccomp blocklist giữ nguyên — pasta chạy host-side, ngoài
   filter; không trigger luật D25 hai bản sao).
5. **Egress test deterministic, không phụ thuộc internet**: listener
   `0.0.0.0:$HPORT` host-side; trong sandbox wget tới IP host
   (`hostname -I`). Source passt `fwd.c nat_outbound()`: dst không phải
   gateway → đi qua nguyên vẹn → socket host tới đúng listener. Control
   NET11C (không pasta) chứng minh đường này KHÔNG có sẵn.
6. **Port-forward**: `-t FPORT:8080` qua env `MINICT_PASTA_FWD` (syntax SPEC
   `22:23` xác nhận từ `pasta --help`); mặc định của pasta là `auto`
   (forward mọi port được bind trong ns) — chọn explicit cho gate rõ ràng.
7. **DNS để sau** (không trong prototype): pasta không tự ghi resolv.conf
   (chỉ `--dhcp-dns`/`-D`); NET11 dùng IP-literal. `apk add online` (GĐ1
   mục 1 residual) cần DNS → phiên kế: inject resolv.conf vào private rootfs
   hoặc `--dns-forward`.
8. **Fail-closed mọi đường**: không tun → `up --pasta` lỗi + dọn instance;
   `MINICT_NO_NETNS=1` + `--pasta` → từ chối (pasta cần netns riêng);
   sha256 lệch → abort trước build. Suite: không tun → SKIP(125) có nhãn,
   không PASS giả; grep expected-failure chạy TRƯỚC `down` (anti-vacuous,
   lớp AL4/FS6).

## Số đo (máy dev, perf-check trước/sau merge)

- cold_start / rss / disk mặc định: KHÔNG đổi — pasta spawn out-of-band sau
  khi `up` trả về, không nằm trong cold_start (D34 A/B rule). Perf-check
  đo lại trong evidence `journal/2026-11-19/evidence/pasta-egress.txt`.
- AB1 (attach latency incl. route wait) + A/B cold start có/không pasta: chỉ
  đo được trên CI (dev env-blocked) — SCR lấy từ log CI run của head SHA và
  ghi link vào PR + daily-log; ngưỡng: pasta attach < 30 s (thường ~1 s),
  cold_start vẫn ≤ 100 ms vì pasta không nằm trong đường đo.

## Test mới (tests/test_pasta.sh — 7 case, vào `make test`)

- PT1 pin+verify+build; PT2 version stamp — PASS mọi nơi.
- NET11C control (không pasta không tới được host IP) — PASS mọi nơi.
- NET10 tap+route; NET11 egress; NET12 port-forward; AB1 latency — SKIP khi
  không tun (máy dev), PASS thật trên CI. Suite tổng sau merge: 105 case
  (dev: 101 PASS + 4 SKIP; CI có tun: 105 PASS).

## Rủi ro đã khai

- CI runner có thể không có `/dev/net/tun` (D34 nói "kiểm lại bằng probe")
  → suite SKIP thay vì đỏ; NET10–12 chỉ PASS thật khi device tồn tại.
- pasta `-t` bind host port: nếu FPORT bị chiếm → pasta lỗi → up fail-closed
  → case FAIL rõ (port chọn random 19700–19900, trùng xác suất thấp).
- Single-source pin (passt.top): đã nêu ở mục 1.
