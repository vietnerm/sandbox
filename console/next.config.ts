import type { NextConfig } from "next";

// GĐ3 (manifest §7): the console ships as STATIC files only — `next build`
// writes ./out, and `minictd --ui-dir console/out` serves it on the same
// 127.0.0.1 listener as the API. No Node process at runtime, so the
// control-plane RSS budget (≤ 30 MB) is untouched.
const nextConfig: NextConfig = {
  output: "export",
  trailingSlash: false,
  images: { unoptimized: true },
  reactStrictMode: true,
  poweredByHeader: false,
};

export default nextConfig;
