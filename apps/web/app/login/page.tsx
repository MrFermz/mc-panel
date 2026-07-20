"use client";

import * as React from "react";
import { useMutation } from "@tanstack/react-query";
import { apiSend, ApiError } from "@/lib/api";
import { userResponseSchema } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function LoginPage() {
  const t = useT();
  const [username, setUsername] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  // ค้าง pending ไว้ระหว่างเปลี่ยนหน้า — ปุ่มต้องไม่กลับมากดได้อีกหลัง login สำเร็จ
  const [navigating, setNavigating] = React.useState(false);

  const login = useMutation({
    mutationFn: () =>
      apiSend(
        "POST",
        "/api/auth/login",
        { username, password },
        userResponseSchema,
      ),
    onSuccess: ({ user }) => {
      setNavigating(true);
      // full navigation เพื่อให้ middleware เห็น cookie ใหม่แน่นอน
      window.location.assign(
        user.must_change_password ? "/change-password" : "/",
      );
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        if (err.code === "invalid_credentials") {
          setError(t("login.invalidCredentials"));
        } else if (err.code === "rate_limited") {
          setError(t("login.rateLimited"));
        } else {
          setError(err.message);
        }
      } else {
        setError(t("common.unreachable"));
      }
    },
  });

  const pending = login.isPending || navigating;

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    login.mutate();
  };

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-xl">{t("login.title")}</CardTitle>
          <CardDescription>{t("login.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="username">{t("login.username")}</Label>
              {/* พิมพ์ case ไหนก็ login ได้ — backend lower ให้ก่อนเทียบเสมอ
                  autoCapitalize="none" แค่กันคีย์บอร์ดมือถือขึ้นตัวใหญ่ให้เอง ไม่ได้แก้สิ่งที่พิมพ์ */}
              <Input
                id="username"
                type="text"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">{t("login.password")}</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {error && <p className="text-destructive text-sm">{error}</p>}
            <Button type="submit" loading={pending}>
              {pending ? t("login.signingIn") : t("login.signIn")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
