#!/usr/bin/env bash
# check-docs.sh — fail when README drifts from the repo (issues #26, #27).
#
# Checks (all run, then exit 1 if any failed):
#   D1  every bullet under README "## Known Limitations" links to journal/ or
#       docs/ (README points at the evidence, it does not restate it)
#   D2  every relative markdown link in README.md and docs/*.md resolves to an
#       existing file or directory
#   D3  README contains none of the stale phrases in tools/docs-stale-phrases.txt
#       (when a limitation is fixed, the fixing PR adds the old sentence there)
#   D4  README "## Platform Support" agrees with idea/manifest.md §5 on the
#       target level of the kernel and cgroups rows (manifest is the source
#       of truth: the code has no minimum-kernel constant)
#
# Usage: tools/check-docs.sh [repo-root]   (default: the repo holding this file)
set -uo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
README="$ROOT/README.md"
MANIFEST="$ROOT/idea/manifest.md"
STALE="$ROOT/tools/docs-stale-phrases.txt"
FAIL=0

bad() { echo "FAIL $1: $2"; FAIL=1; }

[[ -f "$README" ]] || { echo "no README.md under $ROOT"; exit 1; }

# section <file> <heading>: body lines of "## <heading>" up to the next "## "
section() {
    awk -v h="## $2" '
        $0 == h          { on = 1; next }
        on && /^## /     { exit }
        on               { print }' "$1"
}

# ---- D1 -------------------------------------------------------------------
kl="$(section "$README" "Known Limitations")"
if [[ -z "$kl" ]]; then
    bad D1 "README has no '## Known Limitations' section"
else
    n=0
    while IFS= read -r line; do
        [[ "$line" == "- "* ]] || continue
        n=$((n + 1))
        if ! grep -Eq '\]\((\./)?(journal|docs)/[^)]+\)' <<<"$line"; then
            bad D1 "Known Limitations bullet without a journal/ or docs/ link: ${line:0:80}"
        fi
    done <<<"$kl"
    [[ $n -gt 0 ]] || bad D1 "Known Limitations has no '- ' bullets"
fi

# ---- D2 -------------------------------------------------------------------
shopt -s nullglob
for f in "$README" "$ROOT"/docs/*.md; do
    dir="$(dirname "$f")"
    # inline links only: [text](target) — skip URLs, anchors, mailto
    while IFS= read -r target; do
        t="${target%%#*}"
        t="${t%% *}"            # drop an optional "title"
        [[ -z "$t" ]] && continue
        case "$t" in http://*|https://*|mailto:*) continue ;; esac
        if [[ "$t" == /* ]]; then p="$ROOT$t"; else p="$dir/$t"; fi
        [[ -e "$p" ]] || bad D2 "${f#"$ROOT"/}: broken link -> $target"
    done < <(grep -oE '\]\([^)]+\)' "$f" | sed -E 's/^\]\((.*)\)$/\1/')
done

# ---- D3 -------------------------------------------------------------------
if [[ -f "$STALE" ]]; then
    while IFS= read -r phrase; do
        [[ -z "$phrase" || "$phrase" == \#* ]] && continue
        if grep -Fqi -- "$phrase" "$README"; then
            bad D3 "README still says a fixed limitation: '$phrase' (see tools/docs-stale-phrases.txt)"
        fi
    done < "$STALE"
else
    bad D3 "missing $STALE"
fi

# ---- D4 -------------------------------------------------------------------
# row_cell <markdown-table-text> <row-label-regex> <column-index>
row_cell() {
    awk -F'|' -v re="$2" -v c="$3" '
        NF > 3 { lab = $2; gsub(/^[ \t*]+|[ \t*]+$/, "", lab)
                 if (lab ~ re) { v = $(c + 1); gsub(/^[ \t]+|[ \t]+$/, "", v); print v; exit } }' <<<"$1"
}
ps="$(section "$README" "Platform Support")"
ms="$(sed -n '/^## 5\./,/^## 6\./p' "$MANIFEST" 2>/dev/null)"
if [[ -z "$ps" ]]; then
    bad D4 "README has no '## Platform Support' section"
elif [[ -z "$ms" ]]; then
    bad D4 "cannot find manifest §5 in idea/manifest.md"
else
    # kernel: manifest "≥ 6.1" must appear verbatim in the README row
    mk="$(row_cell "$ms" '^Linux kernel$' 2)"
    rk="$(row_cell "$ps" '^Kernel$' 2)"
    mkv="$(grep -oE '[0-9]+\.[0-9]+' <<<"$mk" | head -1)"
    if [[ -z "$mkv" ]]; then
        bad D4 "manifest §5 has no kernel version"
    elif ! grep -Fq "$mkv" <<<"$rk"; then
        bad D4 "Platform Support kernel '$rk' != manifest §5 '$mk'"
    fi
    # cgroups: the manifest's version token (v1/v2) must match the README row
    mc="$(row_cell "$ms" '^cgroups$' 2 | grep -oE 'v[12]' | head -1)"
    rc="$(row_cell "$ps" '^cgroups$' 2 | grep -oE 'v[12]' | head -1)"
    if [[ -z "$mc" || "$mc" != "$rc" ]]; then
        bad D4 "Platform Support cgroups '${rc:-?}' != manifest §5 '${mc:-?}'"
    fi
fi

if [[ $FAIL -eq 0 ]]; then echo "check-docs: PASS (D1–D4)"; fi
exit $FAIL
