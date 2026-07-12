import type { z } from "zod";
import {
  errorBodySchema,
  fileContentResponseSchema,
  fileListResponseSchema,
  serverPropertiesResponseSchema,
  type FileContentResponse,
  type FileListResponse,
  type ServerPropertiesResponse,
} from "@/lib/types";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

async function handleAuthRedirect(code: string): Promise<boolean> {
  if (typeof window === "undefined") return false;
  const path = window.location.pathname;

  if (code === "unauthorized" && path !== "/login") {
    // cookie อาจยังค้างอยู่ทั้งที่ session หมดอายุ — ถ้าปล่อยไว้ middleware จะเด้ง
    // /login กลับมาหน้าเดิมเป็น loop เลยขอ logout เคลียร์ cookie ก่อนเสมอ
    try {
      await fetch("/api/auth/logout", {
        method: "POST",
        credentials: "same-origin",
      });
    } catch {
      // เคลียร์ไม่ได้ก็ปล่อยให้ server ปฏิเสธเอง
    }
    window.location.assign("/login");
    return true;
  }
  if (code === "password_change_required" && path !== "/change-password") {
    window.location.assign("/change-password");
    return true;
  }
  return false;
}

async function request(path: string, init?: RequestInit): Promise<unknown> {
  const res = await fetch(path, {
    credentials: "same-origin",
    ...init,
    headers: {
      ...(init?.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });

  let body: unknown;
  if (res.status !== 204) {
    try {
      body = await res.json();
    } catch {
      body = undefined;
    }
  }

  if (!res.ok) {
    const parsed = errorBodySchema.safeParse(body);
    const code = parsed.success ? parsed.data.code : "unknown_error";
    const message = parsed.success
      ? parsed.data.message
      : `Request failed (HTTP ${res.status})`;
    await handleAuthRedirect(code);
    throw new ApiError(res.status, code, message);
  }

  return body;
}

export async function apiGet<T>(
  path: string,
  schema: z.ZodType<T, z.ZodTypeDef, unknown>,
): Promise<T> {
  return schema.parse(await request(path));
}

export async function apiSend<T>(
  method: "POST" | "PATCH" | "PUT" | "DELETE",
  path: string,
  body: unknown,
  schema: z.ZodType<T, z.ZodTypeDef, unknown>,
): Promise<T> {
  return schema.parse(
    await request(path, {
      method,
      body: body === undefined ? undefined : JSON.stringify(body),
    }),
  );
}

export async function apiSendVoid(
  method: "POST" | "PATCH" | "PUT" | "DELETE",
  path: string,
  body?: unknown,
): Promise<void> {
  await request(path, {
    method,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

// ---------- file manager helpers (path เป็น query param — encode ให้ถูก) ----------

const filesBase = (serverId: string) => `/api/servers/${serverId}/files`;

export function listFiles(
  serverId: string,
  path: string,
): Promise<FileListResponse> {
  return apiGet(
    `${filesBase(serverId)}?path=${encodeURIComponent(path)}`,
    fileListResponseSchema,
  );
}

export function readFileContent(
  serverId: string,
  path: string,
): Promise<FileContentResponse> {
  return apiGet(
    `${filesBase(serverId)}/content?path=${encodeURIComponent(path)}`,
    fileContentResponseSchema,
  );
}

export function writeFileContent(
  serverId: string,
  path: string,
  content: string,
): Promise<void> {
  return apiSendVoid("PUT", `${filesBase(serverId)}/content`, { path, content });
}

export function makeDir(serverId: string, path: string): Promise<void> {
  return apiSendVoid("POST", `${filesBase(serverId)}/dir`, { path });
}

export function renameFile(
  serverId: string,
  from: string,
  to: string,
): Promise<void> {
  return apiSendVoid("POST", `${filesBase(serverId)}/rename`, { from, to });
}

export function deleteFile(serverId: string, path: string): Promise<void> {
  return apiSendVoid(
    "DELETE",
    `${filesBase(serverId)}?path=${encodeURIComponent(path)}`,
  );
}

// ---------- server.properties ----------

export function getServerProperties(
  serverId: string,
): Promise<ServerPropertiesResponse> {
  return apiGet(
    `/api/servers/${serverId}/properties`,
    serverPropertiesResponseSchema,
  );
}

export function saveServerProperties(
  serverId: string,
  values: Record<string, string>,
): Promise<void> {
  return apiSendVoid("PUT", `/api/servers/${serverId}/properties`, { values });
}

// ---------- users ----------

export function deleteUser(userId: string): Promise<void> {
  return apiSendVoid("DELETE", `/api/users/${userId}`);
}
