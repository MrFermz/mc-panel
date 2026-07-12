import { NextRequest, NextResponse } from "next/server";

// เช็คแค่ว่า "มี" cookie mc_session หรือไม่ — ความถูกต้องของ session
// ตรวจโดย control-plane ทุก request อยู่แล้ว (middleware ไม่มี JWT secret)
export function middleware(req: NextRequest) {
  const hasSession = req.cookies.has("mc_session");
  const { pathname } = req.nextUrl;

  if (!hasSession && pathname !== "/login") {
    return NextResponse.redirect(new URL("/login", req.url));
  }
  if (hasSession && pathname === "/login") {
    return NextResponse.redirect(new URL("/", req.url));
  }
  return NextResponse.next();
}

export const config = {
  matcher: [
    "/((?!api|ws|healthz|_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|ico|webmanifest)).*)",
  ],
};
