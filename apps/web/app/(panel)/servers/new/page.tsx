"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useUiStore } from "@/lib/settings/ui-store";
import { Skeleton } from "@/components/ui/skeleton";

// New Server เป็น modal แล้ว — route ตรงนี้คงไว้ให้เข้าได้ แต่เด้งกลับ dashboard
// แล้วเปิด modal ให้ UX สม่ำเสมอ
export default function NewServerRedirectPage() {
  const router = useRouter();
  const openNewServer = useUiStore((s) => s.openNewServer);

  React.useEffect(() => {
    openNewServer();
    router.replace("/");
  }, [openNewServer, router]);

  return (
    <div className="grid gap-4">
      <Skeleton className="h-10 w-64" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}
