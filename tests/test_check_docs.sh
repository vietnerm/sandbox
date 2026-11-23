#!/usr/bin/env bash
# test_check_docs.sh — tools/check-docs.sh self-test with negative controls
# (issue #26: "tự tạo 1 câu stale → CI đỏ"). Each case mutates a throwaway
# copy of README/manifest/docs and expects check-docs to PASS or FAIL.
#   DC0  real repo docs                         -> PASS
#   DC1  Known Limitations bullet without link  -> FAIL (D1)
#   DC2  broken relative link in README         -> FAIL (D2)
#   DC3  stale phrase back in README            -> FAIL (D3)
#   DC4  manifest kernel target changed         -> FAIL (D4)
#   DC5  README cgroups row says v1             -> FAIL (D4)
set -uo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
CHECK="$REPO/tools/check-docs.sh"
PASS=0; FAIL=0

fresh() { # -> path of a throwaway repo copy holding only what check-docs reads
    local t; t="$(mktemp -d)"
    mkdir -p "$t/tools" "$t/idea"
    cp "$REPO/README.md" "$t/"
    cp "$REPO/idea/manifest.md" "$t/idea/"
    cp "$REPO/tools/docs-stale-phrases.txt" "$t/tools/"
    cp -r "$REPO/docs" "$t/docs"
    # links point into journal/ and other repo dirs: mirror them as symlinks
    local d
    for d in "$REPO"/*; do
        [[ -e "$t/$(basename "$d")" ]] || ln -s "$d" "$t/$(basename "$d")"
    done
    echo "$t"
}

case_() { # <id> <desc> <expect pass|fail> <mutation-fn>
    local id="$1" desc="$2" expect="$3" fn="$4" t out rc verdict
    t="$(fresh)"
    "$fn" "$t"
    out="$(bash "$CHECK" "$t" 2>&1)"; rc=$?
    [[ -n "${DC_VERBOSE:-}" ]] && sed 's/^/     | /' <<<"$out"
    rm -rf "$t"
    if [[ "$expect" == pass ]]; then [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL
    else [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL; fi
    printf '%-4s %-48s %s\n' "$id" "$desc" "$verdict"
    if [[ $verdict == FAIL ]]; then echo "     output: $(head -3 <<<"$out" | tr '\n' ' ')"; FAIL=$((FAIL+1)); else PASS=$((PASS+1)); fi
}

m_none()   { :; }
m_nolink() { sed -i '/^## Known Limitations/,/^## /{0,/^- /s/^- .*/- Một giới hạn không có link bằng chứng./}' "$1/README.md"; }
m_broken() { printf '\nXem [file mất](journal/2099-01-01/none.md).\n' >> "$1/README.md"; }
m_stale()  { printf '\nKhông /proc trong sandbox (host seccomp chặn mount proc).\n' >> "$1/README.md"; }
m_kernel() { sed -i 's/^| Linux kernel | ≥ 6\.1 |/| Linux kernel | ≥ 6.6 |/' "$1/idea/manifest.md"; }
m_cgv1()   { sed -i 's/^| cgroups | v2 cho/| cgroups | v1 cho/' "$1/README.md"; }

echo "=== check-docs self-test ($(date '+%F %T')) ==="
case_ DC0 "real repo docs pass"                     pass m_none
case_ DC1 "limitation bullet without link fails"   fail m_nolink
case_ DC2 "broken relative link fails"             fail m_broken
case_ DC3 "stale phrase in README fails"           fail m_stale
case_ DC4 "manifest kernel target drift fails"     fail m_kernel
case_ DC5 "README cgroups v1 vs manifest v2 fails" fail m_cgv1
echo "--- $PASS PASS, $FAIL FAIL"
[[ $FAIL -eq 0 ]]
