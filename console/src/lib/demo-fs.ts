/* ── Demo filesystem ──────────────────────────────────────────────────
   Per-sandbox in-browser filesystem used by the demo Files (FTP) manager
   and the demo shell inside the Terminal page. Both read/write the same
   store, so a file saved in the manager shows up in `cat` in the
   terminal immediately. Seeded per image flavour (node / python / rust)
   and persisted to localStorage so edits survive a reload. */

export interface FsNode {
  type: "dir" | "file";
  content?: string;
  /** "utf8" text or "base64" payload (uploads) */
  encoding?: "utf8" | "base64";
  mtime: number;
  mode: string; // "644" | "755"
  owner: string;
}

export class FsError extends Error {
  code: string;
  constructor(code: string, message: string) {
    super(message);
    this.code = code;
  }
}

/* ── path helpers ───────────────────────────────────────────────────── */

export function normalizePath(p: string): string {
  if (!p || p === ".") return "/";
  const abs = p.startsWith("/");
  const out: string[] = [];
  for (const seg of p.split("/")) {
    if (!seg || seg === ".") continue;
    if (seg === "..") {
      if (out.length) out.pop();
      continue;
    }
    out.push(seg);
  }
  return (abs ? "/" : "") + out.join("/") || "/";
}

export function resolvePath(cwd: string, p: string): string {
  if (!p || p === ".") return normalizePath(cwd);
  if (p.startsWith("~")) return normalizePath("/home/sandbox" + p.slice(1));
  return normalizePath(p.startsWith("/") ? p : `${cwd}/${p}`);
}

export function joinPath(dir: string, name: string): string {
  return normalizePath(`${dir}/${name}`);
}

export function dirName(p: string): string {
  const n = normalizePath(p);
  const i = n.lastIndexOf("/");
  return i <= 0 ? "/" : n.slice(0, i);
}

export function baseName(p: string): string {
  const n = normalizePath(p);
  return n === "/" ? "/" : n.slice(n.lastIndexOf("/") + 1);
}

/* ── store ──────────────────────────────────────────────────────────── */

const store = new Map<string, Record<string, FsNode>>();
const LS_PREFIX = "sandboxd-demo-fs:";

const now = () => Date.now();
const file = (content: string, opts: Partial<FsNode> = {}): FsNode => ({
  type: "file",
  content,
  encoding: "utf8",
  mtime: now(),
  mode: "644",
  owner: "sandbox",
  ...opts,
});
const dir = (opts: Partial<FsNode> = {}): FsNode => ({
  type: "dir",
  mtime: now(),
  mode: "755",
  owner: "sandbox",
  ...opts,
});

const KEY = (id: string) => LS_PREFIX + id;

export function fsUsage(sandboxId: string): { files: number; dirs: number; bytes: number } {
  const fs = getFs(sandboxId);
  let files = 0,
    dirs = 0,
    bytes = 0;
  for (const [p, n] of Object.entries(fs)) {
    if (n.type === "dir") dirs++;
    else {
      files++;
      bytes += n.encoding === "base64" ? Math.round((n.content?.length ?? 0) * 0.75) : n.content?.length ?? 0;
    }
    if (p === "/") dirs--;
  }
  return { files: Math.max(0, files), dirs: Math.max(0, dirs), bytes };
}

function persist(sandboxId: string, fs: Record<string, FsNode>) {
  try {
    localStorage.setItem(KEY(sandboxId), JSON.stringify(fs));
  } catch {
    /* quota / private mode — keep memory only */
  }
}

export function getFs(sandboxId: string): Record<string, FsNode> {
  let fs = store.get(sandboxId);
  if (fs) return fs;
  try {
    const raw = localStorage.getItem(KEY(sandboxId));
    if (raw) {
      fs = JSON.parse(raw) as Record<string, FsNode>;
      store.set(sandboxId, fs);
      return fs;
    }
  } catch {
    /* corrupted seed → reseed */
  }
  fs = seedFs(sandboxId);
  store.set(sandboxId, fs);
  persist(sandboxId, fs);
  return fs;
}

export function resetFs(sandboxId: string): void {
  store.delete(sandboxId);
  try {
    localStorage.removeItem(KEY(sandboxId));
  } catch {
    /* ignore */
  }
}

/* ── core operations (throw FsError, callers map to HTTP codes) ─────── */

export function fsStat(sandboxId: string, path: string): FsNode | null {
  const fs = getFs(sandboxId);
  return fs[normalizePath(path)] ?? null;
}

export interface FsEntry {
  name: string;
  path: string;
  type: "dir" | "file";
  size: number;
  mode: string;
  mtime: number;
  owner: string;
}

function toEntry(path: string, n: FsNode): FsEntry {
  return {
    name: baseName(path),
    path,
    type: n.type,
    size: n.type === "file" ? (n.encoding === "base64" ? Math.round((n.content?.length ?? 0) * 0.75) : n.content?.length ?? 0) : 0,
    mode: n.mode,
    mtime: n.mtime,
    owner: n.owner,
  };
}

export function fsList(sandboxId: string, path: string): FsEntry[] {
  const fs = getFs(sandboxId);
  const p = normalizePath(path);
  const node = fs[p];
  if (!node) throw new FsError("not_found", `no such file or directory: ${p}`);
  if (node.type !== "dir") throw new FsError("not_dir", `${p} is not a directory`);
  const prefix = p === "/" ? "/" : p + "/";
  const out: FsEntry[] = [];
  for (const [full, n] of Object.entries(fs)) {
    if (!full.startsWith(prefix)) continue;
    const rest = full.slice(prefix.length);
    if (!rest || rest.includes("/")) continue;
    out.push(toEntry(full, n));
  }
  out.sort((a, b) => (a.type === b.type ? a.name.localeCompare(b.name) : a.type === "dir" ? -1 : 1));
  return out;
}

export function fsRead(sandboxId: string, path: string): { node: FsNode; path: string } {
  const fs = getFs(sandboxId);
  const p = normalizePath(path);
  const node = fs[p];
  if (!node) throw new FsError("not_found", `no such file or directory: ${p}`);
  if (node.type !== "file") throw new FsError("is_dir", `${p} is a directory`);
  return { node, path: p };
}

/** true when the payload looks binary (NUL byte or heavy non-printables) */
export function looksBinary(s: string): boolean {
  if (s.includes("\0")) return true;
  const sample = s.slice(0, 8000);
  let bad = 0;
  for (let i = 0; i < sample.length; i++) {
    const c = sample.charCodeAt(i);
    if (c < 9 || (c > 13 && c < 32)) bad++;
  }
  return sample.length > 0 && bad / sample.length > 0.08;
}

export function fsWrite(sandboxId: string, path: string, content: string, encoding: "utf8" | "base64" = "utf8"): FsEntry {
  const fs = getFs(sandboxId);
  const p = normalizePath(path);
  if (p === "/") throw new FsError("bad_request", "cannot write /");
  const parent = fs[dirName(p)];
  if (!parent || parent.type !== "dir") throw new FsError("not_found", `parent directory missing: ${dirName(p)}`);
  if (encoding === "utf8" && looksBinary(content)) encoding = "base64";
  const prev = fs[p];
  fs[p] = {
    type: "file",
    content,
    encoding,
    mtime: now(),
    mode: prev?.mode ?? "644",
    owner: prev?.owner ?? "sandbox",
  };
  if (dirName(p) !== "/") fs[dirName(p)].mtime = now();
  persist(sandboxId, fs);
  return toEntry(p, fs[p]);
}

export function fsMkdir(sandboxId: string, path: string, recursive = false): FsEntry {
  const fs = getFs(sandboxId);
  const p = normalizePath(path);
  if (!p || p === "/") return toEntry("/", fs["/"]);
  if (fs[p]) throw new FsError("exists", `${p} already exists`);
  const parent = dirName(p);
  if (!fs[parent]) {
    if (!recursive) throw new FsError("not_found", `parent directory missing: ${parent}`);
    fsMkdir(sandboxId, parent, true);
  }
  fs[p] = dir();
  persist(sandboxId, fs);
  return toEntry(p, fs[p]);
}

/** create an empty file (or truncate if exists — like `touch` semantics) */
export function fsTouch(sandboxId: string, path: string): FsEntry {
  const fs = getFs(sandboxId);
  const p = normalizePath(path);
  if (fs[p]?.type === "dir") throw new FsError("is_dir", `${p} is a directory`);
  if (fs[p]) {
    fs[p].mtime = now();
    persist(sandboxId, fs);
    return toEntry(p, fs[p]);
  }
  return fsWrite(sandboxId, p, "");
}

export function fsRemove(sandboxId: string, path: string, recursive = false): void {
  const fs = getFs(sandboxId);
  const p = normalizePath(path);
  if (p === "/") throw new FsError("bad_request", "refusing to remove /");
  const node = fs[p];
  if (!node) throw new FsError("not_found", `no such file or directory: ${p}`);
  const prefix = p + "/";
  const children = Object.keys(fs).filter((k) => k.startsWith(prefix));
  if (children.length && !recursive) throw new FsError("dir_not_empty", `${p} is a non-empty directory (use recursive)`);
  delete fs[p];
  for (const k of children) delete fs[k];
  persist(sandboxId, fs);
}

export function fsMove(sandboxId: string, from: string, to: string, overwrite = false): FsEntry {
  const fs = getFs(sandboxId);
  const src = normalizePath(from);
  let dst = normalizePath(to);
  const node = fs[src];
  if (!node) throw new FsError("not_found", `no such file or directory: ${src}`);
  if (src === "/") throw new FsError("bad_request", "cannot move /");
  // mv dir name → into existing dir
  if (fs[dst]?.type === "dir") dst = joinPath(dst, baseName(src));
  if (fs[dst] && !overwrite) throw new FsError("exists", `${dst} already exists`);
  if (dst === src) return toEntry(src, node);
  if (dst.startsWith(src + "/")) throw new FsError("bad_request", "cannot move a directory into itself");
  const prefix = src + "/";
  const moving = Object.keys(fs).filter((k) => k === src || k.startsWith(prefix));
  for (const k of moving) {
    const nk = dst + k.slice(src.length);
    fs[nk] = fs[k];
    delete fs[k];
  }
  persist(sandboxId, fs);
  return toEntry(dst, fs[dst]);
}

export function fsCopy(sandboxId: string, from: string, to: string, recursive = false): FsEntry {
  const fs = getFs(sandboxId);
  const src = normalizePath(from);
  let dst = normalizePath(to);
  const node = fs[src];
  if (!node) throw new FsError("not_found", `no such file or directory: ${src}`);
  if (fs[dst]?.type === "dir") dst = joinPath(dst, baseName(src));
  if (node.type === "dir" && Object.keys(fs).some((k) => k.startsWith(src + "/")) && !recursive)
    throw new FsError("is_dir", `${src} is a directory (use -r)`);
  const prefix = src + "/";
  for (const k of Object.keys(fs)) {
    if (k !== src && !k.startsWith(prefix)) continue;
    fs[dst + k.slice(src.length)] = JSON.parse(JSON.stringify(fs[k])) as FsNode;
  }
  persist(sandboxId, fs);
  return toEntry(dst, fs[dst]);
}

/** every path in the tree (for `find`, completion…) */
export function fsAllPaths(sandboxId: string, under = "/"): string[] {
  const fs = getFs(sandboxId);
  const u = normalizePath(under);
  const prefix = u === "/" ? "/" : u + "/";
  return Object.keys(fs).filter((k) => k.startsWith(prefix) && k !== "/");
}

/* ── seeds ──────────────────────────────────────────────────────────── */

const README_NODE = `# agent-orchestrator

Multi-agent orchestration service for the 4AiCoding platform.

## Quick start
    npm install
    npm run dev

## Layout
- src/agent/orchestrator.ts — task planner + worker pool
- src/lib/config.ts — env-driven settings
- logs/ — rotating app logs

Endpoints are exposed on port 3000 (preview URL on the Network page).
`;

const PKG = `{
  "name": "agent-orchestrator",
  "version": "1.4.2",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "tsx watch src/index.ts",
    "build": "tsc -p tsconfig.json",
    "test": "vitest run",
    "lint": "eslint src"
  },
  "dependencies": {
    "fastify": "^4.26.0",
    "pino": "^9.1.0",
    "zod": "^3.23.8"
  },
  "devDependencies": {
    "tsx": "^4.16.0",
    "typescript": "^5.5.0",
    "vitest": "^2.0.0"
  }
}
`;

const INDEX_TS = `import { buildServer } from "./server.js";
import { logger } from "./lib/logger.js";
import { config } from "./lib/config.js";

const server = await buildServer();
server.listen({ port: config.port, host: "0.0.0.0" }, (err) => {
  if (err) {
    logger.error(err, "boot failed");
    process.exit(1);
  }
  logger.info({ port: config.port }, "orchestrator listening");
});
`;

const ORCHESTRATOR_TS = `import { z } from "zod";

export const Task = z.object({
  id: z.string(),
  kind: z.enum(["plan", "execute", "review"]),
  payload: z.record(z.unknown()),
  retries: z.number().int().min(0).default(0),
});

export type Task = z.infer<typeof Task>;

const MAX_WORKERS = Number(process.env.MAX_WORKERS ?? 8);

export class Orchestrator {
  private queue: Task[] = [];
  private active = 0;

  submit(task: Task): void {
    this.queue.push(task);
    this.drain();
  }

  private drain(): void {
    while (this.active < MAX_WORKERS && this.queue.length) {
      const task = this.queue.shift()!;
      this.active++;
      void this.run(task).finally(() => {
        this.active--;
        this.drain();
      });
    }
  }

  private async run(task: Task): Promise<void> {
    // demo implementation — real workers call the model gateway here
    console.log("running", task.id, task.kind);
  }
}
`;

const LOGGER_TS = `import pino from "pino";

export const logger = pino({
  level: process.env.LOG_LEVEL ?? "info",
  transport: process.env.NODE_ENV === "production" ? undefined : { target: "pino-pretty" },
});
`;

const CONFIG_TS = `export const config = {
  port: Number(process.env.PORT ?? 3000),
  gatewayUrl: process.env.MODEL_GATEWAY ?? "http://model-gateway.internal:8080",
  maxWorkers: Number(process.env.MAX_WORKERS ?? 8),
};
`;

const SERVER_TS = `import Fastify from "fastify";
import { Orchestrator, Task } from "./agent/orchestrator.js";

export async function buildServer() {
  const app = Fastify({ logger: false });
  const orch = new Orchestrator();

  app.get("/health", async () => ({ ok: true }));

  app.post<{ Body: Task }>("/tasks", async (req, reply) => {
    orch.submit(req.body);
    return reply.code(202).send({ accepted: true });
  });

  return app;
}
`;

const APP_LOG = `[2026-11-17T08:12:04Z] INFO  orchestrator listening on :3000
[2026-11-17T08:12:05Z] INFO  worker pool ready (8 workers)
[2026-11-17T08:12:22Z] INFO  task plan-0f31 accepted
[2026-11-17T08:12:23Z] WARN  gateway latency 820ms above budget
[2026-11-17T08:13:01Z] INFO  task plan-0f31 completed in 57.3s
[2026-11-17T08:13:44Z] ERROR upstream registry timeout, retrying (1/3)
[2026-11-17T08:14:02Z] INFO  retry ok — registry healthy
`;

const BASHRC = `# ~/.bashrc — sandbox image default
export PS1='\\u@\\h:\\w\\$ '
alias ll='ls -la'
alias gs='git status -sb'
export EDITOR=vi
export TERM=xterm-256color
`;

function seedNode(project: string): Record<string, FsNode> {
  const root = `/home/sandbox/${project}`;
  const fs: Record<string, FsNode> = {
    "/": dir({ mode: "755", owner: "root" }),
    "/bin": dir({ owner: "root" }),
    "/etc": dir({ owner: "root" }),
    "/etc/os-release": file('PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"\nID=debian\n', { owner: "root" }),
    "/etc/hostname": file("sbx.local\n", { owner: "root" }),
    "/tmp": dir({ mode: "777", owner: "root" }),
    "/var/log": dir({ owner: "root" }),
    "/usr/bin": dir({ owner: "root" }),
    "/home": dir({ owner: "root" }),
    "/home/sandbox": dir(),
    "/home/sandbox/.bashrc": file(BASHRC),
    "/home/sandbox/.gitconfig": file('[user]\n  name = sandbox\n  email = sandbox@4aicoding.dev\n[init]\n  defaultBranch = main\n'),
    [root]: dir(),
    [`${root}/README.md`]: file(README_NODE),
    [`${root}/package.json`]: file(PKG),
    [`${root}/tsconfig.json`]: file(`{\n  "compilerOptions": {\n    "target": "ES2022",\n    "module": "NodeNext",\n    "strict": true,\n    "outDir": "dist"\n  },\n  "include": ["src"]\n}\n`),
    [`${root}/.env.example`]: file("PORT=3000\nLOG_LEVEL=info\nMODEL_GATEWAY=http://model-gateway.internal:8080\nMAX_WORKERS=8\n"),
    [`${root}/.gitignore`]: file("node_modules/\ndist/\n.env\nlogs/\n"),
    [`${root}/src`]: dir(),
    [`${root}/src/index.ts`]: file(INDEX_TS),
    [`${root}/src/server.ts`]: file(SERVER_TS),
    [`${root}/src/agent`]: dir(),
    [`${root}/src/agent/orchestrator.ts`]: file(ORCHESTRATOR_TS),
    [`${root}/src/agent/tools.ts`]: file(`export const tools = [\n  { name: "shell", desc: "run a shell command in the sandbox" },\n  { name: "fs", desc: "read/write files in the workspace" },\n  { name: "http", desc: "egress through the proxy allowlist" },\n] as const;\n`),
    [`${root}/src/lib`]: dir(),
    [`${root}/src/lib/logger.ts`]: file(LOGGER_TS),
    [`${root}/src/lib/config.ts`]: file(CONFIG_TS),
    [`${root}/test`]: dir(),
    [`${root}/test/orchestrator.test.ts`]: file(`import { describe, it, expect } from "vitest";\nimport { Orchestrator } from "../src/agent/orchestrator.js";\n\ndescribe("Orchestrator", () => {\n  it("queues and drains tasks", () => {\n    const o = new Orchestrator();\n    o.submit({ id: "t1", kind: "plan", payload: {} });\n    expect(o).toBeTruthy();\n  });\n});\n`),
    [`${root}/scripts`]: dir(),
    [`${root}/scripts/deploy.sh`]: file("#!/bin/sh\nset -eu\nnpm run build\ndocker build -t 4aicoding/agent-orchestrator:latest .\necho \"deploy ok\"\n", { mode: "755" }),
    [`${root}/public`]: dir(),
    [`${root}/public/index.html`]: file(`<!doctype html>\n<html>\n  <head><meta charset="utf-8"><title>orchestrator</title></head>\n  <body>\n    <h1>agent-orchestrator</h1>\n    <p>POST /tasks to enqueue work.</p>\n  </body>\n</html>\n`),
    [`${root}/logs`]: dir(),
    [`${root}/logs/app.log`]: file(APP_LOG),
    [`${root}/node_modules`]: dir(),
    [`${root}/node_modules/.package-lock.json`]: file('{ "locked": true, "note": "trimmed demo tree" }\n'),
    [`${root}/data`]: dir(),
    [`${root}/data/queue.json`]: file('[\n  { "id": "plan-0f31", "kind": "plan", "status": "done" },\n  { "id": "exec-8a12", "kind": "execute", "status": "queued" }\n]\n'),
  };
  return fs;
}

const PY_MAIN = `"""Pipeline entrypoint — extract → transform → load."""
import logging

from pipeline.extract import fetch_sources
from pipeline.load import write_warehouse
from pipeline.transform import normalize

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("pipeline")


def run() -> None:
    log.info("fetching sources")
    frames = fetch_sources(["s3://lake/raw/events"])
    log.info("normalizing %d frames", len(frames))
    clean = normalize(frames)
    log.info("writing warehouse")
    write_warehouse(clean, table="events_clean")
    log.info("pipeline done")


if __name__ == "__main__":
    run()
`;

function seedPython(project: string): Record<string, FsNode> {
  const root = `/home/sandbox/${project}`;
  const fs: Record<string, FsNode> = {
    "/": dir({ mode: "755", owner: "root" }),
    "/etc": dir({ owner: "root" }),
    "/etc/os-release": file('PRETTY_NAME="Ubuntu 24.04 LTS"\nID=ubuntu\n', { owner: "root" }),
    "/etc/hostname": file("sbx.local\n", { owner: "root" }),
    "/tmp": dir({ mode: "777", owner: "root" }),
    "/var/log": dir({ owner: "root" }),
    "/usr/bin": dir({ owner: "root" }),
    "/home": dir({ owner: "root" }),
    "/home/sandbox": dir(),
    "/home/sandbox/.bashrc": file(BASHRC),
    [root]: dir(),
    [`${root}/README.md`]: file(`# ${project}\n\nNightly ETL pipeline. Runs on python:3.12-slim.\n\n    pip install -r requirements.txt\n    python -m pipeline.main\n\nData lands in data/processed as parquet.\n`),
    [`${root}/pyproject.toml`]: file(`[project]\nname = "${project}"\nversion = "0.9.0"\nrequires-python = ">=3.12"\ndependencies = ["pandas>=2.2", "pyarrow>=17", "requests>=2.32"]\n\n[tool.ruff]\nline-length = 110\n`),
    [`${root}/requirements.txt`]: file("pandas>=2.2\npyarrow>=17\nrequests>=2.32\npytest>=8\n"),
    [`${root}/.env.example`]: file("LAKE_ENDPOINT=s3://lake\nWAREHOUSE_DSN=postgres://wh.internal:5432/analytics\nLOG_LEVEL=INFO\n"),
    [`${root}/pipeline`]: dir(),
    [`${root}/pipeline/__init__.py`]: file('__version__ = "0.9.0"\n'),
    [`${root}/pipeline/main.py`]: file(PY_MAIN),
    [`${root}/pipeline/extract.py`]: file(`import requests\n\n\ndef fetch_sources(uris: list[str]) -> list[bytes]:\n    out = []\n    for uri in uris:\n        # demo: real impl signs the S3 request through the egress proxy\n        resp = requests.get(uri, timeout=30)\n        resp.raise_for_status()\n        out.append(resp.content)\n    return out\n`),
    [`${root}/pipeline/transform.py`]: file(`import pandas as pd\n\n\ndef normalize(frames: list[bytes]) -> pd.DataFrame:\n    df = pd.concat([pd.read_parquet(bytes(b)) for b in frames])\n    df = df.drop_duplicates(subset=["event_id"])\n    df["ts"] = pd.to_datetime(df["ts"], utc=True)\n    return df\n`),
    [`${root}/pipeline/load.py`]: file(`import pandas as pd\n\n\ndef write_warehouse(df: pd.DataFrame, table: str) -> None:\n    df.to_parquet(f"data/processed/{table}.parquet", index=False)\n    print(f"wrote {len(df)} rows to {table}")\n`),
    [`${root}/tests`]: dir(),
    [`${root}/tests/test_transform.py`]: file(`from pipeline.transform import normalize\n\n\ndef test_normalize_dedups():\n    assert callable(normalize)\n`),
    [`${root}/data`]: dir(),
    [`${root}/data/raw`]: dir(),
    [`${root}/data/raw/sample.csv`]: file("event_id,ts,kind,value\nevt-001,2026-11-16T10:00:00Z,click,1.0\nevt-002,2026-11-16T10:00:05Z,view,0.5\nevt-003,2026-11-16T10:01:12Z,click,1.0\n"),
    [`${root}/data/processed`]: dir(),
    [`${root}/data/processed/.gitkeep`]: file(""),
    [`${root}/notebooks`]: dir(),
    [`${root}/notebooks/analysis.ipynb`]: file('{"cells": [], "metadata": {}, "nbformat": 4, "nbformat_minor": 5}\n'),
    [`${root}/logs`]: dir(),
    [`${root}/logs/pipeline.log`]: file("2026-11-17 02:00:01 INFO fetching sources\n2026-11-17 02:00:19 INFO normalizing 3 frames\n2026-11-17 02:00:31 INFO wrote 3 rows to events_clean\n2026-11-17 02:00:31 INFO pipeline done\n"),
  };
  return fs;
}

function seedRust(project: string): Record<string, FsNode> {
  const root = `/home/sandbox/${project}`;
  const fs: Record<string, FsNode> = {
    "/": dir({ mode: "755", owner: "root" }),
    "/etc": dir({ owner: "root" }),
    "/etc/os-release": file('PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"\nID=debian\n', { owner: "root" }),
    "/tmp": dir({ mode: "777", owner: "root" }),
    "/var/log": dir({ owner: "root" }),
    "/usr/bin": dir({ owner: "root" }),
    "/home": dir({ owner: "root" }),
    "/home/sandbox": dir(),
    "/home/sandbox/.bashrc": file(BASHRC),
    [root]: dir(),
    [`${root}/README.md`]: file(`# ${project}\n\nEvent-log parser service (axum). Parse, validate and route NDJSON events.\n\n    cargo run --release\n`),
    [`${root}/Cargo.toml`]: file(`[package]\nname = "parser-svc"\nversion = "0.3.1"\nedition = "2021"\n\n[dependencies]\naxum = "0.7"\nserde = { version = "1", features = ["derive"] }\nserde_json = "1"\ntokio = { version = "1", features = ["full"] }\ntracing = "0.1"\n`),
    [`${root}/src`]: dir(),
    [`${root}/src/main.rs`]: file(`use axum::{routing::get, Router};\n\nmod parser;\n\n#[tokio::main]\nasync fn main() {\n    tracing_subscriber::fmt().init();\n    let app = Router::new().route("/health", get(|| async { "ok" }));\n    let listener = tokio::net::TcpListener::bind("0.0.0.0:8080").await.unwrap();\n    axum::serve(listener, app).await.unwrap();\n}\n`),
    [`${root}/src/parser.rs`]: file(`use serde::Deserialize;\n\n#[derive(Debug, Deserialize)]\npub struct Event {\n    pub id: String,\n    pub kind: String,\n    pub ts: i64,\n}\n\npub fn parse_line(line: &str) -> Result<Event, serde_json::Error> {\n    serde_json::from_str(line)\n}\n`),
    [`${root}/tests`]: dir(),
    [`${root}/tests/parser_test.rs`]: file(`use parser_svc::parser::parse_line;\n\n#[test]\nfn parses_minimal_event() {\n    let e = parse_line(r#"{"id":"e1","kind":"click","ts":1}"#).unwrap();\n    assert_eq!(e.id, "e1");\n}\n`),
    [`${root}/target`]: dir(),
    [`${root}/target/.keep`]: file(""),
    [`${root}/logs`]: dir(),
    [`${root}/logs/svc.log`]: file("2026-11-17T06:40:11Z INFO parser-svc listening on :8080\n2026-11-17T06:41:02Z INFO batch=128 parsed ok\n"),
  };
  return fs;
}

function seedGeneric(project: string): Record<string, FsNode> {
  const root = `/home/sandbox/${project}`;
  const fs: Record<string, FsNode> = {
    "/": dir({ mode: "755", owner: "root" }),
    "/etc": dir({ owner: "root" }),
    "/etc/os-release": file('PRETTY_NAME="Alpine Linux 3.20"\nID=alpine\n', { owner: "root" }),
    "/tmp": dir({ mode: "777", owner: "root" }),
    "/var/log": dir({ owner: "root" }),
    "/home": dir({ owner: "root" }),
    "/home/sandbox": dir(),
    "/home/sandbox/.bashrc": file(BASHRC),
    [root]: dir(),
    [`${root}/README.md`]: file(`# ${project}\n\nWorkspace sandbox. Drop your project files here.\n`),
    [`${root}/data`]: dir(),
    [`${root}/logs`]: dir(),
    [`${root}/logs/boot.log`]: file("sandbox agent ready\n"),
  };
  return fs;
}

/** image → flavour */
function flavour(image: string): "node" | "python" | "rust" | "generic" {
  if (image.includes("node")) return "node";
  if (image.includes("python")) return "python";
  if (image.includes("rust")) return "rust";
  return "generic";
}

/** project dir name from the sandbox's external.project_id when known */
export function seedFs(sandboxId: string, projectName = "workspace", image = "4aicoding/base:node22"): Record<string, FsNode> {
  const f = flavour(image);
  const project = projectName || "workspace";
  switch (f) {
    case "node":
      return seedNode(project);
    case "python":
      return seedPython(project);
    case "rust":
      return seedRust(project);
    default:
      return seedGeneric(project);
  }
}

/** (re)seed with an explicit flavour — used by demo-mode when creating a sandbox */
export function ensureFs(sandboxId: string, projectName: string, image: string): Record<string, FsNode> {
  const existing = store.get(sandboxId);
  if (existing) return existing;
  try {
    const raw = localStorage.getItem(KEY(sandboxId));
    if (raw) {
      const fs = JSON.parse(raw) as Record<string, FsNode>;
      store.set(sandboxId, fs);
      return fs;
    }
  } catch {
    /* reseed below */
  }
  const fs = seedFs(sandboxId, projectName, image);
  store.set(sandboxId, fs);
  persist(sandboxId, fs);
  return fs;
}
