"use client";

import { useQuery } from "@tanstack/react-query";
import { apiGet } from "@/lib/api";
import { userResponseSchema } from "@/lib/types";

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => apiGet("/api/auth/me", userResponseSchema),
    staleTime: 30_000,
  });
}
