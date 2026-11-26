/**
 * files-parse — pure helpers for the console Files view over execv2
 * (#208, D79 — phương án 1: adapter-only, daemon không đổi).
 *
 * The daemon has no files API; every operation runs through
 * `POST /v1/sandboxes/{id}/execv2`. SAFETY RULE: every path is passed as an
 * argv OPERAND (`sh -c <CONSTANT-SCRIPT> sh <path>`), never interpolated
 * into the script text — a filename containing `;`, `$()`, backticks or
 * newlines cannot break out because the script is a constant and the path
 * is a quoted positional ($1). Listing uses busybox `stat -c` (verified on
 * the repo rootfs 2026-11-26, MD67):
 *
 *     stat -c "%F\t%s\t%Y\t%a\t%n" <path>
 *     → "regular file\t3\t1795687538\t664\t/probe/a.txt"
 *       "directory\t4096\t1795687538\t775\t/probe/subdir"
 *
 * %n is LAST so a tab inside a filename stays part of the name (we join the
 * remainder); a NEWLINE inside a filename splits rows — those rows have
 * fewer than 5 columns and are SKIPPED (documented v1 limitation).
 */

/** The busybox stat format the list script prints (TAB-separated). */
export const FS_STAT_FMT = "%F\\t%s\\t%Y\\t%a\\t%n";

/**
 * List script — CONSTANT (never receives user text). `$1` is the directory
 * operand. Three globs cover dotfiles but skip "." and ".."; `[ -e ]` drops
 * unmatched globs (empty dir); trailing `true` forces exit 0 so a stat that
 * races a removal does not fail the listing.
 */
export const FS_LIST_SCRIPT =
  'for f in "$1"/* "$1"/.[!.]* "$1"/..?*; do [ -e "$f" ] && stat -c "%F\\t%s\\t%Y\\t%a\\t%n" "$f"; done; true';

/** stat -c "%s\t%Y\t%a" <path> — size, epoch mtime, octal mode. */
export const FS_STAT_ONE_SCRIPT = 'stat -c "%F\t%s\\t%Y\\t%a" "$1"';

/** Hard cap for one readFile/writeFile round-trip (daemon maxBodyBytes is 16 MB). */
export const FS_MAX_FILE = 1_000_000;

export interface RawFsEntry {
  type: "dir" | "file";
  size: number;
  mtime: number;
  mode: string;
  name: string;
}

/** Parses `stat -c` rows; skips malformed rows (name-with-newline artifact). */
export function parseStatEntries(out: string): RawFsEntry[] {
  const rows: RawFsEntry[] = [];
  for (const line of out.split("\n")) {
    if (line === "") continue;
    const cols = line.split("\t");
    if (cols.length < 5) continue;
    const type: RawFsEntry["type"] = cols[0] === "directory" ? "dir" : "file";
    const size = Number(cols[1]);
    const mtime = Number(cols[2]);
    const mode = cols[3];
    const name = cols.slice(4).join("\t");
    if (name === "" || Number.isNaN(size) || Number.isNaN(mtime)) continue;
    rows.push({ type, size, mtime, mode, name });
  }
  return rows;
}

/** Absolute-path guard shared by every files op (argv operand, never script text). */
export function assertSafePath(p: string | undefined | null): string {
  const path = String(p ?? "");
  if (!path.startsWith("/")) {
    throw new Error(`bad_path: path phải tuyệt đối (bắt đầu bằng /): ${path.slice(0, 80)}`);
  }
  if (path.includes("\0")) throw new Error("bad_path: path chứa NUL");
  if (path.length > 4096) throw new Error("bad_path: path quá dài (> 4096)");
  return path;
}

/** Upload name guard — a name is ONE path segment, never a path. */
export function assertSafeName(name: string | undefined): string {
  const n = String(name ?? "");
  if (n === "" || n === "." || n === ".." || n.includes("/") || n.includes("\0") || n.includes("\\")) {
    throw new Error(`bad_name: tên file không hợp lệ: ${JSON.stringify(n.slice(0, 80))}`);
  }
  return n;
}

export function joinPath(dir: string, name: string): string {
  return dir === "/" ? `/${name}` : `${dir.replace(/\/+$/, "")}/${name}`;
}

export function utf8Bytes(s: string): Uint8Array {
  return new TextEncoder().encode(s);
}

/** base64 of raw bytes — chunked btoa (btoa chokes on large spread args). */
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

/** Decode best-effort: valid UTF-8 → text; binary → base64 round-trip. */
export function decodeBest(bytes: Uint8Array): { content: string; encoding: "utf8" | "base64" } {
  try {
    return { content: new TextDecoder("utf-8", { fatal: true }).decode(bytes), encoding: "utf8" };
  } catch {
    return { content: bytesToB64(bytes), encoding: "base64" };
  }
}
