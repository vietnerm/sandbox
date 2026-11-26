# Root Makefile — entry points required by PROMPT.MD / CI.
# The binary build itself lives in sandbox/Makefile.

.PHONY: all build minictd console rootfs rootfs-alpine rootfs-debian test test-escape test-escape-alpine test-seccomp test-netns test-alpine test-debian test-web test-stack test-minictd test-pasta unicode-check test-unicode test-cve-gate test-cve-kernel check-docs test-check-docs perf-check cve-check bench clean help

all: build

build:
	$(MAKE) -C sandbox

# GĐ2 control plane: minictd daemon (Go, stdlib only) + minict-exec (setns)
minictd:
	$(MAKE) -C minictd all

# GĐ3 web console: Next.js STATIC export -> console/out, served by
# `minictd --ui-dir console/out` (no Node process at runtime). Build-time
# only needs Node >= 20; deps pinned in console/package-lock.json and
# installed without lifecycle scripts.
console:
	cd console && npm ci --ignore-scripts --no-audit --no-fund && npx tsc --noEmit && NEXT_TELEMETRY_DISABLED=1 npx next build && test -f out/index.html

rootfs:
	bash sandbox/mkrootfs.sh rootfs

rootfs-alpine:
	bash sandbox/mkrootfs-alpine.sh rootfs-alpine

rootfs-debian:
	bash tools/fetch-debian-rootfs.sh rootfs-debian

# isolation suite (31 cases) + seccomp blocklist suite (21 cases)
# + netns suite (7 cases) + alpine-rootfs suite (9 cases)
# + debian-rootfs suite (8 cases: DB1–DB6/DB8 always run, DB7 env-gated SKIP
#   without /dev/net/tun — D46; rootfs-debian fetched pinned+sha256, gitignored)
# + escape-alpine suite (31 cases) + cve-check tool suite (4 cases)
# + pasta connectivity suite (11 cases: PT1/PT2/NET11C/PT3C/PT4C always run,
#   NET10/NET11/NET12/PT3/PT4/AB1 env-gated SKIP without /dev/net/tun — D37/D42/D43)
test: build rootfs
	bash tests/run-suites.sh

test-escape: test

# pasta connectivity suite standalone (issue #57, D37; apk online #79, D43)
test-pasta: build rootfs
	bash tests/test_pasta.sh

# full web stack suite (nginx + fcgiwrap + CGI + SQLite in ONE instance,
# issue #85, D45; env-aware SKIP without the stack packages)
test-stack: build rootfs-alpine
	bash tests/test_stack.sh

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

test-debian: build rootfs-debian
	bash tests/test_debian.sh

test-web: build rootfs
	bash tests/test_web.sh

test-cg: build minictd rootfs
	bash tests/test_cg.sh

# minictd: go unit tests (issue #50 + #91 + #168 + #176 + #179) + e2e (63 cases MD0-MD61+XSS): real daemon -> patched minict -> minict-exec
test-minictd: build rootfs minictd
	cd minictd && $(or $(GO),go) test -count=1 ./...
	bash tests/test_minictd.sh

# perf-guard gate: cold start <= 100 ms, RSS <= 10240 kB, disk <= 20 MB
# (thresholds + measured baseline live in docs/perf-baseline.md)
perf-check: build rootfs
	bash tools/perf-check.sh

examples: build rootfs
	@for ex in examples/*/run.sh; do \
		echo "=== $$ex ==="; \
		bash "$$ex" || { echo "FAIL: $$ex"; exit 1; }; \
	done
	@echo "=== examples: all PASS ==="

cve-check:
	python3 tools/cve-check.py

# kernel CVE range evaluator + discovery (owner 2026-11-23), offline fixtures
test-cve-kernel:
	bash tests/test_cve_kernel.sh

# README drift gate (issues #26, #27): Known Limitations links, relative
# links, stale phrases, Platform Support vs manifest §5
check-docs:
	bash tools/check-docs.sh

test-check-docs:
	bash tests/test_check_docs.sh

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
	@echo "make test         - isolation (31) + seccomp (21) + netns (7) + alpine (8) + escape-alpine (31) + cve-check tool (4) + pasta (11; NET10-12/PT3/PT4/AB1 SKIP without tun) + cg (6; CG0-CG5 SKIP without cgroups v2), all suites always run"
	@echo "make test-pasta  - pasta connectivity suite only (issues #57/#79)"
	@echo "make test-stack   - web stack suite (nginx+fcgiwrap+CGI+SQLite, 7 cases; issue #85)"
	@echo "make test-escape  - same suites (alias, CI requirement)"
	@echo "make test-escape-alpine - isolation suite re-run on alpine rootfs (29 cases)"
	@echo "make test-web     - web service suite (6 cases)"
	@echo "make minictd      - build minictd daemon + minict-exec (needs Go >= 1.23)"
	@echo "make console      - build the GĐ3 web console (static export -> console/out; serve with minictd --ui-dir)"
	@echo "make test-minictd - minictd e2e suite (63 cases MD0-MD61+XSS: 44 legacy + 3 overlay D66 + 3 pasta D67 + 3 obs D68 + 2 mục8 D69 + 6 ui-sec D71 + 1 api-del D72 + XSS, env-gated)"
	@echo "make test-seccomp - seccomp blocklist suite (20 cases)"
	@echo "make test-netns   - netns isolation suite (7 cases)"
	@echo "make test-alpine  - alpine rootfs suite (8 cases)"
	@echo "make perf-check   - cold start / RSS / disk vs thresholds"
	@echo "make cve-check    - NVD check of the CVE watchlist"
	@echo "make test-cve-kernel - cve-kernel.py self-test (16 checks, offline)"
	@echo "make check-docs   - README drift gate (links, stale phrases, Platform Support vs manifest)"
	@echo "make test-check-docs - check-docs self-test (6 cases, negative controls)"
	@echo "make unicode-check - scan for hidden/bidi Unicode (Trojan Source)"
	@echo "make test-unicode  - unicode-guard self-test (13 cases)"
	@echo "make bench        - full benchmark tables"
