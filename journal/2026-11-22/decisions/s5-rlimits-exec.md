# D54 — S5: spec rlimits cho exec'd processes (issue #121, Phiên 19, 2026-11-22)

## Ngữ cảnh

GAPS.md S5: payload path (`minict.c apply_spec_rlimits`, D23) áp dụng
`ulimit nofile=…` / `nproc=…` từ MINICT_SPEC BEFORE capdrop. Exec path
(`minict-exec.c`) join instance SAU capdrop và bỏ qua bước này — process
exec'd vào instance chạy với host default rlimit, vượt giới hạn operator
đặt (vd. `nofile=64` bị bỏ qua, exec mở 1024+ fd). Lỗ đo được (repro
pre-fix trong issue #121): run `ulimits=["nofile=64:64"]`, exec
`ulimit -n` → host default, không 64.

## Quyết định

Chọn **truyền rlimit qua env-file với prefix `MINICT_RLIMIT_`**, parse
riêng trong minict-exec (không setenv):

1. **Daemon (`exec.go` setns mode)**: load `spec.json` (byte-exact
   RunSpec), parse `spec.Ulimits` (nofile/nproc only — parity
   `buildSpecMinict`), append `MINICT_RLIMIT_<NAME>=soft[:hard]` vào
   env-file content (sau req.Env lines). NAME uppercase (NOFILE/NPROC).
   PidsLimit NOT mapped cho exec (xem residual).
2. **minict-exec.c**: trong `load_env_file`, detect prefix `MINICT_RLIMIT_`
   → `record_rlimit()` vào mảng riêng `g_spec_rlimit[]`; KHÔNG add vào
   `g_envkv` (R2 parity — rlimit config không leak vào env của exec'd cmd).
   Parse soft/hard: decimal hoặc "unlimited" (RLIM_INFINITY), soft>hard →
   die (config lie, parity minict.c). Fail-closed exit 125 cho mọi parse
   error (contract bug, không drop im lặng).
3. **Apply trong child**: `apply_spec_rlimits()` gọi SAU `harden_exec_fds`
   (RLIMIT_CORE=0 đã set, KHÔNG bị overwrite) và TRƯỚC `install_seccomp` +
   `drop_all_caps`. Lý do thứ tự (D23 parity): raising hard limit cần
   CAP_SYS_RESOURCE, held trong joined userns U1 đến khi capdrop — sau
   capdrop chỉ lower được. setrlimit không trong seccomp blocklist nên
   order vs install_seccomp không bắt buộc, nhưng giữ "limits → filters →
   caps → exec" cho rõ ràng.
4. **Parity hai resource**: chỉ NOFILE + NPROC (minict.c parser cũng chỉ
   hai). Resources khác (memory/cpu/pids) do cgroups v2 quản (D48), không
   qua rlimit.

## Phủ nhận / residual

- **PidsLimit NOT mapped cho exec**: under cgroups v2, exec'd process nằm
  trong instance cgroup → `pids.max` đã bound (D48). Không cgroups →
  per-uid nproc approximation mang hazard S2 (shared limit starve shim);
  để payload path alone (payload map PidsLimit→nproc chỉ khi cgroups
  không enforce). Residual: exec trên host không cgroups không có pid bound
  riêng — nhưng cũng không có pid bound cho payload (S2 residual).
- **`loadSpec` fail**: spec.json missing (instance tạo trước S5 hoặc
  corrupt) → `os.IsNotExist` → return empty rlimit lines (no regression,
  exec behaves as before). Lỗi khác → fail-closed (return error → API 500).
- **Không mutant skip-apply local**: dev env không Go → không build
  minictd → không chạy MD37 local. CI runner (ubuntu-24.04, Go stable)
  chạy MD37 + mutant. Build C clean (`-Wall -Wextra`), selftest seccomp OK.

## Tests

- **MD37** (`tests/test_minictd.sh`): run instance `ulimits=["nofile=64:64"]`,
  exec `sh -c 'ulimit -n'` → stdout == "64" (busybox `ulimit` đọc
  RLIMIT_NOFILE soft). Negative (mutant, chạy SCR): skip `apply_spec_rlimits()`
  call → MD37 đỏ (ulimit -n = host default, ≠ 64).
- **MD0** (seccomp parity minict.c vs minict-exec.c): giữ nguyên — S5 không
  đụng `install_seccomp`/blocklist/enum (code mới ở section rlimit riêng,
  `awk` extraction của MD0 không取 section đó).
- **R2 parity** (MD10): rlimit config (`MINICT_RLIMIT_*` entries) KHÔNG xuất
  hiện trong `env` output của exec'd cmd — loader tách riêng, không setenv;
  MD10 đã check daemon env/token không leak, MD37 ngầm kiểm (exec chạy
  `ulimit -n`, không in rlimit entries).

## Bài học đo được

- **Env-file làm channel đa năng**: thay thêm flag `--rlimit` (repeatable,
  phức tạp cho daemon), dùng prefix `MINICT_RLIMIT_` trong env-file có sẵn.
  Trade-off: phải tách riêng trong loader để không leak vào env cmd (R2).
- **D23 áp dụng cho exec path**: luật "setrlimit BEFORE capdrop" không chỉ
  cho payload — mọi process join userns và cần raising hard limit đều phải
  apply trước capdrop. Exec path join SAU capdrop của payload, nhưng CHILD
  của minict-exec có full cap U1 trước `drop_all_caps()` của chính nó →
  window raise hard limit tồn tại (đo được qua logic, e2e CI MD37).
- **cve-check NVD transient**: NVD rate-limited 2026-11-22 (lần đầu trong
  phiên) → cve-check timeout 60s, JSON không write kịp. .md output có 7
  watchlist rows; JSON synthesized từ .md (field `cve`+`cvss`); cve-gate
  rc=0 → 0 CVE mới ngoài watchlist. Caveat AUDIT §F giữ nguyên (fixed-list
  script không chứng minh 0 CVE mới tuyệt đối).
