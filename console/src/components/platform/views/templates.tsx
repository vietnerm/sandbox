import { useCallback, useEffect, useMemo, useState } from "react";
import { CalendarClock, Camera, ChevronRight, Database, HardDrive, Layers, Plus, Rocket, Trash2 } from "lucide-react";
import { toast } from "sonner";
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
  deleteSnapshot,
  listSnapshots,
  type SnapshotInfo,
  isDemo,
} from "@/console/api";
import { navigate as go } from "@/console/ui";
import { usePlatform } from "@/lib/platform-store";
import { cn } from "@/lib/utils";
import { KpiCard, Mono, PageHeader, Panel } from "../primitives";

/* Templates — the snapshots-as-templates library. Every card is a frozen
   workspace copy the control plane stores under its LibraryRoot:
   GET/POST /v1/snapshots + DELETE /v1/snapshots/{id}. A new sandbox is
   cloned from one by passing from_snapshot on POST /v1/sandboxes. The
   source sandbox must be stopped before capture (the API answers 409
   for a running source, exactly like sandboxd does). */

function fmtBytes(n?: number): string {
  if (!n || n < 0) return "—";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${u[i]}`;
}

function fmtDate(iso: string): string {
  const d = new Date(iso);
  return isNaN(d.getTime()) ? "—" : d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

const btn =
  "inline-flex items-center gap-2 rounded-lg border px-3.5 py-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40";

export function TemplatesView() {
  const sandboxes = usePlatform((s) => s.sandboxes);
  const refreshAll = usePlatform((s) => s.refreshAll);

  const [snaps, setSnaps] = useState<SnapshotInfo[] | null>(null);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);

  // create dialog state
  const [open, setOpen] = useState(false);
  const [sourceId, setSourceId] = useState("");
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);

  const load = useCallback(() => {
    listSnapshots()
      .then((d) => {
        setSnaps(d.snapshots);
        setError("");
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  useEffect(load, [load]);

  const stopped = useMemo(
    () => sandboxes.filter((b) => b.status === "Stopped" || b.status === "Error"),
    [sandboxes],
  );

  const totalBytes = useMemo(() => (snaps ?? []).reduce((a, s) => a + (s.size_bytes ?? 0), 0), [snaps]);
  const latest = useMemo(
    () => [...(snaps ?? [])].sort((a, b) => (a.created_at < b.created_at ? 1 : -1))[0],
    [snaps],
  );

  const openDialog = () => {
    setSourceId(stopped[0]?.id ?? "");
    setName("");
    setOpen(true);
  };

  const create = async () => {
    if (!sourceId || !name.trim() || creating) return;
    setCreating(true);
    try {
      const snap = await createSnapshot(sourceId, name.trim());
      toast.success(`Template "${snap.name}" captured`, { description: `${fmtBytes(snap.size_bytes)} · ready to clone` });
      setOpen(false);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Capture failed");
    } finally {
      setCreating(false);
    }
  };

  const remove = async (snap: SnapshotInfo) => {
    if (!confirm(`Delete template "${snap.name}"? Sandboxes cloned from it are independent copies and stay unaffected.`)) return;
    setBusyId(snap.id);
    try {
      await deleteSnapshot(snap.id);
      toast.success(`Deleted ${snap.name}`);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Delete failed");
    } finally {
      setBusyId(null);
    }
  };

  const sourceName = (id?: string) => {
    const s = sandboxes.find((b) => b.id === id);
    return s?.name ?? id ?? "—";
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Templates"
        subtitle="Frozen workspace copies you can clone into new sandboxes"
        vi="Thư viện mẫu — bản chép không gian làm việc"
        actions={
          <button
            onClick={openDialog}
            className="inline-flex items-center gap-2 rounded-lg bg-neon px-4 py-2.5 text-sm font-bold text-navy transition-all hover:bg-neon/90 hover:shadow-[0_0_24px_var(--glow-mid)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Plus className="size-4" /> New template
          </button>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <KpiCard icon={<Layers className="size-4" />} label="Templates" vi="mẫu khả dụng" value={snaps?.length ?? "—"} accent="text-neon" />
        <KpiCard icon={<HardDrive className="size-4" />} label="Library size" vi="dung lượng thư viện" value={fmtBytes(totalBytes)} accent="text-info" />
        <KpiCard
          icon={<CalendarClock className="size-4" />}
          label="Last capture"
          vi="lần chụp gần nhất"
          value={latest ? fmtDate(latest.created_at) : "—"}
        />
      </div>

      {error && <Panel className="p-6 text-sm text-danger">Could not load templates: {error}</Panel>}

      {/* template grid */}
      {snaps && snaps.length > 0 && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {snaps.map((s) => (
            <Panel key={s.id} className="flex min-w-0 flex-col gap-4 p-5">
              <div className="flex items-start justify-between gap-3">
                <div className="grid size-10 shrink-0 place-items-center rounded-lg border border-line bg-navy/60">
                  <Database className="size-4.5 text-info" />
                </div>
                <span
                  className={cn(
                    "rounded-full border px-2 py-0.5 font-mono text-[10px] capitalize",
                    s.status === "ready" ? "border-neon/30 bg-neon/10 text-neon" : s.status === "error" ? "border-danger/30 bg-danger/10 text-danger" : "border-warn/30 bg-warn/10 text-warn",
                  )}
                >
                  {s.status}
                </span>
              </div>
              <div className="min-w-0">
                <h3 className="truncate text-sm font-semibold text-ink">{s.name}</h3>
                <Mono className="mt-1 block truncate text-[11px] text-gray-500">{isDemo() ? s.base_image : s.id}</Mono>
              </div>
              <div className="space-y-1.5 border-t border-line-soft pt-3 text-[11px] text-gray-500">
                <div className="flex items-center justify-between gap-2">
                  <span>size</span>
                  <Mono className="text-gray-300">{fmtBytes(s.size_bytes)}</Mono>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <span>source</span>
                  <Mono className="truncate text-gray-300">{sourceName(s.source_sandbox_id)}</Mono>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <span>created</span>
                  <Mono className="text-gray-300">{fmtDate(s.created_at)}</Mono>
                </div>
              </div>
              <div className="flex items-center gap-2 pt-0.5">
                <button
                  onClick={() => go(`/sandboxes/new?from=${encodeURIComponent(s.id)}`)}
                  className={cn(btn, "flex-1 justify-center border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}
                >
                  <Rocket className="size-3.5" /> {s.base_image === "backup" ? "Restore" : "New sandbox"}
                </button>
                <button
                  onClick={() => remove(s)}
                  disabled={busyId === s.id}
                  className={cn(btn, "border-line bg-surface text-gray-400 hover:border-danger/40 hover:text-danger")}
                  aria-label={`Delete template ${s.name}`}
                >
                  <Trash2 className="size-3.5" />
                </button>
              </div>
            </Panel>
          ))}
        </div>
      )}

      {/* empty state */}
      {snaps && snaps.length === 0 && (
        <Panel className="flex flex-col items-center gap-3 p-12 text-center">
          <div className="grid size-12 place-items-center rounded-xl border border-line bg-navy/60">
            <Camera className="size-5 text-gray-500" />
          </div>
          <h3 className="text-sm font-semibold text-ink">No templates yet</h3>
          <p className="max-w-md text-sm text-gray-500">
            Stop a sandbox, then capture its workspace as a reusable template. New sandboxes cloned from a template boot
            with the same files — node_modules and caches are re-installed on first start.
          </p>
          <button onClick={openDialog} className={cn(btn, "mt-1 border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}>
            <Plus className="size-3.5" /> Capture the first template
          </button>
        </Panel>
      )}

      {/* create dialog */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="border-line bg-surface sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-ink">Capture a template</DialogTitle>
            <DialogDescription className="text-gray-500">
              Chụp bản mẫu từ một sandbox đã dừng — toàn bộ workspace sẽ được đóng băng thành thư viện.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-1">
            <div className="space-y-1.5">
              <label className="text-xs text-gray-400" htmlFor="tpl-source">
                Source sandbox <span className="text-gray-600">· must be stopped</span>
              </label>
              <select
                id="tpl-source"
                value={sourceId}
                onChange={(e) => setSourceId(e.target.value)}
                className="h-10 w-full rounded-lg border border-line bg-navy/60 px-3 font-mono text-sm text-gray-100 outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="" disabled>
                  {stopped.length ? "choose a sandbox" : "no stopped sandboxes"}
                </option>
                {stopped.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.name} — {b.id}
                  </option>
                ))}
              </select>
              {stopped.length === 0 && (
                <p className="text-[11px] text-warn">
                  Running sandboxes must be stopped first — the API refuses a live source with 409.
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-gray-400" htmlFor="tpl-name">
                Template name
              </label>
              <input
                id="tpl-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. node22-ci-base"
                className="h-10 w-full rounded-lg border border-line bg-navy/60 px-3 font-mono text-sm text-gray-100 placeholder:text-gray-600 outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>
            <div className="flex items-start gap-2 rounded-lg border border-line bg-navy/40 p-3">
              <ChevronRight className="mt-0.5 size-3.5 shrink-0 text-gray-500" />
              <p className="text-[11px] leading-relaxed text-gray-500">
                {isDemo() ? (
                  <>
                    POST <Mono className="text-gray-400">/v1/snapshots</Mono> {"{ source_sandbox_id, name }"} — the copy
                    excludes <Mono className="text-gray-400">node_modules</Mono>,{" "}
                    <Mono className="text-gray-400">.next</Mono>, <Mono className="text-gray-400">.venv</Mono> and other
                    generated caches.
                  </>
                ) : (
                  <>
                    POST <Mono className="text-gray-400">/v1/sandboxes/&lt;name&gt;/save-template</Mono> — copies the whole
                    rootfs of a stopped instance; clone it later as image{" "}
                    <Mono className="text-gray-400">template:&lt;name&gt;</Mono>.
                  </>
                )}
              </p>
            </div>
          </div>
          <DialogFooter>
            <button onClick={() => setOpen(false)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
              Cancel
            </button>
            <button
              onClick={create}
              disabled={!sourceId || !name.trim() || creating}
              className={cn(btn, "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}
            >
              <Camera className="size-3.5" /> {creating ? "Capturing…" : "Capture"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
