# Root Makefile — entry points required by PROMPT.MD / CI.
# The binary build itself lives in sandbox/Makefile.

.PHONY: all build rootfs test test-escape test-seccomp test-netns test-web perf-check cve-check bench clean help

all: build

build:
	$(MAKE) -C sandbox

rootfs:
	bash sandbox/mkrootfs.sh rootfs

# isolation suite (17 cases) + seccomp blocklist suite (20 cases)
# + netns suite (7 cases) — ALL suites run even if an earlier one fails
# (tests/run-suites.sh), aggregate exit code (issue #14).
test: build rootfs
	bash tests/run-suites.sh

test-escape: test

test-seccomp: build rootfs
	bash tests/test_seccomp.sh

test-netns: build rootfs
	bash tests/test_netns.sh

test-web: build rootfs
	bash tests/test_web.sh

# perf-guard gate: cold start <= 100 ms, RSS <= 10240 kB, disk <= 20 MB
# (thresholds + measured baseline live in docs/perf-baseline.md)
perf-check: build rootfs
	bash tools/perf-check.sh

cve-check:
	python3 tools/cve-check.py

bench: build rootfs
	bash benchmarks/bench.sh

clean:
	$(MAKE) -C sandbox clean

help:
	@echo "make build        - build minict binary"
	@echo "make rootfs       - build busybox rootfs (rootless)"
	@echo "make test         - isolation (17) + seccomp (20) + netns (7), all suites always run"
	@echo "make test-escape  - same suites (alias, CI requirement)"
	@echo "make test-web     - web service suite (6 cases)"
	@echo "make test-seccomp - seccomp blocklist suite (20 cases)"
	@echo "make test-netns   - netns isolation suite (7 cases)"
	@echo "make perf-check   - cold start / RSS / disk vs thresholds"
	@echo "make cve-check    - NVD check of the CVE watchlist"
	@echo "make bench        - full benchmark tables"
