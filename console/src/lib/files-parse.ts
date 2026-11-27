/**
 * files-parse — pure helpers for the console Files view over execv2
 * (#208, D79 — phương án 1: adapter-only, daemon không đổi; #219, D83 —
 * tên file dạng bytes: mọi path/tên đi qua wire dưới dạng base64).
 *
 * The daemon has no files API; every operation runs through
 * `POST /v1/sandboxes/{id}/execv2`. SAFETY RULES:
 *
 * 1. Scripts are CONSTANTS — user text never lands in the script body.
 * 2. #219 (D83): a path travels as a base64 ARGV OPERAND and is decoded
 *    inside the sandbox by `FS_DECODE_SNIPPET` — still a quoted positional,
 *    never interpolated. Raw bytes (invalid UTF-8, newline, tab) cannot be
 *    mangled by Go's JSON string encoding (json.Marshal replaces invalid
 *    UTF-8 with U+FFFD — measured on the real daemon, 2026-11-27: a file
 *    named `t<0xE1><0xBA>ext.txt` came back as `t\ufffd\ufffdext.txt`).
 * 3. The list prints one row per entry:
 *      type\tsize\tmtime\tmode\tpath_b64\tname_b64
 *    base64 alphabet has no tab/newline → a newline inside a filename can
 *    no longer split rows (v1 limitation removed).
 * 4. Display strings (`name`/`path`) are DERIVATES, never used as paths:
 *    valid UTF-8 → text; otherwise `<tên không phải UTF-8>` + hex dump
 *    (git `core.quotepath` philosophy: bytes are the truth).
 */

/**
 * Decode prefix — `$1` is a base64 operand; `$p` becomes the RAW path bytes.
 * `echo x` keeps trailing newlines that command substitution would strip,
 * `${p%x}` removes the marker. busybox `base64 -d` accepts unwrapped input
 * (measured on the repo rootfs, #219 repro 2026-11-27).
 */
export const FS_DECODE_SNIPPET = 'p=$(printf %s "$1" | base64 -d; echo x); p=${p%x}; ';

/** Same for a second operand (`$2` → `$q`) — move has two paths. */
export const FS_DECODE_SNIPPET2 = 'q=$(printf %s "$2" | base64 -d; echo x); q=${q%x}; ';

/**
 * List script — CONSTANT. `$1` = directory as base64. One row per entry:
 * `type\tsize\tmtime\tmode\tpath_b64\tname_b64`. Three globs cover dotfiles
 * but skip "." and ".."; `[ -e ] || continue` drops unmatched globs (empty
 * dir); trailing `true` forces exit 0 so a stat racing a removal does not
 * fail the listing. The basename comes from a shell suffix expansion (raw
 * bytes, newline-safe, no basename applet). `tr -d '\n'` guards the
 * GNU-wrapped-base64 case; the busybox rootfs emits one line (measured
 * 2026-11-27).
 */
export const FS_LIST_SCRIPT_B64 =
  FS_DECODE_SNIPPET +
  'for f in "$p"/* "$p"/.[!.]* "$p"/..?*; do [ -e "$f" ] || continue; ' +
  // busybox stat does NOT interpret "\\t" escapes (unlike printf/tr — measured
  // 2026-11-27), so the format below carries REAL TAB characters (TS \t).
  "m=$(stat -c \"%F\t%s\t%Y\t%a\" \"$f\") || continue; " +
  "pb=$(printf %s \"$f\" | base64 | tr -d '\\n'); " +
  "nb=$(printf %s \"${f##*/}\" | base64 | tr -d '\\n'); " +
  "printf '%s\\t%s\\t%s\\n' \"$m\" \"$pb\" \"$nb\"; done; true";

/** stat one path — `type<TAB>size<TAB>mtime<TAB>mode` (operand = base64).
 *  REAL TAB characters — busybox stat does not interpret \\t escapes. */
export const FS_STAT_ONE_SCRIPT_B64 = FS_DECODE_SNIPPET + "stat -c \"%F\t%s\t%Y\t%a\" \"$p\"";

/** base64 the file content to stdout (operand = base64). */
export const FS_READ_SCRIPT_B64 = FS_DECODE_SNIPPET + 'base64 "$p"';

/** Write stdin into the path (operand = base64). */
export const FS_WRITE_SCRIPT_B64 = FS_DECODE_SNIPPET + 'cat > "$p"';

/** Create a directory (operand = base64). */
export const FS_MKDIR_SCRIPT_B64 = FS_DECODE_SNIPPET + 'mkdir -p "$p"';

/** Create an empty file (operand = base64). */
export const FS_TOUCH_SCRIPT_B64 = FS_DECODE_SNIPPET + '[ -e "$p" ] || : > "$p"';

/** Move/rename two base64 operands. */
export const FS_MOVE_SCRIPT_B64 = FS_DECODE_SNIPPET + FS_DECODE_SNIPPET2 + 'mv "$p" "$q"';

/** Recursive delete (operand = base64; adapter refuses the workspace root). */
export const FS_DELETE_SCRIPT_B64 = FS_DECODE_SNIPPET + 'rm -rf -- "$p"';

/** Hard cap for one readFile/writeFile round-trip (daemon maxBodyBytes is 16 MB). */
export const FS_MAX_FILE = 1_000_000;

export interface RawFsEntry {
  type: "dir" | "file";
  size: number;
  mtime: number;
  mode: string;
  /** raw full-path bytes, base64 (#219) — source of truth for every op */
  path_b64: string;
  /** raw basename bytes, base64 (#219) */
  name_b64: string;
}

/**
 * Parses `type\tsize\tmtime\tmode\tpath_b64\tname_b64` rows. base64 never
 * contains tab/newline, so a well-formed row has exactly 6 columns; other
 * rows are skipped (trailing-newline artifact).
 */
export function parseStatEntries(out: string): RawFsEntry[] {
  const rows: RawFsEntry[] = [];
  for (const line of out.split("\n")) {
    if (line === "") continue;
    const cols = line.split("\t");
    if (cols.length !== 6) continue;
    const type: RawFsEntry["type"] = cols[0] === "directory" ? "dir" : "file";
    const size = Number(cols[1]);
    const mtime = Number(cols[2]);
    const mode = cols[3];
    const path_b64 = cols[4];
    const name_b64 = cols[5];
    const B64RE = /^[A-Za-z0-9+/]+={0,2}$/;
    if (path_b64 === "" || name_b64 === "" || Number.isNaN(size) || Number.isNaN(mtime)) continue;
    if (!B64RE.test(path_b64) || !B64RE.test(name_b64)) continue;
    rows.push({ type, size, mtime, mode, path_b64, name_b64 });
  }
  return rows;
}

/**
 * Path guard over the base64 WIRE form (#219): decodes to raw bytes and
 * checks there — absolute (first byte `/`), no NUL byte, length cap. The
 * check runs on BYTES, not on a display string, so an invalid-UTF-8 path
 * is judged by what will actually hit the filesystem. Returns the
 * normalized base64 (whitespace stripped).
 */
export function assertSafePathB64(b64: string | undefined | null): string {
  const b = b64ToBytes(String(b64 ?? ""));
  if (b.length === 0 || b[0] !== 0x2f /* / */) {
    throw new Error("bad_path: path phải tuyệt đối (bắt đầu bằng /)");
  }
  if (b.includes(0)) throw new Error("bad_path: path chứa NUL");
  if (b.length > 4096) throw new Error("bad_path: path quá dài (> 4096 bytes)");
  return bytesToB64(b);
}

/** Upload/name guard — a name is ONE path segment, never a path. Names the
 *  USER types are valid UTF-8 by construction (browser input); recovery
 *  renames also land here (#219). */
export function assertSafeName(name: string | undefined): string {
  const n = String(name ?? "");
  if (n === "" || n === "." || n === ".." || n.includes("/") || n.includes("\0") || n.includes("\\")) {
    throw new Error(`bad_name: tên file không hợp lệ: ${JSON.stringify(n.slice(0, 80))}`);
  }
  return n;
}

/** Join a base64 dir with a UTF-8 name at BYTE level (#219) — the dir may
 *  hold invalid-UTF-8 bytes, so never round-trip it through a JS string. */
export function joinPathB64(dirB64: string, name: string): string {
  const bytes = Array.from(b64ToBytes(dirB64));
  while (bytes.length > 1 && bytes[bytes.length - 1] === 0x2f) bytes.pop();
  const joined =
    bytes.length === 1 && bytes[0] === 0x2f
      ? [0x2f, ...Array.from(utf8Bytes(name))]
      : [...bytes, 0x2f, ...Array.from(utf8Bytes(name))];
  return bytesToB64(new Uint8Array(joined));
}

/** Parent directory of a base64 path (byte level); root maps to root. */
export function parentB64(pathB64: string): string {
  const bytes = Array.from(b64ToBytes(pathB64));
  while (bytes.length > 1 && bytes[bytes.length - 1] === 0x2f) bytes.pop();
  const cut = bytes.lastIndexOf(0x2f);
  if (cut <= 0) return bytesToB64(new Uint8Array([0x2f]));
  return bytesToB64(new Uint8Array(bytes.slice(0, cut)));
}

export interface PathCrumb {
  /** base64 of this absolute path (all segments up to and incl. this one) */
  b64: string;
  /** best-effort display label (may be the invalid-UTF-8 placeholder) */
  label: string;
  /** false → the raw segment bytes are not valid UTF-8 */
  valid: boolean;
}

/** Split a base64 path into breadcrumb segments at BYTE level (#219). */
export function crumbsFromB64(pathB64: string): PathCrumb[] {
  const bytes = b64ToBytes(pathB64);
  if (bytes.length === 0 || bytes[0] !== 0x2f) return [];
  const out: PathCrumb[] = [];
  let acc: number[] = [];
  for (let i = 1; i <= bytes.length; i++) {
    if (i === bytes.length || bytes[i] === 0x2f) {
      const b64 = bytesToB64(new Uint8Array([0x2f, ...acc]));
      const d = decodeFsName(bytesToB64(new Uint8Array(acc)));
      out.push({ b64, label: d.display, valid: d.valid });
      acc = [];
    } else {
      acc.push(bytes[i]);
    }
  }
  return out;
}

export function utf8Bytes(s: string): Uint8Array {
  return new TextEncoder().encode(s);
}

/** base64 of raw bytes — chunked (String.fromCharCode spread has arg limits). */
export function bytesToB64(bytes: Uint8Array): string {
  let bin = "";
  const CH = 0x8000;
  for (let i = 0; i < bytes.length; i += CH) {
    bin += String.fromCharCode(...bytes.subarray(i, i + CH));
  }
  return btoa(bin);
}

export function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64.replace(/\s+/g, ""));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/** base64 → strict UTF-8 text (throws on invalid bytes — demo boundary). */
export function b64ToText(b64: string): string {
  return new TextDecoder("utf-8", { fatal: true }).decode(b64ToBytes(b64));
}

/** Encode a valid-UTF-8 JS string into the base64 wire form (#219). */
export function encodePathB64(s: string): string {
  return bytesToB64(utf8Bytes(s));
}

export interface FsNameDisplay {
  /** what the UI shows ("tiếng.txt" or the invalid-UTF-8 placeholder) */
  display: string;
  /** false → the raw bytes are not valid UTF-8 (needs recovery) */
  valid: boolean;
  /** hex dump prefix ("hex: 74 e1 ba …"), empty when valid */
  hex: string;
}

const INVALID_NAME_PLACEHOLDER = "<tên không phải UTF-8>";

/**
 * Display form of a base64 name (#219): valid UTF-8 → the text itself;
 * otherwise a placeholder + hex dump of the raw bytes (git `core.quotepath`
 * philosophy: bytes are the truth, the string is a display derivate that
 * may not exist). Hex is capped so a pathological name cannot blow up the
 * table row.
 */
export function decodeFsName(b64: string): FsNameDisplay {
  const bytes = b64ToBytes(b64);
  try {
    return { display: new TextDecoder("utf-8", { fatal: true }).decode(bytes), valid: true, hex: "" };
  } catch {
    const HEX_CAP = 24;
    const shown = bytes.subarray(0, HEX_CAP);
    const hex =
      Array.from(shown, (b) => b.toString(16).padStart(2, "0")).join(" ") +
      (bytes.length > HEX_CAP ? ` … (+${bytes.length - HEX_CAP})` : "");
    return { display: INVALID_NAME_PLACEHOLDER, valid: false, hex: `hex: ${hex}` };
  }
}

/** Decode best-effort: valid UTF-8 → text; binary → base64 round-trip. */
export function decodeBest(bytes: Uint8Array): { content: string; encoding: "utf8" | "base64" } {
  try {
    return { content: new TextDecoder("utf-8", { fatal: true }).decode(bytes), encoding: "utf8" };
  } catch {
    return { content: bytesToB64(bytes), encoding: "base64" };
  }
}
