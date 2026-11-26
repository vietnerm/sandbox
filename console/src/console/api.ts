// Client for the GĐ3 console. The views speak the sandboxd-shaped /v1
// API they were designed against; ./minictd.ts translates every call onto
// the minictd API (minictd/docs/API.md) served from the same 127.0.0.1
// origin, authenticated with the owner's Bearer token.
//
// Demo mode (fake in-browser data, src/lib/demo-mode.ts) is OFF by default
// and only turns on explicitly — `?demo=1` in the URL or a build with
// NEXT_PUBLIC_CONSOLE_DEMO=1 — so a real deployment never shows made-up
// sandboxes when the daemon is down.

export const CSRF_HEADER = "X-Sandboxd-CSRF";
export const APPROVAL_HEADER = "X-Sandboxd-Approval";

import { activateDemo, demoApi, DemoApiError } from "@/lib/demo-mode";
import { MinictdError, minictdApi, minictdSse } from "./minictd";

export type Role = "admin" | "operator" | "viewer";

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

/** A gated request was parked for an admin's approval (HTTP 202). */
export interface ApprovalPending {
  approval_id: string;
  status: "pending";
  action: string;
  target: string;
  message: string;
}

export function isApprovalPending(v: unknown): v is ApprovalPending {
  return typeof v === "object" && v !== null && "approval_id" in v && (v as ApprovalPending).status === "pending";
}

let demoFlag: boolean | null = null;

/** true only when demo data was asked for explicitly */
export function isDemo(): boolean {
  if (demoFlag === null) {
    let fromUrl = false;
    try {
      fromUrl = new URLSearchParams(window.location.search).get("demo") === "1";
      if (fromUrl) sessionStorage.setItem("console-demo", "1");
      else fromUrl = sessionStorage.getItem("console-demo") === "1";
    } catch {
      /* no window / storage */
    }
    demoFlag = fromUrl || process.env.NEXT_PUBLIC_CONSOLE_DEMO === "1";
    if (demoFlag) activateDemo();
  }
  return demoFlag;
}

function tryDemo<T>(method: string, path: string, body?: unknown): T {
  try {
    const r = demoApi(method as "GET", path, body);
    if (r.status === 204) return undefined as T;
    return r.data as T;
  } catch (e) {
    if (e instanceof DemoApiError) throw new ApiError(e.status, e.code, e.message);
    throw e;
  }
}

export async function api<T = unknown>(
  method: string,
  path: string,
  body?: unknown,
  _headers: Record<string, string> = {},
): Promise<T> {
  if (isDemo()) return tryDemo<T>(method, path, body);
  try {
    return await minictdApi<T>(method, path, body);
  } catch (e) {
    if (e instanceof MinictdError) throw new ApiError(e.status, e.code, e.message);
    throw e;
  }
}

export const get = <T>(p: string) => api<T>("GET", p);
export const post = <T>(p: string, b?: unknown, h?: Record<string, string>) => api<T>("POST", p, b, h);
export const put = <T>(p: string, b?: unknown, h?: Record<string, string>) => api<T>("PUT", p, b, h);
export const patch = <T>(p: string, b?: unknown) => api<T>("PATCH", p, b);
export const del = <T>(p: string, h?: Record<string, string>) => api<T>("DELETE", p, undefined, h);

// --- types (subset of docs/openapi.yaml) ---------------------------------

export interface Sandbox {
  id: string;
  status: "creating" | "running" | "stopped" | "error" | string;
  image: string;
  command?: string[];
  env?: Record<string, string>;
  resources: { cpus: number; memory_mb: number };
  pids_limit?: number; // GĐ3 mục 1 (issue #179): from HostConfig.PidsLimit
  state_notes?: string[]; // GĐ3 mục 1: daemon notes (quota warnings, cgroup fallback, etc.)
  lifecycle: { idle_timeout_secs: number; keep_alive: boolean };
  egress: string[];
  egress_mode?: NetMode;
  egress_deny?: string[];
  ports: number[];
  preview?: { url: string; port?: number; status: string };
  visibility: string;
  idle_policy: string;
  node?: string;
  external?: { user_id?: string; project_id?: string };
  error_message?: string;
  last_active_at?: string;
  created_at: string;
}

/** Per-sandbox network mode: nothing out, only the allow list, or everything but the block list. */
export type NetMode = "none" | "allowlist" | "open";

export interface EgressRules {
  mode: NetMode;
  allow: string[];
  deny: string[];
}

export interface NetEvent {
  seq: number;
  time: string;
  method: string;
  host: string;
  port: string;
  ip?: string;
  allowed: boolean;
  reason?: string;
}

export interface NetHost {
  host: string;
  ips: string[];
  ports: string[];
  allowed: number;
  denied: number;
  bytes_up: number;
  bytes_down: number;
  first_seen: string;
  last_seen: string;
  last_allowed: boolean;
  last_reason?: string;
}

export interface NetActivity {
  sandbox_id: string;
  recording: boolean;
  rules: EgressRules;
  cursor: string;
  events: NetEvent[];
  hosts: NetHost[];
}

/** project_id that marks the one-click default sandbox. */
export const DEFAULT_SANDBOX_TAG = "default";

export interface AuthStatus {
  enabled: boolean;
  authenticated: boolean;
  setup_needed: boolean;
}

export interface Me {
  kind: "user" | "service" | string;
  user: string;
  role: Role;
}

export interface Policy {
  images: { allowed: string[] };
  ports: { ranges: { min: number; max: number }[] };
  resources: { max_cpus: number; max_memory_mb: number; max_ports_per_sandbox: number };
  egress: { mode: "strict" | "permissive"; allow: string[] };
  lifecycle: { allow_keep_alive: boolean; max_idle_timeout_secs: number };
  approvals: string[];
}

export interface Approval {
  id: string;
  action: string;
  target: string;
  payload: string;
  status: string;
  requested_by: string;
  decided_by?: string;
  reason?: string;
  created_at: number;
  expires_at: number;
}

export interface User {
  id: string;
  username: string;
  role: Role;
  disabled: boolean;
  created_at: string;
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  role?: Role;
  scopes?: string[];
  expires_at?: string;
  sandbox_id?: string;
  created_at: string;
  last_used_at?: string;
}

/** Fleet membership (M6). role 'local' = the control plane itself. */
export interface NodeAmount {
  cpus: number;
  memory_mb: number;
  disk_mb: number;
}

export interface NodeUsage extends NodeAmount {
  sandboxes: number;
}

export interface NodeInfo {
  node_id: string;
  role: "local" | "worker" | string;
  status: "online" | "offline" | string;
  drain_state: "active" | "drain" | string;
  address?: string;
  version?: string;
  capacity: NodeAmount;
  used: NodeUsage;
  sandboxes: number;
  registered_at: string;
  heartbeat_at: string;
  /** Latest host sample the node sent with its heartbeat (absent = never reported). */
  telemetry?: HostTelemetry;
  /** Values file a capacity change request edits (absent = no GitOps mapping). */
  change_file?: string;
}

/** Host telemetry: the machine a node runs on (VPS, Docker host or k3s node). */
export interface HostTelemetry {
  collected_at: string;
  hostname?: string;
  os?: string;
  arch?: string;
  kernel?: string;
  uptime_sec?: number;
  in_container?: boolean;
  cpu: { cores: number; usage_percent: number; load1: number; load5: number; load15: number };
  memory: { total_mb: number; used_mb: number; available_mb: number; swap_total_mb?: number; swap_used_mb?: number };
  disk: { path?: string; total_mb: number; used_mb: number; free_mb: number };
  network: { interfaces?: string[]; rx_bytes: number; tx_bytes: number; rx_bps: number; tx_bps: number; host_namespace: boolean };
}

export interface NodeMetricPoint {
  t: string;
  cpu_percent: number;
  load1: number;
  mem_used_mb: number;
  mem_total_mb: number;
  disk_used_mb: number;
  disk_total_mb: number;
  rx_bps: number;
  tx_bps: number;
}

export interface NodeMetrics {
  node: NodeInfo;
  history: NodeMetricPoint[];
  step: string;
  window: string;
  now: string;
}

export interface NodeChangePR {
  number: number;
  url: string;
  title: string;
  state: "open" | "closed" | "merged" | string;
  branch: string;
  created_at?: string;
  merged_at?: string;
}

export interface NodeChange {
  node_id: string;
  dry_run: boolean;
  repo?: string;
  base: string;
  file: string;
  current: NodeAmount;
  proposed: NodeAmount;
  diff: string[];
  content?: string;
  can_open_pr: boolean;
  pr?: NodeChangePR;
}

export interface NodeChangeList {
  node_id: string;
  repo?: string;
  base?: string;
  file: string;
  can_open_pr: boolean;
  /** Capacity in the values file on the base branch. */
  current?: NodeAmount;
  error?: string;
  requests: NodeChangePR[];
}

export interface AuditEntry {
  id: number;
  at: number;
  actor_kind: string;
  actor_name?: string;
  action: string;
  target?: string;
  detail?: string;
}

export interface DomainInfo {
  domain: string;
  tls_mode: string;
  acme_email?: string;
  running: { domain: string; tls_mode: string };
  restart_required: boolean;
  console_host: string;
  preview_example: string;
}

/** Subscribes to a live stream (logs / metrics); returns a stop function. */
export function sse(path: string, onData: (line: string) => void, onEnd?: () => void): () => void {
  if (!isDemo()) return minictdSse(path, onData, onEnd);
  const es = new EventSource(path, { withCredentials: true });
  es.onmessage = (e) => onData(e.data);
  es.addEventListener("end", () => {
    es.close();
    onEnd?.();
  });
  es.onerror = () => {
    es.close();
    onEnd?.();
  };
  return () => es.close();
}

// --- template views (merged from the SBX Console template) -----------------

export interface Overview {
  sandboxes: { total: number; running: number; stopped: number; creating: number; error: number };
  users: { total: number; admins: number; operators: number; viewers: number; disabled: number };
  ports_exposed: number;
  approvals_open: number;
  net_events_24h: { total: number; blocked: number };
  recent: Sandbox[];
  audit: { at: string; actor: string; action: string; target?: string; detail?: string }[];
}

/** One user × sandbox cell of the R/W/X matrix. W/X imply R (API enforces). */
export interface PermGrant {
  user_id: string;
  sandbox_id: string;
  r: boolean;
  w: boolean;
  x: boolean;
}

export interface PermMatrix {
  users: { id: string; username: string; role: Role; disabled: boolean }[];
  sandboxes: { id: string; status: string; image: string }[];
  grants: PermGrant[];
}

export interface Monitoring {
  fleet: { total: number; running: number; avg_cpu: number; mem_used_mb: number; mem_limit_mb: number };
  sandboxes: { id: string; cpu_percent: number; mem_usage_bytes: number; mem_limit_bytes: number; pids: number }[];
  history: { t: string; cpu: number; mem_mb: number }[];
}

export const getOverview = () => get<Overview>("/v1/overview");
export const getPermissions = () => get<PermMatrix>("/v1/permissions");
export const putPermissions = (grants: PermGrant[]) =>
  put<{ ok: boolean; grants: number }>("/v1/permissions", { grants });
export type Range = "8m" | "1h" | "6h" | "24h";
export const getMonitoring = (range: Range = "8m") =>
  get<Monitoring>(range === "8m" ? "/v1/monitoring" : `/v1/monitoring?range=${range}`);

// --- sandbox workspace files (FTP manager + terminal) ----------------------

/** one row of a directory listing (workspace file browser) */
export interface FsEntry {
  name: string;
  path: string;
  type: "dir" | "file";
  size: number;
  mode: string;
  mtime: number;
  owner: string;
}

export interface FsListResp {
  path: string;
  /** the sandbox home the UI treats as "root" of the workspace */
  home: string;
  entries: FsEntry[];
}

export interface FsReadResp {
  path: string;
  content: string;
  encoding: "utf8" | "base64";
  mode: string;
  mtime: number;
  size: number;
}

const filesPath = (id: string, qs: Record<string, string>) =>
  `/v1/sandboxes/${id}/files?` + new URLSearchParams(qs).toString();

export const listFiles = (id: string, path: string) =>
  get<FsListResp>(filesPath(id, { path }));

export const readFile = (id: string, path: string) =>
  get<FsReadResp>(`/v1/sandboxes/${id}/files/read?` + new URLSearchParams({ path }).toString());

export const writeFile = (id: string, path: string, content: string) =>
  put<{ ok: boolean; path: string; size: number }>(`/v1/sandboxes/${id}/files/content`, { path, content });

export const createFsEntry = (id: string, path: string, type: "file" | "dir") =>
  post<FsEntry>(`/v1/sandboxes/${id}/files`, { path, type });

export const moveFsEntry = (id: string, from: string, to: string) =>
  post<{ ok: boolean; path: string }>(`/v1/sandboxes/${id}/files/move`, { from, to });

export const uploadFsFile = (id: string, dir: string, name: string, content: string, encoding: "utf8" | "base64" = "utf8") =>
  post<FsEntry>(`/v1/sandboxes/${id}/files/upload`, { path: dir, name, content, encoding });

export const deleteFsEntry = (id: string, path: string) =>
  del<void>(filesPath(id, { path }));

export const resetFs = (id: string) =>
  post<{ ok: boolean }>(`/v1/sandboxes/${id}/files/reset`, {});

// --- snapshots-as-templates (GET/POST /v1/snapshots, GET/DELETE /v1/snapshots/{id}) ---

/** one row of the template library (a frozen workspace copy) */
export interface SnapshotInfo {
  id: string;
  name: string;
  /** ready | capturing | error */
  status: string;
  source_sandbox_id?: string;
  base_image: string;
  /** private | public */
  visibility: string;
  size_bytes?: number;
  created_at: string;
}

export interface SnapshotsListResp {
  snapshots: SnapshotInfo[];
}

export const listSnapshots = () => get<SnapshotsListResp>("/v1/snapshots");

/** POST /v1/snapshots — the source sandbox must be stopped (409 otherwise) */
export const createSnapshot = (source_sandbox_id: string, name: string, kind: "template" | "backup" = "template") =>
  post<SnapshotInfo>("/v1/snapshots", { source_sandbox_id, name, kind });

export const getSnapshot = (id: string) => get<SnapshotInfo>(`/v1/snapshots/${id}`);

export const deleteSnapshot = (id: string) => del<void>(`/v1/snapshots/${id}`);

// --- per-sandbox volumes (GET /v1/sandboxes/{id}/volumes) --------------------

export interface VolumeInfo {
  /** workspace | volume | tmpfs | bind … */
  type: string;
  destination: string;
  used_bytes?: number;
  read_only?: boolean;
}

export const getVolumes = (id: string) =>
  get<{ volumes: VolumeInfo[] }>(`/v1/sandboxes/${id}/volumes`);

// --- fleet capacity (GET /v1/capacity) ---------------------------------------

export interface FleetCapacity {
  capacity: { cpus: number; memory_mb: number; disk_mb: number };
  used: { sandboxes: number; cpus: number; memory_mb: number; disk_mb: number };
  nodes: number;
  online: number;
  offline: number;
  draining: number;
}

export const getFleetCapacity = () => get<FleetCapacity>("/v1/capacity");

// --- per-sandbox lifecycle (PATCH /lifecycle, POST/DELETE /keep-alive) -------

export interface LifecycleInfo {
  idle_timeout_secs: number;
  keep_alive: boolean;
}

export const patchLifecycle = (id: string, p: { idle_timeout_secs?: number }) =>
  patch<LifecycleInfo>(`/v1/sandboxes/${id}/lifecycle`, p);

export const setKeepAlive = (id: string, on: boolean) =>
  on
    ? post<LifecycleInfo>(`/v1/sandboxes/${id}/keep-alive`)
    : del<void>(`/v1/sandboxes/${id}/keep-alive`);
