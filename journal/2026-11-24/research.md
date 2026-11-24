# Research — 2026-11-24 (SDDA Phiên 30, issue #28 / examples)

Chủ đề: example smoke tests; busybox httpd loopback; /proc env-gating.

## Nguồn 1 — busybox httpd + wget (busybox docs)
- Link: https://busybox.net/downloads/BusyBox.html
- `httpd -f -p 127.0.0.1:PORT -h /www` + `wget -q -O- URL` — loopback self-contained test.

## Nguồn 2 — /proc/self/status fields (proc(5))
- Link: https://man7.org/linux/man-pages/man5/proc.5.html
- NoNewPrivs, Seccomp fields in /proc/self/status. Env-gated: /proc mount in pidns needs
  kernel ≥5.10 + host procfs fully-visible (D39). Dev 5.10 fails (mount_too_revealing).

## Nguồn 3 — Docker smoke tests + examples (docs.docker.com)
- Link: https://docs.docker.com/samples/
- Docker samples double as smoke tests. minict examples = same pattern (run.sh + README).

## Nguồn 4 — Makefile + CI examples job (GNU make + GitHub Actions)
- Link: https://www.gnu.org/software/make/manual/
- `make examples` target runs all run.sh. ci.yml `examples` job = G7 (needs-human).

**Trade-off:** examples self-contained (loopback only, no host bridge) — simpler but doesn't
test the webproxy bridge (D16). ci.yml job deferred (G7).

**Áp dụng:** 3 examples + make target. Dev 3/3 PASS. CI green.
