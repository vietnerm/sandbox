"use client";

import { Toaster } from "sonner";
import { useTheme } from "@/lib/theme-store";

/* toasts follow the console theme (dark by default), not the OS setting */
export function ThemedToaster() {
  const theme = useTheme((s) => s.theme);
  return <Toaster theme={theme} position="bottom-right" richColors closeButton />;
}
