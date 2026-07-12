// เขียน cookie ฝั่ง client สำหรับ UI preference (theme/lang) — ไม่ใช่ auth token
// path=/ ให้ทุกหน้าอ่านได้, SameSite=Lax, อายุ 1 ปี, ไม่ HttpOnly (ต้องอ่านฝั่ง client ได้)
const ONE_YEAR_SECONDS = 60 * 60 * 24 * 365;

export function writeCookie(name: string, value: string): void {
  if (typeof document === "undefined") return;
  document.cookie = `${name}=${encodeURIComponent(
    value,
  )}; path=/; max-age=${ONE_YEAR_SECONDS}; samesite=lax`;
}
