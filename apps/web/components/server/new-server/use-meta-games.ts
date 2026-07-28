"use client";

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "@/lib/api";
import { metaGamesResponseSchema, type MetaGame } from "@/lib/types";

// รายการเกมที่ instance นี้รองรับ + สิทธิ์สร้างของ user คนนี้ (field can_create)
// — web ไม่ได้ตัดสินใจเองว่ารองรับเกมอะไรบ้าง มาจาก registry ฝั่ง backend เสมอ
// ใช้ร่วมกันระหว่างหน้าเลือกเกมกับ wizard (react-query dedupe ด้วย key เดียวกัน)
export function useMetaGames() {
  const query = useQuery({
    queryKey: ["meta", "games"],
    queryFn: () => apiGet("/api/meta/games", metaGamesResponseSchema),
    staleTime: 300_000,
  });
  const games: MetaGame[] = query.data?.games ?? [];
  return { games, pending: query.isPending, error: query.isError };
}
