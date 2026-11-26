import { useCallback, useEffect, useState } from "react";
import { Camera, Cpu, ExternalLink, FolderOpen, HardDrive, MemoryStick, Network, Pause, Play, RotateCw, SquareTerminal, Timer } from "lucide-react";
import { toast } from "sonner";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  createSnapshot,
  get as apiGet,
  getVolumes,
  patchLifecycle,
  post as apiPost,
  setKeepAlive,
  sse,
  type Sandbox as ApiSandbox,
  type VolumeInfo,
} from "@/console/api";
import { navigate as go } from "@/console/ui";
import { usePlatform } from "@/lib/platform-store";
import { cn } from "@/lib/utils";
import { filesSupported, viewSupported } from "@/lib/features";
import { resLabel } from "@/lib/platform-data";
import { AreaChart } from "../charts";
import { KpiCard, Mono, Panel, SectionTitle, StatusPill } from "../primitives";

/* Sandbox detail in the SBX template layout, fed only by sandboxd:
   GET /v1/sandboxes/{id} for the facts and the metrics SSE stream for the
   live numbers + charts. Exec/logs/keep-alive/delete live on the
   Terminal page and egress rules + live traffic on the Network page. */

interface Metrics {
  status: string;
  stats: { cpu_percent: number; mem_usage_bytes: number; mem_limit_bytes: number; pids: number };
}

const HISTORY = 90; // samples kept for the charts (≈3 min at 2s)

function fmtBytes(n: number): string {
  if (!n) return "0 B";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${u[i]}`;
}

function fmtTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return isNaN(d.getTime()) ? "—" : d.toLocaleString();
}

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-line-soft py-2.5 last:border-0">
      <Mono className="shrink-0 text-xs text-gray-500">{k}</Mono>
      <div className="min-w-0 text-right text-[13px] text-gray-200 [overflow-wrap:anywhere]">{children}</div>
    </div>
  );
}

const btn =
  "inline-flex items-center gap-2 rounded-lg border px-3.5 py-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40";

export function SandboxDetailView() {
  const sandbox = usePlatform((s) => s.sandboxes.find((b) => b.id === s.selectedSandboxId));
  const closeSandbox = usePlatform((s) => s.closeSandbox);
  const setSandboxStatus = usePlatform((s) => s.setSandboxStatus);
  const restartSandbox = usePlatform((s) => s.restartSandbox);
  const refreshAll = usePlatform((s) => s.refreshAll);
  const me = usePlatform((s) => s.me);
  const canWrite = me?.role === "admin" || me?.role === "operator";

  const id = sandbox?.id;
  const status = sandbox?.status;
  const [info, setInfo] = useState<ApiSandbox | null>(null);
  const [m, setM] = useState<Metrics | null>(null);
  const [cpu, setCpu] = useState<number[]>([]);
  const [mem, setMem] = useState<number[]>([]);

  // volumes (GET /v1/sandboxes/{id}/volumes)
  const [volumes, setVolumes] = useState<VolumeInfo[] | null>(null);
  // lifecycle edit state
  const [idleDraft, setIdleDraft] = useState("");
  const [lifeBusy, setLifeBusy] = useState(false);
  // snapshot capture dialog
  const [snapOpen, setSnapOpen] = useState(false);
  const [snapName, setSnapName] = useState("");
  const [snapBusy, setSnapBusy] = useState(false);
  // minictd keeps two kinds: template (clonable dir) and backup (tar.gz restore)
  const [snapKind, setSnapKind] = useState<"template" | "backup">("template");
  const sandboxdOnly = viewSupported("network"); // egress/lifecycle exist on sandboxd only

  const reloadInfo = useCallback(
    () =>
      apiGet<ApiSandbox>(`/v1/sandboxes/${id}`)
        .then((d) => setInfo(d))
        .catch(() => setInfo(null)),
    [id],
  );

  useEffect(() => {
    if (!id) return;
    let live = true;
    apiGet<ApiSandbox>(`/v1/sandboxes/${id}`)
      .then((d) => live && setInfo(d))
      .catch(() => live && setInfo(null));
    return () => {
      live = false;
    };
  }, [id, status]);

  /* volumes refresh with the sandbox status */
  useEffect(() => {
    if (!id) return;
    let live = true;
    getVolumes(id)
      .then((d) => live && setVolumes(d.volumes))
      .catch(() => live && setVolumes(null));
    return () => {
      live = false;
    };
  }, [id, status]);

  useEffect(() => {
    setCpu([]);
    setMem([]);
    setM(null);
    if (!id || status !== "Running") return;
    return sse(`/v1/sandboxes/${id}/metrics/stream?interval=2`, (line) => {
      try {
        const f: Metrics = JSON.parse(line);
        setM(f);
        const st = f.stats;
        if (st && st.mem_limit_bytes > 0) {
          setCpu((xs) => [...xs, st.cpu_percent].slice(-HISTORY));
          setMem((xs) => [...xs, (st.mem_usage_bytes / st.mem_limit_bytes) * 100].slice(-HISTORY));
        }
      } catch {
        /* partial frame */
      }
    });
  }, [id, status]);

  if (!sandbox) return null;
  const running = sandbox.status === "Running";
  const st = m?.stats;
  const ports = info?.ports ?? [];
  const lc = info?.lifecycle;
  const wsUsed = volumes?.find((v) => v.type === "workspace")?.used_bytes;

  /* POST/DELETE /v1/sandboxes/{id}/keep-alive */
  const toggleKeepAlive = async (on: boolean) => {
    if (!id) return;
    setLifeBusy(true);
    try {
      await setKeepAlive(id, on);
      toast.success(on ? "Keep-alive on" : "Keep-alive off", { description: on ? "The sandbox opts out of idle suspension." : "The sandbox will suspend when idle." });
      await reloadInfo();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not change keep-alive");
    } finally {
      setLifeBusy(false);
    }
  };

  /* PATCH /v1/sandboxes/{id}/lifecycle { idle_timeout_secs } */
  const saveIdle = async () => {
    if (!id || !idleDraft.trim()) return;
    const secs = Number(idleDraft);
    if (!(secs >= 0)) {
      toast.error("Idle timeout must be 0 or more seconds");
      return;
    }
    setLifeBusy(true);
    try {
      await patchLifecycle(id, { idle_timeout_secs: secs });
      toast.success("Idle timeout saved", { description: `${secs}s · applies to the next idle check` });
      setIdleDraft("");
      await reloadInfo();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not save the idle timeout");
    } finally {
      setLifeBusy(false);
    }
  };

  /* capture template: a running source must be stopped first (API 409s),
     so "stop & capture" stops, waits for the API to confirm, then POSTs */
  const capture = async (stopFirst: boolean) => {
    if (!id || !snapName.trim() || snapBusy) return;
    setSnapBusy(true);
    try {
      if (stopFirst) {
        await apiPost(`/v1/sandboxes/${id}/stop`);
        for (let i = 0; i < 20; i++) {
          const cur = await apiGet<ApiSandbox>(`/v1/sandboxes/${id}`);
          if (cur.status === "stopped") break;
          await new Promise((r) => setTimeout(r, 500));
        }
        await refreshAll();
      }
      const snap = await createSnapshot(id, snapName.trim(), snapKind);
      toast.success(`${snapKind === "backup" ? "Backup" : "Template"} "${snap.name}" captured`, {
        description: `Open Templates to ${snapKind === "backup" ? "restore" : "clone"} it into a new sandbox (${snap.id}).`,
      });
      setSnapOpen(false);
      setSnapName("");
      go("/templates");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Capture failed");
    } finally {
      setSnapBusy(false);
    }
  };

  return (
    <div className="space-y-5">
      {/* ── header ───────────────────────────────────────────────── */}
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <button
            onClick={() => {
              closeSandbox();
              go("/sandboxes");
            }}
            className="grid size-9 shrink-0 place-items-center rounded-lg border border-line text-gray-400 outline-none transition-colors hover:bg-fg/5 hover:text-gray-200 focus-visible:ring-2 focus-visible:ring-ring"
            aria-label="Back to sandbox list"
          >
            <span className="text-sm">←</span>
          </button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate text-[24px] font-bold tracking-tight text-ink">{sandbox.name}</h1>
              {sandbox.name !== sandbox.id && (
                <Mono className="rounded-md border border-line bg-surface px-2 py-1 text-xs text-info">{sandbox.id}</Mono>
              )}
              <span className="rounded-md border border-line bg-surface px-2.5 py-1.5">
                <StatusPill status={sandbox.status} />
              </span>
            </div>
            <Mono className="mt-1 block text-[11px] text-gray-500 [overflow-wrap:anywhere]">
              {sandbox.image} · node {sandbox.host} · {sandbox.env}
            </Mono>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2.5">
          <button onClick={() => go(`/sandboxes/${sandbox.id}/terminal`)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
            <SquareTerminal className="size-3.5" /> Terminal
          </button>
          {filesSupported() && (
            <button onClick={() => go(`/sandboxes/${sandbox.id}/files`)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
              <FolderOpen className="size-3.5" /> Files
            </button>
          )}
          {canWrite && (
            <button
              onClick={() => {
                setSnapName(`${sandbox.name}-${new Date().toISOString().slice(0, 10)}`);
                setSnapOpen(true);
              }}
              className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}
            >
              <Camera className="size-3.5" /> Snapshot
            </button>
          )}
          {sandboxdOnly && (
            <button
              onClick={() => {
                usePlatform.getState().setNetworkFocus(sandbox.id);
                go("/network");
              }}
              className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}
            >
              <Network className="size-3.5" /> Network
            </button>
          )}
          {canWrite && (
            <>
              <button
                onClick={() => restartSandbox(sandbox.id)}
                disabled={sandbox.status === "Starting"}
                className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}
              >
                <RotateCw className={cn("size-3.5", sandbox.status === "Starting" && "animate-spin")} /> Restart
              </button>
              {running ? (
                <button
                  onClick={() => setSandboxStatus(sandbox.id, "Stopped")}
                  className={cn(btn, "border-warn/40 bg-warn/10 text-warn hover:bg-warn/20")}
                >
                  <Pause className="size-3.5" /> Stop
                </button>
              ) : (
                <button
                  onClick={() => setSandboxStatus(sandbox.id, "Running")}
                  disabled={sandbox.status === "Starting"}
                  className={cn(btn, "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}
                >
                  <Play className="size-3.5" /> Start
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {info?.error_message && (
        <Panel className="border-danger/40 p-4 text-sm text-danger">{info.error_message}</Panel>
      )}

      {/* ── KPIs ─────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <KpiCard
          icon={<Cpu className="size-4" />}
          label="CPU"
          vi={sandbox.vcpu ? `limit ${sandbox.vcpu} vCPU` : "no CPU limit reported"}
          value={st ? `${st.cpu_percent.toFixed(1)}%` : "—"}
          accent="text-neon"
        />
        <KpiCard
          icon={<MemoryStick className="size-4" />}
          label="Memory"
          vi={st ? `of ${fmtBytes(st.mem_limit_bytes)}` : sandbox.memGb ? `limit ${sandbox.memGb} GB` : "—"}
          value={st ? fmtBytes(st.mem_usage_bytes) : "—"}
          accent="text-info"
        />
        <KpiCard
          icon={<SquareTerminal className="size-4" />}
          label="Processes"
          vi="pids"
          value={st ? st.pids : "—"}
        />
        <KpiCard icon={<Timer className="size-4" />} label="Uptime" vi="since created" value={sandbox.uptime} />
      </div>

      {/* ── charts ───────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {(
          [
            ["CPU usage", "Mức sử dụng CPU", cpu, "neon"],
            ["Memory usage", "Mức sử dụng bộ nhớ", mem, "cyan"],
          ] as const
        ).map(([title, vi, data, tone]) => (
          <Panel key={title} className="p-6">
            <SectionTitle title={title} vi={vi} right={<Mono className="text-[10px] text-gray-500">{running ? "live · 2s" : "stopped"}</Mono>} />
            {data.length > 1 ? (
              <AreaChart data={[...data]} tone={tone} height={200} showXLabels={false} />
            ) : (
              <div className="grid h-[200px] place-items-center text-sm text-gray-500">
                {running ? "Collecting samples…" : "Start the sandbox to see live metrics."}
              </div>
            )}
          </Panel>
        ))}
      </div>

      {/* ── facts ────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[2fr_1fr]">
        {/* GĐ3 mục 1 (issue #179): disk quota warning from daemon state notes */}
        {sandbox.stateNotes?.some((n) => /quota/i.test(n)) && (
          <div className="rounded-lg border border-warn/30 bg-warn/10 p-3 text-[11px] leading-relaxed text-warn">
            <strong>Disk quota:</strong>{" "}
            {sandbox.stateNotes.filter((n) => /quota/i.test(n)).join("; ")}
          </div>
        )}
        <Panel className="p-6">
          <SectionTitle title="Sandbox" vi="Thông tin sandbox" />
          <div className="mt-3">
            <Row k="id">
              <Mono>{sandbox.id}</Mono>
            </Row>
            <Row k="image">
              <Mono>{sandbox.image}</Mono>
            </Row>
            <Row k="command">
              <Mono>{info?.command?.length ? info.command.join(" ") : "image default"}</Mono>
            </Row>
            <Row k="resources">
              {resLabel(sandbox)}
            </Row>
            <Row k="node">{sandbox.host}</Row>
            <Row k="created">{fmtTime(info?.created_at)}</Row>
            <Row k="last active">{fmtTime(info?.last_active_at)}</Row>
            {sandboxdOnly && (
              <Row k="keep-alive">
                {info?.lifecycle?.keep_alive ? "on" : "off"}
                {info?.lifecycle ? ` · idle timeout ${info.lifecycle.idle_timeout_secs || "default"}s` : ""}
              </Row>
            )}
          </div>
        </Panel>
        <Panel className="p-6">
          <SectionTitle title="Network" vi="Mạng & cổng" />
          <div className="mt-3">
            {sandboxdOnly ? (
              <>
                <Row k="egress">{sandbox.env}</Row>
                <Row k="allow">{info?.egress?.length ? <Mono>{info.egress.join(", ")}</Mono> : "—"}</Row>
                <Row k="block">{info?.egress_deny?.length ? <Mono>{info.egress_deny.join(", ")}</Mono> : "—"}</Row>
              </>
            ) : (
              <Row k="netns">loopback + pasta port-forward</Row>
            )}
            <Row k="ports">{ports.length ? <Mono>{ports.join(", ")}</Mono> : "none"}</Row>
            <Row k="preview">
              {info?.preview?.url ? (
                <a href={info.preview.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-info hover:underline">
                  {info.preview.url.replace(/^https?:\/\//, "")} <ExternalLink className="size-3" />
                </a>
              ) : (
                "—"
              )}
            </Row>
          </div>
          {sandboxdOnly && <button
            onClick={() => {
              usePlatform.getState().setNetworkFocus(sandbox.id);
              go("/network");
            }}
            className="mt-4 inline-flex w-full items-center justify-center gap-2 rounded-lg border border-neon/30 bg-neon/10 px-4 py-2 text-sm text-neon transition-colors hover:bg-neon/20"
          >
            <Network className="size-4" /> Rules & live traffic
          </button>}
        </Panel>
      </div>

      {/* ── lifecycle + volumes ─────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {sandboxdOnly && <Panel className="p-6">
          <SectionTitle
            title="Lifecycle"
            vi="Vòng đời & giữ phiên"
            right={<Mono className="text-[10px] text-gray-500">PATCH /lifecycle · POST /keep-alive</Mono>}
          />
          <div className="mt-4">
            <div className="flex items-center justify-between gap-4 border-b border-line-soft pb-3.5">
              <div className="min-w-0">
                <div className="text-sm text-gray-200">Keep alive</div>
                <p className="mt-0.5 text-xs text-gray-500">Opts this sandbox out of idle suspension — it never reaps while on.</p>
              </div>
              <Switch
                checked={!!lc?.keep_alive}
                onCheckedChange={(v) => toggleKeepAlive(v)}
                disabled={!canWrite || lifeBusy}
                aria-label="Keep alive"
                className="data-[state=checked]:bg-neon data-[state=unchecked]:bg-line"
              />
            </div>
            <div className="flex flex-wrap items-end justify-between gap-3 pt-3.5">
              <div className="min-w-0">
                <div className="text-sm text-gray-200">Idle timeout</div>
                <p className="mt-0.5 text-xs text-gray-500">
                  Current: <Mono className="text-gray-300">{lc?.idle_timeout_secs ? `${lc.idle_timeout_secs}s` : "instance default"}</Mono>
                </p>
              </div>
              <div className="flex items-center gap-2">
                <input
                  value={idleDraft}
                  onChange={(e) => setIdleDraft(e.target.value.replace(/[^0-9]/g, ""))}
                  placeholder="seconds"
                  inputMode="numeric"
                  aria-label="Idle timeout in seconds"
                  className="h-9 w-28 rounded-lg border border-line bg-navy/60 px-3 font-mono text-xs text-gray-100 placeholder:text-gray-600 outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
                <button
                  onClick={saveIdle}
                  disabled={!canWrite || lifeBusy || !idleDraft.trim()}
                  className={cn(btn, "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}
                >
                  Save
                </button>
              </div>
            </div>
          </div>
        </Panel>}

        <Panel className={cn("p-6", !sandboxdOnly && "xl:col-span-2")}>
          <SectionTitle
            title="Volumes"
            vi="Lưu trữ gắn vào"
            right={
              <Mono className="text-[10px] text-gray-500">
                {wsUsed ? `${fmtBytes(wsUsed)} in workspace` : "GET /volumes"}
              </Mono>
            }
          />
          <div className="mt-3">
            {(volumes ?? []).map((v) => (
              <Row key={`${v.type}-${v.destination}`} k={v.destination}>
                <span className="inline-flex flex-wrap items-center justify-end gap-2">
                  <span className="rounded-md border border-line bg-surface px-1.5 py-0.5 font-mono text-[10px] text-gray-400">{v.type}</span>
                  {v.read_only && <span className="rounded-md border border-warn/30 bg-warn/10 px-1.5 py-0.5 font-mono text-[10px] text-warn">read-only</span>}
                  {v.used_bytes !== undefined && <Mono className="text-[11px] text-gray-400">{fmtBytes(v.used_bytes)}</Mono>}
                </span>
              </Row>
            ))}
            {!volumes && <p className="py-2 text-xs text-gray-500">Volumes unavailable for this sandbox.</p>}
            {sandboxdOnly && (
              <p className="pt-2 text-[11px] text-gray-600">
                The workspace volume is what a template snapshot freezes — caches like node_modules are excluded.
              </p>
            )}
          </div>
        </Panel>
      </div>

      {/* ── snapshot capture dialog ─────────────────────────────── */}
      <Dialog open={snapOpen} onOpenChange={setSnapOpen}>
        <DialogContent className="border-line bg-surface sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-ink">Snapshot {sandbox.name}</DialogTitle>
            <DialogDescription className="text-gray-500">
              Template: sao chép rootfs để tạo instance mới. Backup: nén tar.gz để khôi phục sau.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-1">
            <fieldset className="grid grid-cols-2 gap-2">
              <legend className="mb-1.5 text-xs text-gray-400">Kind · Loại</legend>
              {(["template", "backup"] as const).map((k) => (
                <label
                  key={k}
                  className={cn(
                    "flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm focus-within:ring-2 focus-within:ring-ring",
                    snapKind === k ? "border-neon/40 bg-neon/10 text-neon" : "border-line text-gray-300 hover:bg-fg/5",
                  )}
                >
                  <input type="radio" name="snap-kind" value={k} checked={snapKind === k} onChange={() => setSnapKind(k)} className="sr-only" />
                  {k === "template" ? "Template" : "Backup (tar.gz)"}
                </label>
              ))}
            </fieldset>
            <div className="space-y-1.5">
              <label className="text-xs text-gray-400" htmlFor="snap-name">
                Name · Tên <span className="text-gray-600">(a-z 0-9 . _ -)</span>
              </label>
              <input
                id="snap-name"
                value={snapName}
                onChange={(e) => setSnapName(e.target.value)}
                className="h-10 w-full rounded-lg border border-line bg-navy/60 px-3 font-mono text-sm text-gray-100 placeholder:text-gray-600 outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>
            {running ? (
              <div className="rounded-lg border border-warn/30 bg-warn/10 p-3 text-[11px] leading-relaxed text-warn">
                The sandbox is running — a snapshot needs a stopped source (the API answers 409 for a live workspace).
                &quot;Stop &amp; capture&quot; stops it first, waits for the API to confirm, then captures.
              </div>
            ) : (
              <p className="text-[11px] text-gray-500">
                Source is stopped — minictd can capture right away.
              </p>
            )}
          </div>
          <DialogFooter>
            <button onClick={() => setSnapOpen(false)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
              Cancel
            </button>
            {running ? (
              <button
                onClick={() => capture(true)}
                disabled={snapBusy || !snapName.trim()}
                className={cn(btn, "border-warn/40 bg-warn/10 text-warn hover:bg-warn/20")}
              >
                <Camera className="size-3.5" /> {snapBusy ? "Stopping & capturing…" : "Stop & capture"}
              </button>
            ) : (
              <button
                onClick={() => capture(false)}
                disabled={snapBusy || !snapName.trim()}
                className={cn(btn, "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}
              >
                <Camera className="size-3.5" /> {snapBusy ? "Capturing…" : "Capture"}
              </button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
