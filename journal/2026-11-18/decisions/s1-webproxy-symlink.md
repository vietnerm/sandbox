# D31 — webproxy host bridge kháng symlink (S1, CWE-59 link following)

- Ngày: 2026-11-18. Issue: #41 (`type:security`, `priority:p1`). Nhánh:
  `sec/s1-webproxy-symlink`. Phạm vi: chỉ host-side `tools/webproxy.py` + caller
  `sandbox/minict.sh`; payload và giao thức 2 chặng (D16) không đổi.

## Bối cảnh

Cầu web 2 chặng D16: host `webproxy.py` nối TCP `127.0.0.1:<port>` → unix socket
`<rootfs>/.minict-proxy.sock`. Socket nằm trên disk rootfs của instance; payload
chạy cùng host uid (single-uid mapping) và ghi được vào rootfs riêng của mình, nên
payload **thay socket bằng symlink được** (hoặc symlink một ancestor dir) — host
proxy `asyncio.open_unix_connection(path)` resolution theo symlink **mỗi lần
connect** → payload chạm được unix socket tuỳ ý của host cùng uid (dbus, ssh-agent,
systemd private…). Lớp tấn công giống CVE-2018-15664 (docker cp symlink-race) và
CVE-2026-7374 (OpenShift — symlink để truy cập unix socket host, CWE-59).

## Quyết định — resolver kháng symlink, fail-closed, per-connection

1. **Root dir mở 1 lần** lúc startup: `os.open(root, O_RDONLY|O_DIRECTORY|
   O_NOFOLLOW|O_CLOEXEC)` — rootfs dir không phải thứ payload đổi được (là mount
   root của chính nó); startup sai → thoát rc=2.
2. **Mọi thành phần in-root đi bằng openat + O_NOFOLLOW**: ancestor dir mở
   `O_RDONLY|O_DIRECTORY|O_NOFOLLOW` nối tiếp bằng `dir_fd`; thành phần cuối mở
   `O_PATH|O_NOFOLLOW`. Symlink ở **bất kỳ** tầng nào → open fail → từ chối
   connection (không fallback, không retry theo path).
3. **Thành phần cuối phải là socket**: `fstat` trên fd O_PATH — không `S_ISSOCK`
   → đóng connection. Lưu ý trung thực: check này là **defense-in-depth** (fail
   nhanh + log rõ), không có mutant quan sát được từ ngoài (connect vào file
   thường cũng ENOTSOCK như nhau) — không test nào claim phủ nó (tránh test
   vacuous kiểu #38).
4. **Connect qua `/proc/self/fd/<fd>`**: magic link của kernel resolve đúng inode
   mà fd đã pin → xoá cửa sổ TOCTOU giữa check và connect (pattern socket
   activation của systemd; đối chiếu phân tích O_PATH của cyphar cho
   CVE-2018-15664). Payload swap path sau khi mở → vẫn nối vào socket relay
   GỐC, không bao giờ socket của attacker.
5. **Per-connection, không cache**: resolver chạy lại mỗi connection; tamper
   xảy ra sau N connection vẫn bị chặn ở connection N+1.
6. **CLI**: `--unix <full-path>` → `--root <rootfs-dir> --sock <in-root-path>`
   (resolver cần ranh giới rootfs để đi openat; caller duy nhất `minict.sh web`
   đã cập nhật). minictd (Go) khi mở GĐ2 phải dùng pattern tương tự ở bridge
   host-side của nó — GAPS S1 đã cập nhật ghi chú fix.

## Trade-off

- Chi phí: 1–3 syscall openat/connection (µs, host-side) — không chạm cold path
  của minict; perf-check PASS không đổi (1.97 ms / 1188 kB / 2.04 MB).
- Mất khả năng "tự chữa": nếu relay chết nhưng socket file còn (stale), proxy
  vẫn refuse như cũ — không đổi so với trước.
- O_NOFOLLOW chặn cả symlink "hợp lệ" do người dùng chủ đích tạo — chấp nhận:
  bridge là đường ống hệ thống, không phải API công khai.

## Test + negative control (matrix, cùng ngày)

Suite mới: WEB7 (symlink→file thường: refuse, fail-closed), WEB8 (symlink→socket
host của attacker: **zero leak**, marker chấp nhận-connection phải vắng mặt),
WEB9 (standalone resolver: positive control qua openat chain + `/proc/self/fd`
connect trên kernel 5.10; ancestor symlink → refuse + zero leak, attacker bind
trùng basename `real.sock` để leak quan sát được chứ không chỉ refusal).

Mutant (bắt buộc theo SIA — mỗi test mới phải bắt được mutant):

| Mutant | Thay đổi | Kỳ vọng | Kết quả |
|---|---|---|---|
| M1 | bỏ resolver, `connect(path)` như code cũ | WEB8 FAIL + WEB9 FAIL | đúng (2 FAIL) |
| M2 | bỏ O_NOFOLLOW thành phần cuối | WEB8 FAIL, WEB9 PASS | đúng (1 FAIL) |
| M3 | bỏ O_NOFOLLOW ancestor | WEB9 FAIL, WEB8 PASS | đúng (1 FAIL) |

WEB1–WEB6 PASS mọi mutant (chức năng chính không vỡ). Suite gốc 9/9 ×2 runs.

## Bài học

- Test bỏ mutant qua: lần chạy đầu WEB9 FAIL dưới M2 **không phải** do mutant mà
  do **random port dải 22000–22999 đụng dải PORT3** (PORT2+500 tối đa 22499) —
  proxy EADDRINUSE chết → positive control fail im lặng → sai kết luận mutant.
  Fix: dải port tách bạch (23000+) + check listener trước khi assert refusal +
  report nguyên nhân fail loud ("positive control failed"). Port allocation giữa
  các case trong cùng suite phải không chồng lấn.
