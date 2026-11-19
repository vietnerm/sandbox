#!/usr/bin/env bash
# test_cve_check.sh — tool-level suite for tools/cve-check.py (issue #44, D35).
#
# The daily CVE check must NEVER crash on malformed/transient NVD data:
# #44 reproduced live 2026-11-18 (Phiên 4) — an NVD row in a transient state
# made fetch_row return an error row, and the md renderer evaluated
# row['cvss'] BEFORE checking "error" -> KeyError 'cvss' -> the whole check
# died (CI cve-daily and Bước 6 both use it).
#
# All cases run OFFLINE: they import the module and exercise the pure
# functions (parse_nvd, render_cvss) with fixture dicts. No network.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
TOOL="$REPO/tools/cve-check.py"

PASS=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|fail> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass) [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        fail) [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL ;;
    esac
    printf '%-5s %-56s %s\n' "$id" "$desc" "$verdict"
    if [[ "$verdict" == FAIL* ]]; then
        echo "      output: $(echo "$out" | head -2 | tr '\n' ' ')"
        FAIL=$((FAIL+1))
    else
        PASS=$((PASS+1))
    fi
    return 0
}

# CV1: render an error row (the exact #44 repro: NVD transient -> error row
# without 'cvss' key) through render_cvss — the OLD code KeyError'ed here.
cv1() { python3 - "$TOOL" <<'PY'
import importlib.util, sys
spec = importlib.util.spec_from_file_location('cc', sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
row = {"cve": "CVE-2026-99999", "component": "kernel", "error": "timed out", "why": "w"}
out = m.render_cvss(row)
print(out)
assert out.startswith("CVSS: ERR (timed out)"), out
PY
}
cv2() { python3 - "$TOOL" <<'PY'
import importlib.util, sys
spec = importlib.util.spec_from_file_location('cc', sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
# malformed metrics: entry without cvssData (transient NVD states)
r = m.parse_nvd({"metrics": {"cvssMetricV31": [{}]}})
assert r["cvss"] == "" and r["severity"] == "" and r["desc"] == "", r
r2 = m.parse_nvd({})
assert r2["cvss"] == "", r2
r3 = m.parse_nvd({"descriptions": [{}], "metrics": {}})
assert r3["desc"] == "", r3
print("parse_nvd defensive OK")
PY
}
cv3() { python3 - "$TOOL" <<'PY'
import importlib.util, sys
spec = importlib.util.spec_from_file_location('cc', sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
out = m.render_cvss({"cve": "C", "component": "k", "cvss": "9.8", "severity": "CRITICAL", "why": "w"})
assert out == "CVSS: 9.8 CRITICAL", out
# V30 fallback when V31 missing
r = m.parse_nvd({"metrics": {"cvssMetricV30": [{"cvssData": {"baseScore": 7.5, "baseSeverity": "HIGH"}}]}})
assert r["cvss"] == "7.5" and r["severity"] == "HIGH", r
print("normal render + V30 fallback OK")
PY
}
# negative control of the TOOL change itself: simulate the OLD renderer
# (error-check AFTER the f-string) — it must KeyError on the same fixture,
# proving CV1 would have caught the regression
cv4() { python3 - <<'PY'
row = {"cve": "CVE-X", "component": "k", "error": "timed out", "why": "w"}
try:
    cvss = f"CVSS: {row['cvss']} {row.get('severity', '')}".strip()
    if "error" in row:
        cvss = f"CVSS: ERR ({row['error']})"
except KeyError as e:
    print("old renderer KeyError reproduced:", e)
    raise SystemExit(0)
print("old renderer did NOT crash — control broken")
raise SystemExit(1)
PY
}

echo "=== cve-check tool tests ($(date '+%F %T')) ==="
run_case CV1 "error row renders ERR, no KeyError (#44 repro)" pass cv1
run_case CV2 "parse_nvd defensive: malformed/empty NVD dicts" pass cv2
run_case CV3 "normal row + cvssMetricV30 fallback" pass cv3
run_case CV4 "control: OLD renderer KeyErrors on error row" pass cv4

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
