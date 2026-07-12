import type { NextConfig } from "next";

// Dev ไม่มี Caddy คั่น — proxy /api และ /healthz ไป control-plane ตรง ๆ
// (production request พวกนี้ถูก Caddy ดักก่อนถึง Next อยู่แล้ว rewrite นี้จึงไม่ถูกใช้)
// WebSocket proxy ผ่าน rewrites ไม่เสถียร — ฝั่ง client ใช้ NEXT_PUBLIC_WS_BASE แทน (ดู lib/ws.ts)
const apiTarget = process.env.API_PROXY_TARGET ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiTarget}/api/:path*` },
      { source: "/healthz", destination: `${apiTarget}/healthz` },
    ];
  },
};

export default nextConfig;
