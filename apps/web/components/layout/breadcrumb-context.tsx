"use client";

import * as React from "react";

export interface BreadcrumbItemData {
  label: string;
  href?: string;
}

interface BreadcrumbContextValue {
  items: BreadcrumbItemData[];
  setItems: (items: BreadcrumbItemData[]) => void;
}

const BreadcrumbContext = React.createContext<BreadcrumbContextValue | null>(
  null,
);

export function BreadcrumbProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [items, setItems] = React.useState<BreadcrumbItemData[]>([]);
  const value = React.useMemo(() => ({ items, setItems }), [items]);
  return (
    <BreadcrumbContext.Provider value={value}>
      {children}
    </BreadcrumbContext.Provider>
  );
}

export function useBreadcrumbs(): BreadcrumbItemData[] {
  const ctx = React.useContext(BreadcrumbContext);
  return ctx?.items ?? [];
}

// ตั้ง trail ของหน้าปัจจุบันตอน mount/เปลี่ยนค่า แล้วเคลียร์ตอน unmount
// (serialize items เป็น dependency กัน loop จาก array ที่สร้างใหม่ทุก render)
export function useSetBreadcrumbs(items: BreadcrumbItemData[]): void {
  const ctx = React.useContext(BreadcrumbContext);
  const setItems = ctx?.setItems;
  const serialized = JSON.stringify(items);
  React.useEffect(() => {
    if (!setItems) return;
    setItems(JSON.parse(serialized) as BreadcrumbItemData[]);
    return () => setItems([]);
  }, [serialized, setItems]);
}
