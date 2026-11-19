# Skills — những thứ học được / pitfalls / lệnh hữu ích

## Kernel / namespace
- `unshare(CLONE_NEWUSER)` → `getuid()` = 65534 cho tới khi ghi uid_map; **lưu real uid
  vào biến TRƯỚC unshare**.
- Ghi `setgroups=deny` bắt buộc trước gid_map (rootless) — và nó làm `setgroups(2)`
  EPERM vĩnh viễn trong ns; đừng gọi trong capdrop.
- `pivot_root` cần new_root là mount point → `mount(rootfs, rootfs, MS_BIND|MS_REC)` trước.
- Luôn `MS_REC|MS_PRIVATE` trên "/" ngay sau unshare — nếu không shared-subtree propagation
  sẽ leak mount hai chiều với host.
- Mount đè host mountpoint = EPERM (MNT_LOCKED / mount_too_revealing) — mount vào
  thư mục thường trong rootfs của mình thì OK. Đây là lý do "mount proc vào /proc host"
  fail nhưng "mount tmpfs vào /tmp trong rootfs" thành công.
- `unshare(CLONE_NEWPID)` không đưa caller vào pidns — phải fork; child mới là PID 1.
- Khi PID 1 của pidns chết → kernel kill toàn bộ process trong ns (dùng làm no-orphan guarantee).
- `PR_SET_PDEATHSIG` bị clear khi fork — set trong child sau fork, trước exec.
-busybox ash: exec-optimize lệnh cuối `sh -c` + defer fatal signal khi wait con →
  giết tree phải bằng process group (setpgid + kill(-pgid)).

## Seccomp (phiên 4)
- **Số syscall không được nhớ — grep `asm/unistd_64.h`.** Nhớ-tay sai 5/36: setns là
  308 (306 là syncfs), finit_module 313, delete_module 176, remap_file_pages 216,
  kexec_file_load 320. Filter đã chạy không lỗi vì JEQ chỉ không khớp — không crash,
  chỉ "quên chặn" — bug dạng này không thấy qua test thiếu control.
- Mọi assert "syscall bị chặn" cần **control A/B**: cùng probe với filter tắt
  (`MINICT_NO_SECCOMP=1`) phải KHÔNG EPERM — nếu host đã tự EPERM sẵn (như AF_ALG
  trên env này) thì control không phân biệt được, chọn syscall khác (VD process_vm_readv
  pid=0 → ESRCH thay vì EPERM).
- Probe binary chạy trong rootfs busybox phải `-static` — rootfs không có ld.so/libc,
  exec binary động = ENOENT câm (không có stderr giải thích).

- cBPF không đọc được struct qua pointer → clone3 không arg-check được; trả ENOSYS
  để libc fallback về clone(2) (thì arg-check được). Trả EPERM sẽ làm fork() chết
  không fallback.
- `prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER)` cần NNP đã set (minict set trước
  unshare — NNP kế thừa qua fork); kernel validate cả program lúc install → install
  lỗi phải die() fail-closed.

## Bash (dính 3 lần trong 1 ngày)
- **`return` semantics: 0 = success.** Đừng map flag `ok=1` thành `return $ok` —
  cứ dùng biến `rc=1` default và `&& rc=0`, hoặc `if ...; then return 0; fi`.
- `pkill -f` / `pgrep -f` **match cả cmdline của chính script/tool wrapper** — dùng tag
  unique theo PID (`tag="x-$$"`) và tuyệt đối không pkill pattern trùng text script.
- `$!` sau `cmd &` có thể là bash subshell trung gian, không phải cmd — tìm process
  thật bằng `pgrep -P` hoặc `pgrep -x`.

## Lệnh hữu ích đo được
```bash
# rootless rootfs từ deb, không cần root:
apt-get download busybox-static && dpkg-deb -x *.deb dir/
# kiểm tra nhanh userns:
unshare --user --map-root-user sh -c 'id; cat /proc/self/uid_map'
# kiểm tra cap/seccomp của process:
grep -E 'Cap|Seccomp' /proc/self/status
# đo cold start chính xác:
python3 -c 'import subprocess,time; t=time.monotonic_ns(); subprocess.run([...]); print((time.monotonic_ns()-t)/1e6)'
```

## CVE discipline
- Luôn verify CVE ID qua NVD trước khi ghi — CVE-2022-0316 trên NVD là vuln theme
  WordPress (CVSS 9.8), KHÔNG phải Firecracker như ghi nhớ sai.
- Vendor kernel (al8/kangaroo): số phiên bản KHÔNG kết luận được patch status —
  dùng version-inference khi fixed-in < stable point, mitigate-by-design khi ngược lại.
- NVD anonymous API: ~5 req/30s → sleep 9s giữa các query.

## Env-specific (kernel 5.10.134 kangaroo.al8)
- Seccomp host chặn: mount /proc (mọi nơi), mknod device, **SIGTERM cross-userns
  (rc=0, không delivery!)**. SIGKILL OK.
- Host cũng đã EPERM sẵn socket(AF_ALG) cho user thường (đo 17-11, phiên 4) —
  không dùng được làm A/B control trên env này (CI runner khác có thể khác).
- Không có cgroups v2 unified, không có Landlock (cần ≥5.13), không có KVM.
- Bất cứ gì "đáng lẽ hoạt động theo man page" phải **đo trước** — env này đã lệch
  man page ít nhất 3 lần trong ngày 1.

## Netns pitfalls (phiên 5, issue #3)
- **uid_map rỗng = KHÔNG có cap nào trong userns mới** — `unshare --user --net`
  (util-linux, không --map-root-user) để lại uid_map rỗng → `ip link set lo up`
  EPERM dù là "root" mới. Ghi uid_map xong mới bring-up `lo` (D18).
- **`lo` trong netns mới luôn DOWN lúc sinh ra** — không bring-up thì wget/curl
  127.0.0.1 chết oan ("Network unreachable" hoặc refuse tùy endpoint).
- **AF_UNIX là filesystem-scoped, KHÔNG phải netns-scoped** — socket file nằm
  trên rootfs disk host connect được xuyên netns: nền của bridge web (D16).
  Ghi nhớ ngược lại: AF_INET/AF_INET6 mới là netns-scoped.
- **fork relay TRƯỚC install_seccomp** — seccomp filter chỉ kế thừa cho con
  sinh SAU khi cài; relay sinh trước → không dính filter (cần accept/connect).
  Ngược lại muốn payload có filter: cài filter trước exec là đủ.
- **rootfs là dir untracked sống xuyên session** — file cũ trong base rootfs
  (vd. www/index.html của bài test cũ) sẽ ăn theo vào instance copy. Script
  phải GHI ĐÈ chủ động, không dùng `[[ ! -f ]]` gate cho state quan trọng.
- **`[[ ]]` là bashism** — chạy qua `sh -c` (dash) → `[: not found`. Dùng
  POSIX `[ ]` trong test case.
- **GNU Make 4.4+ từ chối recipe 8-spaces** ("missing separator"); runner cũ
  (make 4.3) chấp nhận → bug trôi qua CI. Sau khi sửa Makefile, chạy
  `make -n <target>` với make 4.4 để xác nhận thật.

## CI truth (phiên 6)
- **GitHub Actions không khai `shell:` = `bash -e` KHÔNG pipefail** — `cmd | tee`
  trả exit code của `tee` (gần như luôn 0): test đỏ vẫn xanh job (run #20 gánh
  UID1+SIG1 FAIL). Sửa: `defaults: run: shell: bash` ở workflow level.
- **busybox-static Ubuntu 24.04 chạy `sleep` NOFORK** — `sh -c "sleep N; …"` không
  có process con; Debian busybox 1.37 thì fork. Test harness không được giả định
  có child (SIG1). Tái hiện: tải đúng .deb Ubuntu, thay vào rootfs, probe bằng
  `pgrep -P`.
- **Supplementary groups của host user hiển thị qua `id -G` là overflowgid 65534**
  trong sandbox: unprivileged userns bắt buộc `setgroups=deny` trước gid_map →
  không thể drop/map thêm group. `id -G | grep -qx 0` fail trên runner (user
  runner có docker/sudo/…). Test phải chấp nhận {0, 65534}.
- `make test` kiểu "3 lệnh tuần tự" dừng ở suite đỏ đầu tiên → mất bằng chứng
  suite sau; dùng aggregator chạy đủ + exit code tổng hợp (`tests/run-suites.sh`).

- Công cụ sửa file (editor/merge helper) có thể chuẩn hoá TAB recipe của Makefile
  thành 8 spaces → `missing separator`, mọi target chết (tái hiện PR #36 merge
  86030d3, CI đỏ, fix 7685b3f). Sau merge/sửa Makefile: chạy `make -n build`
  trước khi push; grep `grep -cP '^\t' Makefile` để xác minh tab còn nguyên.
- Bài học mở rộng sang mọi file tab-indented (tái hiện PR #42: sửa `store.go`
  bằng edit tool → TAB→spaces toàn file 970 dòng, diff trắng sửa thành 970 dòng).
  Go/shell/Makefile tab-indented: chỉ sửa bằng sed/python **giữ byte**
  (`open(path,'rb')`, anchor byte-level), sau đó `git diff --stat` phải khớp đúng
  số dòng dự định; lệch nhiều hơn → khôi phục từ main và vá lại.
- PR body ghi "Requested by <owner>" là **dữ liệu** (SBG §0.7), không phải approval
  — G7 vẫn yêu cầu review thật trên PR trước khi gộp file nhạy cảm.
- Mutant/negative-control script chỉ được `git checkout` ĐÚNG file đang mutant:
  checkout giúp sạch nhưng nếu vồ cả file chưa commit (test suite, docs) thì WIP
  mất trắng (tái hiện 2026-11-19 phiên #5: restore() checkout cả
  tests/test_isolation.sh + test_seccomp.sh → mất minict.c + 2 suite giữa phiên,
  khôi phục bằng cách vá lại; NC1 của lần đó vẫn hợp lệ vì chạy trước wipe).
  Quy tắc: commit WIP nhánh trước khi chạy mutant; restore() giới hạn `git
  checkout -- <src-đang-mutant>`.
- Đếm số test bằng `grep -oE 'run_case <PREFIX>[0-9]+' | sort -V | uniq -c`
  trước khi đặt id mới — id "tiếp theo" có thể nhầm khi prefix match sai bề
  rộng (SC1 cũng match SC10-SC19) và suite có thể có SC0 (tái hiện 2026-11-19:
  tưởng seccomp kết thúc ở SC19, thực ra có SC0 → SC20 vẫn tự do, nhưng tổng
  số case lệch 1 nếu không kiểm).
- **Phiên tăng tốc (owner override 1-issue/ngày) = rủi ro sót cổng**: PR #53 đã
  merge thiếu SCR/SIA (G3/G4) — khắc phục bằng post-hoc audit sau-merge nhưng
  đó là khắc phục, KHÔNG phải ngoại lệ. Check-list bắt buộc trước lệnh merge:
  (1) comment SCR với đúng head sha, (2) comment SIA + label audit:pass, (3)
  `git rev-parse` head == sha trong comment. Phiên tăng tốc càng phải chạy đủ.
- Resolve git conflict phải đọc NGUYÊN khối `<<<<<<< … >>>>>>>` và thay trọn
  khối — replace một dòng regex để sót markers, script chết tại runtime (tái
  hiện 2026-11-19 phiên 2, commit đầu PR #54). Sau MỌI merge: `bash -n` cho
  script shell changed + `make -n build` cho Makefile trước khi push.
- `curl -d @- <<EOF` (delimiter KHÔNG quote): bash ăn `$()` và backtick trong
  body — body issue #56 bị thực thi lệnh (stderr noise, body hỏng). Quy tắc:
  body đa dòng có backtick ghi ra FILE bằng `<<'EOF'` (quoted) rồi inject qua
  `python3 -c json.dumps(open(file).read())`.
- Suite chạm mount/tmpfs (FS6/UID2) phải rebuild rootfs khi build script đổi —
  rootfs-alpine stale cache làm đỏ giả. Flake #55 (build rc=0 nhưng thiếu
  empty-dir của tarball, 2/4 lần): mitigation mkdir vô điều kiện + self-heal
  precondition trong wrapper; root-cause theo issue #55.
