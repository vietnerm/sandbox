/* Which console views exist on minictd (GĐ3). minictd is single-user and
   single-node (manifest §4: no RBAC, no cluster, no SDN), so the sandboxd
   views for users, approvals, nodes, egress policy and server settings
   are hidden unless the console runs in explicit demo mode. */
import { isDemo } from "@/console/api";
import type { ViewKey } from "./platform-data";

const MINICTD_VIEWS: ReadonlySet<ViewKey> = new Set(["overview", "sandboxes", "create", "templates", "monitoring"]);

export function viewSupported(v: ViewKey): boolean {
  return isDemo() || MINICTD_VIEWS.has(v);
}

/** minictd serves the file manager through the console adapter's execv2 bridge. */
export const filesSupported = () => true;
