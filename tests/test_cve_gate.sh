#!/usr/bin/env bash
# test_cve_gate.sh — tools/cve-gate.py decides when cve-daily opens a
# security issue (issue #56): only CVSS >= 8 CVEs NOT in the watchlist.
#
# Cases:
#   CG1  all critical CVEs already in watchlist -> nothing new (no issue)
#   CG2  one critical CVE outside the watchlist -> printed (issue opens)
#   CG3  CVE outside watchlist but CVSS < 8 -> not printed
#   CG4  NVD error row (issue #44) -> not printed, run still succeeds
#   CG5  malformed report -> exit 2 (fail loud, never silently "nothing new")
#   CG6  real repo watchlist + yesterday's real report shape -> nothing new
#   CG7  CVE id not matching CVE-YYYY-NNNN (e.g. quote injection) -> exit 2
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
GATE="$REPO/tools/cve-gate.py"
WORK="$(mktemp -d)"
trap 'rm -f "$WORK"/*; rmdir "$WORK"' EXIT

PASS=0; FAIL=0
check() { # <id> <desc> <expected-output> <expected-rc> <report-json> [watchlist-file]
    local id="$1" desc="$2" want="$3" want_rc="$4" rep="$5" wl="${6:-$WORK/wl.md}"
    printf '%s' "$rep" > "$WORK/rep.json"
    local out rc
    out="$(python3 -I "$GATE" "$WORK/rep.json" "$wl" 2>/dev/null)"; rc=$?
    if [[ "$out" == "$want" && $rc -eq $want_rc ]]; then
        printf '%-5s %-58s PASS\n' "$id" "$desc"; PASS=$((PASS+1))
    else
        printf '%-5s %-58s FAIL\n' "$id" "$desc"
        echo "      got out='$out' rc=$rc, want out='$want' rc=$want_rc"
        FAIL=$((FAIL+1))
    fi
}

cat > "$WORK/wl.md" <<'EOF'
| CVE-2024-1086 | kernel | 7.8 | nf_tables | mitigated |
| CVE-2026-31431 | kernel | 9.8 | Copy Fail | AF_ALG blocked |
EOF

echo "=== cve-gate tests ($(date '+%F %T')) ==="

check CG1 "all critical CVEs known -> nothing new" "" 0 \
  '[{"cve":"CVE-2026-31431","cvss":"9.8"},{"cve":"CVE-2024-1086","cvss":"7.8"}]'
check CG2 "critical CVE outside watchlist -> printed" "CVE-2026-99999" 0 \
  '[{"cve":"CVE-2026-31431","cvss":"9.8"},{"cve":"CVE-2026-99999","cvss":"8.1"}]'
check CG3 "unknown CVE with CVSS < 8 -> not printed" "" 0 \
  '[{"cve":"CVE-2026-88888","cvss":"7.9"}]'
check CG4 "NVD error row -> not printed, rc 0" "" 0 \
  '[{"cve":"CVE-2026-77777","error":"timed out"}]'
check CG5 "malformed report -> rc 2" "" 2 '{"not":"a list"}'
check CG7 "malformed CVE id -> rc 2, nothing printed" "" 2 \
  '[{"cve":"CVE-2026-1\\\" in:title x","cvss":"9.9"}]'

# CG6: the repo's own watchlist must cover every CVE cve-check.py tracks
# today, otherwise cve-daily would open an issue on the next run.
cg6() {
    local ids rep
    ids=$(grep -oE 'CVE-[0-9]{4}-[0-9]+' "$REPO/tools/cve-check.py" | sort -u)
    rep="["
    for c in $ids; do rep+="{\"cve\":\"$c\",\"cvss\":\"9.9\"},"; done
    rep="${rep%,}]"
    check CG6 "every CVE in cve-check.py is in docs/cve-watchlist.md" "" 0 \
      "$rep" "$REPO/docs/cve-watchlist.md"
}
cg6

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
