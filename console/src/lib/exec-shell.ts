/* Line-mode web terminal over minictd exec (GĐ3 mục 2: "web terminal vào
   instance qua exec của GĐ2").

   minictd has no PTY endpoint yet, so each line typed is run as one
   `POST /v1/sandboxes/{name}/execv2` (`/bin/sh -c`). The working directory
   is carried between commands by the shell itself: every command runs as
   `cd "$CWD"; <line>` and reports its final $PWD after a marker byte.
   Interactive programs (vi, top, a REPL) need a PTY and are out of scope;
   the banner says so. Same surface as DemoShell so the view can swap them. */

import { execv2 } from "@/console/minictd";

const MARK = "\u001e__MINICTD_PWD__";

const GRN = (s: string) => `\x1b[32m${s}\x1b[0m`;
const BLU = (s: string) => `\x1b[34m${s}\x1b[0m`;
const DIM = (s: string) => `\x1b[90m${s}\x1b[0m`;
const RED = (s: string) => `\x1b[31m${s}\x1b[0m`;

export interface ExecShellOpts {
  sandboxId: string;
  write: (s: string) => void;
  onExit?: () => void;
}

/** single-quote a string for /bin/sh */
const shq = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;

export class ExecShell {
  private o: ExecShellOpts;
  private cwd = "/";
  private line = "";
  private cursor = 0;
  private history: string[] = [];
  private histIdx = -1;
  private busy = false;
  private pendingEsc = "";
  private closed = false;

  constructor(opts: ExecShellOpts) {
    this.o = opts;
  }

  private out(s: string) {
    this.o.write(s.replace(/\r?\n/g, "\r\n"));
  }

  prompt(): string {
    return `${GRN("root")}@${BLU(this.o.sandboxId)}:${BLU(this.cwd)}# `;
  }

  banner(): void {
    this.out(DIM(`— minictd exec · mỗi dòng chạy bằng /bin/sh -c (không PTY: vi/top/REPL chưa hỗ trợ) —\n`));
    this.out(this.prompt());
  }

  private redraw() {
    this.o.write("\r\x1b[K" + this.prompt() + this.line);
    if (this.cursor < this.line.length) this.o.write(`\x1b[${this.line.length - this.cursor}D`);
  }

  handleData(data: string): void {
    if (this.closed) return;
    for (const ch of data) {
      if (this.busy) {
        if (ch === "\x03") this.out(DIM("^C (lệnh vẫn chạy tới timeout 60s)\n"));
        continue;
      }
      if (this.pendingEsc) {
        this.pendingEsc += ch;
        if (this.pendingEsc.length < 3) continue;
        this.handleEsc(this.pendingEsc);
        this.pendingEsc = "";
        continue;
      }
      if (ch === "\x1b") {
        this.pendingEsc = ch;
        continue;
      }
      if (ch === "\r" || ch === "\n") {
        this.o.write("\r\n");
        const cmd = this.line.trim();
        this.line = "";
        this.cursor = 0;
        this.histIdx = -1;
        if (!cmd) {
          this.o.write(this.prompt());
          continue;
        }
        if (this.history[this.history.length - 1] !== cmd) this.history.push(cmd);
        if (this.history.length > 200) this.history.shift();
        void this.run(cmd);
        continue;
      }
      if (ch === "\x7f" || ch === "\b") {
        if (this.cursor > 0) {
          this.line = this.line.slice(0, this.cursor - 1) + this.line.slice(this.cursor);
          this.cursor--;
          this.redraw();
        }
        continue;
      }
      if (ch === "\x03") {
        this.o.write("^C\r\n");
        this.line = "";
        this.cursor = 0;
        this.o.write(this.prompt());
        continue;
      }
      if (ch === "\x0c") {
        this.o.write("\x1b[2J\x1b[H");
        this.redraw();
        continue;
      }
      if (ch < " ") continue;
      this.line = this.line.slice(0, this.cursor) + ch + this.line.slice(this.cursor);
      this.cursor++;
      this.redraw();
    }
  }

  private handleEsc(seq: string) {
    if (seq === "\x1b[A" || seq === "\x1b[B") {
      if (!this.history.length) return;
      if (seq === "\x1b[A") this.histIdx = this.histIdx < 0 ? this.history.length - 1 : Math.max(0, this.histIdx - 1);
      else this.histIdx = this.histIdx < 0 ? -1 : this.histIdx + 1;
      if (this.histIdx >= this.history.length) this.histIdx = -1;
      this.line = this.histIdx < 0 ? "" : this.history[this.histIdx];
      this.cursor = this.line.length;
      this.redraw();
    } else if (seq === "\x1b[D" && this.cursor > 0) {
      this.cursor--;
      this.o.write(seq);
    } else if (seq === "\x1b[C" && this.cursor < this.line.length) {
      this.cursor++;
      this.o.write(seq);
    }
  }

  private async run(cmd: string) {
    if (cmd === "exit" || cmd === "logout") {
      this.closed = true;
      this.out(DIM("— session closed —\n"));
      this.o.onExit?.();
      return;
    }
    if (cmd === "clear") {
      this.o.write("\x1b[2J\x1b[H" + this.prompt());
      return;
    }
    this.busy = true;
    const script = `cd ${shq(this.cwd)} 2>/dev/null || cd /; ${cmd}\n__rc=$?; printf '${MARK}%s' "$PWD"; exit $__rc`;
    try {
      const r = await execv2(this.o.sandboxId, ["/bin/sh", "-c", script], 60);
      let stdout = r.stdout;
      const i = stdout.lastIndexOf(MARK);
      if (i >= 0) {
        const pwd = stdout.slice(i + MARK.length).trim();
        if (pwd.startsWith("/")) this.cwd = pwd;
        stdout = stdout.slice(0, i);
      }
      if (stdout) this.out(stdout.endsWith("\n") ? stdout : stdout + "\n");
      if (r.stderr) this.out(RED(r.stderr.endsWith("\n") ? r.stderr : r.stderr + "\n"));
      if (r.timed_out) this.out(RED("— timed out after 60s —\n"));
      else if (r.exit_code !== 0) this.out(DIM(`[exit ${r.exit_code}]\n`));
    } catch (e) {
      this.out(RED(`${e instanceof Error ? e.message : String(e)}\n`));
    } finally {
      this.busy = false;
      if (!this.closed) this.o.write(this.prompt());
    }
  }
}
