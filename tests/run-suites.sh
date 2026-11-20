#!/usr/bin/env bash
# run-suites.sh — run every test suite even when an earlier one fails, and
# aggregate the exit code at the end.
#
# Why this exists (issue #14 / CI run #20): with three plain recipe lines in
# the Makefile, make stopped at the first red suite, so a red isolation run
# meant seccomp + netns NEVER produced evidence on CI — while the job still
# reported green because of the missing pipefail (fixed together in the same
# issue). Now `make test` always runs all suites and fails if any of them
# failed.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
SUITES=(test_isolation.sh test_seccomp.sh test_netns.sh test_alpine.sh test_escape_alpine.sh test_cve_check.sh test_pasta.sh test_stack.sh)

rc_total=0
failed=()
for s in "${SUITES[@]}"; do
    echo "### suite: $s"
    if bash "$HERE/$s"; then
        echo "### suite: $s — OK"
    else
        rc=$?
        echo "### suite: $s — FAIL (rc=$rc)"
        failed+=("$s")
        rc_total=1
    fi
done

echo "=== run-suites summary: ${#SUITES[@]} suites, failed: ${failed[*]:-none} ==="
exit $rc_total
