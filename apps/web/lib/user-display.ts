// ทุกที่ที่ต้องเรียกชื่อ user ใช้สองตัวนี้เท่านั้น
// (อย่า inline fallback chain เองซ้ำ เดี๋ยวลำดับเพี้ยนกันคนละที่)
type Named = {
  username?: string | null;
  display_name?: string | null;
};

// identifier ที่แสดงต่อ user — username เป็น login identifier เดียวของระบบ
export function userIdent(u: Named): string {
  return u.username || "user";
}

// ชื่อที่อ่านง่ายสำหรับหัวข้อ/แถวตาราง — display name ที่เจ้าของบัญชีตั้งเองมาก่อน
export function userTitle(u: Named): string {
  return u.display_name?.trim() || u.username || "user";
}
