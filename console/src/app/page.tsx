"use client";

import PlatformRoot from "@/components/platform/platform-root";

/**
 * minictd console SPA. Routing is client-side (history API); minictd's
 * --ui-dir handler answers every non-API path with index.html, so deep
 * links such as /sandboxes/<name> survive a hard refresh.
 */
export default function ConsolePage() {
  return <PlatformRoot />;
}
