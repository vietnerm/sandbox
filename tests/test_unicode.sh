#!/usr/bin/env bash
# test_unicode.sh \u2014 tools/unicode-guard.py (hidden / bidi Unicode scanner).
#
# Fixtures are generated at runtime from \u escapes, so this file (and the
# repo) stays free of the very characters it tests for.
#
# Cases:
#   UG1  RLO bidi override (Trojan Source) in a .c file -> error, exit 1
#   UG2  zero-width space inside an identifier -> error
#   UG3  Unicode tag characters (ASCII smuggling) -> error
#   UG4  Vietnamese text, precomposed AND decomposed (NFD) -> clean, exit 0
#   UG5  emoji with VS16 and ZWJ sequence -> clean
#   UG6  --fix strips the hidden chars, keeps Vietnamese bytes, rescan clean
#   UG7  NBSP is a warning only: exit 0, but --strict exits 1
#   UG8  Cyrillic look-alike inside a Latin word -> homoglyph warning
#   UG9  leading BOM is fine, BOM mid-file is an error
#   UG10 variation-selector-supplement payload -> error
#   UG11 --changed BASE scans only files changed since BASE
#   UG12 .unicode-guard-ignore globs and symlinks are skipped
#   UG13 the repository itself is clean
#
# Exit code: 0 if all expected results hold, 1 otherwise.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
GUARD="$REPO/tools/unicode-guard.py"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

PASS=0; FAIL=0
check() { # <id> <desc> <cond-result 0|1>
    if [[ "$3" == 0 ]]; then echo "PASS $1 $2"; PASS=$((PASS+1));
    else echo "FAIL $1 $2"; FAIL=$((FAIL+1)); fi
}
# mk <file> <python-string-literal>: write the decoded text as UTF-8
mk() { python3 -c 'import sys; open(sys.argv[1],"w",encoding="utf-8",newline="").write(eval(sys.argv[2]))' "$1" "$2"; }
# guard <args...> in $T; stdout+stderr in $T/out, returns exit code
guard() { (cd "$T/w" && python3 "$GUARD" "$@") >"$T/out" 2>&1; }

mkdir -p "$T/w"

mk "$T/w/bidi.c" '"int ok = 1; /* \u202e } \u2066 if (admin) \u2069 \u2066 */\n"'
guard bidi.c; rc=$?
check UG1 "RLO/isolate bidi -> error" $(( rc == 1 && $(grep -c 'bidi control' "$T/out") >= 3 ? 0 : 1 ))

mk "$T/w/zw.py" '"is\u200badmin = False\n"'
guard zw.py; rc=$?
check UG2 "zero-width space -> error" $(( rc == 1 ? 0 : 1 ))

mk "$T/w/tag.md" '"hello" + "".join(chr(0xE0000 + ord(c)) for c in "rm -rf /") + "\n"'
guard tag.md; rc=$?
check UG3 "tag chars -> error" $(( rc == 1 && $(grep -c 'tag character' "$T/out") == 8 ? 0 : 1 ))

mk "$T/w/vi.md" '"Ti\u1ebfng Vi\u1ec7t c\u00f3 d\u1ea5u: \u0111\u01b0\u1eddng, ph\u1edf\n" + "Ti\u00ea\u0301ng Vi\u00ea\u0323t (NFD) a\u0300 a\u0309 a\u0303\n"'
guard vi.md; rc=$?
check UG4 "Vietnamese (NFC + NFD) -> clean" $(( rc == 0 && $(grep -c 'error(s), 0 warning' "$T/out") == 1 ? 0 : 1 ))

mk "$T/w/emoji.md" '"\u26a0\ufe0f c\u1ea3nh b\u00e1o, dev \U0001F468\u200d\U0001F4BB, keycap 1\ufe0f\u20e3\n"'
guard emoji.md; rc=$?
check UG5 "emoji VS16 + ZWJ -> clean" $(( rc == 0 && $(grep -c ' 0 error' "$T/out") == 1 ? 0 : 1 ))

mk "$T/w/fix.md" '"Xin ch\u00e0o\u200b th\u1ebf gi\u1edbi \u202eabc\u202c\u00a0end\n"'
mk "$T/w/fix.expected" '"Xin ch\u00e0o th\u1ebf gi\u1edbi abc\u00a0end\n"'
guard --fix fix.md; rc1=$?
guard fix.md; rc2=$?
cmp -s "$T/w/fix.md" "$T/w/fix.expected"; same=$?
check UG6 "--fix strips errors, keeps Vietnamese + NBSP (warn)" $(( rc1 == 0 && rc2 == 0 && same == 0 ? 0 : 1 ))

mk "$T/w/nbsp.sh" '"echo\u00a0hi\n"'
guard nbsp.sh; rc1=$?
guard --strict nbsp.sh; rc2=$?
check UG7 "NBSP warn: exit 0, --strict exit 1" $(( rc1 == 0 && rc2 == 1 ? 0 : 1 ))

mk "$T/w/homo.go" '"if user == \"p\u0430ypal\" {}\n"'
guard homo.go; rc=$?
check UG8 "Cyrillic homoglyph -> warning" $(( rc == 0 && $(grep -c 'homoglyph' "$T/out") == 1 ? 0 : 1 ))

mk "$T/w/bom1.txt" '"\ufeffstart\n"'
mk "$T/w/bom2.txt" '"start\ufeffmid\n"'
guard bom1.txt; rc1=$?
guard bom2.txt; rc2=$?
check UG9 "leading BOM ok, mid BOM error" $(( rc1 == 0 && rc2 == 1 ? 0 : 1 ))

mk "$T/w/vs.js" '"const x = \"a" + "".join(chr(0xE0100 + b) for b in b"evil") + "\";\n"'
guard vs.js; rc=$?
check UG10 "variation selector supplement -> error" $(( rc == 1 && $(grep -c 'variation selector supplement' "$T/out") == 4 ? 0 : 1 ))

mkdir -p "$T/g" && (
    cd "$T/g" && git init -q && git config user.email t@t && git config user.name t
    mk old.c '"int a;\u200b\n"' && git add . && git commit -qm base
    git tag base
    mk new.c '"int b;\n"' && git add . && git commit -qm next
) >/dev/null 2>&1
(cd "$T/g" && python3 "$GUARD" --changed base) >"$T/out" 2>&1; rc1=$?
(cd "$T/g" && python3 "$GUARD") >"$T/out2" 2>&1; rc2=$?
check UG11 "--changed ignores pre-existing dirt, full scan sees it" $(( rc1 == 0 && rc2 == 1 ? 0 : 1 ))

mkdir -p "$T/i/vendor" && (
    cd "$T/i" && git init -q && git config user.email t@t && git config user.name t
    mk vendor/x.c '"\u202e\n"'
    mk real.c '"int c;\n"'
    ln -s /etc/hostname link
    printf '# fixtures\nvendor/*\n' > .unicode-guard-ignore
    git add . && git commit -qm init
) >/dev/null 2>&1
(cd "$T/i" && python3 "$GUARD") >"$T/out" 2>&1; rc=$?
check UG12 "ignore globs + symlinks skipped" $(( rc == 0 && $(grep -c '2 file(s) scanned' "$T/out") == 1 ? 0 : 1 ))

(cd "$REPO" && python3 "$GUARD") >"$T/out" 2>&1; rc=$?
[[ $rc == 0 ]] || cat "$T/out"
check UG13 "repository is clean" $rc

echo "unicode-guard: $PASS passed, $FAIL failed"
[[ $FAIL == 0 ]]
