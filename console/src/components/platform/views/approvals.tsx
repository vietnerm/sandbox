import { useCallback, useEffect, useMemo, useState } from "react";
import { CheckCircle2, ChevronRight, Clock, ShieldCheck, XCircle } from "lucide-react";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { get as apiGet, post as apiPost, type Approval } from "@/console/api";
import { usePlatform } from "@/lib/platform-store";
import { cn } from "@/lib/utils";
import { KpiCard, Mono, PageHeader, Panel, Segmented } from "../primitives";

/* Approvals — the gated-action workflow as a first-class ops page
   (GET /v1/approvals?status=…, POST /v1/approvals/{id}/approve|deny).
   Gated requests are parked here as HTTP 202 with an approval_id; an
   admin approves or denies with an optional reason that lands in the
   audit chain. Settings keeps a compact link card; this page is the
   working surface. */

type Filter = "pending" | "approved" | "denied" | "all";

function fmtWhen(secs: number): string {
  const d = new Date(secs * 1000);
  return isNaN(d.getTime()) ? "—" : d.toLocaleString();
}

const btn =
  "inline-flex items-center gap-2 rounded-lg border px-3.5 py-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40";

export function ApprovalsView() {
  const me = usePlatform((s) => s.me);
  const canDecide = me?.role === "admin";

  const [filter, setFilter] = useState<Filter>("pending");
  const [all, setAll] = useState<Approval[] | null>(null);
  const [error, setError] = useState("");

  // decision dialog
  const [target, setTarget] = useState<{ a: Approval; verb: "approve" | "deny" } | null>(null);
  const [reason, setReason] = useState("");
  const [deciding, setDeciding] = useState(false);

  const load = useCallback(() => {
    // one fetch of the whole queue — the KPI counters reflect everything and
    // the filter tabs slice client-side, so counts never depend on the tab
    apiGet<{ approvals: Approval[] }>("/v1/approvals")
      .then((d) => {
        setAll(d.approvals);
        setError("");
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  useEffect(load, [load]);

  const approvals = useMemo(
    () => (filter === "all" ? all : (all ?? []).filter((a) => a.status === filter)),
    [all, filter],
  );

  const counts = useMemo(() => {
    const c = { pending: 0, approved: 0, denied: 0 };
    for (const a of all ?? []) if (c[a.status as keyof typeof c] !== undefined) c[a.status as keyof typeof c]++;
    return c;
  }, [all]);

  const decide = async () => {
    if (!target || deciding) return;
    setDeciding(true);
    try {
      await apiPost(`/v1/approvals/${target.a.id}/${target.verb}`, reason.trim() ? { reason: reason.trim() } : {});
      toast.success(
        target.verb === "approve" ? `Approved ${target.a.action} · Đã phê duyệt` : `Denied ${target.a.action} · Đã từ chối`,
        { description: target.a.target },
      );
      setTarget(null);
      setReason("");
      load();
      // refresh /v1/overview so the sidebar pending badge follows immediately
      void usePlatform.getState().refreshAll();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Decision failed");
    } finally {
      setDeciding(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Approvals"
        subtitle="Gated actions waiting for an admin decision"
        vi="Yêu cầu cần phê duyệt"
        actions={
          <Segmented
            options={[
              { value: "pending", label: "Pending" },
              { value: "approved", label: "Approved" },
              { value: "denied", label: "Denied" },
              { value: "all", label: "All" },
            ]}
            value={filter}
            onChange={setFilter}
          />
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <KpiCard icon={<Clock className="size-4" />} label="Pending" vi="đang chờ" value={counts.pending} accent={counts.pending ? "text-warn" : "text-data"} />
        <KpiCard icon={<ShieldCheck className="size-4" />} label="Approved" vi="đã duyệt" value={counts.approved} accent="text-neon" />
        <KpiCard icon={<XCircle className="size-4" />} label="Denied" vi="đã từ chối" value={counts.denied} accent="text-danger" />
      </div>

      {error && <Panel className="p-6 text-sm text-danger">Could not load approvals: {error}</Panel>}

      <Panel className="p-6">
        <div className="divide-y divide-line-soft">
          {(approvals ?? []).map((a) => {
            const payload = (() => {
              try {
                return JSON.stringify(JSON.parse(a.payload), null, 0);
              } catch {
                return a.payload;
              }
            })();
            return (
              <div key={a.id} className="py-4 first:pt-0 last:pb-0">
                <div className="flex flex-wrap items-center gap-3">
                  <Mono className="text-xs text-info">{a.action}</Mono>
                  <Mono className="text-xs text-gray-500">{a.target}</Mono>
                  <span
                    className={cn(
                      "rounded-full border px-2 py-0.5 font-mono text-[10px] capitalize",
                      a.status === "pending" ? "border-warn/30 bg-warn/10 text-warn" : a.status === "approved" ? "border-neon/30 bg-neon/10 text-neon" : "border-danger/30 bg-danger/10 text-danger",
                    )}
                  >
                    {a.status}
                  </span>
                  <span className="ml-auto text-xs text-gray-500">
                    by <Mono className="text-gray-300">{a.requested_by}</Mono> · {fmtWhen(a.created_at)}
                  </span>
                </div>
                {payload && payload !== "undefined" && (
                  <Mono className="mt-2 block max-w-full truncate text-[11px] text-gray-600">{payload}</Mono>
                )}
                {a.reason && (
                  <p className="mt-1.5 text-xs text-gray-500">
                    reason — <span className="text-gray-400">{a.reason}</span>
                    {a.decided_by && (
                      <>
                        {" "}
                        · <Mono className="text-gray-500">{a.decided_by}</Mono>
                      </>
                    )}
                  </p>
                )}
                {a.status === "pending" && canDecide && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button onClick={() => setTarget({ a, verb: "approve" })} className={cn(btn, "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20")}>
                      <CheckCircle2 className="size-3.5" /> Approve
                    </button>
                    <button onClick={() => setTarget({ a, verb: "deny" })} className={cn(btn, "border-danger/40 bg-danger/10 text-danger hover:bg-danger/20")}>
                      <XCircle className="size-3.5" /> Deny
                    </button>
                  </div>
                )}
                {a.status === "pending" && !canDecide && (
                  <p className="mt-2 text-[11px] text-gray-600">Waiting for an admin decision.</p>
                )}
              </div>
            );
          })}
          {approvals && approvals.length === 0 && (
            <div className="flex flex-col items-center gap-2 py-10 text-center">
              <ShieldCheck className="size-5 text-gray-500" />
              <p className="text-sm text-gray-400">Nothing {filter === "all" ? "here" : filter} · Không có yêu cầu nào.</p>
              <p className="max-w-md text-xs text-gray-600">
                Requests parked by the policy engine (HTTP 202 with an approval_id) appear here until an admin decides.
              </p>
            </div>
          )}
          {!approvals && !error && <p className="py-4 text-sm text-gray-500">Loading approvals…</p>}
        </div>
      </Panel>

      {/* decision dialog */}
      <Dialog open={!!target} onOpenChange={(v) => !v && setTarget(null)}>
        <DialogContent className="border-line bg-surface sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-ink">
              {target?.verb === "approve" ? "Approve request" : "Deny request"}
            </DialogTitle>
            <DialogDescription className="font-mono text-xs text-gray-500">
              {target?.a.action} → {target?.a.target}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5 py-1">
            <label className="text-xs text-gray-400" htmlFor="apr-reason">
              Reason <span className="text-gray-600">· optional, stored in the audit chain</span>
            </label>
            <textarea
              id="apr-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              placeholder={target?.verb === "approve" ? "e.g. confirmed with team lead" : "e.g. host not on the approved list"}
              className="w-full resize-none rounded-lg border border-line bg-navy/60 px-3 py-2 text-sm text-gray-100 placeholder:text-gray-600 outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <p className="flex items-start gap-1.5 pt-1 text-[11px] text-gray-600">
              <ChevronRight className="mt-0.5 size-3 shrink-0" />
              POST <Mono className="text-gray-500">/v1/approvals/{target?.a.id}/{target?.verb}</Mono>
            </p>
          </div>
          <DialogFooter>
            <button onClick={() => setTarget(null)} className={cn(btn, "border-line bg-surface text-gray-300 hover:bg-fg/5")}>
              Cancel
            </button>
            <button
              onClick={decide}
              disabled={deciding}
              className={cn(
                btn,
                target?.verb === "approve" ? "border-neon/40 bg-neon/10 text-neon hover:bg-neon/20" : "border-danger/40 bg-danger/10 text-danger hover:bg-danger/20",
              )}
            >
              {target?.verb === "approve" ? <CheckCircle2 className="size-3.5" /> : <XCircle className="size-3.5" />}
              {deciding ? "Saving…" : target?.verb === "approve" ? "Approve" : "Deny"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
