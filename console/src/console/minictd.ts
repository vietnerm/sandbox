/* ────────────────────────────────────────────────────────────────────
   minictd adapter (GĐ3 mục 1–3).

   The console views were written against the sandboxd /v1 API (users,
   approvals, nodes, egress policy…). minictd is a single-user control
   plane (manifest §4: no RBAC, no cluster) with a smaller API
   (minictd/docs/API.md). This module answers the sandboxd-shaped calls
   the views make by translating them onto minictd endpoints; anything
   minictd does not have yields ApiError 501 so the view shows an honest
   "not supported" state instead of fake data.

   Auth: minictd takes `Authorization: Bearer <token>` only. The token
   lives in sessionStorage (cleared when the tab closes) and is sent as a
   header, never as a cookie — so there is no ambient credential for a
   cross-site request to ride on (CSRF), and the console is served from
   the same 127.0.0.1 origin as the API (connect-src 'self').
   ──────────────────────────────────────────────────────────────────── */

import type { Sandbox, SnapshotInfo, VolumeInfo } from "./api";

export class MinictdError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

const TOKEN_KEY = "minictd-token";

export function getToken(): string {
  try {
    return sessionStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}

function setToken(t: string) {
  try {
    if (t) sessionStorage.setItem(TOKEN_KEY, t);
    else sessionStorage.removeItem(TOKEN_KEY);
  } catch {
    /* storage blocked: token lives for this page only */
  }
  memToken = t;
}

let memToken = "";
const token = () => getToken() || memToken;

/** raw call to a minictd endpoint */
async function raw(method: string, path: string, body?: unknown): Promise<Response> {
  const h: Record<string, string> = {};
  const t = token();
  if (t) h.Authorization = `Bearer ${t}`;
  if (body !== undefined) h["Content-Type"] = "application/json";
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers: h,
      credentials: "omit",
      cache: "no-store",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new MinictdError(0, "unreachable", "Không kết nối được minictd · minictd unreachable");
  }
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (typeof j?.error === "string") msg = j.error;
    } catch {
      /* not JSON */
    }
    if (res.status === 401) msg = "Token không hợp lệ · invalid token";
    throw new MinictdError(res.status, res.status === 409 ? "conflict" : "error", msg);
  }
  return res;
}

async function json<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await raw(method, path, body);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/* ── minictd wire shapes (minictd/contract) ───────────────────────── */

interface ContainerJSON {
  Id: string;
  State: { Status: string; Running: boolean; Pid: number };
  Config: { Image: string; Labels: Record<string, string> | null; User: string; Cmd: string[] | null };
  Mounts: { Type: string; Source: string; Destination: string; RW: boolean }[] | null;
}

interface InstanceState {
  name: string;
  state: string;
  ports?: number[];
  created_at: string;
  started_at?: string;
  notes?: string[];
  exec_mode?: string;
  rootfs_src?: string;
}

export interface MinictdStats {
  cpu_percent: number;
  mem_usage_bytes: number;
  mem_limit_bytes: number;
  mem_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  block_read_bytes: number;
  block_write_bytes: number;
  pids: number;
}

interface Artifact {
  name: string;
  image: string;
  size_bytes: number;
  created_at: string;
}

const enc = encodeURIComponent;

/* ── per-instance view ─────────────────────────────────────────────── */

interface Snapshot {
  sbx: Sandbox;
  stats?: MinictdStats;
}

async function loadOne(name: string): Promise<Snapshot> {
  const [cj, st] = await Promise.all([
    json<ContainerJSON>("GET", `/v1/sandboxes/${enc(name)}`),
    json<InstanceState>("GET", `/v1/sandboxes/${enc(name)}/state`).catch(() => null),
  ]);
  const running = cj.State.Running;
  let stats: MinictdStats | undefined;
  if (running) stats = await json<MinictdStats>("GET", `/v1/sandboxes/${enc(name)}/stats`).catch(() => undefined);
  const labels = cj.Config.Labels ?? {};
  const ports = st?.ports ?? [];
  const sbx: Sandbox = {
    id: name,
    status: running ? "running" : "stopped",
    image: cj.Config.Image || "base",
    command: cj.Config.Cmd ?? undefined,
    // minictd's inspect does not echo the RunSpec limits back; 0 = unknown
    // (the detail page shows the live cgroup limit from /stats instead)
    resources: { cpus: 0, memory_mb: 0 },
    lifecycle: { idle_timeout_secs: 0, keep_alive: false },
    egress: [],
    egress_mode: "none",
    ports,
    preview: ports.length ? { url: `http://127.0.0.1:${ports[0]}/`, port: ports[0], status: running ? "ready" : "stopped" } : undefined,
    visibility: "private",
    idle_policy: "none",
    node: "local",
    external: labels["minictd.project"] ? { project_id: labels["minictd.project"] } : undefined,
    last_active_at: st?.started_at,
    created_at: (running && st?.started_at) || st?.created_at || new Date(0).toISOString(),
  };
  return { sbx, stats };
}

async function loadAll(): Promise<Snapshot[]> {
  const { names } = await json<{ names: string[] | null }>("GET", "/v1/sandboxes");
  const out = await Promise.all((names ?? []).map((n) => loadOne(n).catch(() => null)));
  return out.filter((x): x is Snapshot => x !== null);
}

/* fleet history for the Monitoring/Overview charts: kept in the browser
   only (roadmap GĐ3 mục 3 — no second collector in the daemon) */
const history: { t: string; cpu: number; mem_mb: number }[] = [];
const HISTORY_MAX = 240;

function fleetOf(all: Snapshot[]) {
  const run = all.filter((x) => x.stats);
  const cpu = run.reduce((a, x) => a + (x.stats?.cpu_percent ?? 0), 0);
  const used = run.reduce((a, x) => a + (x.stats?.mem_usage_bytes ?? 0), 0);
  const limit = run.reduce((a, x) => a + (x.stats?.mem_limit_bytes ?? 0), 0);
  return {
    total: all.length,
    running: all.filter((x) => x.sbx.status === "running").length,
    avg_cpu: run.length ? cpu / run.length : 0,
    mem_used_mb: Math.round(used / 1048576),
    mem_limit_mb: Math.round(limit / 1048576),
  };
}

/* ── create: sandboxd body → minictd RunSpec ───────────────────────── */

const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$/;

function newName(project?: string): string {
  const slug = (project ?? "")
    .toLowerCase()
    .replace(/[^a-z0-9_.-]+/g, "-")
    .replace(/^[^a-z0-9]+/, "")
    .slice(0, 48);
  const rnd = Array.from(crypto.getRandomValues(new Uint8Array(3)), (b) => b.toString(16).padStart(2, "0")).join("");
  return slug ? `${slug}-${rnd}` : `s-${rnd}`;
}

interface CreateBody {
  name?: string;
  image?: string;
  command?: string[];
  env?: Record<string, string>;
  resources?: { cpus?: number; memory_mb?: number };
  network?: { ports?: { container: number; host?: number }[] };
  external?: { project_id?: string };
  from_snapshot?: string;
  pids_limit?: number;
}

export function toRunSpec(b: CreateBody) {
  const name = b.name && NAME_RE.test(b.name) ? b.name : newName(b.external?.project_id);
  const ports = b.network?.ports ?? [];
  const spec: Record<string, unknown> = {
    name,
    hostname: name.slice(0, 63),
    // from_snapshot carries a minictd image ref: template:<n> | backup:<n>
    image: b.from_snapshot || b.image || "base",
    cmd: b.command?.length ? b.command : ["/bin/sh", "-c", "while true; do sleep 3600; done"],
    env: Object.entries(b.env ?? {}).map(([k, v]) => `${k}=${v}`),
    labels: b.external?.project_id ? [`minictd.project=${b.external.project_id}`] : [],
  };
  if (b.resources?.cpus) spec.cpus = String(b.resources.cpus);
  if (b.resources?.memory_mb) spec.memory = `${Math.round(b.resources.memory_mb)}m`;
  if (b.pids_limit) spec.pids_limit = b.pids_limit;
  if (ports.length) spec.pasta_ports = ports.map((p) => `${p.host ?? p.container}:${p.container}`);
  return spec;
}

/* ── templates + backups → the template library rows ──────────────── */

async function listArtifacts(): Promise<SnapshotInfo[]> {
  const [t, b] = await Promise.all([
    json<{ templates: Artifact[] }>("GET", "/v1/templates"),
    json<{ backups: Artifact[] }>("GET", "/v1/backups"),
  ]);
  const row = (a: Artifact, kind: "template" | "backup"): SnapshotInfo => ({
    id: a.image,
    name: a.name,
    status: "ready",
    base_image: kind,
    visibility: "private",
    size_bytes: a.size_bytes,
    created_at: a.created_at,
  });
  return [...(t.templates ?? []).map((a) => row(a, "template")), ...(b.backups ?? []).map((a) => row(a, "backup"))];
}

/* ── exec (web terminal + one-shot exec) ───────────────────────────── */

export interface ExecResult {
  exit_code: number;
  stdout: string;
  stderr: string;
  timed_out: boolean;
}

export async function execv2(name: string, cmd: string[], timeoutS = 60): Promise<ExecResult> {
  const r = await json<{ exit_code: number; stdout?: string; stderr?: string; timed_out?: boolean }>(
    "POST",
    `/v1/sandboxes/${enc(name)}/execv2`,
    { cmd, timeout_s: timeoutS, max_output_bytes: 1_000_000 },
  );
  return { exit_code: r.exit_code, stdout: r.stdout ?? "", stderr: r.stderr ?? "", timed_out: !!r.timed_out };
}

/* ── router ────────────────────────────────────────────────────────── */

const unsupported = (method: string, path: string): never => {
  throw new MinictdError(501, "unsupported", `minictd chưa hỗ trợ ${method} ${path.split("?")[0]}`);
};

export async function minictdApi<T>(method: string, fullPath: string, body?: unknown): Promise<T> {
  const [path, qs = ""] = fullPath.split("?");
  const q = new URLSearchParams(qs);
  const seg = path.split("/").filter(Boolean); // ["v1", ...]
  const r = <X>(x: X) => x as unknown as T;

  // ── auth (single owner, static token) ──
  if (path === "/v1/auth/status") {
    if (!token()) return r({ enabled: true, authenticated: false, setup_needed: false });
    try {
      await json("GET", "/v1/version");
      return r({ enabled: true, authenticated: true, setup_needed: false });
    } catch {
      return r({ enabled: true, authenticated: false, setup_needed: false });
    }
  }
  if (path === "/v1/auth/login" && method === "POST") {
    const t = String((body as { password?: string })?.password ?? "").trim();
    setToken(t);
    try {
      await json("GET", "/v1/version");
    } catch (e) {
      setToken("");
      throw e;
    }
    return r({ ok: true });
  }
  if (path === "/v1/auth/logout") {
    setToken("");
    return r(undefined);
  }
  if (path === "/v1/auth/me") return r({ kind: "user", user: "owner", role: "admin" });

  // ── sandboxes ──
  if (path === "/v1/sandboxes" && method === "GET") return r({ sandboxes: (await loadAll()).map((x) => x.sbx) });
  if (path === "/v1/sandboxes" && method === "POST") {
    const spec = toRunSpec((body ?? {}) as CreateBody);
    await json("POST", "/v1/run", spec);
    return r((await loadOne(spec.name as string)).sbx);
  }
  if (seg[0] === "v1" && seg[1] === "sandboxes" && seg[2]) {
    const id = decodeURIComponent(seg[2]);
    const sub = seg.slice(3).join("/");
    const base = `/v1/sandboxes/${enc(id)}`;
    if (!sub && method === "GET") return r((await loadOne(id)).sbx);
    if (!sub && method === "DELETE") return r(await json("POST", `${base}/remove`));
    if (sub === "start" && method === "POST") return r(await json("POST", `${base}/start`));
    if (sub === "stop" && method === "POST") return r(await json("POST", `${base}/stop`, { timeout_sec: 10 }));
    if (sub === "restart" && method === "POST") {
      await json("POST", `${base}/stop`, { timeout_sec: 10 });
      return r(await json("POST", `${base}/start`));
    }
    if (sub === "stats") return r(await json("GET", `${base}/stats`));
    if (sub === "state") return r(await json("GET", `${base}/state`));
    if (sub === "logs" && method === "GET") {
      const res = await raw("GET", `${base}/logs?tail=${enc(q.get("tail") ?? "200")}`);
      return r(await res.text());
    }
    if (sub === "exec" && method === "POST") {
      const cmd = (body as { cmd?: string[]; command?: string[] })?.cmd ?? (body as { command?: string[] })?.command ?? [];
      return r(await execv2(id, cmd));
    }
    if (sub === "volumes") {
      const cj = await json<ContainerJSON>("GET", base);
      const vols: VolumeInfo[] = [
        { type: "workspace", destination: "/", read_only: false },
        ...(cj.Mounts ?? []).map((m) => ({ type: m.Type, destination: m.Destination, read_only: !m.RW })),
      ];
      return r({ volumes: vols });
    }
    if (sub === "snapshot" && method === "POST") {
      const name = String((body as { name?: string })?.name ?? "");
      return r(await json("POST", `${base}/snapshot`, { backup: name }));
    }
    return unsupported(method, path);
  }

  // ── templates (save-template) + backups (snapshot) ──
  if (path === "/v1/snapshots" && method === "GET") return r({ snapshots: await listArtifacts() });
  if (path === "/v1/snapshots" && method === "POST") {
    const b = (body ?? {}) as { source_sandbox_id?: string; name?: string; kind?: "template" | "backup" };
    const src = enc(b.source_sandbox_id ?? "");
    const name = String(b.name ?? "").trim();
    if (b.kind === "backup") await json("POST", `/v1/sandboxes/${src}/snapshot`, { backup: name });
    else await json("POST", `/v1/sandboxes/${src}/save-template`, { template: name });
    const all = await listArtifacts();
    const want = `${b.kind === "backup" ? "backup" : "template"}:${name}`;
    return r(all.find((s) => s.id === want) ?? { id: want, name, status: "ready", base_image: b.kind ?? "template", visibility: "private", created_at: new Date().toISOString() });
  }
  if (seg[0] === "v1" && seg[1] === "snapshots" && seg[2] && method === "GET") {
    const id = decodeURIComponent(seg[2]);
    const hit = (await listArtifacts()).find((s) => s.id === id || s.name === id);
    if (!hit) throw new MinictdError(404, "not_found", "Không tìm thấy template");
    return r(hit);
  }

  // ── fleet views (computed from per-instance stats) ──
  if (path === "/v1/monitoring") {
    const all = await loadAll();
    const fleet = fleetOf(all);
    history.push({ t: new Date().toISOString(), cpu: fleet.avg_cpu, mem_mb: fleet.mem_used_mb });
    if (history.length > HISTORY_MAX) history.shift();
    return r({
      fleet,
      sandboxes: all
        .filter((x) => x.stats)
        .map((x) => ({
          id: x.sbx.id,
          cpu_percent: x.stats!.cpu_percent,
          mem_usage_bytes: x.stats!.mem_usage_bytes,
          mem_limit_bytes: x.stats!.mem_limit_bytes,
          pids: x.stats!.pids,
        })),
      history: [...history],
    });
  }
  if (path === "/v1/overview") {
    const all = await loadAll();
    const count = (s: string) => all.filter((x) => x.sbx.status === s).length;
    return r({
      sandboxes: { total: all.length, running: count("running"), stopped: count("stopped"), creating: 0, error: 0 },
      users: { total: 1, admins: 1, operators: 0, viewers: 0, disabled: 0 },
      ports_exposed: all.reduce((a, x) => a + x.sbx.ports.length, 0),
      approvals_open: 0,
      net_events_24h: { total: 0, blocked: 0 },
      recent: all.slice(0, 8).map((x) => x.sbx),
      audit: [],
    });
  }
  if (path === "/v1/version" || path === "/v1/info") return r(await json("GET", path));

  return unsupported(method, path);
}

/* ── streams ───────────────────────────────────────────────────────── */

/** follow a text stream (logs) with the Bearer header; one callback per line */
function followText(path: string, onData: (line: string) => void, onEnd?: () => void): () => void {
  const ctl = new AbortController();
  (async () => {
    try {
      const h: Record<string, string> = {};
      if (token()) h.Authorization = `Bearer ${token()}`;
      const res = await fetch(path, { headers: h, signal: ctl.signal, credentials: "omit", cache: "no-store" });
      if (!res.ok || !res.body) throw new Error(String(res.status));
      const rd = res.body.getReader();
      const dec = new TextDecoder();
      let buf = "";
      for (;;) {
        const { value, done } = await rd.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        let i: number;
        while ((i = buf.indexOf("\n")) >= 0) {
          onData(buf.slice(0, i));
          buf = buf.slice(i + 1);
        }
      }
      if (buf) onData(buf);
    } catch {
      /* aborted or stream closed */
    } finally {
      if (!ctl.signal.aborted) onEnd?.();
    }
  })();
  return () => ctl.abort();
}

/** stats poller that speaks the sandboxd metrics/stream frame shape */
function pollStats(id: string, intervalS: number, onData: (line: string) => void): () => void {
  let stop = false;
  const tick = async () => {
    if (stop) return;
    try {
      const st = await json<MinictdStats>("GET", `/v1/sandboxes/${enc(id)}/stats`);
      if (!stop) onData(JSON.stringify({ status: "running", stats: st }));
    } catch {
      /* transient */
    }
    if (!stop) timer = setTimeout(tick, intervalS * 1000);
  };
  let timer: ReturnType<typeof setTimeout> = setTimeout(tick, 0);
  return () => {
    stop = true;
    clearTimeout(timer);
  };
}

export function minictdSse(fullPath: string, onData: (line: string) => void, onEnd?: () => void): () => void {
  const [path, qs = ""] = fullPath.split("?");
  const q = new URLSearchParams(qs);
  const m = path.match(/^\/v1\/sandboxes\/([^/]+)\/(logs|metrics\/stream)$/);
  if (!m) {
    onEnd?.();
    return () => {};
  }
  const id = decodeURIComponent(m[1]);
  if (m[2] === "logs") {
    const tail = q.get("tail") ?? "100";
    const follow = q.get("follow") === "true" || q.get("follow") === "1";
    return followText(`/v1/sandboxes/${enc(id)}/logs?tail=${enc(tail)}${follow ? "&follow=1" : ""}`, onData, onEnd);
  }
  return pollStats(id, Math.max(1, Number(q.get("interval") ?? 2)), onData);
}
