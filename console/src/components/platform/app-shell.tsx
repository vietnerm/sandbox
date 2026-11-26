"use client";

import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { type ViewKey } from "@/lib/platform-data";
import { usePlatform } from "@/lib/platform-store";
import { useTheme } from "@/lib/theme-store";
import { SidebarContent } from "./sidebar";
import { Topbar } from "./topbar";
import { CommandPalette } from "./command-palette";
import { OverviewView } from "./views/overview";
import { UsersView } from "./views/users";
import { CreateSandboxView } from "./views/create-sandbox";
import { MonitoringView } from "./views/monitoring";
import { SandboxListView } from "./views/sandbox-list";
import { SandboxDetailView } from "./views/sandbox-detail";
import { NodeDetailView, NodesView } from "./views/nodes";
import { NetworkView } from "./views/network";
import { SettingsView, settingsSectionFromPath } from "./views/settings";
import { navigate as go, usePath } from "@/console/ui";
import { TerminalView } from "./views/sandbox-terminal";
import { FileManagerView } from "./views/sandbox-files";
import { TemplatesView } from "./views/templates";
import { ApprovalsView } from "./views/approvals";
import { UnsupportedView } from "./unsupported";
import { filesSupported, viewSupported } from "@/lib/features";

/* The SPA shell owns every path. Template views render in-page; the ops
   pages are a sandbox's realtime terminal (PTY) and its workspace file
   manager (FTP). The old /admin/<tab> console is gone — each tab now
   maps to a first-class view (users → Users, nodes → Nodes, domain →
   Network, the rest → Settings). */
type OpsRoute = { kind: "terminal" | "files"; id: string };

function opsFromPath(pathname: string): OpsRoute | null {
  const seg = pathname.split("/").filter(Boolean);
  if (seg[0] === "sandboxes" && seg[1] && seg[2] === "terminal") return { kind: "terminal", id: seg[1] };
  if (seg[0] === "sandboxes" && seg[1] && seg[2] === "files") return { kind: "files", id: seg[1] };
  return null;
}

/** sidebar entry an ops page belongs to */
function opsNav(_: OpsRoute): ViewKey {
  return "sandboxes";
}

/** map a URL path to the template view so deep links survive a hard refresh */
function viewFromPath(pathname: string): {
  view: ViewKey;
  sandboxId: string | null;
  nodeId: string | null;
} {
  const seg = pathname.split("/").filter(Boolean);
  const [head, second, third] = seg;
  switch (head) {
    case undefined:
    case "overview":
      return { view: "overview", sandboxId: null, nodeId: null };
    case "users":
      return { view: "users", sandboxId: null, nodeId: null };
    case "monitoring":
      return { view: "monitoring", sandboxId: null, nodeId: null };
    case "network":
      return { view: "network", sandboxId: null, nodeId: null };
    case "create":
      return { view: "create", sandboxId: null, nodeId: null };
    case "templates":
      return { view: "templates", sandboxId: null, nodeId: null };
    case "approvals":
      return { view: "approvals", sandboxId: null, nodeId: null };
    case "settings":
      // /settings/approvals moved out of settings — the queue is a first-class page now
      if (second === "approvals") return { view: "approvals", sandboxId: null, nodeId: null };
      return { view: "settings", sandboxId: null, nodeId: null };
    case "nodes":
      return { view: "nodes", sandboxId: null, nodeId: second ?? null };
    case "sandboxes":
      if (second === "new") return { view: "create", sandboxId: null, nodeId: null };
      // legacy deep links fold into the new hub views
      if (third === "network") return { view: "network", sandboxId: second ?? null, nodeId: null };
      return { view: "sandboxes", sandboxId: second ?? null, nodeId: null };
    case "admin":
      // /admin/<tab> — the old console's routes, now first-class views
      if (second === "users") return { view: "users", sandboxId: null, nodeId: null };
      if (second === "nodes") return { view: "nodes", sandboxId: null, nodeId: null };
      if (second === "domain") return { view: "network", sandboxId: null, nodeId: null };
      if (second === "approvals") return { view: "approvals", sandboxId: null, nodeId: null };
      return { view: "settings", sandboxId: null, nodeId: null }; // policy / audit / keys
    default:
      return { view: "overview", sandboxId: null, nodeId: null };
  }
}

const viewPath = (v: ViewKey) => (v === "create" ? "/sandboxes/new" : `/${v}`);

/** the URL a settings view should sit on (keeps the sub-section) */
function canonicalSettingsPath(): string {
  const s = settingsSectionFromPath(location.pathname);
  return s && s !== "general" ? `/settings/${s}` : "/settings";
}

export function AppShell() {
  const [boot] = useState(() => viewFromPath(window.location.pathname));
  const [view, setView] = useState<ViewKey>(boot.view);
  const [nodeId, setNodeId] = useState<string | null>(boot.nodeId);
  const path = usePath();
  const ops = opsFromPath(path);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [mobileNav, setMobileNav] = useState(false);
  const selectedSandboxId = usePlatform((s) => s.selectedSandboxId);

  /* sync theme store with the class set by the no-flash boot script */
  useEffect(() => {
    useTheme.getState().hydrate();
  }, []);

  /* live metric polling — KPIs, charts and status pills follow /v1 */
  useEffect(() => {
    const t = setInterval(() => void usePlatform.getState().pollLive(), 5000);
    return () => clearInterval(t);
  }, []);

  /* deep link /sandboxes/<id> — open the detail when the id exists
     (the store is already populated by PlatformRoot before we render) */
  useEffect(() => {
    if (boot.view === "sandboxes" && boot.sandboxId && usePlatform.getState().sandboxes.some((b) => b.id === boot.sandboxId))
      usePlatform.getState().openSandbox(boot.sandboxId);
  }, []);

  /* legacy deep links that now live in the Network hub:
     /sandboxes/<id>/network opens the hub focused on that sandbox */
  useEffect(() => {
    if (boot.view === "network" && boot.sandboxId) usePlatform.getState().setNetworkFocus(boot.sandboxId);
  }, []);

  /* keep the URL in sync so a refresh reopens the same view — but never
     stomp a path that already lives under this view (/nodes/<id>,
     /settings/<section>, /sandboxes/<id>?from=<snapshot> survive refresh +
     back/forward). The viewFromPath guard also stops the intermediate
     render (view state still lagging the new path) from rewriting the URL. */
  useEffect(() => {
    const loc = location.pathname;
    if (opsFromPath(loc)) return; // an ops page owns the URL
    if (viewFromPath(loc).view !== view) return; // URL and view disagree — trust the URL
    const target = view === "settings" ? canonicalSettingsPath() : viewPath(view);
    if (loc === target) return;
    const head = loc.split("/").filter(Boolean)[0];
    const targetHead = target.split("/").filter(Boolean)[0];
    if (head === targetHead) return;
    history.replaceState(null, "", target);
  }, [view, selectedSandboxId, path]);

  /* back/forward between template views (popstate) */
  useEffect(() => {
    if (ops) return;
    const v = viewFromPath(path);
    setView(v.view);
    setNodeId(v.nodeId);
    if (v.view === "sandboxes" && v.sandboxId) usePlatform.getState().openSandbox(v.sandboxId);
    if (v.view === "sandboxes" && !v.sandboxId) usePlatform.getState().closeSandbox();
    if (v.view === "network" && v.sandboxId) usePlatform.getState().setNetworkFocus(v.sandboxId);
  }, [path]);

  const navigate = (v: ViewKey) => {
    if (v !== "sandboxes") usePlatform.getState().closeSandbox();
    setView(v);
    setNodeId(null);
    setMobileNav(false);
    go(viewPath(v));
  };

  const activeNav: ViewKey = ops ? opsNav(ops) : view === "create" ? "sandboxes" : view;

  return (
    <div className="min-h-screen bg-navy">
      {/* desktop sidebar */}
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-[220px] border-r border-line bg-navy lg:block">
        <SidebarContent active={activeNav} onNavigate={navigate} />
      </aside>

      {/* mobile sidebar */}
      <Sheet open={mobileNav} onOpenChange={setMobileNav}>
        <SheetContent side="left" className="w-[240px] p-0 bg-navy border-line">
          <SheetHeader className="sr-only">
            <SheetTitle>Navigation</SheetTitle>
            <SheetDescription>Platform navigation</SheetDescription>
          </SheetHeader>
          <SidebarContent active={activeNav} onNavigate={navigate} />
        </SheetContent>
      </Sheet>

      <div className="flex min-h-screen flex-col lg:pl-[220px]">
        <Topbar
          onOpenPalette={() => setPaletteOpen(true)}
          onOpenMobileNav={() => setMobileNav(true)}
          onNavigate={navigate}
        />

        <main className="grid-bg flex-1">
          <div className="mx-auto w-full max-w-[1440px] p-4 md:p-6 lg:p-8">
            <AnimatePresence mode="wait">
              <motion.div
                key={ops ? path : view}
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -6 }}
                transition={{ duration: 0.18, ease: "easeOut" }}
              >
                {!ops && !viewSupported(view) && <UnsupportedView what={view} />}
                {ops?.kind === "files" && !filesSupported() && <UnsupportedView what="Files" />}
                {ops && (ops.kind !== "files" || filesSupported()) && (
                  <>
                    {ops.kind === "terminal" && <TerminalView id={ops.id} />}
                    {ops.kind === "files" && <FileManagerView id={ops.id} />}
                  </>
                )}
                {!ops && view === "overview" && <OverviewView onNavigate={navigate} />}
                {!ops && viewSupported(view) && view === "users" && <UsersView />}
                {!ops && view === "sandboxes" &&
                  (selectedSandboxId ? (
                    <SandboxDetailView />
                  ) : (
                    <SandboxListView onNavigate={navigate} />
                  ))}
                {!ops && view === "create" && <CreateSandboxView onNavigate={navigate} />}
                {!ops && view === "monitoring" && <MonitoringView />}
                {!ops && viewSupported(view) && view === "network" && <NetworkView />}
                {!ops && view === "templates" && <TemplatesView />}
                {!ops && viewSupported(view) && view === "approvals" && <ApprovalsView />}
                {!ops && viewSupported(view) && view === "nodes" && (nodeId ? <NodeDetailView id={nodeId} /> : <NodesView />)}
                {!ops && viewSupported(view) && view === "settings" && <SettingsView />}
              </motion.div>
            </AnimatePresence>
          </div>
        </main>
      </div>

      <CommandPalette open={paletteOpen} setOpen={setPaletteOpen} onNavigate={navigate} />
    </div>
  );
}
