

import {
  Activity,
  BadgeCheck,
  Box,
  Layers,
  LayoutDashboard,
  Network,
  Server,
  Settings,
  Users,
} from "lucide-react";
import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";
import { type ViewKey } from "@/lib/platform-data";
import { usePlatform } from "@/lib/platform-store";
import { get as apiGet } from "@/console/api";
import { UserAvatar } from "./primitives";
import { viewSupported } from "@/lib/features";

/* daily-operations nav first, platform administration below. Templates is
   the snapshots-as-templates library (/v1/snapshots); Approvals is the
   gated-action queue with a live pending badge from /v1/overview. */
const MAIN_NAV: Array<{ key: ViewKey; label: string; vi: string; icon: typeof Users }> = [
  { key: "overview",   label: "Overview",   vi: "Tổng quan",   icon: LayoutDashboard },
  { key: "sandboxes",  label: "Sandboxes",  vi: "Môi trường",  icon: Box },
  { key: "templates",  label: "Templates",  vi: "Mẫu",         icon: Layers },
  { key: "monitoring", label: "Monitoring", vi: "Giám sát",    icon: Activity },
];

const SYSTEM_NAV: Array<{ key: ViewKey; label: string; vi: string; icon: typeof Users }> = [
  { key: "nodes",     label: "Nodes",     vi: "Máy chủ",    icon: Server },
  { key: "network",   label: "Network",   vi: "Mạng",       icon: Network },
  { key: "users",     label: "Users",     vi: "Người dùng", icon: Users },
  { key: "approvals", label: "Approvals", vi: "Phê duyệt",  icon: BadgeCheck },
  { key: "settings",  label: "Settings",  vi: "Cài đặt",    icon: Settings },
];

export function SidebarContent({
  active,
  onNavigate,
}: {
  active: ViewKey;
  onNavigate: (v: ViewKey) => void;
}) {
  const me = usePlatform((s) => s.me);
  const [version, setVersion] = useState("");
  useEffect(() => {
    // minictd: GET /v1/version (token) → "minictd/0.1.0 (…)"
    apiGet<{ version?: string }>("/v1/version")
      .then((v) => v?.version && setVersion(v.version.split(" ")[0].replace(/^minictd\//, "")))
      .catch(() => {});
  }, []);
  const displayName = me?.username ?? "operator";
  const displayRole = me?.role === "admin" ? "Admin" : me?.role === "operator" ? "Developer" : "Viewer";
  const approvalsOpen = usePlatform((s) => s.overviewNums?.approvalsOpen ?? 0);
  const NavItem = ({ item }: { item: (typeof MAIN_NAV)[number] }) => {
    const isActive = active === item.key;
    const Icon = item.icon;
    const pending = item.key === "approvals" ? approvalsOpen : 0;
    return (
      <button
        onClick={() => onNavigate(item.key)}
        aria-current={isActive ? "page" : undefined}
        className={cn(
          "group relative flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring",
          isActive
            ? "bg-neon/10 text-ink font-medium"
            : "text-gray-400 hover:bg-fg/[0.04] hover:text-gray-200"
        )}
      >
        {/* 3px neon active bar */}
        <span
          className={cn(
            "absolute left-0 top-1/2 -translate-y-1/2 h-6 w-[3px] rounded-full transition-all",
            isActive ? "bg-neon shadow-[0_0_8px_var(--glow-live)]" : "bg-transparent"
          )}
        />
        <Icon
          className={cn(
            "size-[18px] shrink-0 transition-colors",
            isActive ? "text-neon" : "text-gray-500 group-hover:text-gray-300"
          )}
        />
        <span className="flex-1 text-left">{item.label}</span>
        {pending > 0 && (
          <span
            className="rounded-full border border-warn/40 bg-warn/15 px-1.5 py-0.5 font-mono text-[10px] font-semibold leading-none text-warn"
            aria-label={`${pending} pending approvals`}
          >
            {pending}
          </span>
        )}
        <span
          className={cn(
            "text-[10px] transition-opacity",
            isActive ? "text-gray-400 opacity-100" : "text-gray-600 opacity-0 group-hover:opacity-100"
          )}
        >
          {item.vi}
        </span>
      </button>
    );
  };

  return (
    <div className="flex h-full flex-col">
      {/* brand */}
      <div className="flex items-center gap-3 px-5 pt-5 pb-6">
        <div className="grid size-9 place-items-center rounded-lg border border-neon/30 bg-neon/10">
          <Box className="size-4.5 text-neon" />
        </div>
        <div className="min-w-0">
          <div className="text-sm font-bold tracking-tight text-ink leading-tight">
            minictd
          </div>
          <div className="font-mono text-[9.5px] text-gray-500 tracking-wider uppercase">
            Rootless Console
          </div>
        </div>
      </div>

      {/* main nav */}
      <nav className="flex-1 space-y-1 px-3" aria-label="Main">
        {MAIN_NAV.map((item) => (
          <NavItem key={item.key} item={item} />
        ))}

        {SYSTEM_NAV.some((i) => viewSupported(i.key)) && (
          <div className="px-3 pt-6 pb-2 text-[10px] font-semibold tracking-[0.14em] text-gray-600 uppercase">
            System
          </div>
        )}
        {SYSTEM_NAV.filter((item) => viewSupported(item.key) && (item.key !== "nodes" || me?.role !== "viewer")).map((item) => (
          <NavItem key={item.key} item={item} />
        ))}
      </nav>

      {/* footer */}
      <div className="border-t border-line-soft p-4">
        <div className="flex items-center gap-2.5 rounded-lg px-1 py-1">
          <UserAvatar name={displayName} colorIdx={0} size="sm" />
          <div className="min-w-0 flex-1">
            <div className="truncate text-xs font-medium text-gray-200">{displayName}</div>
            <div className="font-mono text-[10px] text-admin">{displayRole}</div>
          </div>
        </div>
        <div className="mt-3 flex items-center justify-between px-1">
          <span className="font-mono text-[10px] text-gray-600">{version ? `v${version.replace(/^v/, "")}` : ""}</span>
          <span className="inline-flex min-w-0 items-center gap-1.5 font-mono text-[10px] text-gray-500">
            <span className="size-1.5 shrink-0 rounded-full bg-neon dot-live" />
            <span className="truncate">{location.hostname}</span>
          </span>
        </div>
      </div>
    </div>
  );
}
