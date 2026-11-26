"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  CheckCircle2,
  ExternalLink,
  FileKey2,
  GitCompareArrows,
  Globe,
  Layers,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Save,
  Server,
  ShieldCheck,
  ShieldX,
  Waypoints,
  XCircle,
} from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { toast } from "sonner";
import {
  APPROVAL_HEADER,
  ApiError,
  del as apiDel,
  get as apiGet,
  post as apiPost,
  put as apiPut,
  type DomainInfo,
  type EgressRules,
  type NetActivity,
  type NetHost,
  type NetMode,
  type Policy,
} from "@/console/api";
import { usePlatform } from "@/lib/platform-store";
import { cn } from "@/lib/utils";
import { CheckBox, FancySelect, FieldRow, ModeCard, TableShell, Td, TextArea, TextField, Th, ViewTabs } from "../controls";
import { KpiCard, Mono, PageHeader, Panel, SectionTitle } from "../primitives";

/* ══ Network — the platform network hub ═══════════════════════════════
   Five sub-sections live here as IN-PAGE tabs (client state + a sliding
   underline). Nothing changes the URL, so switching is instant and
   smooth — unlike the old console whose tabs were routes.
     · Overview        — fleet egress posture, proxy, DNS check
     · Egress policy   — instance-wide policy (strict / permissive)
     · Sandbox traffic — per-sandbox rules + live request monitor
     · Ports           — exposed preview ports across the fleet
     · Domain & TLS    — console domain, DNS, certificates            */

type TabId = "overview" | "policy" | "traffic" | "ports" | "domain";

/* ── host matching, mirroring internal/policy.HostAllowed ───────────── */

function ipv4(s: string): number | null {
  const m = s.match(/^(\d+)\.(\d+)\.(\d+)\.(\d+)$/);
  if (!m) return null;
  const b = m.slice(1).map(Number);
  if (b.some((x) => x > 255)) return null;
  return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0;
}

function entryMatches(entry: string, host: string): boolean {
  if (entry.includes("/")) {
    const [net, bits] = entry.split("/");
    const a = ipv4(host);
    const n = ipv4(net);
    if (a === null || n === null) return false;
    const mask = Number(bits) === 0 ? 0 : (0xffffffff << (32 - Number(bits))) >>> 0;
    return (a & mask) === (n & mask);
  }
  if (entry.startsWith("*.")) return host.endsWith(entry.slice(1));
  return host === entry || host.endsWith("." + entry);
}

export function verdict(rules: EgressRules, host: string): { ok: boolean; why: string } {
  const listed = (list: string[], h: string) => list.some((e) => entryMatches(e, h));
  if (listed(rules.deny, host)) return { ok: false, why: "block list" };
  if (rules.mode === "none") return { ok: false, why: "no network" };
  if (rules.mode === "open") return { ok: true, why: "open" };
  return listed(rules.allow, host) ? { ok: true, why: "allow list" } : { ok: false, why: "not on allow list" };
}

const uniq = (xs: string[]) => [...new Set(xs)].sort();
const without = (xs: string[], drop: string[]) => xs.filter((x) => !drop.includes(x));

function fmtBytes(n?: number): string {
  if (!n) return "0 B";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
}

function fmtTime(v?: string | number): string {
  if (v === undefined || v === "" || v === 0) return "—";
  const d = typeof v === "number" ? new Date(v * 1000) : new Date(v);
  return isNaN(d.getTime()) ? String(v) : d.toLocaleString();
}

async function call<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    const msg = e instanceof ApiError ? `${e.message} (${e.code})` : e instanceof Error ? e.message : String(e);
    throw new Error(msg);
  }
}

/* ══ shell ═════════════════════════════════════════════════════════════ */

export function NetworkView() {
  const sandboxes = usePlatform((s) => s.sandboxes);
  const focusId = usePlatform((s) => s.networkFocusSandbox);
  const clearFocus = usePlatform((s) => s.clearNetworkFocus);
  const [tab, setTab] = useState<TabId>("overview");

  /* deep link from a sandbox detail — jump straight to its traffic tab */
  useEffect(() => {
    if (focusId) {
      setTab("traffic");
      clearFocus();
    }
  }, [focusId, clearFocus]);

  const tabs = [
    { id: "overview" as const, label: "Overview", vi: "Tổng quan", icon: Waypoints },
    { id: "policy" as const, label: "Egress policy", vi: "Chính sách", icon: ShieldCheck },
    { id: "traffic" as const, label: "Sandbox traffic", vi: "Lưu lượng", icon: Activity, count: sandboxes.length },
    { id: "ports" as const, label: "Ports & previews", vi: "Cổng", icon: Layers },
    { id: "domain" as const, label: "Domain & TLS", vi: "Tên miền", icon: Globe },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Network"
        subtitle="Egress policy, sandbox traffic, preview ports and the console domain"
        vi="Chính sách ra ngoài · lưu lượng · cổng · tên miền"
      />

      {/* in-page tabs — pure client state, sliding underline, no routing */}
      <div>
        <ViewTabs tabs={tabs} value={tab} onChange={setTab} />
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={tab}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.14, ease: "easeOut" }}
            className="mt-5 space-y-5"
          >
            {tab === "overview" && <OverviewTab onGo={setTab} />}
            {tab === "policy" && <PolicyTab />}
            {tab === "traffic" && <TrafficTab />}
            {tab === "ports" && <PortsTab />}
            {tab === "domain" && <DomainTab />}
          </motion.div>
        </AnimatePresence>
      </div>
    </div>
  );
}

/* ══ 1 · Overview ═════════════════════════════════════════════════════ */

interface Verification {
  ok: boolean;
  hint?: string;
  checks: { host: string; addrs: string[]; ok: boolean; error?: string }[];
}

function OverviewTab({ onGo }: { onGo: (t: TabId) => void }) {
  const me = usePlatform((s) => s.me);
  const overviewNums = usePlatform((s) => s.overviewNums);
  const sandboxes = usePlatform((s) => s.sandboxes);
  const isAdmin = me?.role === "admin";

  const [policy, setPolicy] = useState<Policy | null>(null);
  const [domain, setDomain] = useState<DomainInfo | null>(null);
  const [err] = useState("");
  const [expect, setExpect] = useState("");
  const [ver, setVer] = useState<Verification | null>(null);
  const [checking, setChecking] = useState(false);

  const load = () => {
    apiGet<Policy>("/v1/policy").then(setPolicy).catch(() => setPolicy(null));
    apiGet<DomainInfo>("/v1/domain")
      .then(setDomain)
      .catch(() => setDomain(null));
  };
  useEffect(load, []);

  const dnsCheck = async () => {
    setChecking(true);
    try {
      const ips = expect.split(/[\s,]+/).filter(Boolean);
      const r = await call(() => apiPost<Verification>("/v1/domain/verify", ips.length ? { expect_ips: ips } : {}));
      setVer(r);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "DNS check failed");
    } finally {
      setChecking(false);
    }
  };

  const modes = sandboxes.reduce<Record<string, number>>((acc, s) => {
    const key = s.env;
    acc[key] = (acc[key] ?? 0) + 1;
    return acc;
  }, {});

  return (
    <>
      {err && <Panel className="p-4 text-sm text-danger">{err}</Panel>}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <KpiCard
          icon={<ShieldCheck className="size-4" />}
          label="Instance egress policy"
          vi={policy?.egress.mode === "strict" ? "chỉ host trong danh sách" : "mở cho mọi host"}
          value={policy?.egress.mode ?? "—"}
          accent={policy?.egress.mode === "strict" ? "text-warn" : "text-neon"}
        />
        <KpiCard
          icon={<Globe className="size-4" />}
          label="Console domain"
          vi={domain?.running.tls_mode === "off" ? "TLS tắt" : `TLS ${domain?.running.tls_mode ?? "—"}`}
          value={domain?.running.domain || "(console host)"}
          accent="text-info"
        />
        <KpiCard
          icon={<Layers className="size-4" />}
          label="Ports exposed"
          vi="cổng preview đang mở"
          value={overviewNums?.portsExposed ?? 0}
        />
        <KpiCard
          icon={<ShieldX className="size-4" />}
          label="Blocked requests · 24h"
          vi="yêu cầu bị chặn"
          value={overviewNums?.netBlocked ?? 0}
          accent={(overviewNums?.netBlocked ?? 0) > 0 ? "text-warn" : "text-neon"}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Panel className="p-6">
          <SectionTitle title="Fleet egress posture" vi="Trạng thái ra ngoài của các sandbox" />
          <div className="mt-4 space-y-2.5">
            {Object.entries(modes).map(([k, v]) => (
              <div key={k} className="flex items-center justify-between gap-3 border-b border-line-soft py-2 last:border-0">
                <span className="text-sm text-gray-300">{k}</span>
                <span className="inline-flex items-center gap-2">
                  <Mono className="font-mono text-sm text-ink">{v}</Mono>
                  <span className="font-mono text-[10px] text-gray-600">sandbox(es)</span>
                </span>
              </div>
            ))}
            {modes.size === 0 && <p className="text-sm text-gray-500">No sandboxes yet.</p>}
          </div>
          <div className="mt-5 rounded-xl border border-line bg-navy/40 p-4">
            <Mono className="text-[11px] leading-relaxed text-gray-400">
              Traffic leaves only through the egress proxy. Cloud metadata, loopback and private networks stay blocked in
              every mode. Changes apply to the next connection without a restart.
            </Mono>
          </div>
          <div className="mt-4 flex flex-wrap gap-2.5">
            <button
              onClick={() => onGo("policy")}
              className="inline-flex items-center gap-2 rounded-lg border border-neon/30 bg-neon/10 px-3.5 py-2 text-xs font-medium text-neon transition-colors hover:bg-neon/20"
            >
              <ShieldCheck className="size-3.5" /> Edit egress policy
            </button>
            <button
              onClick={() => onGo("traffic")}
              className="inline-flex items-center gap-2 rounded-lg border border-line px-3.5 py-2 text-xs font-medium text-gray-300 transition-colors hover:bg-fg/5"
            >
              <Activity className="size-3.5" /> Live sandbox traffic
            </button>
          </div>
        </Panel>

        <Panel className="p-6">
          <SectionTitle title="DNS check" vi="Kiểm tra DNS" right={<Mono className="text-[10px] text-gray-500">{domain?.console_host ?? ""}</Mono>} />
          <p className="mt-3 text-xs leading-relaxed text-gray-500">
            Resolves the console host and preview wildcard. Pass expected server IPs to verify the A records after a
            deploy. · Phân giải tên miền console và wildcard preview.
          </p>
          <div className="mt-4 flex flex-col gap-2.5 sm:flex-row">
            <TextField value={expect} onChange={setExpect} placeholder="expected server IPs (optional)" className="flex-1" mono ariaLabel="Expected server IPs" />
            <button
              onClick={dnsCheck}
              disabled={checking}
              className="inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-lg border border-line bg-surface px-4 text-sm font-medium text-gray-200 transition-colors hover:bg-fg/5 disabled:opacity-40"
            >
              <RefreshCw className={cn("size-3.5", checking && "animate-spin")} /> Check DNS
            </button>
          </div>
          {ver && (
            <div className="mt-4 rounded-xl border border-line bg-navy/50 p-4">
              <p className={cn("mb-2 flex items-center gap-2 text-xs font-medium", ver.ok ? "text-neon" : "text-danger")}>
                {ver.ok ? <CheckCircle2 className="size-4" /> : <XCircle className="size-4" />}
                {ver.ok ? "DNS looks right · DNS trông ổn" : (ver.hint ?? "DNS does not match.")}
              </p>
              {ver.checks.map((c) => (
                <div key={c.host} className="flex flex-wrap items-center gap-2 py-1 text-sm">
                  {c.ok ? <CheckCircle2 className="size-4 text-neon" /> : <XCircle className="size-4 text-danger" />}
                  <Mono className="text-xs text-gray-300">{c.host}</Mono>
                  <Mono className="text-xs text-gray-500">→ {c.addrs.join(", ") || "—"}</Mono>
                  {c.error && <Mono className="text-xs text-danger">{c.error}</Mono>}
                </div>
              ))}
            </div>
          )}
          {isAdmin && (
            <button
              onClick={() => onGo("domain")}
              className="mt-4 inline-flex items-center gap-2 rounded-lg border border-line px-3.5 py-2 text-xs font-medium text-gray-300 transition-colors hover:bg-fg/5"
            >
              <Globe className="size-3.5" /> Domain & certificates
            </button>
          )}
        </Panel>
      </div>
    </>
  );
}

/* ══ 2 · Egress policy (instance-wide) ════════════════════════════════ */

function PolicyTab() {
  const me = usePlatform((s) => s.me);
  const isAdmin = me?.role === "admin";
  const [data, setData] = useState<Policy | null>(null);
  const [loadErr, setLoadErr] = useState("");
  const [mode, setMode] = useState<"strict" | "permissive">("strict");
  const [allow, setAllow] = useState("");
  const [approval, setApproval] = useState("");
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);

  const load = () => {
    setLoadErr("");
    apiGet<Policy>("/v1/policy")
      .then((p) => {
        setData(p);
        if (!dirty) {
          setMode(p.egress.mode);
          setAllow(p.egress.allow.join("\n"));
        }
      })
      .catch((e) => setLoadErr(e instanceof Error ? e.message : String(e)));
  };
  useEffect(load, []);

  const save = async () => {
    if (!data) return;
    setBusy(true);
    try {
      const next: Policy = {
        ...data,
        egress: {
          mode,
          allow: allow.split("\n").map((s) => s.trim()).filter(Boolean),
        },
      };
      const h: Record<string, string> = approval ? { [APPROVAL_HEADER]: approval } : {};
      await call(() => apiPut("/v1/policy", next, h));
      toast.success("Egress policy saved · Đã lưu chính sách");
      setDirty(false);
      setApproval("");
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not save the policy");
    } finally {
      setBusy(false);
    }
  };

  if (loadErr && !data) return <Panel className="p-6 text-sm text-danger">Could not load policy: {loadErr}</Panel>;
  if (!data) return <Panel className="p-6 font-mono text-xs text-gray-500">loading…</Panel>;

  return (
    <>
      <Panel className="p-6">
        <SectionTitle
          title="Instance egress policy"
          vi="Chính sách egress toàn instance"
          right={
            dirty ? (
              <Mono className="flex items-center gap-1.5 text-[11px] text-warn">
                <span className="size-1.5 rounded-full bg-warn" /> unsaved changes
              </Mono>
            ) : (
              <Mono className="flex items-center gap-1.5 text-[11px] text-gray-500">
                <span className="size-1.5 rounded-full bg-neon dot-live" /> synced
              </Mono>
            )
          }
        />
        <p className="mt-3 text-xs leading-relaxed text-gray-500">
          Every sandbox is checked against this policy before its own rules apply. In <b className="text-gray-300">strict</b> mode
          sandboxes may only allow hosts listed below; in <b className="text-gray-300">permissive</b> mode each sandbox may
          allow any host. · Mỗi sandbox phải tuân thủ chính sách này trước khi áp dụng danh sách riêng của nó.
        </p>

        <div className="mt-5 grid gap-3 md:grid-cols-2">
          <ModeCard
            title="Strict"
            vi="Chặt — chỉ host được liệt kê"
            desc="Sandboxes may only allow hosts on the instance allowlist below. Everything else needs an admin approval."
            selected={mode === "strict"}
            onSelect={() => {
              setMode("strict");
              setDirty(true);
            }}
            disabled={!isAdmin}
            tone="warn"
          />
          <ModeCard
            title="Permissive"
            vi="Mở — sandbox tự chọn host"
            desc="Sandboxes may allow any host; the instance allowlist becomes a convenience default, not a limit."
            selected={mode === "permissive"}
            onSelect={() => {
              setMode("permissive");
              setDirty(true);
            }}
            disabled={!isAdmin}
          />
        </div>

        <div className="mt-5 grid gap-5 lg:grid-cols-2">
          <FieldRow label="Instance allowlist" vi="danh sách host cho phép" hint="One host per line. example.com includes its subdomains; IPs and CIDR ranges are allowed.">
            <TextArea
              value={allow}
              onChange={(v) => {
                setAllow(v);
                setDirty(true);
              }}
              rows={7}
              mono
              placeholder={"github.com\n*.npmjs.org\n140.82.112.0/20"}
              disabled={!isAdmin}
            />
          </FieldRow>
          <div className="space-y-5">
            <FieldRow label="Approval ID" vi="mã phê duyệt (tùy chọn)" hint="Needed when the change itself is gated by policy (policy.update).">
              <TextField value={approval} onChange={setApproval} placeholder="approval id (optional)" mono disabled={!isAdmin} />
            </FieldRow>
            <div className="rounded-xl border border-line bg-navy/40 p-4">
              <SectionTitle title="Current sandboxd policy" vi="Chính sách hiện tại" />
              <div className="mt-3 space-y-1.5 font-mono text-[11.5px] text-gray-400">
                <div>mode · <span className="text-gray-200">{data.egress.mode}</span></div>
                <div>allowed hosts · <span className="text-gray-200">{data.egress.allow.length ? data.egress.allow.join(", ") : "none"}</span></div>
                <div>gated actions · <span className="text-gray-200">{data.approvals.join(", ") || "none"}</span></div>
              </div>
            </div>
          </div>
        </div>

        {isAdmin && (
          <div className="mt-6 flex flex-wrap items-center gap-3">
            <button
              onClick={save}
              disabled={busy || !dirty}
              className="inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)] disabled:pointer-events-none disabled:opacity-40"
            >
              <Save className="size-4" /> Save policy
            </button>
            <button
              onClick={() => {
                if (!data) return;
                setMode(data.egress.mode);
                setAllow(data.egress.allow.join("\n"));
                setDirty(false);
              }}
              disabled={busy || !dirty}
              className="inline-flex items-center gap-2 rounded-lg border border-line px-4 py-2 text-sm text-gray-300 transition-colors hover:bg-fg/5 disabled:opacity-40"
            >
              Reset
            </button>
          </div>
        )}
      </Panel>
    </>
  );
}

/* ══ 3 · Sandbox traffic — per-sandbox rules + live monitor ═══════════ */

const MODES: { mode: NetMode; title: string; vi: string; text: string; tone: "danger" | "neon" | "info" }[] = [
  { mode: "none", title: "No network", vi: "Không mạng", text: "Nothing leaves the sandbox.", tone: "danger" },
  { mode: "allowlist", title: "Allow list", vi: "Danh sách cho phép", text: "Only the hosts on the allow list; everything else is blocked.", tone: "neon" },
  { mode: "open", title: "Open, monitored", vi: "Mở, có giám sát", text: "Everything except the block list. Every request is shown below.", tone: "info" },
];

function TrafficTab() {
  const me = usePlatform((s) => s.me);
  const sandboxes = usePlatform((s) => s.sandboxes);
  const write = me?.role === "admin" || me?.role === "operator";

  const [id, setId] = useState<string>(sandboxes[0]?.id ?? "");
  useEffect(() => {
    if (!sandboxes.some((s) => s.id === id)) setId(sandboxes[0]?.id ?? "");
  }, [sandboxes, id]);

  if (!sandboxes.length) return <Panel className="p-6 text-sm text-gray-500">No sandboxes yet — create one first.</Panel>;

  return (
    <div className="space-y-5">
      <Panel className="flex flex-col gap-3 p-5 sm:flex-row sm:items-center">
        <div className="min-w-0 flex-1">
          <FieldRow label="Sandbox" vi="chọn sandbox để xem và đổi luật mạng" className="max-w-md">
            <FancySelect
              ariaLabel="Sandbox"
              value={id}
              onChange={setId}
              options={sandboxes.map((s) => ({ value: s.id, label: s.name, vi: `${s.id} · ${s.env}` }))}
            />
          </FieldRow>
        </div>
        <div className="flex items-center gap-2 sm:pb-1">
          {(() => {
            const sbx = sandboxes.find((s) => s.id === id);
            return sbx ? (
              <span className="inline-flex items-center gap-2 rounded-lg border border-line bg-navy/60 px-3 py-2">
                <span className={cn("size-1.5 rounded-full", sbx.status === "Running" ? "bg-neon dot-live" : "bg-gray-500")} />
                <Mono className="text-xs text-gray-300">{sbx.status}</Mono>
              </span>
            ) : null;
          })()}
        </div>
      </Panel>

      <EgressRulesPanel id={id} write={write} />
      <LiveActivity id={id} write={write} />
    </div>
  );
}

interface EgressResp extends EgressRules {
  proxy: string;
}

function EgressRulesPanel({ id, write }: { id: string; write: boolean }) {
  const [egress, setEgress] = useState<EgressResp | null>(null);
  const [policy, setPolicy] = useState<Policy | null>(null);
  const [err, setErr] = useState("");
  const [mode, setMode] = useState<NetMode>("allowlist");
  const [allow, setAllow] = useState("");
  const [deny, setDeny] = useState("");
  const [approval, setApproval] = useState("");
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);

  const load = () => {
    setErr("");
    apiGet<EgressResp>(`/v1/sandboxes/${id}/network/egress`)
      .then((d) => {
        setEgress(d);
        if (!dirty) {
          setMode(d.mode);
          setAllow(d.allow.join("\n"));
          setDeny(d.deny.join("\n"));
        }
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)));
    apiGet<Policy>("/v1/policy").then(setPolicy).catch(() => setPolicy(null));
  };
  useEffect(load, [id]);

  const save = async () => {
    setBusy(true);
    try {
      const next: EgressRules = {
        mode,
        allow: allow.split("\n").map((s) => s.trim()).filter(Boolean),
        deny: deny.split("\n").map((s) => s.trim()).filter(Boolean),
      };
      const h: Record<string, string> = approval ? { [APPROVAL_HEADER]: approval } : {};
      const r = await call(() => apiPut<EgressResp>(`/v1/sandboxes/${id}/network/egress`, next, h));
      setEgress(r);
      toast.success("Network rules saved · Đã lưu luật mạng");
      setDirty(false);
      setApproval("");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not save the rules");
    } finally {
      setBusy(false);
    }
  };

  if (err && !egress) return <Panel className="p-6 text-sm text-danger">Could not load egress rules: {err}</Panel>;

  const strict = policy?.egress.mode === "strict";

  return (
    <Panel className="p-6">
      <SectionTitle
        title="Egress rules"
        vi="Luật ra ngoài"
        right={
          dirty ? (
            <Mono className="flex items-center gap-1.5 text-[11px] text-warn">
              <span className="size-1.5 rounded-full bg-warn" /> unsaved
            </Mono>
          ) : egress ? (
            <Mono className="flex items-center gap-1.5 text-[11px] text-gray-500">
              <span className="size-1.5 rounded-full bg-neon dot-live" /> synced
            </Mono>
          ) : null
        }
      />
      {!egress ? (
        <p className="mt-4 font-mono text-xs text-gray-500">loading…</p>
      ) : (
        <>
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            {MODES.map((m) => (
              <ModeCard
                key={m.mode}
                title={m.title}
                vi={m.vi}
                desc={m.text}
                selected={mode === m.mode}
                onSelect={() => {
                  setMode(m.mode);
                  setDirty(true);
                }}
                disabled={!write}
                tone={m.tone}
              />
            ))}
          </div>

          {policy && (
            <p className="mt-3 text-xs text-gray-500">
              Instance policy: <b className="text-gray-300">{policy.egress.mode}</b>
              {strict && ` (allowed hosts: ${policy.egress.allow.length ? policy.egress.allow.join(", ") : "none"})`}
              {policy.approvals.includes("sandbox.egress_outside_policy") && "; anything outside it needs an admin's approval"}
            </p>
          )}
          {strict && mode === "open" && (
            <p className="mt-2 rounded-lg border border-warn/30 bg-warn/10 px-3.5 py-2.5 text-xs leading-relaxed text-warn">
              Open mode needs the egress policy set to <b>permissive</b> — ask an admin, or switch it in the Egress policy tab.
            </p>
          )}

          <div className="mt-5 grid gap-5 lg:grid-cols-2">
            <FieldRow
              label="Allow list"
              vi="cho phép"
              hint="Used in allow-list mode. example.com includes its subdomains."
            >
              <TextArea
                value={allow}
                onChange={(v) => {
                  setAllow(v);
                  setDirty(true);
                }}
                rows={6}
                mono
                placeholder={"github.com\n*.npmjs.org\n140.82.112.0/20"}
                disabled={!write}
              />
            </FieldRow>
            <FieldRow label="Block list" vi="chặn" hint="Always blocked, in every mode. Hosts, IPs or CIDR ranges.">
              <TextArea
                value={deny}
                onChange={(v) => {
                  setDeny(v);
                  setDirty(true);
                }}
                rows={6}
                mono
                placeholder={"tracker.example\n203.0.113.7\n10.0.0.0/8"}
                disabled={!write}
              />
            </FieldRow>
          </div>

          <p className="mt-4 text-xs leading-relaxed text-gray-500">
            Traffic leaves only through the egress proxy{egress.proxy ? ` (${egress.proxy})` : ""}; changes apply to the next
            connection without a restart. Cloud metadata, loopback and private networks stay blocked in every mode.
          </p>

          {write && (
            <div className="mt-5 flex flex-wrap items-center gap-3">
              <TextField value={approval} onChange={setApproval} placeholder="Approval ID (optional)" mono className="w-56" />
              <button
                onClick={save}
                disabled={busy || !dirty}
                className="inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)] disabled:pointer-events-none disabled:opacity-40"
              >
                <Save className="size-4" /> Save network rules
              </button>
              <button
                onClick={() => {
                  setMode(egress.mode);
                  setAllow(egress.allow.join("\n"));
                  setDeny(egress.deny.join("\n"));
                  setDirty(false);
                }}
                disabled={busy || !dirty}
                className="inline-flex items-center gap-2 rounded-lg border border-line px-4 py-2 text-sm text-gray-300 transition-colors hover:bg-fg/5 disabled:opacity-40"
              >
                Reset
              </button>
            </div>
          )}
        </>
      )}
    </Panel>
  );
}

/* ── live activity monitor ──────────────────────────────────────────── */

const POLL_MS = 2000;
const MAX_FEED = 200;

function LiveActivity({ id, write }: { id: string; write: boolean }) {
  const [hosts, setHosts] = useState<NetHost[]>([]);
  const [feed, setFeed] = useState<NetEventLite[]>([]);
  const [recording, setRecording] = useState(true);
  const [paused, setPaused] = useState(false);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState("");
  const [only, setOnly] = useState<"all" | "allowed" | "blocked">("all");
  const [sel, setSel] = useState<Set<string>>(new Set());
  const cursor = useRef("");
  const rulesRef = useRef<EgressRules | null>(null);

  const loadRules = () =>
    apiGet<EgressResp>(`/v1/sandboxes/${id}/network/egress`)
      .then((r) => {
        rulesRef.current = { mode: r.mode, allow: r.allow, deny: r.deny };
      })
      .catch(() => {});

  useEffect(() => {
    setHosts([]);
    setFeed([]);
    setSel(new Set());
    cursor.current = "";
    void loadRules();
  }, [id]);

  useEffect(() => {
    if (paused) return;
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      try {
        const q = cursor.current ? `?after=${cursor.current}` : "";
        const a = await apiGet<NetActivity>(`/v1/sandboxes/${id}/network/activity${q}`);
        if (!live) return;
        cursor.current = a.cursor;
        rulesRef.current = a.rules;
        setRecording(a.recording);
        setHosts(a.hosts);
        if (a.events.length) setFeed((f) => [...a.events.slice().reverse(), ...f].slice(0, MAX_FEED));
        setError("");
      } catch (e) {
        if (live) setError(e instanceof Error ? e.message : String(e));
      }
      if (live) timer = setTimeout(tick, POLL_MS);
    };
    void tick();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [id, paused]);

  const rules = rulesRef.current;
  const shown = useMemo(() => {
    if (!rules) return [];
    const f = filter.trim().toLowerCase();
    return hosts.filter((h) => {
      if (f && !h.host.includes(f) && !h.ips.some((ip) => ip.includes(f))) return false;
      const ok = verdict(rules, h.host).ok;
      return only === "all" || (only === "allowed" ? ok : !ok);
    });
  }, [hosts, filter, only, rules]);

  const toggle = (h: string) =>
    setSel((s) => {
      const n = new Set(s);
      if (n.has(h)) n.delete(h);
      else n.add(h);
      return n;
    });
  const allShown = shown.length > 0 && shown.every((h) => sel.has(h.host));
  const toggleAll = () => setSel(allShown ? new Set() : new Set(shown.map((h) => h.host)));
  const picked = [...sel];

  const done = () => setSel(new Set());
  const apply = async (next: EgressRules, msg: string) => {
    const h: Record<string, string> = {};
    try {
      await call(() => apiPut<EgressResp>(`/v1/sandboxes/${id}/network/egress`, next, h));
      toast.success(msg);
      rulesRef.current = next;
      void loadRules();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not apply the rules");
    }
  };
  const block = (items: string[]) =>
    apply({ mode: rules?.mode ?? "allowlist", allow: without(rules?.allow ?? [], items), deny: uniq([...(rules?.deny ?? []), ...items]) }, `Blocked ${items.join(", ")}`).then(done);
  const allow = (items: string[]) =>
    apply(
      {
        mode: rules?.mode === "none" ? "allowlist" : (rules?.mode ?? "allowlist"),
        allow: rules?.mode === "open" ? (rules?.allow ?? []) : uniq([...(rules?.allow ?? []), ...items]),
        deny: without(rules?.deny ?? [], items),
      },
      `Allowed ${items.join(", ")}`,
    ).then(done);

  return (
    <Panel className="p-6">
      <SectionTitle
        title="Live network activity"
        vi="Lưu lượng mạng trực tiếp"
        right={
          <span className="flex items-center gap-2.5">
            <span className={cn("inline-flex items-center gap-1.5 font-mono text-[11px]", paused ? "text-gray-500" : "text-neon")}>
              <span className={cn("size-1.5 rounded-full", paused ? "bg-gray-500" : "bg-neon dot-live")} />
              {paused ? "paused" : "live"}
            </span>
            <button
              onClick={() => setPaused((p) => !p)}
              className="inline-flex items-center gap-1.5 rounded-lg border border-line px-2.5 py-1.5 text-xs text-gray-300 transition-colors hover:bg-fg/5"
            >
              {paused ? <Play className="size-3" /> : <Pause className="size-3" />} {paused ? "Resume" : "Pause"}
            </button>
            <button
              onClick={() => setFeed([])}
              className="rounded-lg border border-line px-2.5 py-1.5 text-xs text-gray-300 transition-colors hover:bg-fg/5"
            >
              Clear feed
            </button>
          </span>
        }
      />
      {!recording && (
        <p className="mt-3 rounded-lg border border-warn/30 bg-warn/10 px-3.5 py-2.5 text-xs text-warn">
          The egress proxy is not configured on this node, so no requests are recorded.
        </p>
      )}
      {error && <p className="mt-3 text-xs text-danger">{error}</p>}
      <p className="mt-3 text-xs leading-relaxed text-gray-500">
        Every request the sandbox makes through the proxy appears here within {POLL_MS / 1000}s. Pick hosts with the
        checkboxes and block or allow them; the status column shows what the current rules do to each host. · Mỗi yêu cầu
        qua proxy sẽ hiện ở đây; đánh dấu ô chọn rồi chặn hoặc cho phép.
      </p>

      <div className="mt-4 flex flex-col gap-2.5 sm:flex-row">
        <TextField value={filter} onChange={setFilter} placeholder="Filter host or IP…" ariaLabel="Filter host" className="flex-1" />
        <FancySelect
          ariaLabel="Host filter"
          className="sm:w-52"
          value={only}
          onChange={(v) => setOnly(v as typeof only)}
          options={[
            { value: "all", label: "All hosts", vi: "tất cả" },
            { value: "allowed", label: "Allowed now", vi: "được phép" },
            { value: "blocked", label: "Blocked now", vi: "bị chặn" },
          ]}
        />
      </div>

      {write && (
        <div className="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-line bg-navy/40 px-3 py-2.5">
          <Mono className="text-[11px] text-gray-400">{picked.length} selected · đã chọn</Mono>
          <span className="flex-1" />
          <button
            disabled={!picked.length}
            onClick={() => block(picked)}
            className="inline-flex items-center gap-1.5 rounded-lg border border-danger/40 bg-danger/10 px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/20 disabled:pointer-events-none disabled:opacity-40"
          >
            <ShieldX className="size-3.5" /> Block selected
          </button>
          <button
            disabled={!picked.length}
            onClick={() => allow(picked)}
            className="inline-flex items-center gap-1.5 rounded-lg border border-neon/30 bg-neon/10 px-3 py-1.5 text-xs font-medium text-neon transition-colors hover:bg-neon/20 disabled:pointer-events-none disabled:opacity-40"
          >
            <ShieldCheck className="size-3.5" /> Allow selected
          </button>
        </div>
      )}

      <div className="mt-4">
        {!rules ? (
          <p className="font-mono text-xs text-gray-500">loading…</p>
        ) : (
          <TableShell>
            <thead>
              <tr>
                {write && (
                  <Th className="w-10">
                    <CheckBox checked={allShown} onChange={toggleAll} ariaLabel="Select all" />
                  </Th>
                )}
                <Th>Host</Th>
                <Th>IPs</Th>
                <Th>Requests</Th>
                <Th>Traffic</Th>
                <Th>Last seen</Th>
                <Th>Now</Th>
                {write && <Th className="text-right">Action</Th>}
              </tr>
            </thead>
            <tbody>
              {shown.length === 0 && (
                <tr>
                  <td colSpan={write ? 8 : 7} className="px-3.5 py-6 text-center text-sm text-gray-500">
                    {hosts.length ? "No host matches the filter." : "No requests yet. Start something in the sandbox that uses the network."}
                  </td>
                </tr>
              )}
              {shown.map((h) => {
                if (!rules) return null;
                const v = verdict(rules, h.host);
                return (
                  <tr key={h.host} className={cn("transition-colors", sel.has(h.host) ? "bg-neon/[0.05]" : "hover:bg-fg/[0.02]")}>
                    {write && (
                      <Td>
                        <CheckBox checked={sel.has(h.host)} onChange={() => toggle(h.host)} ariaLabel={`Select ${h.host}`} />
                      </Td>
                    )}
                    <Td>
                      <Mono className="text-xs text-gray-100">{h.host}</Mono>
                    </Td>
                    <Td>
                      <Mono className="text-[11px] text-gray-500">{h.ips.length ? h.ips.join(", ") : "—"}</Mono>
                    </Td>
                    <Td>
                      <span className="text-xs text-neon">{h.allowed} ok</span>
                      {h.denied > 0 && <span className="text-xs text-danger"> · {h.denied} blocked</span>}
                    </Td>
                    <Td>
                      <Mono className="text-[11px] text-gray-400">
                        ↑{fmtBytes(h.bytes_up)} ↓{fmtBytes(h.bytes_down)}
                      </Mono>
                    </Td>
                    <Td>
                      <Mono className="text-[11px] text-gray-500">{new Date(h.last_seen).toLocaleTimeString()}</Mono>
                    </Td>
                    <Td>
                      <span
                        className={cn(
                          "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 font-mono text-[10px]",
                          v.ok ? "border-neon/30 bg-neon/10 text-neon" : "border-danger/30 bg-danger/10 text-danger",
                        )}
                        title={v.why}
                      >
                        {v.ok ? "allowed" : "blocked"}
                      </span>
                    </Td>
                    {write && (
                      <Td className="text-right">
                        {v.ok ? (
                          <button
                            disabled={!write}
                            onClick={() => block([h.host])}
                            className="rounded-lg border border-danger/40 px-2.5 py-1 text-[11px] text-danger transition-colors hover:bg-danger/10"
                          >
                            Block
                          </button>
                        ) : (
                          <button
                            disabled={!write}
                            onClick={() => allow([h.host])}
                            className="rounded-lg border border-neon/30 px-2.5 py-1 text-[11px] text-neon transition-colors hover:bg-neon/10"
                          >
                            Allow
                          </button>
                        )}
                      </Td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </TableShell>
        )}
      </div>

      <div className="mt-5">
        <SectionTitle title="Request feed" vi="Dòng yêu cầu" right={<Mono className="text-[10px] text-gray-500">{feed.length} events</Mono>} />
        <div className="mt-3 max-h-56 space-y-1 overflow-y-auto rounded-xl border border-line bg-navy/50 p-2 [scrollbar-width:thin]">
          {feed.length === 0 ? (
            <p className="px-2 py-4 text-center text-xs text-gray-600">Waiting for requests…</p>
          ) : (
            feed.map((e) => (
              <div key={e.seq} className={cn("flex flex-wrap items-center gap-2 rounded-md px-2 py-1 font-mono text-[11px]", e.allowed ? "text-gray-300" : "bg-danger/5 text-gray-400")}>
                <span className="text-gray-600">{new Date(e.time).toLocaleTimeString()}</span>
                <span className={e.allowed ? "text-neon" : "text-danger"}>{e.allowed ? "ALLOW" : "BLOCK"}</span>
                <span>{e.method}</span>
                <span className="text-gray-200">
                  {e.host}:{e.port}
                </span>
                {e.ip && <span className="text-gray-600">{e.ip}</span>}
                {e.reason && <span className="text-gray-600">{e.reason}</span>}
              </div>
            ))
          )}
        </div>
      </div>
    </Panel>
  );
}

interface NetEventLite {
  seq: number;
  time: string;
  method: string;
  host: string;
  port: string;
  ip?: string;
  allowed: boolean;
  reason?: string;
}

/* ══ 4 · Ports & previews ═════════════════════════════════════════════ */

interface PortsResp {
  ports: { container: number; preview_url: string }[];
}

function PortsTab() {
  const me = usePlatform((s) => s.me);
  const sandboxes = usePlatform((s) => s.sandboxes);
  const write = me?.role === "admin" || me?.role === "operator";

  const [rows, setRows] = useState<Array<{ id: string; name: string; status: string; port: number; url: string }>>([]);
  const [loading, setLoading] = useState(true);
  const [id, setId] = useState<string>(sandboxes[0]?.id ?? "");
  const [newPort, setNewPort] = useState("");
  const [busy, setBusy] = useState(false);

  const load = () => {
    setLoading(true);
    const list = sandboxes.length ? sandboxes : [];
    Promise.all(
      list.map((s) =>
        apiGet<PortsResp>(`/v1/sandboxes/${s.id}/network/ports`)
          .then((r) => r.ports.map((p) => ({ id: s.id, name: s.name, status: s.status, port: p.container, url: p.preview_url })))
          .catch(() => [] as typeof rows),
      ),
    )
      .then((rs) => setRows(rs.flat()))
      .finally(() => setLoading(false));
  };
  useEffect(() => {
    if (sandboxes.length) load();
  }, [sandboxes]);

  useEffect(() => {
    if (!sandboxes.some((s) => s.id === id)) setId(sandboxes[0]?.id ?? "");
  }, [sandboxes, id]);

  const expose = async () => {
    const n = Number(newPort);
    if (!Number.isInteger(n) || n < 1 || n > 65535) {
      toast.error("Port must be an integer between 1 and 65535.");
      return;
    }
    setBusy(true);
    try {
      await call(() => apiPost(`/v1/sandboxes/${id}/network/ports`, { container: n }));
      toast.success(`Port ${n} exposed · Cổng ${n} đã mở`);
      setNewPort("");
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not expose the port");
    } finally {
      setBusy(false);
    }
  };

  const remove = async (port: number) => {
    setBusy(true);
    try {
      await call(() => apiDel(`/v1/sandboxes/${id}/network/ports/${port}`));
      toast.success(`Port ${port} removed · Đã gỡ cổng ${port}`);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not remove the port");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="grid gap-4 lg:grid-cols-[1.2fr_1fr]">
        <Panel className="p-6">
          <SectionTitle title="Expose a port" vi="Mở cổng preview" />
          <p className="mt-3 text-xs leading-relaxed text-gray-500">
            Exposing a port rebuilds the container; the workspace is kept. The preview URL follows
            <Mono className="mx-1 text-gray-300">https://&lt;sandbox-id&gt;.preview.&lt;host&gt;</Mono>. · Mở cổng sẽ tạo lại
            container, dữ liệu được giữ nguyên.
          </p>
          <div className="mt-4 flex flex-col gap-2.5 sm:flex-row">
            <div className="min-w-0 flex-1">
              <FancySelect
                ariaLabel="Sandbox"
                value={id}
                onChange={setId}
                options={sandboxes.map((s) => ({ value: s.id, label: s.name, vi: s.id }))}
              />
            </div>
            <TextField
              value={newPort}
              onChange={setNewPort}
              type="number"
              min={1}
              max={65535}
              placeholder="container port"
              mono
              className="sm:w-40"
              ariaLabel="Container port"
              onKeyDown={(e) => e.key === "Enter" && expose()}
            />
            <button
              onClick={expose}
              disabled={busy || !id || !newPort}
              className="inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-lg bg-neon px-4 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)] disabled:pointer-events-none disabled:opacity-40"
            >
              <Plus className="size-4" /> Expose
            </button>
          </div>
        </Panel>
        <Panel className="p-6">
          <SectionTitle title="How previews work" vi="Cách preview hoạt động" />
          <div className="mt-3 space-y-2.5 text-xs leading-relaxed text-gray-400">
            <p className="flex gap-2.5">
              <Waypoints className="mt-0.5 size-4 shrink-0 text-neon" /> Every exposed port gets a wildcard-host preview URL routed
              by the edge proxy — no extra config per sandbox.
            </p>
            <p className="flex gap-2.5">
              <Server className="mt-0.5 size-4 shrink-0 text-info" /> Port ranges are capped by the instance policy
              (max ports per sandbox) and rebuilds keep volumes intact.
            </p>
            <p className="flex gap-2.5">
              <FileKey2 className="mt-0.5 size-4 shrink-0 text-warn" /> Previews are private by default; making one public is a
              gated action in the policy.
            </p>
          </div>
        </Panel>
      </div>

      <Panel className="p-6">
        <SectionTitle
          title="Exposed ports across the fleet"
          vi="Cổng đang mở trên toàn hệ thống"
          right={
            <button onClick={load} className="inline-flex items-center gap-1.5 rounded-lg border border-line px-2.5 py-1.5 text-xs text-gray-300 transition-colors hover:bg-fg/5">
              <RefreshCw className="size-3" /> Refresh
            </button>
          }
        />
        <div className="mt-4">
          {loading ? (
            <p className="font-mono text-xs text-gray-500">loading…</p>
          ) : rows.length === 0 ? (
            <p className="py-6 text-center text-sm text-gray-500">No ports exposed anywhere: no sandbox has a preview yet.</p>
          ) : (
            <TableShell>
              <thead>
                <tr>
                  <Th>Sandbox</Th>
                  <Th>Status</Th>
                  <Th>Port</Th>
                  <Th>Preview URL</Th>
                  {write && <Th className="text-right">Action</Th>}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={`${r.id}-${r.port}`} className="transition-colors hover:bg-fg/[0.02]">
                    <Td>
                      <Mono className="text-xs text-gray-100">{r.name}</Mono>
                      <Mono className="ml-2 text-[10px] text-gray-600">{r.id}</Mono>
                    </Td>
                    <Td>
                      <span className={cn("inline-flex items-center gap-1.5 text-xs", r.status === "Running" ? "text-neon" : "text-gray-400")}>
                        <span className={cn("size-1.5 rounded-full", r.status === "Running" ? "bg-neon dot-live" : "bg-gray-500")} />
                        {r.status}
                      </span>
                    </Td>
                    <Td>
                      <Mono className="text-xs text-info">{r.port}</Mono>
                    </Td>
                    <Td>
                      <a href={r.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-info hover:underline">
                        {r.url.replace(/^https?:\/\//, "")} <ExternalLink className="size-3" />
                      </a>
                    </Td>
                    {write && (
                      <Td className="text-right">
                        {r.id === id ? (
                          <button
                            onClick={() => remove(r.port)}
                            disabled={busy}
                            className="rounded-lg border border-danger/40 px-2.5 py-1 text-[11px] text-danger transition-colors hover:bg-danger/10 disabled:opacity-40"
                          >
                            Remove
                          </button>
                        ) : (
                          <button
                            onClick={() => setId(r.id)}
                            className="rounded-lg border border-line px-2.5 py-1 text-[11px] text-gray-400 transition-colors hover:bg-fg/5"
                          >
                            manage in sandbox
                          </button>
                        )}
                      </Td>
                    )}
                  </tr>
                ))}
              </tbody>
            </TableShell>
          )}
        </div>
      </Panel>
    </>
  );
}

/* ══ 5 · Domain & TLS ═════════════════════════════════════════════════ */

interface Cert {
  id: string;
  subject: string;
  dns_names: string[];
  not_after: string;
  default: boolean;
}

function DomainTab() {
  const me = usePlatform((s) => s.me);
  const isAdmin = me?.role === "admin";
  const [data, setData] = useState<DomainInfo | null>(null);
  const [err, setErr] = useState("");
  const [form, setForm] = useState({ domain: "", tls_mode: "off", acme_email: "" });
  const [certs, setCerts] = useState<{ enabled: boolean; certificates: Cert[] } | null>(null);
  const [cert, setCert] = useState({ cert_pem: "", key_pem: "" });
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);

  const load = () => {
    setErr("");
    apiGet<DomainInfo>("/v1/domain")
      .then((d) => {
        setData(d);
        if (!dirty) setForm({ domain: d.domain, tls_mode: d.tls_mode, acme_email: d.acme_email ?? "" });
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)));
    apiGet<{ enabled: boolean; certificates: Cert[] }>("/v1/domain/certificates").then(setCerts).catch(() => setCerts(null));
  };
  useEffect(load, []);

  const editForm = (patch: Partial<typeof form>) => {
    setForm((f) => ({ ...f, ...patch }));
    setDirty(true);
  };

  const save = async () => {
    setBusy(true);
    try {
      await call(() => apiPut("/v1/domain", { domain: form.domain, tls_mode: form.tls_mode, ...(form.acme_email ? { acme_email: form.acme_email } : {}) }));
      toast.success("Domain saved · Đã lưu tên miền");
      setDirty(false);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not save the domain");
    } finally {
      setBusy(false);
    }
  };

  const upload = async () => {
    setBusy(true);
    try {
      await call(() => apiPost("/v1/domain/certificates", cert));
      toast.success("Certificate installed · Đã cài chứng chỉ");
      setCert({ cert_pem: "", key_pem: "" });
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not install the certificate");
    } finally {
      setBusy(false);
    }
  };

  if (err && !data) return <Panel className="p-6 text-sm text-danger">Could not load domain info: {err}</Panel>;
  if (!data) return <Panel className="p-6 font-mono text-xs text-gray-500">loading…</Panel>;

  return (
    <>
      <Panel className="p-6">
        <SectionTitle title="Console domain & TLS" vi="Tên miền và TLS" />
        {data.restart_required && (
          <p className="mt-3 rounded-lg border border-warn/30 bg-warn/10 px-3.5 py-2.5 text-xs leading-relaxed text-warn">
            Saved settings differ from what is running ({data.running.domain || "console host"}, TLS {data.running.tls_mode}).
            Restart sandboxd to apply them. · Cần khởi động lại để áp dụng.
          </p>
        )}
        <div className="mt-5 grid gap-5 md:grid-cols-3">
          <FieldRow label="Domain" vi="tên miền" hint="Apex or wildcard domain that fronts the console and previews.">
            <TextField value={form.domain} onChange={(v) => editForm({ domain: v })} placeholder="console.example.com" mono disabled={!isAdmin} />
          </FieldRow>
          <FieldRow label="TLS mode" vi="chế độ TLS">
            <FancySelect
              ariaLabel="TLS mode"
              value={form.tls_mode}
              onChange={(v) => editForm({ tls_mode: v })}
              disabled={!isAdmin}
              options={[
                { value: "off", label: "off (HTTP)", vi: "không mã hoá" },
                { value: "custom", label: "custom certificate", vi: "chứng chỉ riêng" },
                { value: "letsencrypt", label: "Let's Encrypt", vi: "tự động qua ACME" },
              ]}
            />
          </FieldRow>
          {form.tls_mode === "letsencrypt" && (
            <FieldRow label="ACME email" vi="email nhận thông báo" hint="Used by Let's Encrypt for expiry notices.">
              <TextField type="email" value={form.acme_email} onChange={(v) => editForm({ acme_email: v })} placeholder="ops@example.com" disabled={!isAdmin} />
            </FieldRow>
          )}
        </div>
        <p className="mt-4 text-xs text-gray-500">
          Console: <Mono className="text-gray-300">{data.console_host}</Mono> · previews:{" "}
          <Mono className="text-info">{data.preview_example}</Mono>
        </p>
        {isAdmin && (
          <button
            onClick={save}
            disabled={busy || !dirty}
            className="mt-5 inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2 text-sm font-semibold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_20px_var(--glow-mid)] disabled:pointer-events-none disabled:opacity-40"
          >
            <Save className="size-4" /> Save domain
          </button>
        )}
      </Panel>

      <Panel className="p-6">
        <SectionTitle title="Certificates" vi="Chứng chỉ TLS" right={<Mono className="text-[10px] text-gray-500">{certs?.enabled ? "storage ready" : "not configured"}</Mono>} />
        {!certs ? (
          <p className="mt-4 font-mono text-xs text-gray-500">loading…</p>
        ) : !certs.enabled ? (
          <p className="mt-3 text-xs leading-relaxed text-gray-500">
            Certificate storage is not configured (SANDBOXD_TRAEFIK_DYNAMIC_DIR). · Chưa cấu hình nơi lưu chứng chỉ.
          </p>
        ) : (
          <>
            {certs.certificates.length === 0 ? (
              <p className="mt-3 text-xs text-gray-500">No custom certificates.</p>
            ) : (
              <div className="mt-4">
                <TableShell className="min-w-0">
                  <thead>
                    <tr>
                      <Th>Id</Th>
                      <Th>DNS names</Th>
                      <Th>Expires</Th>
                      <Th>Flags</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {certs.certificates.map((c) => (
                      <tr key={c.id} className="transition-colors hover:bg-fg/[0.02]">
                        <Td>
                          <Mono className="text-xs text-gray-100">{c.id}</Mono>
                        </Td>
                        <Td>
                          <Mono className="text-[11px] text-gray-400">{c.dns_names.join(", ")}</Mono>
                        </Td>
                        <Td>
                          <Mono className="text-[11px] text-gray-500">{fmtTime(c.not_after)}</Mono>
                        </Td>
                        <Td>{c.default && <span className="rounded-full border border-neon/30 bg-neon/10 px-2 py-0.5 font-mono text-[10px] text-neon">default</span>}</Td>
                      </tr>
                    ))}
                  </tbody>
                </TableShell>
              </div>
            )}
            {isAdmin && (
              <div className="mt-5 grid gap-5 lg:grid-cols-2">
                <FieldRow label="Certificate (PEM, full chain)" vi="chứng chỉ">
                  <TextArea value={cert.cert_pem} onChange={(v) => setCert({ ...cert, cert_pem: v })} rows={5} mono placeholder="-----BEGIN CERTIFICATE-----" />
                </FieldRow>
                <FieldRow label="Private key (PEM)" vi="khóa riêng">
                  <TextArea value={cert.key_pem} onChange={(v) => setCert({ ...cert, key_pem: v })} rows={5} mono placeholder="-----BEGIN PRIVATE KEY-----" />
                </FieldRow>
                <div className="lg:col-span-2">
                  <button
                    onClick={upload}
                    disabled={busy || !cert.cert_pem || !cert.key_pem}
                    className="inline-flex items-center gap-2 rounded-lg border border-neon/30 bg-neon/10 px-4 py-2 text-sm font-medium text-neon transition-colors hover:bg-neon/20 disabled:pointer-events-none disabled:opacity-40"
                  >
                    <GitCompareArrows className="size-4" /> Install certificate
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </Panel>
    </>
  );
}
