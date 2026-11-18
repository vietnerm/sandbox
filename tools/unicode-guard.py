#!/usr/bin/env python3
"""unicode-guard - find (and optionally strip) hidden / bidi Unicode.

Defends against Trojan Source (CVE-2021-42574), invisible-character
smuggling (zero-width, Unicode "tag" block, variation-selector payloads) and
stray control characters. Stdlib only, no network, so CI never has to pip
install a third-party scanner.

Legitimate Vietnamese text is untouched: precomposed letters (U+1EAF, U+1ED9,
U+1EEF...) and combining tone marks (U+0300-U+0323) are ordinary visible text.

Severity
  error  bidi controls, zero-width/invisible chars, tag chars, variation
         selectors outside emoji, C0/C1 controls, U+2028/2029, BOM not at the
         start of a file. Fails the scan; removed by --fix.
  warn   unusual spaces (NBSP, en/em/ideographic space...), Latin words mixed
         with Cyrillic/Greek look-alike letters (homoglyphs), invalid UTF-8.
         Reported only; never fail the scan and never auto-fixed.

Usage
  unicode-guard.py                   scan every file tracked by git
  unicode-guard.py FILE...           scan only these files
  unicode-guard.py --changed BASE    scan files changed since git rev BASE
  unicode-guard.py --fix [...]       rewrite files, removing error findings
  --github                           also emit ::error/::warning annotations
  --strict                           warnings fail the scan too

Exit codes: 0 clean, 1 findings (errors, or warnings with --strict), 2 usage.

Paths listed in .unicode-guard-ignore (fnmatch globs, one per line, '#'
comments) are skipped. Keep that list empty unless a file truly must carry
such characters.
"""
import argparse
import fnmatch
import os
import subprocess
import sys
import unicodedata

BIDI = {
    0x061C: "ARABIC LETTER MARK",
    0x200E: "LEFT-TO-RIGHT MARK",
    0x200F: "RIGHT-TO-LEFT MARK",
    0x202A: "LEFT-TO-RIGHT EMBEDDING",
    0x202B: "RIGHT-TO-LEFT EMBEDDING",
    0x202C: "POP DIRECTIONAL FORMATTING",
    0x202D: "LEFT-TO-RIGHT OVERRIDE",
    0x202E: "RIGHT-TO-LEFT OVERRIDE",
    0x2066: "LEFT-TO-RIGHT ISOLATE",
    0x2067: "RIGHT-TO-LEFT ISOLATE",
    0x2068: "FIRST STRONG ISOLATE",
    0x2069: "POP DIRECTIONAL ISOLATE",
}

INVISIBLE = {
    0x00AD: "SOFT HYPHEN",
    0x034F: "COMBINING GRAPHEME JOINER",
    0x115F: "HANGUL CHOSEONG FILLER",
    0x1160: "HANGUL JUNGSEONG FILLER",
    0x17B4: "KHMER VOWEL INHERENT AQ",
    0x17B5: "KHMER VOWEL INHERENT AA",
    0x180E: "MONGOLIAN VOWEL SEPARATOR",
    0x200B: "ZERO WIDTH SPACE",
    0x200C: "ZERO WIDTH NON-JOINER",
    0x200D: "ZERO WIDTH JOINER",
    0x2028: "LINE SEPARATOR",
    0x2029: "PARAGRAPH SEPARATOR",
    0x2060: "WORD JOINER",
    0x2061: "FUNCTION APPLICATION",
    0x2062: "INVISIBLE TIMES",
    0x2063: "INVISIBLE SEPARATOR",
    0x2064: "INVISIBLE PLUS",
    0x206A: "INHIBIT SYMMETRIC SWAPPING",
    0x206B: "ACTIVATE SYMMETRIC SWAPPING",
    0x206C: "INHIBIT ARABIC FORM SHAPING",
    0x206D: "ACTIVATE ARABIC FORM SHAPING",
    0x206E: "NATIONAL DIGIT SHAPES",
    0x206F: "NOMINAL DIGIT SHAPES",
    0x3164: "HANGUL FILLER",
    0xFEFF: "ZERO WIDTH NO-BREAK SPACE (BOM)",
    0xFFA0: "HALFWIDTH HANGUL FILLER",
    0xFFF9: "INTERLINEAR ANNOTATION ANCHOR",
    0xFFFA: "INTERLINEAR ANNOTATION SEPARATOR",
    0xFFFB: "INTERLINEAR ANNOTATION TERMINATOR",
}

ODD_SPACES = {0x00A0, 0x1680, 0x202F, 0x205F, 0x3000, *range(0x2000, 0x200B)}

ALLOWED_CTRL = {0x09, 0x0A, 0x0D, 0x0C}  # tab, LF, CR, form feed


def is_emoji_base(ch):
    cp = ord(ch)
    return (cp >= 0x1F000 or 0x2190 <= cp <= 0x2BFF or 0x2300 <= cp <= 0x23FF
            or 0x2600 <= cp <= 0x27BF or cp in (0x00A9, 0x00AE, 0x203C,
            0x2049, 0x2122, 0x2139, 0x3030, 0x303D, 0x3297, 0x3299)
            or unicodedata.category(ch) == "So")


def classify(text, i):
    """Return (severity, description) for text[i], or None if fine."""
    ch = text[i]
    cp = ord(ch)
    if cp < 0x80 and (cp >= 0x20 and cp != 0x7F or cp in ALLOWED_CTRL):
        return None
    if cp in BIDI:
        return "error", "bidi control " + BIDI[cp]
    if cp == 0xFEFF and i == 0:
        return None  # a leading BOM is ugly but harmless
    if cp == 0x200D:
        # ZWJ inside an emoji sequence (man + ZWJ + laptop) is legitimate.
        prev = text[i - 1] if i > 0 else ""
        nxt = text[i + 1] if i + 1 < len(text) else ""
        if prev and nxt and (is_emoji_base(prev) or ord(prev) in (0xFE0F,)
                             or 0x1F3FB <= ord(prev) <= 0x1F3FF) and is_emoji_base(nxt):
            return None
    if cp in INVISIBLE:
        return "error", "invisible " + INVISIBLE[cp]
    if 0xE0000 <= cp <= 0xE007F:
        return "error", "tag character U+%04X (ASCII smuggling)" % cp
    if 0xFE00 <= cp <= 0xFE0F:
        prev = text[i - 1] if i > 0 else ""
        if cp in (0xFE0E, 0xFE0F) and prev and (is_emoji_base(prev) or prev in "0123456789#*"):
            return None  # emoji / text presentation selector, e.g. U+26A0 U+FE0F
        return "error", "variation selector U+%04X outside emoji" % cp
    if 0xE0100 <= cp <= 0xE01EF:
        return "error", "variation selector supplement U+%05X (smuggling)" % cp
    if cp < 0x20 or cp == 0x7F or 0x80 <= cp <= 0x9F:
        return "error", "control character U+%04X" % cp
    if cp in ODD_SPACES:
        return "warn", "unusual space " + unicodedata.name(ch, "U+%04X" % cp)
    return None


def homoglyph_words(line):
    """Yield (col, word) for words mixing Latin with Cyrillic/Greek letters."""
    word, start = "", 0
    for col, ch in enumerate(line + " "):
        if ch.isalnum() or ch == "_":
            if not word:
                start = col
            word += ch
            continue
        if word:
            scripts = set()
            for c in word:
                if c.isalpha():
                    n = unicodedata.name(c, "")
                    if n.startswith("LATIN"):
                        scripts.add("Latin")
                    elif n.startswith("CYRILLIC"):
                        scripts.add("Cyrillic")
                    elif n.startswith("GREEK"):
                        scripts.add("Greek")
            if "Latin" in scripts and len(scripts) > 1:
                yield start, word
            word = ""


def git(*args):
    return subprocess.run(["git", *args], check=True, capture_output=True,
                          text=True).stdout


def load_ignore(root):
    path = os.path.join(root, ".unicode-guard-ignore")
    try:
        with open(path, encoding="utf-8") as f:
            return [l.strip() for l in f if l.strip() and not l.startswith("#")]
    except OSError:
        return []


def scan_file(path, fix):
    try:
        raw = open(path, "rb").read()
    except OSError as e:
        return [("warn", 0, 0, "cannot read: %s" % e)], False
    if b"\0" in raw[:8192]:
        return [], False  # binary
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as e:
        return [("warn", 0, 0, "invalid UTF-8 at byte %d" % e.start)], False

    findings, keep = [], []
    line, col = 1, 0
    for i, ch in enumerate(text):
        res = classify(text, i)
        if res:
            findings.append((res[0], line, col + 1,
                             "%s (U+%04X)" % (res[1], ord(ch))))
        if not (fix and res and res[0] == "error"):
            keep.append(ch)
        if ch == "\n":
            line, col = line + 1, 0
        else:
            col += 1
    for n, l in enumerate(text.split("\n"), 1):
        for c, w in homoglyph_words(l):
            findings.append(("warn", n, c + 1,
                             "word %r mixes Latin with Cyrillic/Greek (homoglyph?)" % w))

    changed = False
    if fix and any(f[0] == "error" for f in findings):
        new = "".join(keep)
        if new != text:
            with open(path, "w", encoding="utf-8", newline="") as f:
                f.write(new)
            changed = True
    return findings, changed


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("files", nargs="*")
    ap.add_argument("--changed", metavar="BASE")
    ap.add_argument("--fix", action="store_true")
    ap.add_argument("--github", action="store_true")
    ap.add_argument("--strict", action="store_true")
    a = ap.parse_args(argv)

    if a.files and a.changed:
        ap.error("give FILEs or --changed, not both")
    if a.changed:
        files = git("diff", "--name-only", "--diff-filter=ACMR",
                    a.changed + "...HEAD").split("\n")
    elif a.files:
        files = a.files
    else:
        files = git("ls-files", "-z").split("\0")
    ignore = load_ignore(".")
    # Symlinks are skipped (rootfs-alpine has hundreds; never follow them
    # out of the tree), as are paths in .unicode-guard-ignore.
    files = [f for f in files if f and os.path.isfile(f) and not os.path.islink(f)
             and not any(fnmatch.fnmatch(f, g) for g in ignore)]

    errors = warns = fixed = 0
    for path in files:
        findings, changed = scan_file(path, a.fix)
        fixed += changed
        for sev, line, col, msg in findings:
            if sev == "error":
                errors += 1
            else:
                warns += 1
            tag = "FIXED" if changed and sev == "error" else sev.upper()
            print("%s:%d:%d: %s: %s" % (path, line, col, tag, msg))
            if a.github and not (changed and sev == "error"):
                level = "error" if sev == "error" else "warning"
                print("::%s file=%s,line=%d,col=%d::%s" % (level, path, line, col, msg))

    print("unicode-guard: %d file(s) scanned, %d error(s), %d warning(s)%s"
          % (len(files), errors, warns,
             ", %d file(s) fixed" % fixed if a.fix else ""), file=sys.stderr)
    if a.fix:
        return 1 if a.strict and warns else 0
    return 1 if errors or (a.strict and warns) else 0


if __name__ == "__main__":
    sys.exit(main())
