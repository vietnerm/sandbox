# D76 — Track M M2 residuals d–i: stack pasta_ports, ready exec, http timeout, down error policy (+ fix 404 mapping daemon)

- **Ngày:** 2026-11-26 (Phiên 40)
- **Issue:** #203 · **Nhánh:** `feat/203-m2-residuals`
- **Tiền đề:** D75 (#199 — stack.json up|down|ps a–c), D67 (#165 — grammar pasta_ports), roadmap Track M M2 "Còn thiếu, làm trước M3" (owner review 2026-11-26).

## Quyết định

1. **d — `pasta_ports` trong stack.json** (map thẳng `RunSpec.PastaPorts`):
   - `ServiceDef.PastaPorts []string` → `spec.PastaPorts` nguyên trạng; daemon
     re-validate fail-closed (D67 vẫn là nguồn sự thật).
   - Client pre-validate cùng grammar (`validatePastaPortsStack`): host
     1024..65535, ns 1..65535, `[tcp|udp/]`, không range/space, ≤ 16, không dup
     host bind. LÝ DO: lỗi config phải chặn ở PARSE (trước khi chạy instance
   nào), không phải sau khi rollback oan một phần stack.
2. **e — ready `exec` + validate parse-time**: `ReadyDef.Cmd`; `waitForReady`
   case "exec" → `ExecV2{Cmd, TimeoutS:5}`, ExitCode 0 = ready (mô hình
   healthcheck compose); fail → poll tới deadline (payload có thể đang boot).
   `validateReady` chặn type lạ + port/cmd sai LÚC PARSE, bỏ đường "unsupported"
   lúc chạy (giữ default fail-closed cho caller trực tiếp).
3. **f — http timeout**: `readyHTTPClient()` = `&http.Client{Timeout: 2s}` tạo
   trong `waitForReady` (http.Get dùng DefaultClient KHÔNG timeout — có thể treo
   quá timeout_s). Port http/tcp validate 1..65535 parse-time.
4. **g — down error policy**: `errors.Is(err, contract.ErrNotFound)` → bỏ qua
   im lặng; lỗi khác → gom, in, `stackDown` trả error (CLI exit ≠ 0);
   "removed" chỉ in khi Remove thật; stop lỗi (không NotFound) → KHÔNG remove.
5. **h — MD64/MD65**: MD64 env-gated pasta (cùng gate MD47–49): stack 2 service
   db(exec-ready)+web(pasta fwd + tcp-ready), curl host qua forward, down sạch
   dir + listener; MD65 chạy mọi nơi: ready-fail rollback, down-missing exit 0
   không in "removed", down 1 sống 1 thiếu chỉ in "live removed".
6. **i — README + examples**: README dòng `stack.yaml` → `stack.json` +
   nhãn `thử nghiệm`; thêm `examples/04-stack/` (stack.json mẫu + README).
7. **Refactor nhỏ**: stackUp/Down/Ps/waitForReady nhận `contract.Runtime`
   (interface) thay vì `*contract.HTTPClient` — unit-test được policy bằng
   stub runtime; không đổi wire/CLI.
8. **Fix daemon 404 mapping (phát lộ bởi MD65)**: `Store.lock()` trả raw
   `*fs.PathError` khi instance dir không tồn tại → `mapErr` → **500**, trong
   khi comment của chính lock() và runtime.go:206 ghi rõ "Stop/Start/Remove
   treat a missing dir as not-found" / "one contract.ErrNotFound (404) — same
   mapping as exec/stop". Fix: lock() map `os.IsNotExist` → `contract.ErrNotFound`;
   Stop/Start → 404 (docker parity); Remove giữ idempotent nil (204).
   Callers cập nhật `os.IsNotExist(err) || errors.Is(err, contract.ErrNotFound)`.
   Đổi daemon ở đây KHÔNG vi phạm "Daemon không đổi" của M2 (ý là không thêm
   tính năng daemon cho stack): đây là bug mapping lỗi có test chứng minh
   (MD65 đỏ trước fix, xanh sau fix) + unit `TestMissingInstanceErrNotFound`.

## Hai phép thử (manifest §3)
- **Còn nhẹ không?** Có — client-only helpers + 1 fix mapping lỗi 1 dòng logic
  trong lock(); không thêm dep, không process nền, không YAML parser.
- **Bỏ đi core còn hữu dụng không?** Có — xoá stack.go/examples thì daemon +
  CLI gốc vẫn nguyên hoạt động (chỉ mất lớp tiện ích multi-instance).

## Bằng chứng
- `evidence/m2-residuals.txt`: make test 10 suites 0 FAIL; test-web 9/9;
  test-minictd 58 PASS + 9 SKIP + 0 FAIL (MD65 PASS sau fix, MD64 SKIP
  env-gated — PASS thật trên runner CI có /dev/net/tun); perf 2.00 ms /
  1192 KB / 2.05 MB; host baseline unchanged; go unit toàn bộ ok.
- Mutants unit: parse-validation tests chặn từng nhánh validate; stub-runtime
  tests chặn cả 4 nhánh policy của stackDown (missing/happy/remove-err/stop-err).
