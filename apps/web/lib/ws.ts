// Production เสิร์ฟผ่าน Caddy origin เดียว — WS ต่อ same-origin ได้ตรง ๆ
// Dev ใช้ NEXT_PUBLIC_WS_BASE (ws://localhost:8080) เพราะ Next rewrites proxy WS ไม่เสถียร
function wsUrl(path: string): string {
  const base = process.env.NEXT_PUBLIC_WS_BASE;
  if (base) {
    return `${base.replace(/\/+$/, "")}${path}`;
  }
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}${path}`;
}

export function consoleWsUrl(serverId: string): string {
  return wsUrl(`/ws/servers/${encodeURIComponent(serverId)}/console`);
}

// stream เหตุการณ์ realtime ระดับ panel (stats/status/jobs ของทุก server + node)
// อ่านอย่างเดียว — client ไม่ส่งอะไรกลับ
export function eventsWsUrl(): string {
  return wsUrl(`/ws/events`);
}
