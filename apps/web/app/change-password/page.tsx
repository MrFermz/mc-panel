"use client";

import { ChangePasswordDialog } from "@/components/change-password-dialog";

// forced flow: middleware/redirect พามาที่ route นี้เมื่อ must_change_password
// render dialog แบบปิดไม่ได้ (มีแค่ปุ่ม Logout) จนกว่าจะตั้งรหัสใหม่
export default function ChangePasswordPage() {
  return (
    <main className="min-h-screen">
      <ChangePasswordDialog open forced onOpenChange={() => {}} />
    </main>
  );
}
