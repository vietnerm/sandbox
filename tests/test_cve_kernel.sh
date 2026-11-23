#!/usr/bin/env bash
# test_cve_kernel.sh — tools/cve-kernel.py range logic + discovery, offline
# (NVD answers from fixtures; no network). Cases:
#   CK1  multi-range CVE: runner 7.0 not affected, 6.1 -> fixed in 6.1.76,
#        6.6 -> fixed in 6.6.15, 6.12 not affected
#   CK2  open-ended range from 6.13: runner 7.0 affected, 6.12 not affected
#   CK3  NVD without CPE data -> "unknown", never "not affected"
#   CK4  versionEndIncluding 6.1.50 -> fixed in 6.1.51; rc-only CPE ignored
#   CK5  vendor kernel string 5.10.134-013.15.kangaroo.al8 parsed as 5.10.134
#   CK6  discovery: recent kernel CVE listed as NEW; one published > 120 days
#        ago is NOT re-reported
#   CK7  gate json: only issue-worthy rows (CK2 yes, CK1 no) -> cve-gate
#        prints exactly the new critical one
#   CK8  line 7.0 with no fix -> "affected, no fix in line"
#   CK9  bad kernel spec -> exit 2
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
TOOL="$REPO/tools/cve-kernel.py"
W="$(mktemp -d)"; trap 'rm -rf "$W"' EXIT
mkdir -p "$W/cache"
RECENT="$(date -u -d '-3 days' +%Y-%m-%dT00:00:00.000)"
OLD="2020-01-01T00:00:00.000"

cve() { # <id> <score> <published> <configurations-json>
    printf '{"id":"%s","published":"%s","descriptions":[{"lang":"en","value":"test %s"}],"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":%s,"baseSeverity":"X"}}]},"configurations":%s}' "$1" "$3" "$1" "$2" "$4"
}
m() { # <criteria-version> [k=v ...] -> one cpeMatch
    local v="$1"; shift; local extra=""
    for kv in "$@"; do extra+=",\"${kv%%=*}\":\"${kv#*=}\""; done
    printf '{"vulnerable":true,"criteria":"cpe:2.3:o:linux:linux_kernel:%s:*:*:*:*:*:*:*"%s}' "$v" "$extra"
}
conf() { local IFS=,; printf '[{"nodes":[{"cpeMatch":[%s]}]}]' "$*"; }

A=$(cve CVE-2099-0001 7.8 "$OLD" "$(conf "$(m '*' versionStartIncluding=2.6.12 versionEndExcluding=5.15.150)" "$(m '*' versionStartIncluding=5.16 versionEndExcluding=6.1.76)" "$(m '*' versionStartIncluding=6.2 versionEndExcluding=6.6.15)" "$(m '*' versionStartIncluding=6.7 versionEndExcluding=6.7.3)")")
B=$(cve CVE-2099-0002 9.1 "$RECENT" "$(conf "$(m '*' versionStartIncluding=6.13)")")
C=$(cve CVE-2099-0003 8.0 "$OLD" '[]')
D=$(cve CVE-2099-0004 7.0 "$OLD" "$(conf "$(m '*' versionStartIncluding=6.1 versionEndIncluding=6.1.50)" '{"vulnerable":true,"criteria":"cpe:2.3:o:linux:linux_kernel:6.6:rc1:*:*:*:*:*:*"}')")
OLDNEW=$(cve CVE-2099-0005 9.8 "$OLD" "$(conf "$(m '*' versionStartIncluding=6.13)")")
printf '{"vulnerabilities":[{"cve":%s}]}' "$A" > "$W/cache/cve-CVE-2099-0001.json"
printf '{"vulnerabilities":[{"cve":%s}]}' "$C" > "$W/cache/cve-CVE-2099-0003.json"
printf '{"vulnerabilities":[{"cve":%s}]}' "$D" > "$W/cache/cve-CVE-2099-0004.json"
printf '{"vulnerabilities":[{"cve":%s},{"cve":%s}]}' "$B" "$OLDNEW" > "$W/cache/recent-CRITICAL.json"
cat > "$W/wl.md" <<'MD'
| CVE | Component | CVSS |
|---|---|---|
| CVE-2099-0001 (multi) | kernel | 7.8 |
| CVE-2099-0003 (no cpe) | kernel | 8.0 |
| CVE-2099-0004 (incl end) | kernel | 7.0 |
| CVE-2099-0009 (engine) | runc | 8.6 |
MD

K="7.0.0-1017-azure,6.1,6.6,6.12,5.10.134-013.15.kangaroo.al8,7.0"
python3 -I "$TOOL" --kernels "$K" --watchlist "$W/wl.md" --discover-days 2 \
    --nvd-cache "$W/cache" --json "$W/out.json" --gate-json "$W/gate.json" >/dev/null
RC=$?

PASS=0; FAIL=0
ok() { printf '%-4s %-62s %s\n' "$1" "$2" "$3"; [[ $3 == PASS ]] && PASS=$((PASS+1)) || FAIL=$((FAIL+1)); }
k() { python3 -I -c "import json,sys; r={x['cve']:x for x in json.load(open(sys.argv[1]))}; print(r[sys.argv[2]]['kernels'][sys.argv[3]] if sys.argv[2] in r else 'ABSENT')" "$W/out.json" "$1" "$2"; }
t() { [[ "$3" == "$4" ]] && ok "$1" "$2" PASS || { ok "$1" "$2" FAIL; echo "     got '$3' want '$4'"; }; }

[[ $RC -eq 0 ]] || echo "tool rc=$RC"
t CK1 "multi-range: runner 7.0 not affected"          "$(k CVE-2099-0001 7.0.0-1017-azure)" "not affected"
t CK1 "multi-range: 6.1 line fixed in 6.1.76"         "$(k CVE-2099-0001 6.1)"  "fixed in 6.1.76"
t CK1 "multi-range: 6.6 line fixed in 6.6.15"         "$(k CVE-2099-0001 6.6)"  "fixed in 6.6.15"
t CK1 "multi-range: 6.12 line not affected"           "$(k CVE-2099-0001 6.12)" "not affected"
t CK2 "open range from 6.13: runner 7.0 affected"     "$(k CVE-2099-0002 7.0.0-1017-azure)" "affected (upstream range)"
t CK2 "open range from 6.13: 6.12 not affected"       "$(k CVE-2099-0002 6.12)" "not affected"
t CK3 "no CPE data -> unknown"                        "$(k CVE-2099-0003 7.0.0-1017-azure)" "unknown (NVD has no CPE yet)"
t CK4 "versionEndIncluding 6.1.50 -> fixed in 6.1.51" "$(k CVE-2099-0004 6.1)"  "fixed in 6.1.51"
t CK4 "rc-only CPE (6.6:rc1) ignored"                 "$(k CVE-2099-0004 6.6)"  "not affected"
t CK5 "vendor 5.10.134 string in 2.6.12..5.15.150"    "$(k CVE-2099-0001 5.10.134-013.15.kangaroo.al8)" "affected (upstream range)"
t CK6 "recent kernel CVE discovered"                  "$(python3 -I -c "import json,sys;r=[x for x in json.load(open(sys.argv[1])) if x['cve']=='CVE-2099-0002'];print(r[0]['origin'],r[0]['in_watchlist'])" "$W/out.json")" "discovered False"
t CK6 "CVE published > 120 days ago not re-reported"   "$(k CVE-2099-0005 7.0)" "ABSENT"
t CK7 "gate json keeps only issue-worthy rows"        "$(python3 -I -c "import json,sys;print(' '.join(sorted(x['cve'] for x in json.load(open(sys.argv[1])))))" "$W/gate.json")" "CVE-2099-0001 CVE-2099-0002"
t CK7 "cve-gate on it prints only the NEW critical"   "$(python3 -I "$REPO/tools/cve-gate.py" "$W/gate.json" "$W/wl.md")" "CVE-2099-0002"
t CK8 "7.0 line with open range -> no fix in line"    "$(k CVE-2099-0002 7.0)" "affected, no fix in line"
python3 -I "$TOOL" --kernels "foo" --nvd-cache "$W/cache" >/dev/null 2>&1; rc=$?
t CK9 "bad kernel spec -> exit 2"                     "$rc" "2"
echo "--- $PASS PASS, $FAIL FAIL"
[[ $FAIL -eq 0 ]]
