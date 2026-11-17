# Root Makefile — entry points required by PROMPT.MD / CI.
# The binary build itself lives in sandbox/Makefile.

.PHONY: all build rootfs test test-escape test-seccomp test-web perf-check cve-check bench clean help

all: build

build:
	$(MAKE) -C sandbox

rootfs:
	bash sandbox/mkrootfs.sh rootfs

# isolation + escape suite (16 cases) + seccomp blocklist suite (16 cases)
# (the escape/isolation suites are currently flat scripts; splitting them
# into one suite per area is roadmap work)
test: build rootfs
	bash tests/test_isolation.sh
	bash tests/test_seccomp.sh

test-escape: test

test-seccomp: build rootfs
	bash tests/test_seccomp.sh

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
	@echo "make test         - isolation/escape (16) + seccomp (16) suites"
	@echo "make test-escape  - same suites (alias, CI requirement)"
	@echo "make test-web     - web service suite (6 cases)"
	@echo "make test-seccomp - seccomp blocklist suite (16 cases)"
	@echo "make perf-check   - cold start / RSS / disk vs thresholds"
	@echo "make cve-check    - NVD check of the CVE watchlist"
	@echo "make bench        - full benchmark tables"
