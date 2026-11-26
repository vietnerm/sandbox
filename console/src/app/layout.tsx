import type { Metadata, Viewport } from "next";
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "sonner/dist/styles.css";
import "./globals.css";
import "@/console/console.css";
import { ThemedToaster } from "@/components/themed-toaster";

export const metadata: Metadata = {
  title: "minictd Console",
  description: "Bảng điều khiển minictd — quản lý instance rootless (GĐ3).",
  icons: { icon: "/favicon.svg" },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
};

/* theme-boot.js is a file, not an inline script, so it stays allowed
   under minictd's CSP (script-src 'self'). It sets the theme class on
   <html> before first paint; dark is the default. */
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="vi" suppressHydrationWarning>
      <head>
        {/* eslint-disable-next-line @next/next/no-sync-scripts */}
        <script src="/theme-boot.js" />
      </head>
      <body>
        {children}
        <ThemedToaster />
      </body>
    </html>
  );
}
