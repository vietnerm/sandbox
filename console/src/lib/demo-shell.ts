/* ── Demo shell (PTY emulator) ────────────────────────────────────────
   When no sandboxd PTY WebSocket is reachable, the Terminal page runs
   this shell locally. It behaves like a real bash session against the
   demo filesystem (src/lib/demo-fs.ts): cd/ls/cat/rm/mv/echo>… all
   mutate the same tree the Files manager shows, so edits made in the
   FTP manager are visible here instantly and vice versa.

   Supported input: line editing (backspace/ctrl+c/ctrl+l/ctrl+u),
   ↑/↓ history, tab completion for commands and paths, ~ and relative
   paths, pipes are NOT supported (single command per line). */

import {
  FsError,
  baseName,
  dirName,
  ensureFs,
  fsAllPaths,
  fsCopy,
  fsList,
  fsMove,
  fsMkdir,
  fsRead,
  fsRemove,
  fsStat,
  fsTouch,
  fsUsage,
  fsWrite,
  resetFs,
  resolvePath,
} from "./demo-fs";

export interface DemoShellOpts {
  sandboxId: string;
  project: string;
  image: string;
  egressMode?: string;
  memLimitMb?: number;
  diskGb?: number;
  write: (data: string) => void;
  /** called when the user types exit / ctrl+d */
  onExit?: () => void;
}

const R = "\x1b[0m";
const B = (s: string) => `\x1b[1m${s}${R}`;
const DIM = (s: string) => `\x1b[90m${s}${R}`;
const RED = (s: string) => `\x1b[31m${s}${R}`;
const GRN = (s: string) => `\x1b[32m${s}${R}`;
const YLW = (s: string) => `\x1b[33m${s}${R}`;
const BLU = (s: string) => `\x1b[36m${s}${R}`;

export function modeToRwx(mode: string, isDir: boolean): string {
  const bit = (c: string) => (c === "0" ? "---" : c === "4" ? "r--" : c === "5" ? "r-x" : c === "6" ? "rw-" : c === "7" ? "rwx" : "---");
  const [u, g, o] = mode.padStart(3, "0").split("");
  return `${isDir ? "d" : "-"}${bit(u)}${bit(g)}${bit(o)}`;
}

function fmtBytes(n: number): string {
  if (n < 1024) return `${n}`;
  const u = ["K", "M", "G", "T"];
  let i = -1;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)}${u[i]}`;
}

function padEnd(s: string, n: number): string {
  return s.length >= n ? s : s + " ".repeat(n - s.length);
}

/** pad on the VISIBLE length (ANSI escapes stripped) */
function padVisible(s: string, n: number): string {
  const visible = s.replace(/\x1b\[[0-9;]*m/g, "");
  return visible.length >= n ? s : s + " ".repeat(n - visible.length);
}

const COMMANDS = [
  "help", "pwd", "ls", "ll", "cd", "cat", "head", "tail", "echo", "touch", "mkdir", "rm", "cp", "mv",
  "tree", "find", "grep", "wc", "du", "df", "free", "ps", "top", "env", "whoami", "id", "uname",
  "date", "uptime", "curl", "git", "node", "npm", "python3", "pip", "cargo", "which", "history",
  "clear", "exit", "reseed", "edit",
] as const;

function tokenize(line: string): string[] {
  const out: string[] = [];
  let cur = "";
  let q: string | null = null;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (q) {
      if (c === q) q = null;
      else cur += c;
      continue;
    }
    if (c === '"' || c === "'") {
      q = c;
      continue;
    }
    if (c === " ") {
      if (cur) out.push(cur);
      cur = "";
      continue;
    }
    cur += c;
  }
  if (cur) out.push(cur);
  return out;
}

export class DemoShell {
  private cwd: string;
  private line = "";
  private cursor = 0; // caret inside `line`
  private history: string[] = [];
  private histIdx = -1;
  private started = Date.now();
  private pendingEsc = "";
  private o: DemoShellOpts;

  constructor(opts: DemoShellOpts) {
    this.o = opts;
    // make sure the workspace exists under the right project name before
    // the first cd/ls — the same seed the Files manager will list later
    ensureFs(opts.sandboxId, opts.project || "workspace", opts.image);
    const preferred = `/home/sandbox/${opts.project || "workspace"}`;
    const st = fsStat(opts.sandboxId, preferred);
    const home = fsStat(opts.sandboxId, "/home/sandbox");
    this.cwd = st?.type === "dir" ? preferred : home?.type === "dir" ? "/home/sandbox" : "/";
  }

  /* ── output helpers ─────────────────────────────────────────────── */

  private out(s = ""): void {
    this.o.write(s.replace(/\n/g, "\r\n"));
  }

  private outln(s = ""): void {
    this.out(s + "\n");
  }

  prompt(): string {
    const short = this.cwd.replace("/home/sandbox", "~");
    return `${GRN("sandbox")}@${BLU(this.o.sandboxId)}:${BLU(short)}$ `;
  }

  private redrawPrompt(): void {
    this.out("\r\x1b[K" + this.prompt() + this.line);
    if (this.cursor < this.line.length) {
      // move caret back
      this.out(`\x1b[${this.line.length - this.cursor}D`);
    }
  }

  /* ── input from the terminal ────────────────────────────────────── */

  handleData(data: string): void {
    for (const ch of data) {
      const c = ch.codePointAt(0) ?? 0;
      if (this.pendingEsc) {
        this.handleEsc(this.pendingEsc + ch);
        continue;
      }
      if (ch === "\x1b") {
        this.pendingEsc = ch;
        continue;
      }
      if (ch === "\r" || ch === "\n") {
        this.out("\r\n");
        const cmd = this.line.trim();
        this.line = "";
        this.cursor = 0;
        if (cmd) {
          if (this.history[this.history.length - 1] !== cmd) this.history.push(cmd);
          if (this.history.length > 200) this.history.shift();
          this.execute(cmd);
        }
        if (!this.closed) this.outln(this.prompt());
        continue;
      }
      if (ch === "\x7f") {
        // backspace
        if (this.cursor > 0) {
          this.line = this.line.slice(0, this.cursor - 1) + this.line.slice(this.cursor);
          this.cursor--;
          this.out("\b\x1b[P");
        }
        continue;
      }
      if (ch === "\x03") {
        // ctrl+c
        this.outln("^C");
        this.line = "";
        this.cursor = 0;
        this.out(this.prompt());
        continue;
      }
      if (ch === "\x04") {
        // ctrl+d
        this.outln("exit");
        this.cmdExit();
        continue;
      }
      if (ch === "\x0c") {
        // ctrl+l
        this.out("\x1b[2J\x1b[H" + this.prompt() + this.line);
        continue;
      }
      if (ch === "\x15") {
        // ctrl+u — clear line
        this.out("\r\x1b[K" + this.prompt());
        this.line = "";
        this.cursor = 0;
        continue;
      }
      if (ch === "\t") {
        this.complete();
        continue;
      }
      if (c >= 32) {
        this.line = this.line.slice(0, this.cursor) + ch + this.line.slice(this.cursor);
        this.cursor++;
        if (this.cursor === this.line.length) this.out(ch === " " ? "\x1b[K " : ch);
        else {
          this.out("\r\x1b[K" + this.prompt() + this.line);
          this.redrawPromptAfterCaret();
        }
      }
    }
  }

  private handleEsc(seq: string): void {
    if (seq === "\x1b") {
      this.pendingEsc = "";
      return;
    }
    if (seq === "\x1b[" ) return; // keep accumulating
    this.pendingEsc = "";
    if (seq === "\x1b[A") {
      // up
      if (this.history.length === 0) return;
      this.histIdx = this.histIdx === -1 ? this.history.length - 1 : Math.max(0, this.histIdx - 1);
      this.line = this.history[this.histIdx];
      this.cursor = this.line.length;
      this.out("\r\x1b[K" + this.prompt() + this.line);
    } else if (seq === "\x1b[B") {
      // down
      if (this.histIdx === -1) return;
      this.histIdx++;
      if (this.histIdx >= this.history.length) {
        this.histIdx = -1;
        this.line = "";
      } else {
        this.line = this.history[this.histIdx];
      }
      this.cursor = this.line.length;
      this.out("\r\x1b[K" + this.prompt() + this.line);
    } else if (seq === "\x1b[C" && this.cursor < this.line.length) {
      this.cursor++;
      this.out("\x1b[C");
    } else if (seq === "\x1b[D" && this.cursor > 0) {
      this.cursor--;
      this.out("\x1b[D");
    } else if (seq === "\x1b[3~") {
      // delete key
      if (this.cursor < this.line.length) {
        this.line = this.line.slice(0, this.cursor) + this.line.slice(this.cursor + 1);
        this.out("\x1b[P");
      }
    } else if (seq === "\x1b[H" || seq === "\x1b[1~") {
      this.cursor = 0;
      this.out("\r");
      this.out(`${this.line.length ? "" : ""}`);
      this.redrawPromptAfterCaret();
    } else if (seq === "\x1b[F" || seq === "\x1b[4~") {
      this.cursor = this.line.length;
      this.redrawPromptAfterCaret();
    }
  }

  private redrawPromptAfterCaret(): void {
    this.out(`\x1b[${this.line.length - this.cursor}D`);
  }

  /* ── tab completion ─────────────────────────────────────────────── */

  private complete(): void {
    const before = this.line.slice(0, this.cursor);
    const frag = (/[^\s]*$/.exec(before))?.[0] ?? "";
    const isFirst = before.slice(0, before.length - frag.length).trim() === "";
    let matches: string[] = [];
    if (isFirst) {
      matches = COMMANDS.filter((c) => c.startsWith(frag));
    } else {
      const dirPart = frag.includes("/") ? frag.slice(0, frag.lastIndexOf("/") + 1) : "";
      const namePart = frag.slice(dirPart.length);
      const base = resolvePath(this.cwd, dirPart || ".");
      try {
        matches = fsList(this.o.sandboxId, base)
          .filter((e) => !e.name.startsWith(".") || namePart.startsWith("."))
          .filter((e) => e.name.startsWith(namePart))
          .map((e) => e.name + (e.type === "dir" ? "/" : ""));
      } catch {
        matches = [];
      }
    }
    if (!matches.length) return;
    const seg = isFirst ? frag : frag.slice(frag.lastIndexOf("/") + 1); // the segment being completed
    let insert = "";
    if (matches.length === 1) {
      insert = matches[0].slice(seg.length) + (isFirst ? " " : "");
    } else {
      let prefix = matches[0];
      for (const m of matches) while (!m.startsWith(prefix)) prefix = prefix.slice(0, -1);
      insert = prefix.slice(seg.length);
    }
    const head = before.slice(0, before.length - seg.length);
    if (insert) {
      this.line = head + seg + insert + this.line.slice(this.cursor);
      this.cursor = before.length + insert.length;
    }
    this.out("\r\x1b[K" + this.prompt() + this.line);
    this.redrawPromptAfterCaret();
    if (matches.length > 1) {
      this.out("\r\n");
      this.outln(matches.map((m) => padVisible(m, 26)).join(""));
      this.out(this.prompt() + this.line);
      this.redrawPromptAfterCaret();
    }
  }

  /* ── execution ──────────────────────────────────────────────────── */

  closed = false;

  private err(msg: string): void {
    this.outln(RED(msg));
  }

  private ok(msg: string): void {
    this.outln(msg);
  }

  private execute(raw: string): void {
    // support `a && b` (sequential), the common case
    const parts = raw.split("&&").map((s) => s.trim()).filter(Boolean);
    for (const p of parts) {
      this.runOne(p);
      if (this.closed) return;
    }
  }

  private runOne(raw: string): void {
    // echo redirections
    const redirect = /(?:^|\s)(>>|>)\s*("[^"]*"|'[^']*'|[^\s]+)\s*$/.exec(raw);
    let cmdLine = raw;
    let redirectTarget: string | null = null;
    let append = false;
    if (redirect && /^echo\b/.test(raw)) {
      redirectTarget = redirect[2].replace(/^["']|["']$/g, "");
      append = redirect[1] === ">>";
      cmdLine = raw.slice(0, redirect.index).trim();
    }
    const tokens = tokenize(cmdLine);
    if (!tokens.length) return;
    const [name, ...args] = tokens;
    const flags = args.filter((a) => a.startsWith("-")).join("");
    const rest = args.filter((a) => !a.startsWith("-"));

    try {
      switch (name) {
        case "help": return this.cmdHelp();
        case "pwd": return this.ok(this.cwd);
        case "ls":
        case "ll": return this.cmdLs(rest, flags + (name === "ll" ? "l" : ""));
        case "cd": return this.cmdCd(rest[0]);
        case "cat": return this.cmdCat(rest);
        case "head": return this.cmdHeadTail(rest, args, "head");
        case "tail": return this.cmdHeadTail(rest, args, "tail");
        case "echo": {
          const text = cmdLine.replace(/^echo\s*/, "").replace(/^["']|["']$/g, "");
          if (redirectTarget) {
            const p = resolvePath(this.cwd, redirectTarget);
            const prev = append ? (this.readFileSafe(p)?.content ?? "") : "";
            fsWrite(this.o.sandboxId, p, prev + (prev && !prev.endsWith("\n") ? "\n" : "") + text + "\n");
            return;
          }
          return this.ok(text);
        }
        case "touch": {
          if (!rest.length) return this.err("touch: missing operand");
          for (const f of rest) fsTouch(this.o.sandboxId, resolvePath(this.cwd, f));
          return;
        }
        case "mkdir": {
          if (!rest.length) return this.err("mkdir: missing operand");
          for (const d of rest) fsMkdir(this.o.sandboxId, resolvePath(this.cwd, d), flags.includes("p"));
          return;
        }
        case "rm": {
          if (!rest.length) return this.err("rm: missing operand");
          for (const f of rest) fsRemove(this.o.sandboxId, resolvePath(this.cwd, f), flags.includes("r"));
          return;
        }
        case "cp": {
          if (rest.length < 2) return this.err("cp: missing destination");
          fsCopy(this.o.sandboxId, resolvePath(this.cwd, rest[0]), resolvePath(this.cwd, rest[1]), flags.includes("r"));
          return;
        }
        case "mv": {
          if (rest.length < 2) return this.err("mv: missing destination");
          fsMove(this.o.sandboxId, resolvePath(this.cwd, rest[0]), resolvePath(this.cwd, rest[1]), true);
          return;
        }
        case "tree": return this.cmdTree(rest[0]);
        case "find": return this.cmdFind(rest, args);
        case "grep": return this.cmdGrep(rest);
        case "wc": return this.cmdWc(rest, flags);
        case "du": return this.cmdDu(rest[0]);
        case "df": return this.cmdDf();
        case "free": return this.cmdFree();
        case "ps": return this.cmdPs();
        case "top": return this.cmdPs(true);
        case "env": return this.cmdEnv();
        case "whoami": return this.ok("sandbox");
        case "id": return this.ok("uid=1000(sandbox) gid=1000(sandbox) groups=1000(sandbox),27(sudo)");
        case "uname": return this.ok(flags.includes("a") ? "Linux sbx.local 6.8.0-45-generic #45-Ubuntu SMP x86_64 GNU/Linux" : "Linux");
        case "date": return this.ok(new Date().toString());
        case "uptime": return this.cmdUptime();
        case "curl": return this.cmdCurl(rest[0]);
        case "git": return this.cmdGit(rest);
        case "node": return this.ok(rest[0] === "-v" || rest[0] === "--version" ? "v22.11.0" : `node: eval is disabled in the demo shell — try \`help\``);
        case "npm": return this.ok(rest[0] === "-v" || rest[0] === "--version" ? "10.9.0" : "npm demo — use the Files page to edit package.json");
        case "python3": return this.ok(rest[0] === "-V" || rest[0] === "--version" ? "Python 3.12.7" : "python3 demo — scripts run on the real image");
        case "pip": return this.ok("pip 24.2 from /usr/lib/python3.12/site-packages/pip (python 3.12)");
        case "cargo": return this.ok(rest[0] === "--version" ? "cargo 1.83.0" : "cargo demo");
        case "which": return this.ok(rest[0] && COMMANDS.includes(rest[0] as (typeof COMMANDS)[number]) ? `/usr/bin/${rest[0]}` : `${rest[0]} not found`);
        case "history": return this.history.forEach((h, i) => this.ok(`${String(i + 1).padStart(4)}  ${h}`));
        case "clear": return this.out("\x1b[2J\x1b[H");
        case "edit": return this.ok(DIM(`hint: open the Files tab — the editor writes the same workspace`));
        case "vi": case "vim": case "nano": case "emacs":
          return this.ok(YLW(`${name}: interactive editors are not available here.`) + DIM(` Use the Files page editor or \`edit\`.`));
        case "reseed": {
          resetFs(this.o.sandboxId);
          return this.ok(GRN("workspace reseeded"));
        }
        case "exit": return this.cmdExit();
        default: return this.err(`bash: ${name}: command not found — type ${B("help")}`);
      }
    } catch (e) {
      if (e instanceof FsError) return this.err(`${name}: ${e.message}`);
      return this.err(`${name}: ${String(e)}`);
    }
  }

  private readFileSafe(p: string) {
    try {
      return fsRead(this.o.sandboxId, p).node;
    } catch {
      return null;
    }
  }

  private cmdExit(): void {
    this.closed = true;
    this.outln(DIM("logout — demo session closed. Click Restart session to reconnect."));
    this.o.onExit?.();
  }

  private cmdHelp(): void {
    const rows: Array<[string, string]> = [
      ["ls [-la] [path]", "list directory contents"],
      ["cd [path]", "change directory (~ supported)"],
      ["pwd", "print working directory"],
      ["cat / head / tail <file>", "print file contents"],
      ["echo text > file", "write / append (>>) text to a file"],
      ["touch / mkdir [-p] <name>", "create file / directory"],
      ["rm [-r] / cp [-r] / mv <a> <b>", "remove / copy / move"],
      ["tree [path]", "show the workspace as a tree"],
      ["grep <pat> <file> · find [path]", "search inside files / by name"],
      ["wc / du / df / free", "sizes and usage"],
      ["ps / top / env / uptime", "processes and environment"],
      ["git status|log|branch", "simulated git snapshot"],
      ["node -v · python3 -V · npm -v", "toolchain versions"],
      ["edit", "tip: edit files in the Files tab"],
      ["reseed", "reset the demo workspace"],
      ["clear · exit", "clear screen · close session"],
    ];
    this.outln(B("demo shell — sandboxd PTY emulator"));
    this.outln(DIM("the full bash is available on a real node; these are the demo equivalents.\n"));
    for (const [c, d] of rows) this.outln(`  ${BLU(padEnd(c, 30))}${d}`);
    this.outln("");
  }

  private cmdLs(paths: string[], flags: string): void {
    const long = flags.includes("l");
    const all = flags.includes("a");
    const targets = paths.length ? paths : ["."];
    for (const t of targets) {
      const p = resolvePath(this.cwd, t);
      const st = fsStat(this.o.sandboxId, p);
      if (!st) return this.err(`ls: cannot access '${t}': No such file or directory`);
      if (st.type === "file") {
        if (long) this.outln(this.longRow(t, st));
        else this.outln(t);
        return;
      }
      if (targets.length > 1) this.outln(`${p}:`);
      let entries = fsList(this.o.sandboxId, p);
      if (!all) entries = entries.filter((e) => !e.name.startsWith("."));
      if (long) {
        this.outln(DIM(`total ${entries.length}`));
        for (const e of entries) this.outln(this.longRow(e.name, fsStat(this.o.sandboxId, e.path)!));
      } else {
        const names = entries.map((e) => (e.type === "dir" ? BLU(B(e.name + "/")) : e.name.startsWith(".") ? DIM(e.name) : e.name));
        // column-ish output, 4 per row, padded on visible width
        for (let i = 0; i < names.length; i += 4) {
          this.outln(names.slice(i, i + 4).map((n) => padVisible(n, 24)).join(""));
        }
      }
      if (targets.length > 1) this.outln("");
    }
  }

  private longRow(name: string, st: { type: "dir" | "file"; mode: string; owner: string; mtime: number; content?: string }): string {
    const size = st.type === "file" ? st.content?.length ?? 0 : 4096;
    const d = new Date(st.mtime);
    const dt = `${d.toLocaleString("en", { month: "short" })} ${String(d.getDate()).padStart(2, " ")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
    const label = st.type === "dir" ? BLU(B(name)) : name;
    return `${modeToRwx(st.mode, st.type === "dir")} 1 ${padEnd(st.owner, 8)}${padEnd(st.owner, 8)}${padEnd(fmtBytes(size), 6)} ${dt} ${label}`;
  }

  private cmdCd(p?: string): void {
    const target = !p || p === "~" ? "/home/sandbox" : resolvePath(this.cwd, p);
    const st = fsStat(this.o.sandboxId, target);
    if (!st) return this.err(`cd: ${p}: No such file or directory`);
    if (st.type !== "dir") return this.err(`cd: ${p}: Not a directory`);
    this.cwd = target;
  }

  private cmdCat(files: string[]): void {
    if (!files.length) return this.err("cat: missing operand");
    for (const f of files) {
      const p = resolvePath(this.cwd, f);
      const node = this.readFileSafe(p);
      if (!node) return this.err(`cat: ${f}: No such file or directory`);
      if (node.encoding === "base64") return this.err(`cat: ${f}: binary file (open it from the Files page)`);
      this.out(node.content ?? "");
      if (!(node.content ?? "").endsWith("\n")) this.out("\n");
    }
  }

  private cmdHeadTail(rest: string[], args: string[], kind: "head" | "tail"): void {
    const nArg = args.indexOf("-n");
    const n = nArg >= 0 ? Number(args[nArg + 1]) || 10 : 10;
    const files = rest.filter((r) => !(r === "-n" || r === String(args[args.indexOf("-n") + 1])));
    if (!files.length) return this.err(`${kind}: missing operand`);
    const node = this.readFileSafe(resolvePath(this.cwd, files[0]));
    if (!node) return this.err(`${kind}: ${files[0]}: No such file or directory`);
    const lines = (node.content ?? "").replace(/\n$/, "").split("\n");
    const slice = kind === "head" ? lines.slice(0, n) : lines.slice(-n);
    this.outln(slice.join("\n"));
  }

  private cmdTree(start?: string): void {
    const root = resolvePath(this.cwd, start ?? ".");
    const st = fsStat(this.o.sandboxId, root);
    if (!st) return this.err(`tree: ${start ?? root}: No such file or directory`);
    if (st.type !== "dir") return this.ok(root);
    const all = fsAllPaths(this.o.sandboxId, root).sort();
    this.outln(BLU(B(root)));
    let dirs = 0,
      filesN = 0;
    const draw = (dir: string, indent: string) => {
      const kids = all
        .filter((p) => {
          const rel = p.slice(dir.length + 1);
          return p.startsWith(dir + "/") && !rel.includes("/");
        })
        .sort();
      kids.forEach((k, i) => {
        const last = i === kids.length - 1;
        const st2 = fsStat(this.o.sandboxId, k)!;
        this.outln(`${indent}${last ? "└── " : "├── "}${st2.type === "dir" ? BLU(B(baseName(k) + "/")) : baseName(k)}`);
        if (st2.type === "dir") {
          dirs++;
          draw(k, indent + (last ? "    " : "│   "));
        } else filesN++;
      });
    };
    draw(root, "");
    this.outln(`\n${dirs} directories, ${filesN} files`);
  }

  private cmdFind(rest: string[], args: string[]): void {
    const start = resolvePath(this.cwd, rest[0] ?? ".");
    const nameIdx = args.indexOf("-name");
    const pat = nameIdx >= 0 ? (args[nameIdx + 1] ?? "").replace(/[*]/g, "") : "";
    const paths = fsAllPaths(this.o.sandboxId, start).filter((p) => (pat ? baseName(p).includes(pat) : true));
    if (!paths.length) return this.err(`find: \`${start}\`: No such file or directory`);
    for (const p of paths) this.outln(p);
  }

  private cmdGrep(rest: string[]): void {
    if (rest.length < 1) return this.err("grep: usage: grep <pattern> <file>");
    const pat = rest[0];
    const files = rest.slice(1);
    if (!files.length) return this.err("grep: missing file operand");
    let hits = 0;
    for (const f of files) {
      const node = this.readFileSafe(resolvePath(this.cwd, f));
      if (!node) return this.err(`grep: ${f}: No such file or directory`);
      (node.content ?? "").split("\n").forEach((l, i) => {
        if (l.toLowerCase().includes(pat.toLowerCase())) {
          hits++;
          const idx = l.toLowerCase().indexOf(pat.toLowerCase());
          const line =
            l.slice(0, idx) + YLW(l.slice(idx, idx + pat.length)) + l.slice(idx + pat.length);
          this.outln(`${DIM(`${f}:${i + 1}`)}: ${line}`);
        }
      });
    }
    if (!hits) this.outln(DIM("(no matches)"));
  }

  private cmdWc(rest: string[], flags: string): void {
    if (!rest.length) return this.err("wc: missing operand");
    const node = this.readFileSafe(resolvePath(this.cwd, rest[0]));
    if (!node) return this.err(`wc: ${rest[0]}: No such file or directory`);
    const content = node.content ?? "";
    const lines = content.split("\n").length - (content.endsWith("\n") ? 1 : 0);
    const words = content.split(/\s+/).filter(Boolean).length;
    if (flags.includes("l")) return this.ok(`${lines} ${rest[0]}`);
    this.ok(`${String(lines).padStart(6)}${String(words).padStart(7)}${String(content.length).padStart(8)} ${rest[0]}`);
  }

  private cmdDu(start?: string): void {
    const usage = fsUsage(this.o.sandboxId);
    this.outln(`${fmtBytes(usage.bytes)}\t${start ?? "."}`);
  }

  private cmdDf(): void {
    const used = fsUsage(this.o.sandboxId).bytes;
    const total = (this.o.diskGb ?? 50) * 1024 * 1024 * 1024;
    this.outln(`${B("Filesystem")}      ${B("Size")}  ${B("Used")}  ${B("Avail")}  ${B("Use%")}  ${B("Mounted on")}`);
    const pct = Math.max(1, Math.min(99, Math.round((used / total) * 100)));
    this.outln(`/dev/sbx      ${padEnd(fmtBytes(total), 6)}${padEnd(fmtBytes(used), 6)}${padEnd(fmtBytes(total - used), 6)}${padEnd(`${pct}%`, 5)}  /`);
  }

  private cmdFree(): void {
    const totalMb = this.o.memLimitMb ?? 8192;
    const usedMb = Math.round(totalMb * 0.34);
    const rows: Array<[string, string, string, string]> = [
      ["", "total", "used", "free"],
      ["Mem:", `${totalMb}Mi`, `${usedMb}Mi`, `${totalMb - usedMb}Mi`],
      ["Swap:", "8192Mi", "128Mi", "8064Mi"],
    ];
    for (const r of rows) this.outln(r.map((c, i) => (i === 0 ? padEnd(c, 8) : padEnd(c, 9))).join(""));
  }

  private cmdPs(top = false): void {
    const upS = Math.floor((Date.now() - this.started) / 1000) + 86400;
    const hh = String(Math.floor(upS / 3600)).padStart(2, "0");
    const mm = String(Math.floor((upS % 3600) / 60)).padStart(2, "0");
    if (top) {
      this.outln(`top - ${new Date().toLocaleTimeString()} up ${hh}:${mm},  1 user,  load average: 0.42, 0.31, 0.28`);
      this.outln(`Tasks:  12 total,   1 running`);
      this.outln(`%Cpu(s): ${String(12 + Math.floor(Math.random() * 20)).padStart(4)}.0 us`);
      this.outln("");
    }
    this.outln(`${B("  PID TTY      STAT   TIME COMMAND")}`);
    const rows = [
      [1, "?", "Ss", "0:02", "/sandboxd agent --id " + this.o.sandboxId],
      [11, "pts/0", "Ss", "0:00", "-bash"],
      [42, "pts/0", "S", "0:04", this.o.image.includes("python") ? "python3 -m pipeline.main" : this.o.image.includes("rust") ? "./target/release/parser-svc" : "node /home/sandbox/index.js"],
      [77, "pts/0", "R", "0:00", top ? "top" : "ps aux"],
    ] as Array<[number, string, string, string, string]>;
    for (const [pid, tty, stat, time, cmd] of rows) {
      this.outln(`${String(pid).padStart(5)} ${padEnd(tty, 8)} ${padEnd(stat, 6)} ${padEnd(time, 6)} ${cmd}`);
    }
  }

  private cmdEnv(): void {
    const env: Record<string, string> = {
      PATH: "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
      HOME: "/home/sandbox",
      USER: "sandbox",
      SHELL: "/bin/bash",
      TERM: "xterm-256color",
      LANG: "C.UTF-8",
      SANDBOX_ID: this.o.sandboxId,
      SANDBOX_IMAGE: this.o.image,
      SANDBOX_EGRESS: this.o.egressMode ?? "allowlist",
      MODEL_GATEWAY: "http://model-gateway.internal:8080",
    };
    for (const [k, v] of Object.entries(env)) this.outln(`${k}=${v}`);
  }

  private cmdUptime(): void {
    const upS = Math.floor((Date.now() - this.started) / 1000) + 86400 * 1;
    const hh = Math.floor(upS / 3600);
    const mm = Math.floor((upS % 3600) / 60);
    this.outln(
      ` ${new Date().toLocaleTimeString()}  up ${hh}:${String(mm).padStart(2, "0")},  1 user,  load average: 0.42, 0.31, 0.28`,
    );
  }

  private cmdCurl(url?: string): void {
    if (!url) return this.err("curl: missing URL");
    const host = url.replace(/^https?:\/\//, "").split("/")[0];
    const allowed = (mode: string | undefined) => mode === "open" || true; // demo: pretend the proxy answers
    if (!allowed(this.o.egressMode)) return this.err(`curl: (7) egress blocked for ${host}`);
    this.outln(DIM(`* connecting through egress-proxy.4aicoding.internal:3128 …`));
    this.outln(DIM(`* demo response for ${host}`));
    this.outln(`{"ok":true,"host":"${host}","via":"egress-proxy","ts":"${new Date().toISOString()}"}`);
  }

  private cmdGit(args: string[]): void {
    const sub = args[0];
    if (sub === "status") {
      this.outln(`On branch ${B("main")}`);
      this.outln(`Your branch is up to date with 'origin/main'.`);
      this.outln("");
      this.outln(`Changes not staged for commit:`);
      this.outln(`  ${RED("modified:   src/agent/orchestrator.ts")}`);
      this.outln("");
      this.outln(`Untracked files:`);
      this.outln(`  ${RED("logs/app.log")}`);
      this.outln("");
      this.outln(`no changes added to commit`);
      return;
    }
    if (sub === "log") {
      const commits = [
        ["a1f44e0", "feat(orchestrator): bounded worker pool", "minh.nguyen", 5],
        ["b7c93d1", "fix(config): read MODEL_GATEWAY from env", "linh.tran", 27],
        ["c5d28b6", "chore: bump fastify to 4.26", "admin", 51],
      ] as Array<[string, string, string, number]>;
      for (const [h, msg, who, hAgo] of commits) {
        this.outln(`${YLW("commit " + h)}`);
        this.outln(`Author: ${who} <${who}@4aicoding.dev>`);
        this.outln(DIM(`    ${hAgo} hours ago`));
        this.outln("");
        this.outln(`    ${msg}`);
        this.outln("");
      }
      return;
    }
    if (sub === "branch") {
      this.outln(`* ${GRN("main")}`);
      this.outln("  feat/tools");
      this.outln("  release/1.4");
      return;
    }
    this.err(`git: '${sub ?? ""}' is not supported in the demo shell (status · log · branch)`);
  }

  /** replay of the welcome banner, used on session start / restart */
  banner(): void {
    const line = DIM("─".repeat(56));
    this.outln(line);
    this.outln(`${B("sandboxd PTY")} ${DIM("·")} ${GRN("connected")}${DIM(" — demo session (no live node)")} `);
    this.outln(DIM(`sandbox ${this.o.sandboxId} · image ${this.o.image} · user sandbox`));
    this.outln(DIM(`type ${B("help")} for the demo command list — files shared with the ${B("Files")} tab`));
    this.outln(line);
    this.outln("");
    this.out(this.prompt());
  }
}
