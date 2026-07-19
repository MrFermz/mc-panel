"use client";

import * as React from "react";
import type { Server } from "@/lib/types";

export interface BreadcrumbItemData {
  label: string;
  href?: string;
}

// server ที่ header ปัจจุบันผูกอยู่ (โชว์ status badge + ปุ่มสั่งงาน) — canOperate gate ปุ่ม
export interface PageServer {
  server: Server;
  canOperate: boolean;
}

interface PageHeaderContextValue {
  items: BreadcrumbItemData[];
  setItems: (items: BreadcrumbItemData[]) => void;
  pageServer: PageServer | null;
  setPageServer: (value: PageServer | null) => void;
}

const PageHeaderContext = React.createContext<PageHeaderContextValue | null>(
  null,
);

export function BreadcrumbProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [items, setItems] = React.useState<BreadcrumbItemData[]>([]);
  const [pageServer, setPageServer] = React.useState<PageServer | null>(null);
  const value = React.useMemo(
    () => ({ items, setItems, pageServer, setPageServer }),
    [items, pageServer],
  );
  return (
    <PageHeaderContext.Provider value={value}>
      {children}
    </PageHeaderContext.Provider>
  );
}

export function useBreadcrumbs(): BreadcrumbItemData[] {
  const ctx = React.useContext(PageHeaderContext);
  return ctx?.items ?? [];
}

export function usePageServer(): PageServer | null {
  const ctx = React.useContext(PageHeaderContext);
  return ctx?.pageServer ?? null;
}

// ตั้ง trail ของหน้าปัจจุบันตอน mount/เปลี่ยนค่า แล้วเคลียร์ตอน unmount
// (serialize items เป็น dependency กัน loop จาก array ที่สร้างใหม่ทุก render)
export function useSetBreadcrumbs(items: BreadcrumbItemData[]): void {
  const ctx = React.useContext(PageHeaderContext);
  const setItems = ctx?.setItems;
  const serialized = JSON.stringify(items);
  React.useEffect(() => {
    if (!setItems) return;
    setItems(JSON.parse(serialized) as BreadcrumbItemData[]);
    return () => setItems([]);
  }, [serialized, setItems]);
}

// ผูก server เข้ากับ header ของหน้า — top bar โชว์ status + ปุ่มสั่งงานจากตัวนี้
// server object เปลี่ยน identity ทุก WS update → sync ค่าล่าสุด, เคลียร์ตอน unmount
export function useSetPageServer(
  server: Server | null | undefined,
  canOperate: boolean,
): void {
  const ctx = React.useContext(PageHeaderContext);
  const setPageServer = ctx?.setPageServer;
  React.useEffect(() => {
    if (!setPageServer) return;
    setPageServer(server ? { server, canOperate } : null);
  }, [setPageServer, server, canOperate]);
  React.useEffect(() => {
    return () => {
      setPageServer?.(null);
    };
  }, [setPageServer]);
}
