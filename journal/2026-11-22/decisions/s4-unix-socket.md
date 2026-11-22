# D53 — S4: unix-socket listener fail-closed (issue #116, Phiên 18, 2026-11-22)

## Ngữ cảnh

GAPS.md S4: API của minictd chỉ serve HTTP trên TCP (mặc định 127.0.0.1:7601).
Loopback TCP được chia sẻ giữa mọi user trên host — user khác connect được nếu
biết port; bearer token là tuyến phòng thủ duy nhất và đi clear trong HTTP
header. Mục tiêu manifest (VPS cluster) cần transport security cho non-loopback.
GAPS đề xuất "unix-socket listener or TLS".

## Quyết định

Chọn **unix-socket listener** (không TLS trong phiên này):

1. **Transport security = perms filesystem** (mô hình podman rootless service):
   socket 0600 trong dir 0700 → chỉ uid owner connect được (unix(7)). Không PKI,
   không cert rotation, đúng posture rootless (không root, không setuid).
2. **Token auth GIỮ NGUYÊN trên mọi transport** (R3, không flag "no-auth-on-unix"):
   same-uid process (payload bị chiếm — chính là threat model chính của sandbox)
   vẫn connect được socket; auth là lớp phòng thủ sâu và giữ MỘT code-path auth
   duy nhất (MD2 check chạy trên cả hai transport qua MD36).
3. **Fail-closed cứng ở listener** (`cmd/minictd/listener.go`):
   - socket path là symlink → refuse (posture D31 của #41 — anti-hijack);
   - non-socket file tại path → refuse (không ghi đè im lặng);
   - stale socket (run trước chết không dọn) → replace (unlink + rebind);
   - dir lỏng (khác 0700) → chmod 0700; chmod lỗi hoặc mode cuối sai → die;
   - socket bind xong chmod 0600 + Lstat verify — umask của operator không ảnh hưởng.
4. **TCP trở thành opt-in**: `--listen ""` tắt TCP (unix-only posture, MD36 check
   bằng `ss -tlnp` theo pid); cả hai rỗng → die "no listener" (không đoán).
5. **Cleanup**: socket unlink khi shutdown sạch (SIGTERM/SIGINT) — stale chỉ xảy
   ra khi crash, và openUnixSocket xử lý được.

## Phủ nhận / residual

- Unix socket chỉ bảo vệ LOCAL — TLS cross-host cho multi-node thật là residual
  (ghi rõ GAPS.md S4 + roadmap GĐ2 mục 1).
- `$MINICTD_SOCKET`/`--socket` của CLI thắng `--addr` khi cả hai được đặt — tài
  liệu hoá trong README (transport resolve: socket > addr > default TCP).

## Tests (mỗi assertion có mutant đối chứng — không vacuous)

- Unit (`cmd/minictd/listener_test.go`, 6 case): happy (dir 0755→0700, socket
  0600, dial được), stale-socket replace (tạo bằng raw syscall.Bind — Go
  net.Listen tự unlink khi Close nên không tạo được stale thật), symlink refuse,
  regular-file refuse (+ file không bị đụng), file-as-dir refuse, empty path.
- E2e (`tests/test_minictd.sh`):
  - **MD35**: boot `--listen :S4 --unix-socket` với dir PRE-CREATED 0755 →
    healthz 200 trên CẢ HAI transport, stat socket == 600, stat dir == 700.
    Mutant đối chứng: M1 (bỏ chmod socket 0600) → stat đỏ; M2 (bỏ chmod dir)
    → stat dir đỏ.
  - **MD36**: boot `--listen ""` unix-only → `ss -tlnp` không có listener thuộc
    pid daemon (mutant M-tcp: bind TCP thêm → đỏ), healthz qua socket 200,
    no-token qua socket → 401 (mutant M-auth: skip auth trên unix → đỏ),
    CLI `sandbox --socket ... list` rc=0.

## Bài học đo được

- Go `net.Listen("unix")` từ lâu đã unlink socket khi Close — stale socket chỉ
  tạo được qua crash/c-bind; test stale dùng raw `syscall.Bind` để mô phỏng đúng.
- `ss -tlnp` lọc theo `pid=<n>,` là cách chắc nhất khẳng định "daemon này KHÔNG
  bind TCP" — probe connect tới port trống chỉ chứng minh "port đó rỗng".

## Residual từ SIA PR #118 (Phiên 18)

- **TOCTOU Lstat→Remove→ListenUnix** (#119, from-audit low): cửa sổ swap file cùng
  path giữa check và bind — chỉ same-uid exploit được (dir đã ép 0700 trước),
  không elevation; fix hướng bind-then-verify-by-inode khi GĐ2 chạm lại.
- **M3 provenance**: M3 (disable symlink-branch) bị mask ở mức branch — nhánh
  non-socket refuse vẫn chặn symlink (defense in depth, property giữ). Mutant
  chạy trên working-tree SDDA (unit TestOpenUnixSocketRefusesSymlink PASS =
  property giữ); SCR chạy 4/4 mutant còn lại (M1/M2/M-tcp/M-auth) trên clone trắng.
- **Dual-transport opt-in**: `--unix-socket` + `--listen` cùng đặt = phục vụ cả
  hai (unix-only là opt-in qua `--listen ""`); log listening ghi rõ `tcp=off`;
  README tài liệu resolve order. Không đổi code (SIA low #4).
