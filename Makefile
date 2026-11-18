# Root Makefile — entry points required by PROMPT.MD / CI.
# The binary build itself lives in sandbox/Makefile.

.PHONY: all build minictd rootfs rootfs-alpine test test-escape test-seccomp test-netns test-alpine test-web test-minictd unicode-check test-unicode perf-check cve-check bench clean help

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

# isolation suite (17 cases) + seccomp blocklist suite (20 cases)
# + netns suite (7 cases) + alpine-rootfs suite (8 cases) — ALL suites run
# even if an earlier one fails (tests/run-suites.sh), aggregate exit code
# (issue #14).
test: build rootfs
	bash tests/run-suites.sh

test-escape: test

test-seccomp: build rootfs
	bash tests/test_seccomp.sh

test-netns: build rootfs
	bash tests/test_netns.sh

test-alpine: build rootfs-alpine
	bash tests/test_alpine.sh

test-web: build rootfs
	bash tests/test_web.sh

# minictd e2e (19 cases): real daemon -> patched minict -> minict-exec
test-minictd: build rootfs minictd
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

bench: build rootfs
	bash benchmarks/bench.sh

clean:
	$(MAKE) -C sandbox clean
	$(MAKE) -C minictd clean

help:
	@echo "make build        - build minict binary"
	@echo "make rootfs       - build busybox rootfs (rootless)"
	@echo "make rootfs-alpine- build alpine minirootfs rootfs (rootless)"
	@echo "make test         - isolation (17) + seccomp (20) + netns (7) + alpine (8), all suites always run"
	@echo "make test-escape  - same suites (alias, CI requirement)"
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
