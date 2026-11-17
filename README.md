# sandbox-lab

Nghiên cứu + hiện thực hóa sandbox nhẹ rootless cho full web service stack —
không Docker, không root, chỉ cơ chế kernel Linux có sẵn.

## Kết quả hiện tại (2026-11-17)

**minict v0.1** — mini-container ~260 dòng C:

| metric | đo được | target |
|---|---|---|
| cold start | 1.2 ms (p50) | < 2s |
| RAM overhead | ~1.1 MB | < 30 MB |
| disk base | 2.0 MB | < 50 MB |
| rootless | ✅ (uid 1001) | bắt buộc |
| escape tests | 16 PASS (13 cũ + 3 gap có chủ đích) + 6 web tests | — |
| web trong sandbox | busybox httpd, HTTP 200, bind 127.0.0.1, ready 18ms | DoD mục 1 |

Isolation: user ns + mount ns + pid ns + uts ns + ipc ns, pivot_root (host fs
tàng hình), capability drop, no_new_privs, PDEATHSIG + pidns teardown.
Isolated dirs: tmpfs /tmp; /dev = bind 4 node (null/zero/urandom/random).

## Quick start

```bash
make -C sandbox              # build minict
bash sandbox/mkrootfs.sh     # build busybox rootfs (~2MB, rootless)
bash tests/test_isolation.sh # 16 isolation tests

# lifecycle
./sandbox/minict.sh up demo            # instance với private rootfs copy
./sandbox/minict.sh web demo 8080      # web: busybox httpd trong sandbox, bind 127.0.0.1
curl -s http://127.0.0.1:8080/         # HTTP 200 từ bên trong sandbox
./sandbox/minict.sh exec demo -- /bin/sh -c 'echo hi && id && hostname'
./sandbox/minict.sh shell demo         # shell vào instance fs
./sandbox/minict.sh ps
./sandbox/minict.sh down demo
./sandbox/minict.sh bench              # benchmark
```

## Cấu trúc

```
sandbox/minict.c        core (C, 1 file, 0 dependency ngoài libc)
sandbox/mkrootfs.sh     rootfs builder (apt-get download + dpkg-deb -x, rootless)
sandbox/minict.sh       lifecycle CLI
tests/test_isolation.sh escape/isolation tests (16 cases)
tests/test_web.sh web-in-sandbox tests (6 cases)
Makefile                entry points: build/test/test-escape/test-web/perf-check/cve-check
.github/workflows/      ci, cve-daily, perf-guard, issue-triage, research-log
benchmarks/             bench.sh + kết quả hàng ngày
research/               research notes hàng ngày
docs/decision-log/      quyết định + lý do
docs/skills.md          pitfalls + lệnh hữu ích (cập nhật hàng ngày)
docs/roadmap.md         đã xong / đang làm / tiếp theo
tools/cve-check.py      CVE check hằng ngày (NVD API, watchlist trong file)
docs/cve-watchlist.md   bảng CVE kernel/engine + trạng thái + mitigation
docs/daily-log-*.md     nhật ký hằng ngày (Research/Develop/CVE/Validation)
tests/evidence/         bằng chứng test + integrity check host
```

## Giới hạn đã biết (ngày 1)

- Web bind loopback-only (chưa có netns — D7); không httpd.conf/auth/proxy/CGI
  (CVE-2026-124739/88829 re-check trước khi bật).
- Không netns (chia sẻ host network), không /proc trong sandbox (host seccomp chặn
  mount proc), không cgroup limits (env chỉ có cgroups v1), không overlayfs
  (private copy 2MB/instance thay thế, 25ms), `exec` tạo ns set mới trên cùng
  rootfs chứ không setns vào daemon (rootless không đủ caps để setns).
- SIGTERM từ host vào process trong userns bị môi trường nuốt (rc=0, không delivery) —
  cleanup dựa SIGKILL + PDEATHSIG + pidns teardown. Chi tiết: docs/decision-log/2026-11-17.md.

## Quy trình hằng ngày
A Research → B Develop → C CVE check → D No-Compromise Validation → daily-log.
Chi tiết: docs/roadmap.md (mục "Quy trình hằng ngày").

## Đang đọc
- research/2026-11-17.md — cơ chế userns/mountns/pivot_root + kết quả đo thực nghiệm
- docs/roadmap.md — kế hoạch seccomp-bpf → netns → full web stack
