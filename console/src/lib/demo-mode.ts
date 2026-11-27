/* ── Demo mode ────────────────────────────────────────────────────────
   The real console talks to sandboxd (Go) on the same origin. In this
   Next.js port there is no sandboxd behind the dev server, so api.ts
   falls back to this module whenever /v1/* answers 404/5xx or the fetch
   itself fails. The demo keeps the exact same response shapes, in
   memory, and mutates on writes so every screen stays fully usable. */

import type {
  ApiKey,
  Approval,
  AuditEntry,
  DomainInfo,
  Monitoring,
  NetActivity,
  NetMode,
  NodeChangeList,
  NodeInfo,
  NodeMetrics,
  Overview,
  Policy,
  Role,
  Sandbox,
  SnapshotInfo,
  User,
} from "@/console/api";
import {
  FsError,
  ensureFs,
  fsList,
  fsMkdir,
  fsMove,
  fsRead,
  fsRemove,
  fsTouch,
  fsWrite,
  normalizePath,
  resetFs,
} from "@/lib/demo-fs";
import { b64ToText } from "@/lib/files-parse";

/* #219 (D83): the view sends file paths as base64-of-raw-bytes. Demo paths
 * are in-browser strings (always valid UTF-8), so decode strictly here. */
const demoPath = (b64: string | null | undefined, fallback?: string): string => {
  if (b64) return b64ToText(b64);
  if (fallback !== undefined) return fallback;
  throw new DemoApiError(400, "bad_request", "path_b64 required");
};

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export class DemoApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

/* ── the in-memory instance ─────────────────────────────────────────── */

let idSeq = 100;
const nid = (p: string) => `${p}-${++idSeq}`;

const hoursAgo = (h: number) => new Date(Date.now() - h * 3600e3).toISOString();
const secsAgo = (s: number) => Math.floor((Date.now() - s * 1000) / 1000);

export const demoState = {
  authEnabled: true,
  authed: true,
  setupNeeded: false,
  me: { kind: "user", user: "admin", role: "admin" as Role },

  users: [
    { id: "usr-001", username: "admin", role: "admin" as Role, disabled: false, created_at: hoursAgo(2160) },
    { id: "usr-002", username: "linh.tran", role: "operator" as Role, disabled: false, created_at: hoursAgo(1440) },
    { id: "usr-003", username: "minh.nguyen", role: "operator" as Role, disabled: false, created_at: hoursAgo(720) },
    { id: "usr-004", username: "hoa.pham", role: "viewer" as Role, disabled: false, created_at: hoursAgo(336) },
    { id: "usr-005", username: "old-service", role: "viewer" as Role, disabled: true, created_at: hoursAgo(4000) },
  ] as User[],

  sandboxes: [
    mkSandbox({
      id: "sbx-a1f44e02",
      project: "agent-orchestrator",
      image: "4aicoding/base:node22",
      status: "running",
      cpus: 4,
      memory_mb: 8192,
      egress_mode: "allowlist",
      egress: ["github.com", "*.npmjs.org", "registry.npmmirror.com"],
      egress_deny: ["*.facebook.com"],
      ports: [3000, 9229],
      node: "local",
      createdH: 26,
    }),
    mkSandbox({
      id: "sbx-b7c93d10",
      project: "py-data-pipeline",
      image: "4aicoding/base:python3.12",
      status: "running",
      cpus: 8,
      memory_mb: 16384,
      egress_mode: "open",
      ports: [8888],
      node: "local",
      createdH: 9,
    }),
    mkSandbox({
      id: "sbx-c5d28b64",
      project: "rust-parser-svc",
      image: "4aicoding/base:rust1.83",
      status: "running",
      cpus: 4,
      memory_mb: 4096,
      egress_mode: "none",
      ports: [],
      node: "worker-01",
      createdH: 3,
    }),
    mkSandbox({
      id: "sbx-d9e71042",
      project: "docs-scraper",
      image: "4aicoding/base:python3.12",
      status: "stopped",
      cpus: 2,
      memory_mb: 2048,
      egress_mode: "allowlist",
      egress: ["docs.python.org", "readthedocs.io"],
      ports: [8000],
      node: "local",
      createdH: 52,
    }),
    mkSandbox({
      id: "sbx-e3a96f88",
      project: "legacy-worker",
      image: "4aicoding/base:node20",
      status: "error",
      cpus: 2,
      memory_mb: 2048,
      egress_mode: "allowlist",
      egress: [],
      ports: [],
      node: "worker-01",
      createdH: 78,
      error: "image pull back-off: registry unreachable",
    }),
  ] as Sandbox[],

  policy: {
    images: { allowed: ["4aicoding/base:node22", "4aicoding/base:python3.12", "4aicoding/base:rust1.83", "alpine:3.20"] },
    ports: { ranges: [{ min: 1024, max: 65535 }] },
    resources: { max_cpus: 16, max_memory_mb: 32768, max_ports_per_sandbox: 8 },
    egress: { mode: "strict" as const, allow: ["github.com", "*.npmjs.org", "pypi.org", "registry.npmmirror.com", "assets.4aicoding.dev"] },
    lifecycle: { allow_keep_alive: true, max_idle_timeout_secs: 86400 },
    approvals: ["sandbox.delete_keep_alive", "sandbox.egress_outside_policy"],
  } as Policy,

  approvals: [
    {
      id: "apr-4b2c91",
      action: "sandbox.egress_outside_policy",
      target: "sbx-b7c93d10",
      payload: JSON.stringify({ host: "huggingface.co" }),
      status: "pending",
      requested_by: "linh.tran",
      created_at: secsAgo(240),
      expires_at: secsAgo(-660),
    },
    {
      id: "apr-9f1e77",
      action: "sandbox.delete_keep_alive",
      target: "sbx-a1f44e02",
      payload: JSON.stringify({ keep_alive: true }),
      status: "approved",
      requested_by: "minh.nguyen",
      decided_by: "admin",
      reason: "confirmed with team lead",
      created_at: secsAgo(5400),
      expires_at: secsAgo(4500),
    },
  ] as Approval[],

  keys: [
    { id: "key-01", name: "ci-runner", prefix: "sbxk-9a2f", role: "operator" as Role, scopes: ["sandboxes:read", "sandboxes:write", "sandboxes:exec"], created_at: hoursAgo(600), last_used_at: hoursAgo(1) },
    { id: "key-02", name: "vscode-bridge", prefix: "sbxk-31c8", role: "operator" as Role, scopes: ["sandboxes:read", "sandboxes:exec"], created_at: hoursAgo(240), last_used_at: hoursAgo(4) },
    { id: "key-03", name: "readonly-audit", prefix: "sbxk-77b0", role: "viewer" as Role, scopes: ["health"], created_at: hoursAgo(96) },
  ] as (ApiKey & { key?: string })[],

  /* snapshots-as-templates library (GET /v1/snapshots) — seeded with two
     ready templates so the Templates page has content in demo mode */
  snapshots: [
    {
      id: "snap-01hzx7a2",
      name: "py-pipeline-starter",
      status: "ready",
      source_sandbox_id: "sbx-d9e71042",
      base_image: "4aicoding/base:python3.12",
      visibility: "private",
      size_bytes: 184 * 1024 * 1024,
      created_at: hoursAgo(50),
    },
    {
      id: "snap-01hzx8k9",
      name: "node22-ci-base",
      status: "ready",
      source_sandbox_id: "sbx-a1f44e02",
      base_image: "4aicoding/base:node22",
      visibility: "private",
      size_bytes: 342 * 1024 * 1024,
      created_at: hoursAgo(20),
    },
  ] as SnapshotInfo[],

  audit: [
    { id: 41, at: secsAgo(60), actor_kind: "user", actor_name: "admin", action: "policy.update", target: "instance", detail: "egress allowlist +pypi.org" },
    { id: 40, at: secsAgo(300), actor_kind: "user", actor_name: "linh.tran", action: "sandbox.create", target: "sbx-c5d28b64", detail: "4aicoding/base:rust1.83" },
    { id: 39, at: secsAgo(900), actor_kind: "user", actor_name: "admin", action: "sandbox.restart", target: "sbx-a1f44e02" },
    { id: 38, at: secsAgo(1500), actor_kind: "user", actor_name: "minh.nguyen", action: "auth.login" },
    { id: 37, at: secsAgo(2400), actor_kind: "system", action: "sandbox.error", target: "sbx-e3a96f88", detail: "image pull back-off" },
    { id: 36, at: secsAgo(3600), actor_kind: "user", actor_name: "admin", action: "user.create", target: "hoa.pham", detail: "role viewer" },
    { id: 35, at: secsAgo(5400), actor_kind: "user", actor_name: "admin", action: "approval.approve", target: "apr-9f1e77", detail: "confirmed with team lead" },
    { id: 34, at: secsAgo(7200), actor_kind: "user", actor_name: "linh.tran", action: "sandbox.stop", target: "sbx-d9e71042" },
    { id: 33, at: secsAgo(9000), actor_kind: "user", actor_name: "minh.nguyen", action: "apikey.create", target: "vscode-bridge" },
    { id: 32, at: secsAgo(10800), actor_kind: "system", action: "node.heartbeat", target: "worker-01", detail: "joined fleet" },
  ] as AuditEntry[],

  nodes: [
    mkNode({
      node_id: "local",
      role: "local",
      status: "online",
      address: "10.10.0.11:9090",
      version: "v0.4.0",
      cpus: 16,
      memory_mb: 65536,
      disk_mb: 512000,
      used_cpus: 8,
      used_memory_mb: 22528,
      sandboxes: 3,
      telemetry: true,
    }),
    mkNode({
      node_id: "worker-01",
      role: "worker",
      status: "online",
      address: "10.10.0.42:9443",
      version: "v0.4.0",
      cpus: 8,
      memory_mb: 32768,
      disk_mb: 256000,
      used_cpus: 6,
      used_memory_mb: 14336,
      sandboxes: 2,
      telemetry: true,
    }),
    mkNode({
      node_id: "worker-02",
      role: "worker",
      status: "offline",
      address: "10.10.0.43:9443",
      version: "v0.3.9",
      cpus: 8,
      memory_mb: 32768,
      disk_mb: 256000,
      used_cpus: 0,
      used_memory_mb: 0,
      sandboxes: 0,
      telemetry: false,
    }),
  ] as NodeInfo[],

  domain: {
    domain: "",
    tls_mode: "off",
    acme_email: "",
    running: { domain: "", tls_mode: "off" },
    restart_required: false,
    console_host: "",
    preview_example: "",
  } as DomainInfo,

  /** monitoring history ring buffers, per sandbox */
  hist: {} as Record<string, Array<{ t: string; cpu: number; mem_mb: number }>>,
  netSeq: 500,
};

function mkSandbox(o: {
  id: string;
  project: string;
  image: string;
  status: string;
  cpus: number;
  memory_mb: number;
  egress_mode: NetMode;
  egress?: string[];
  egress_deny?: string[];
  ports?: number[];
  node?: string;
  createdH: number;
  error?: string;
}): Sandbox {
  return {
    id: o.id,
    status: o.status,
    image: o.image,
    command: undefined,
    env: {},
    resources: { cpus: o.cpus, memory_mb: o.memory_mb },
    lifecycle: { idle_timeout_secs: 3600, keep_alive: o.id === "sbx-a1f44e02" },
    egress: o.egress ?? [],
    egress_mode: o.egress_mode,
    egress_deny: o.egress_deny ?? [],
    ports: o.ports ?? [],
    preview: o.ports?.length ? { url: `https://${o.id}.preview.4aicoding.dev`, port: o.ports[0], status: o.status === "running" ? "live" : "idle" } : undefined,
    visibility: "private",
    idle_policy: "suspend",
    node: o.node ?? "local",
    external: { user_id: "usr-001", project_id: o.project },
    error_message: o.error,
    last_active_at: hoursAgo(o.status === "running" ? 0.02 : o.createdH - 1),
    created_at: hoursAgo(o.createdH),
  };
}

function mkNode(o: {
  node_id: string;
  role: string;
  status: string;
  address?: string;
  version?: string;
  cpus: number;
  memory_mb: number;
  disk_mb: number;
  used_cpus: number;
  used_memory_mb: number;
  sandboxes: number;
  telemetry: boolean;
}): NodeInfo {
  return {
    node_id: o.node_id,
    role: o.role,
    status: o.status,
    drain_state: "active",
    address: o.address,
    version: o.version,
    capacity: { cpus: o.cpus, memory_mb: o.memory_mb, disk_mb: o.disk_mb },
    used: { cpus: o.used_cpus, memory_mb: o.used_memory_mb, disk_mb: 0, sandboxes: o.sandboxes },
    sandboxes: o.sandboxes,
    registered_at: hoursAgo(o.role === "local" ? 4320 : 720),
    heartbeat_at: o.status === "online" ? hoursAgo(0.005) : hoursAgo(6),
    telemetry: o.telemetry ? mkTelemetry(o.cpus, o.memory_mb, o.disk_mb) : undefined,
    change_file: o.role === "worker" ? `deploy/helm/sandboxd-worker/values-${o.node_id}.yaml` : undefined,
  };
}

function mkTelemetry(cpus: number, memMb: number, diskMb: number) {
  const memUsed = Math.round(memMb * (0.35 + Math.random() * 0.2));
  const diskUsed = Math.round(diskMb * 0.42);
  return {
    collected_at: new Date().toISOString(),
    hostname: `${cpus >= 16 ? "cp-" : "wk-"}${cpus}x`,
    os: "Ubuntu 24.04 LTS",
    arch: "x86_64",
    kernel: "6.8.0-45-generic",
    uptime_sec: 3600 * (24 + Math.round(Math.random() * 100)),
    in_container: false,
    cpu: { cores: cpus, usage_percent: 20 + Math.random() * 30, load1: 1.2 + Math.random(), load5: 1.0 + Math.random(), load15: 0.9 + Math.random() },
    memory: { total_mb: memMb, used_mb: memUsed, available_mb: memMb - memUsed, swap_total_mb: 8192, swap_used_mb: 128 },
    disk: { path: "/", total_mb: diskMb, used_mb: diskUsed, free_mb: diskMb - diskUsed },
    network: { interfaces: ["eth0", "lo"], rx_bytes: 4.2e9 + Math.random() * 1e8, tx_bytes: 2.8e9 + Math.random() * 1e8, rx_bps: 40_000 + Math.random() * 20_000, tx_bps: 28_000 + Math.random() * 16_000, host_namespace: true },
  };
}

/* deterministic-ish jitter for live metrics */
function jitter(base: number, amp: number, floor = 0): number {
  const v = base + (Math.random() - 0.5) * amp;
  return Math.max(floor, Math.round(v * 10) / 10);
}

function sbxStats(s: Sandbox) {
  if (s.status !== "running") return { cpu_percent: 0, mem_usage_bytes: 0, mem_limit_bytes: s.resources.memory_mb * 1024 * 1024, pids: 0 };
  const seed = s.id.charCodeAt(4) + s.id.charCodeAt(7);
  const cpu = jitter(18 + (seed % 22), 9, 1);
  const memPct = 0.28 + ((seed * 7) % 30) / 100 + Math.random() * 0.04;
  return {
    cpu_percent: cpu,
    mem_usage_bytes: Math.round(s.resources.memory_mb * 1024 * 1024 * memPct),
    mem_limit_bytes: s.resources.memory_mb * 1024 * 1024,
    pids: 18 + (seed % 40),
  };
}

function pushHist(id: string, cpu: number, memMb: number) {
  const arr = (demoState.hist[id] ??= []);
  arr.push({ t: new Date().toISOString(), cpu, mem_mb: memMb });
  if (arr.length > 300) arr.splice(0, arr.length - 300);
}

/* seed 24h of history so the Overview/Monitoring charts render immediately */
function seedHistory() {
  const now = Date.now();
  const stepMin = 5;
  for (const s of demoState.sandboxes) {
    if (s.status !== "running") continue;
    const seed = s.id.charCodeAt(4) + s.id.charCodeAt(7);
    const arr: Array<{ t: string; cpu: number; mem_mb: number }> = [];
    for (let i = 288; i >= 0; i--) {
      const wave = Math.sin((now / 3.6e6 + i) * 0.35 + seed) * 12;
      const cpu = Math.max(2, Math.round(22 + wave + (seed % 9)));
      const mem = Math.round(s.resources.memory_mb * (0.3 + 0.08 * Math.sin(i * 0.2 + seed)));
      arr.push({ t: new Date(now - i * stepMin * 60e3).toISOString(), cpu, mem_mb: mem });
    }
    demoState.hist[s.id] = arr;
  }
}
seedHistory();

/* ── monitoring / overview responses ────────────────────────────────── */

function fleetMon(range: "8m" | "1h" | "6h" | "24h" = "8m"): Monitoring {
  const running = demoState.sandboxes.filter((s) => s.status === "running");
  const stats = running.map((s) => ({ s, st: sbxStats(s) }));
  stats.forEach(({ s, st }) => pushHist(s.id, st.cpu_percent, st.mem_usage_bytes / 1048576));

  const memUsedMb = stats.reduce((a, x) => a + x.st.mem_usage_bytes / 1048576, 0);
  const memLimitMb = running.reduce((a, s) => a + s.resources.memory_mb, 0) || 1;

  // window: 8m → 1s step (live ring), longer ranges → coarse slices of the 24h seed
  const stepMin = range === "8m" ? 0.2 : range === "1h" ? 2 : range === "6h" ? 10 : 30;
  const spanMin = range === "8m" ? 8 : range === "1h" ? 60 : range === "6h" ? 360 : 1440;
  const now = Date.now();
  const history: Monitoring["history"] = [];
  const points = 40;
  for (let i = points; i >= 1; i--) {
    const t = new Date(now - (i * spanMin * 60e3) / points);
    const bucket = stepMin < 1 ? demoState.hist["sbx-a1f44e02"]?.at(-1) : undefined;
    const cpu = bucket && i === 1 ? bucket.cpu : jitter(24 + Math.sin(i * 0.4) * 10, 6, 2);
    const mem = bucket && i === 1 ? bucket.mem_mb : jitter(6000 + Math.sin(i * 0.25) * 1500, 900, 1200);
    history.push({ t: t.toISOString(), cpu, mem_mb: mem });
  }

  return {
    fleet: { total: demoState.sandboxes.length, running: running.length, avg_cpu: Math.round(stats.length ? stats.reduce((a, x) => a + x.st.cpu_percent, 0) / stats.length : 0), mem_used_mb: Math.round(memUsedMb), mem_limit_mb: Math.round(memLimitMb) },
    sandboxes: stats.map(({ s, st }) => ({ id: s.id, cpu_percent: st.cpu_percent, mem_usage_bytes: st.mem_usage_bytes, mem_limit_bytes: st.mem_limit_bytes, pids: st.pids })),
    history,
  };
}

function overview(): Overview {
  const sbx = demoState.sandboxes;
  const usr = demoState.users;
  const running = sbx.filter((s) => s.status === "running").length;
  const creating = sbx.filter((s) => s.status === "creating").length;
  const error = sbx.filter((s) => s.status === "error").length;
  const stopped = sbx.filter((s) => s.status === "stopped").length;
  const ports = sbx.reduce((a, s) => a + (s.status === "running" ? s.ports.length : 0), 0);
  return {
    sandboxes: { total: sbx.length, running, stopped, creating, error },
    users: {
      total: usr.length,
      admins: usr.filter((u) => u.role === "admin").length,
      operators: usr.filter((u) => u.role === "operator").length,
      viewers: usr.filter((u) => u.role === "viewer").length,
      disabled: usr.filter((u) => u.disabled).length,
    },
    ports_exposed: ports,
    approvals_open: demoState.approvals.filter((a) => a.status === "pending").length,
    net_events_24h: { total: 18432 + Math.floor(Math.random() * 90), blocked: 217 + Math.floor(Math.random() * 12) },
    recent: [...sbx].sort((a, b) => (a.created_at < b.created_at ? 1 : -1)),
    audit: demoState.audit.slice(0, 8).map((a) => ({
      at: new Date(a.at * 1000).toISOString(),
      actor: a.actor_name ?? a.actor_kind,
      action: a.action,
      target: a.target,
      detail: a.detail,
    })),
  };
}

function netActivity(id: string): NetActivity {
  const s = demoState.sandboxes.find((x) => x.id === id);
  const mode = s?.egress_mode ?? "allowlist";
  const hosts = ["github.com", "registry.npmjs.org", "pypi.org", "api.anthropic.com", "s3.amazonaws.com", "fonts.gstatic.com"];
  const events: NetActivity["events"] = [];
  const hostsAgg: NetActivity["hosts"] = [];
  let seq = demoState.netSeq;
  for (let i = 0; i < 24; i++) {
    const host = hosts[i % hosts.length];
    const allowed = mode === "open" ? !(host === "s3.amazonaws.com" && i % 5 === 0) : (s?.egress ?? []).some((e) => host === e || (e.startsWith("*.") && host.endsWith(e.slice(1))));
    events.push({
      seq: seq - i,
      time: new Date(Date.now() - i * 4500).toISOString(),
      method: "GET",
      host,
      port: "443",
      ip: `140.82.${(i % 60) + 100}.${(i * 7) % 250}`,
      allowed,
      reason: allowed ? undefined : "not on the allow list",
    });
  }
  for (let i = 0; i < hosts.length; i++) {
    const host = hosts[i];
    const e = events.filter((x) => x.host === host);
    hostsAgg.push({
      host,
      ips: [`140.82.${100 + i}.${20 + i}`],
      ports: ["443"],
      allowed: e.filter((x) => x.allowed).length,
      denied: e.filter((x) => !x.allowed).length,
      bytes_up: 4096 + i * 1024,
      bytes_down: 51200 + i * 4096,
      first_seen: new Date(Date.now() - 3600e3 - i * 600e3).toISOString(),
      last_seen: new Date(Date.now() - i * 4500).toISOString(),
      last_allowed: e[0]?.allowed ?? true,
      last_reason: e[0]?.reason,
    });
  }
  demoState.netSeq += 100;
  return {
    sandbox_id: id,
    recording: true,
    rules: { mode, allow: s?.egress ?? [], deny: s?.egress_deny ?? [] },
    cursor: `cur-${seq}`,
    events,
    hosts: hostsAgg,
  };
}

function nodeMetrics(id: string): NodeMetrics {
  const node = demoState.nodes.find((n) => n.node_id === id);
  if (!node) throw new DemoApiError(404, "not_found", `node ${id} not found`);
  const now = Date.now();
  const total = node.capacity;
  const history = Array.from({ length: 60 }, (_, i) => {
    const k = 60 - i;
    return {
      t: new Date(now - k * 60000).toISOString(),
      cpu_percent: jitter(30 + Math.sin(k * 0.3) * 14, 8, 1),
      load1: jitter(1.4, 0.8, 0.1),
      mem_used_mb: jitter(total.memory_mb * 0.42, total.memory_mb * 0.05, 512),
      mem_total_mb: total.memory_mb,
      disk_used_mb: Math.round(total.disk_mb * 0.42),
      disk_total_mb: total.disk_mb,
      rx_bps: jitter(45000, 18000, 100),
      tx_bps: jitter(32000, 14000, 100),
    };
  });
  return { node, history, step: "1m", window: "1h", now: new Date().toISOString() };
}

/* ── routing ────────────────────────────────────────────────────────── */

const ok = (data?: unknown, status = 200) => ({ status, data });

export function demoApi(method: Method, path: string, body?: unknown): { status: number; data?: unknown } {
  const url = new URL(path, "https://demo.local");
  const p = url.pathname.replace(/^\/v1/, "");
  const q = url.searchParams;
  const seg = p.split("/").filter(Boolean);

  /* --- auth --------------------------------------------------------- */
  if (p === "/auth/status") return ok({ enabled: demoState.authEnabled, authenticated: demoState.authed, setup_needed: demoState.setupNeeded });
  if (p === "/auth/me") {
    if (!demoState.authed) throw new DemoApiError(401, "unauthenticated", "not signed in");
    return ok(demoState.me);
  }
  if (p === "/auth/login" || p === "/auth/setup") {
    const b = body as { username?: string; password?: string };
    if (!b?.username || !b?.password) throw new DemoApiError(400, "bad_request", "username and password are required");
    if ((b.password ?? "").length < 8) throw new DemoApiError(400, "bad_request", "password must be at least 8 characters");
    demoState.authed = true;
    demoState.me = { kind: "user", user: b.username, role: "admin" };
    const u = demoState.users.find((x) => x.username === b.username);
    if (u) demoState.me.role = u.role;
    demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: "user", actor_name: b.username, action: "auth.login" });
    return ok(demoState.me);
  }
  if (p === "/auth/logout") {
    demoState.authed = false;
    return ok(undefined, 204);
  }

  /* --- version ------------------------------------------------------ */
  if (path === "/version") return ok({ version: "v0.4.0-demo", name: "sandboxd" });

  /* --- sandboxes ---------------------------------------------------- */
  if (seg[0] === "sandboxes") {
    const id = seg[1];
    if (!id) {
      if (method === "GET") return ok({ sandboxes: demoState.sandboxes });
      if (method === "POST") {
        const b = body as {
          image?: string;
          command?: string[];
          env?: Record<string, string>;
          resources?: { cpus?: number; memory_mb?: number };
          network?: { egress_mode?: NetMode; allow?: string[]; deny?: string[]; ports?: number[] };
          project_id?: string;
          lifecycle?: { idle_timeout_secs?: number; keep_alive?: boolean };
          from_snapshot?: string;
        };
        const srcSnap = b.from_snapshot ? demoState.snapshots.find((x) => x.id === b.from_snapshot) : undefined;
        if (b.from_snapshot && !srcSnap) throw new DemoApiError(404, "not_found", `no such snapshot ${b.from_snapshot}`);
        const sbx = mkSandbox({
          id: nid("sbx"),
          project: b.project_id || srcSnap?.name || b.image?.split("/").pop()?.replace(/[:.]/g, "-") || "sandbox",
          image: b.image || srcSnap?.base_image || "4aicoding/base:node22",
          status: "creating",
          cpus: b.resources?.cpus ?? 4,
          memory_mb: b.resources?.memory_mb ?? 8192,
          egress_mode: b.network?.egress_mode ?? "allowlist",
          egress: b.network?.allow ?? [],
          egress_deny: b.network?.deny ?? [],
          ports: b.network?.ports ?? [],
          createdH: 0,
        });
        if (srcSnap) sbx.external = { user_id: "usr-001", project_id: srcSnap.name };
        if (b.command) sbx.command = b.command;
        if (b.env) sbx.env = b.env;
        if (b.lifecycle) sbx.lifecycle = { ...sbx.lifecycle, ...b.lifecycle };
        demoState.sandboxes.unshift(sbx);
        demoState.audit.unshift({
          id: ++idSeq,
          at: secsAgo(0),
          actor_kind: demoState.me.kind,
          actor_name: demoState.me.user,
          action: "sandbox.create",
          target: sbx.id,
          detail: srcSnap ? `from snapshot ${srcSnap.name}` : sbx.image,
        });
        setTimeout(() => {
          sbx.status = "running";
        }, 3000);
        return ok(sbx, 201);
      }
    } else {
      const s = demoState.sandboxes.find((x) => x.id === id);
      if (!s) throw new DemoApiError(404, "not_found", `sandbox ${id} not found`);
      if (seg.length === 2) {
        if (method === "GET") return ok(s);
        if (method === "DELETE") {
          demoState.sandboxes = demoState.sandboxes.filter((x) => x.id !== id);
          demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "sandbox.delete", target: id });
          return ok({ deleted: true });
        }
      }
      if (seg[2] === "start" && method === "POST") {
        s.status = "creating";
        setTimeout(() => {
          s.status = "running";
          s.created_at = new Date().toISOString();
        }, 1200);
        return ok(s);
      }
      if (seg[2] === "stop" && method === "POST") {
        s.status = "stopped";
        return ok(s);
      }
      if (seg[2] === "restart" && method === "POST") {
        s.status = "creating";
        setTimeout(() => {
          s.status = "running";
          s.created_at = new Date().toISOString();
        }, 1200);
        return ok(s);
      }
      if (seg[2] === "keep-alive") {
        s.lifecycle.keep_alive = method === "POST";
        return ok(s.lifecycle, method === "DELETE" ? 204 : 200);
      }
      if (seg[2] === "lifecycle" && method === "PATCH") {
        const b = body as { idle_timeout_secs?: number };
        if (b.idle_timeout_secs !== undefined) {
          if (!Number.isFinite(b.idle_timeout_secs) || b.idle_timeout_secs < 0)
            throw new DemoApiError(400, "bad_request", "idle_timeout_secs must be >= 0");
          s.lifecycle.idle_timeout_secs = Math.round(b.idle_timeout_secs);
        }
        return ok(s.lifecycle);
      }
      if (seg[2] === "volumes" && method === "GET") {
        // same shape as v1_observe.go: workspace with a live used_bytes, plus tmpfs mounts
        const seed = s.id.charCodeAt(4) + s.id.charCodeAt(7);
        const used = 96 * 1024 * 1024 + (seed % 17) * 11 * 1024 * 1024;
        return ok({
          volumes: [
            { type: "workspace", destination: "/home/sandbox", used_bytes: used },
            { type: "tmpfs", destination: "/tmp" },
            { type: "tmpfs", destination: "/var/tmp" },
          ],
        });
      }
      if (seg[2] === "exec" && method === "POST") {
        const b = body as { cmd: string[] };
        const cmd = b.cmd?.join(" ") ?? "";
        if (/^ls/.test(cmd)) return ok({ stdout: "src\npackage.json\nnode_modules\nREADME.md\n", stderr: "", exit_code: 0 });
        if (/^echo/.test(cmd)) return ok({ stdout: `${cmd.slice(5)}\n`, stderr: "", exit_code: 0 });
        if (/^cat/.test(cmd)) return ok({ stdout: "// demo file contents — sandboxd would stream the real file\n", stderr: "", exit_code: 0 });
        return ok({ stdout: `demo console for ${id}: ran \`${cmd}\`\n`, stderr: "", exit_code: 0 });
      }
      if (seg[2] === "logs" && method === "GET") {
        // one-shot tail for the Logs panel (follow streams come via the SSE fake)
        const n = Number(q.get("tail") ?? 200);
        return ok(DEMO_LOG_LINES.slice(-Math.max(1, n)).join("\n"));
      }
      if (seg[2] === "files") {
        // workspace file manager — backed by the in-browser demo filesystem
        const project = s.external?.project_id || "workspace";
        ensureFs(id, project, s.image);
        const home = `/home/sandbox/${project}`;
        const fsAudit = (action: string, target: string, detail?: string) =>
          demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action, target, detail });
        const fsRoute = (e: unknown): never => {
          if (e instanceof FsError) {
            const status = e.code === "not_found" || e.code === "not_dir" || e.code === "is_dir" ? 404 : e.code === "exists" ? 409 : 400;
            throw new DemoApiError(status, e.code, e.message);
          }
          throw e;
        };
        try {
          /* list */
          if (seg.length === 3 && method === "GET") {
            const p = normalizePath(demoPath(q.get("path_b64"), q.get("path") || home));
            return ok({ path: p, home, entries: fsList(id, p) });
          }
          /* create file / directory */
          if (seg.length === 3 && method === "POST") {
            const b = body as { path_b64?: string; type?: "file" | "dir" };
            const p = demoPath(b?.path_b64);
            const entry = b.type === "dir" ? fsMkdir(id, p) : fsTouch(id, p);
            fsAudit(b.type === "dir" ? "fs.mkdir" : "fs.create", entry.path);
            return ok(entry, 201);
          }
          /* delete */
          if (seg.length === 3 && method === "DELETE") {
            const p = demoPath(q.get("path_b64"));
            fsRemove(id, p, true);
            fsAudit("fs.delete", p);
            return ok(undefined, 204);
          }
          /* read */
          if (seg[3] === "read" && method === "GET") {
            const p = normalizePath(demoPath(q.get("path_b64"), ""));
            const { node } = fsRead(id, p);
            const size = node.encoding === "base64" ? Math.round((node.content?.length ?? 0) * 0.75) : node.content?.length ?? 0;
            return ok({ path: p, content: node.content ?? "", encoding: node.encoding ?? "utf8", mode: node.mode, mtime: node.mtime, size });
          }
          /* write (save in the editor) */
          if (seg[3] === "content" && method === "PUT") {
            const b = body as { path_b64?: string; content?: string };
            const entry = fsWrite(id, demoPath(b?.path_b64), b.content ?? "");
            return ok({ ok: true, path: entry.path, size: entry.size });
          }
          /* rename / move */
          if (seg[3] === "move" && method === "POST") {
            const b = body as { from_b64?: string; to_b64?: string };
            const from = demoPath(b?.from_b64);
            const to = demoPath(b?.to_b64);
            const entry = fsMove(id, from, to, true);
            fsAudit("fs.move", `${from} → ${entry.path}`);
            return ok({ ok: true, path: entry.path });
          }
          /* upload (text or base64 payload) */
          if (seg[3] === "upload" && method === "POST") {
            const b = body as { path_b64?: string; name?: string; content?: string; encoding?: "utf8" | "base64" };
            const dir = demoPath(b?.path_b64);
            if (!b?.name) throw new DemoApiError(400, "bad_request", "name required");
            const target = normalizePath(`${dir}/${b.name}`);
            const entry = fsWrite(id, target, b.content ?? "", b.encoding ?? "utf8");
            fsAudit("fs.upload", entry.path);
            return ok(entry, 201);
          }
          /* reset workspace (demo) */
          if (seg[3] === "reset" && method === "POST") {
            resetFs(id);
            fsAudit("fs.reset", id, "workspace reseeded");
            return ok({ ok: true });
          }
        } catch (e) {
          fsRoute(e);
        }
      }
      if (seg[2] === "network") {
        if (seg[3] === "ports" && method === "GET") return ok({ ports: s.ports.map((n) => ({ container: n, preview_url: s.preview?.url ?? "" })) });
        if (seg[3] === "ports" && method === "POST") {
          const n = Number((body as { container: number }).container);
          if (!Number.isInteger(n) || n <= 0 || n >= 65536) throw new DemoApiError(400, "bad_request", "invalid port");
          if (!s.ports.includes(n)) s.ports.push(n);
          return ok({ ports: s.ports.map((x) => ({ container: x, preview_url: "" })) }, 201);
        }
        if (seg[3] === "ports" && seg[4] && method === "DELETE") {
          s.ports = s.ports.filter((x) => x !== Number(seg[4]));
          return ok(undefined, 204);
        }
        if (seg[3] === "egress") {
          if (method === "GET") return ok({ mode: s.egress_mode ?? "allowlist", allow: s.egress, deny: s.egress_deny ?? [], proxy: "egress-proxy.4aicoding.internal:3128" });
          if (method === "PUT") {
            const b = body as { mode: NetMode; allow?: string[]; deny?: string[] };
            s.egress_mode = b.mode;
            s.egress = b.allow ?? [];
            s.egress_deny = b.deny ?? [];
            return ok({ mode: s.egress_mode, allow: s.egress, deny: s.egress_deny, proxy: "egress-proxy.4aicoding.internal:3128" });
          }
        }
        if (seg[3] === "activity" && method === "GET") return ok(netActivity(id));
      }
    }
  }

  /* --- users -------------------------------------------------------- */
  if (p === "/users") {
    if (method === "GET") return ok({ users: demoState.users });
    if (method === "POST") {
      const b = body as { username?: string; password?: string; role?: Role };
      if (!b.username) throw new DemoApiError(400, "bad_request", "username required");
      if (demoState.users.some((u) => u.username === b.username)) throw new DemoApiError(409, "conflict", "username already exists");
      const u: User = { id: nid("usr"), username: b.username, role: b.role ?? "operator", disabled: false, created_at: new Date().toISOString() };
      demoState.users.push(u);
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "user.create", target: u.username, detail: `role ${u.role}` });
      return ok(u, 201);
    }
  }
  if (seg[0] === "users" && seg[1]) {
    const u = demoState.users.find((x) => x.id === seg[1]);
    if (!u) throw new DemoApiError(404, "not_found", "user not found");
    if (method === "PATCH") {
      const b = body as { role?: Role; disabled?: boolean; password?: string };
      if (b.role) u.role = b.role;
      if (b.disabled !== undefined) u.disabled = b.disabled;
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "user.update", target: u.username });
      return ok(u);
    }
    if (method === "DELETE") {
      demoState.users = demoState.users.filter((x) => x.id !== seg[1]);
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "user.delete", target: u.username });
      return ok(undefined, 204);
    }
  }

  /* --- policy / permissions ----------------------------------------- */
  if (p === "/policy") {
    if (method === "GET") return ok(demoState.policy);
    if (method === "PUT") {
      demoState.policy = body as Policy;
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "policy.update", target: "instance" });
      return ok(demoState.policy);
    }
  }
  if (p === "/policy/egress" && method === "PUT") {
    const e = body as Policy["egress"];
    demoState.policy.egress = e;
    return ok(e);
  }
  if (p === "/permissions" && method === "PUT") {
    const grants = (body as { grants: unknown[] }).grants ?? [];
    return ok({ ok: true, grants: grants.length });
  }

  /* --- snapshots-as-templates ---------------------------------------- */
  if (seg[0] === "snapshots") {
    const id = seg[1];
    if (!id) {
      if (method === "GET") return ok({ snapshots: demoState.snapshots });
      if (method === "POST") {
        // mirrors v1_snapshots.go: source must exist and be stopped (409 if running)
        const b = body as { source_sandbox_id?: string; name?: string };
        if (!b?.source_sandbox_id || !b?.name)
          throw new DemoApiError(400, "invalid_request", "source_sandbox_id and name are required");
        const src = demoState.sandboxes.find((x) => x.id === b.source_sandbox_id);
        if (!src) throw new DemoApiError(404, "not_found", "no such source sandbox");
        if (src.status === "running")
          throw new DemoApiError(409, "conflict", "source sandbox is running; stop it first (POST /v1/sandboxes/{id}/stop) then snapshot");
        const snap: SnapshotInfo = {
          id: nid("snap"),
          name: b.name.trim(),
          status: "ready",
          source_sandbox_id: src.id,
          base_image: src.image,
          visibility: "private",
          size_bytes: 90 * 1024 * 1024 + Math.floor(Math.random() * 260) * 1024 * 1024,
          created_at: new Date().toISOString(),
        };
        demoState.snapshots.unshift(snap);
        demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "snapshot.create", target: snap.id, detail: `${snap.name} ← ${src.id}` });
        return ok(snap, 201);
      }
    } else {
      const snap = demoState.snapshots.find((x) => x.id === id);
      if (!snap) throw new DemoApiError(404, "not_found", "no such snapshot");
      if (method === "GET") return ok(snap);
      if (method === "DELETE") {
        demoState.snapshots = demoState.snapshots.filter((x) => x.id !== id);
        demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "snapshot.delete", target: id });
        return ok(undefined, 204);
      }
    }
  }

  /* --- fleet capacity (GET /v1/capacity) ------------------------------ */
  if (p === "/capacity" && method === "GET") {
    const nodes = demoState.nodes;
    const cap = nodes.reduce(
      (a, n) => ({ cpus: a.cpus + n.capacity.cpus, memory_mb: a.memory_mb + n.capacity.memory_mb, disk_mb: a.disk_mb + n.capacity.disk_mb }),
      { cpus: 0, memory_mb: 0, disk_mb: 0 },
    );
    const running = demoState.sandboxes.filter((s) => s.status === "running");
    const used = {
      sandboxes: running.length,
      cpus: running.reduce((a, s) => a + (s.resources?.cpus ?? 0), 0),
      memory_mb: running.reduce((a, s) => a + (s.resources?.memory_mb ?? 0), 0),
      disk_mb: 0,
    };
    return ok({
      capacity: cap,
      used,
      nodes: nodes.length,
      online: nodes.filter((n) => n.status === "online").length,
      offline: nodes.filter((n) => n.status !== "online").length,
      draining: nodes.filter((n) => n.drain_state === "drain").length,
    });
  }

  /* --- monitoring / overview ---------------------------------------- */
  if (p === "/monitoring") {
    const r = (q.get("range") ?? "8m") as "8m" | "1h" | "6h" | "24h";
    return ok(fleetMon(r));
  }
  if (p === "/overview") return ok(overview());

  /* --- api keys ----------------------------------------------------- */
  if (p === "/api-keys") {
    if (method === "GET") return ok({ keys: demoState.keys });
    if (method === "POST") {
      const b = body as { name?: string; role?: Role; scopes?: string[]; expires_at?: string; sandbox_id?: string };
      if (!b.name) throw new DemoApiError(400, "bad_request", "name required");
      const secret = `sbxk-${Math.random().toString(36).slice(2, 10)}${Math.random().toString(36).slice(2, 14)}`;
      const k: ApiKey & { key?: string } = {
        id: nid("key"),
        name: b.name,
        prefix: secret.slice(0, 9),
        role: b.role ?? "operator",
        scopes: b.scopes ?? [],
        expires_at: b.expires_at,
        sandbox_id: b.sandbox_id,
        created_at: new Date().toISOString(),
        last_used_at: undefined,
      };
      demoState.keys.unshift(k);
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "apikey.create", target: b.name });
      return ok({ key: secret, ...k }, 201);
    }
  }
  if (seg[0] === "api-keys" && seg[1] && method === "DELETE") {
    demoState.keys = demoState.keys.filter((k) => k.id !== seg[1]);
    demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "apikey.revoke", target: seg[1] });
    return ok(undefined, 204);
  }

  /* --- approvals ---------------------------------------------------- */
  if (p === "/approvals" && method === "GET") {
    const st = q.get("status");
    return ok({ approvals: st ? demoState.approvals.filter((a) => a.status === st) : demoState.approvals });
  }
  if (seg[0] === "approvals" && seg[1] && (seg[2] === "approve" || seg[2] === "deny")) {
    const a = demoState.approvals.find((x) => x.id === seg[1]);
    if (!a) throw new DemoApiError(404, "not_found", "approval not found");
    a.status = seg[2] === "approve" ? "approved" : "denied";
    a.decided_by = demoState.me.user;
    a.reason = (body as { reason?: string })?.reason;
    demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: `approval.${seg[2]}`, target: a.id });
    return ok(a);
  }

  /* --- nodes -------------------------------------------------------- */
  if (seg[0] === "nodes") {
    const id = seg[1];
    if (!id && method === "GET") return ok({ nodes: demoState.nodes });
    const n = demoState.nodes.find((x) => x.node_id === id);
    if (!n) throw new DemoApiError(404, "not_found", `node ${id} not found`);
    if (seg[2] === "metrics" && method === "GET") return ok(nodeMetrics(id));
    if ((seg[2] === "drain" || seg[2] === "undrain") && method === "POST") {
      n.drain_state = seg[2] === "drain" ? "drain" : "active";
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: `node.${seg[2]}`, target: id });
      return ok(n);
    }
    if (seg[2] === "change-requests") {
      if (method === "GET") {
        const list: NodeChangeList = { node_id: id, repo: "kungfufanta/4AiCoding", base: "main", file: n.change_file ?? "deploy/helm/sandboxd-worker/values.yaml", can_open_pr: !!n.change_file, requests: [] };
        return ok(list);
      }
      if (method === "POST") {
        const b = (body ?? {}) as { cpus?: number; memory_mb?: number; disk_mb?: number; dry_run?: boolean };
        return ok({
          node_id: id,
          dry_run: b.dry_run ?? false,
          base: "main",
          file: n.change_file ?? "",
          current: n.capacity,
          proposed: { cpus: b.cpus ?? n.capacity.cpus, memory_mb: b.memory_mb ?? n.capacity.memory_mb, disk_mb: b.disk_mb ?? n.capacity.disk_mb },
          diff: [`- cpus: ${n.capacity.cpus}`, `+ cpus: ${b.cpus ?? n.capacity.cpus}`],
          can_open_pr: true,
        });
      }
    }
  }

  /* --- audit -------------------------------------------------------- */
  if (p === "/audit" && method === "GET") {
    let entries = demoState.audit;
    const action = q.get("action");
    const limit = Number(q.get("limit") ?? 100);
    if (action) entries = entries.filter((e) => e.action.includes(action));
    return ok({ entries: entries.slice(0, limit) });
  }
  if (p === "/audit/verify" && method === "GET") return ok({ ok: true, checked: demoState.audit.length, unchained: 0 });

  /* --- domain ------------------------------------------------------- */
  if (p === "/domain") {
    if (method === "GET") {
      demoState.domain.console_host = typeof location !== "undefined" ? location.hostname : "console.4aicoding.dev";
      demoState.domain.preview_example = `https://<sandbox-id>.preview.${demoState.domain.domain || demoState.domain.console_host}`;
      return ok(demoState.domain);
    }
    if (method === "PUT") {
      const b = body as { domain?: string; tls_mode?: string; acme_email?: string };
      demoState.domain = { ...demoState.domain, domain: b.domain ?? "", tls_mode: b.tls_mode ?? "off", acme_email: b.acme_email ?? "", running: { domain: b.domain ?? "", tls_mode: b.tls_mode ?? "off" }, restart_required: true };
      demoState.audit.unshift({ id: ++idSeq, at: secsAgo(0), actor_kind: demoState.me.kind, actor_name: demoState.me.user, action: "domain.update", target: b.domain });
      return ok(demoState.domain);
    }
  }
  if (p === "/domain/verify" && method === "POST") {
    const d = demoState.domain.domain || demoState.domain.console_host;
    return ok({ ok: true, hint: "Demo mode — DNS checks are simulated.", checks: [{ host: d, addrs: ["203.0.113.10"], ok: true }] });
  }
  if (p === "/domain/certificates") {
    if (method === "GET") return ok({ enabled: demoState.domain.tls_mode === "acme" || demoState.domain.tls_mode === "cert", certificates: demoState.domain.tls_mode === "cert" ? [{ id: "cert-1", subject: demoState.domain.domain, dns_names: [demoState.domain.domain], not_after: new Date(Date.now() + 90 * 86400e3).toISOString(), default: true }] : [] });
    if (method === "POST") return ok({ installed: true }, 201);
  }

  throw new DemoApiError(404, "not_found", `demo handler has no route for ${method} ${path}`);
}

/* ── SSE fakes (metrics + logs) ─────────────────────────────────────── */

const DEMO_LOG_LINES = [
  "sandboxd agent ready — pid 42",
  "GET /health 200 ok 1.2ms",
  "POST /v1/exec 202 accepted",
  "warn: disk usage 42% on /",
  "GET /api/models 200 ok 3.4ms",
  "ws client connected (vscode-bridge)",
  "error: upstream registry timeout, retrying (1/3)",
  "GET /health 200 ok 0.9ms",
  "exec session closed exit_code=0",
  "info: heartbeat sent to control plane",
];

class DemoEventSource {
  static factory(url: string) {
    return new DemoEventSource(url);
  }
  private timer?: ReturnType<typeof setInterval>;
  onmessage: ((e: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(url: string) {
    const u = new URL(url, "https://demo.local");
    const seg = u.pathname.split("/").filter(Boolean); // ["v1","sandboxes",<id>,"metrics"|"logs","stream"?]
    const kind = seg[3]; // "metrics" | "logs"
    const id = seg[2];
    if (kind === "metrics" && seg[4] === "stream") {
      this.timer = setInterval(() => {
        const s = demoState.sandboxes.find((x) => x.id === id);
        if (!s) return;
        const st = sbxStats(s);
        this.onmessage?.({ data: JSON.stringify({ status: s.status, stats: st }) });
      }, 2000);
    } else if (kind === "logs") {
      let i = Math.floor(Math.random() * DEMO_LOG_LINES.length);
      this.timer = setInterval(() => {
        this.onmessage?.({ data: DEMO_LOG_LINES[i++ % DEMO_LOG_LINES.length] });
      }, 900);
    } else {
      setTimeout(() => this.onerror?.(), 50);
    }
  }
  addEventListener(type: string, cb: () => void) {
    if (type === "end") /* demo streams never end */ void cb;
  }
  close() {
    if (this.timer) clearInterval(this.timer);
  }
}

/* ── install hooks ──────────────────────────────────────────────────── */

let installed = false;

/** Swap EventSource for the demo generator (called once demo mode is on). */
function installEventSource() {
  if (installed || typeof window === "undefined") return;
  installed = true;
  const RealES = window.EventSource;
  const Wrapped = function (url: string, cfg?: EventSourceInit) {
    if (url.includes("/v1/")) return DemoEventSource.factory(url) as unknown as EventSource;
    return new RealES(url, cfg);
  } as unknown as typeof EventSource;
  window.EventSource = Wrapped;
}

/** True when /v1/* is being served by the demo layer. */
export function isDemoActive() {
  return installed;
}

export function activateDemo() {
  installEventSource();
}
