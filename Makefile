# Root Makefile — entry points required by PROMPT.MD / CI.
# The binary build itself lives in sandbox/Makefile.

.PHONY: all build minictd rootfs rootfs-alpine test test-escape test-escape-alpine test-seccomp test-netns test-alpine test-web test-minictd test-pasta unicode-check test-unicode test-cve-gate perf-check cve-check bench clean help

all: build

build:
	$(MAKE) -C sandbox

# GĐ2 control plane: minictd daemon (Go, stdlib only) + minict-exec (setns)
minictd:
	$(MAKE) -C minictd all

rootfs:
	bash sandbox/mkrootfs.sh rootfs

rootfs-alpine:
	bash sandbox/mkrootfs-alpine.sh rootfs-alpine

# isolation suite (29 cases) + seccomp blocklist suite (21 cases)
# + netns suite (7 cases) + alpine-rootfs suite (8 cases)
# + escape-alpine suite (29 cases) + cve-check tool suite (4 cases)
# + pasta connectivity suite (9 cases: PT1/PT2/NET11C/PT3C always run,
#   NET10/NET11/NET12/PT3/AB1 env-gated SKIP without /dev/net/tun — D37/D42)
test: build rootfs
	bash tests/run-suites.sh

test-escape: test

# pasta connectivity suite standalone (issue #57, D37)
test-pasta: build rootfs
	bash tests/test_pasta.sh

# the WHOLE isolation/escape suite re-run against the alpine distro rootfs
# (roadmap GĐ1 mục 2 residual, issue #5 — more attack surfaces than busybox)
test-escape-alpine: build rootfs-alpine
	bash tests/test_escape_alpine.sh

test-seccomp: build rootfs
	bash tests/test_seccomp.sh

test-netns: build rootfs
	bash tests/test_netns.sh

test-alpine: build rootfs-alpine
	bash tests/test_alpine.sh

test-web: build rootfs
	bash tests/test_web.sh

# minictd: go unit tests (issue #50 regression) + e2e (20 cases): real daemon -> patched minict -> minict-exec
test-minictd: build rootfs minictd
	cd minictd && $(or $(GO),go) test -count=1 ./...
	bash tests/test_minictd.sh

# perf-guard gate: cold start <= 100 ms, RSS <= 10240 kB, disk <= 20 MB
# (thresholds + measured baseline live in docs/perf-baseline.md)
perf-check: build rootfs
	bash tools/perf-check.sh

cve-check:
	python3 tools/cve-check.py

# hidden / bidi Unicode (Trojan Source) scan of every tracked file; --fix strips
unicode-check:
	python3 tools/unicode-guard.py

test-unicode:
	bash tests/test_unicode.sh

# cve-daily issue gate (issue #56): 7 cases, no network
test-cve-gate:
	bash tests/test_cve_gate.sh

bench: build rootfs
	bash benchmarks/bench.sh

clean:
	$(MAKE) -C sandbox clean
	$(MAKE) -C minictd clean

help:
	@echo "make build        - build minict binary"
	@echo "make rootfs       - build busybox rootfs (rootless)"
	@echo "make rootfs-alpine- build alpine minirootfs rootfs (rootless)"
	@echo "make test         - isolation (29) + seccomp (21) + netns (7) + alpine (8) + escape-alpine (29) + cve-check tool (4) + pasta (9; NET10-12/PT3/AB1 SKIP without tun), all suites always run"
	@echo "make test-pasta  - pasta connectivity suite only (issue #57, D37)"
	@echo "make test-escape  - same suites (alias, CI requirement)"
	@echo "make test-escape-alpine - isolation suite re-run on alpine rootfs (29 cases)"
	@echo "make test-web     - web service suite (6 cases)"
	@echo "make minictd      - build minictd daemon + minict-exec (needs Go >= 1.23)"
	@echo "make test-minictd - minictd e2e suite (19 cases)"
	@echo "make test-seccomp - seccomp blocklist suite (20 cases)"
	@echo "make test-netns   - netns isolation suite (7 cases)"
	@echo "make test-alpine  - alpine rootfs suite (8 cases)"
	@echo "make perf-check   - cold start / RSS / disk vs thresholds"
	@echo "make cve-check    - NVD check of the CVE watchlist"
	@echo "make unicode-check - scan for hidden/bidi Unicode (Trojan Source)"
	@echo "make test-unicode  - unicode-guard self-test (13 cases)"
	@echo "make bench        - full benchmark tables"
