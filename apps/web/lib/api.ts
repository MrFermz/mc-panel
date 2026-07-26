import type { z } from "zod";
import {
  addPlayerResponseSchema,
  errorBodySchema,
  fileContentResponseSchema,
  fileListResponseSchema,
  nextPortResponseSchema,
  playersResponseSchema,
  serverPermissionsResponseSchema,
  serverPropertiesResponseSchema,
  userDirectoryResponseSchema,
  usernameCheckResponseSchema,
  userResponseSchema,
  DEFAULT_GAME,
  type AddPlayerResponse,
  type FileContentResponse,
  type FileListResponse,
  type PermissionRole,
  type PlayersResponse,
  type ServerPermissionsResponse,
  type ServerPropertiesResponse,
  type UserDirectoryResponse,
  type UsernameCheckResponse,
  type UserResponse,
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
      // FormData ต้องปล่อยให้ browser ใส่ multipart boundary เอง — set เองแล้ว parse ฝั่ง server พัง
      ...(init?.body !== undefined && !(init.body instanceof FormData)
        ? { "Content-Type": "application/json" }
        : {}),
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

// ---------- game config file ----------

export function getServerProperties(
  serverId: string,
): Promise<ServerPropertiesResponse> {
  return apiGet(
    `/api/servers/${serverId}/config`,
    serverPropertiesResponseSchema,
  );
}

// catalog + ค่า default ที่ยังไม่ผูก server — wizard ใช้ก่อนสร้าง instance
// catalog เป็นของ game definition จึงต้องบอกว่าถามในนามเกมไหน
export function getPropertiesCatalog(): Promise<ServerPropertiesResponse> {
  return apiGet(
    `/api/meta/config?game=${encodeURIComponent(DEFAULT_GAME)}`,
    serverPropertiesResponseSchema,
  );
}

export function saveServerProperties(
  serverId: string,
  values: Record<string, string>,
): Promise<void> {
  return apiSendVoid("PUT", `/api/servers/${serverId}/config`, { values });
}

// ---------- meta ----------

// port เริ่มต้นที่ไล่หามาจาก game definition — ต้องบอกเกมไปด้วย
export function getNextPort(nodeId: string): Promise<number> {
  return apiGet(
    `/api/meta/next-port?node_id=${encodeURIComponent(nodeId)}&game=${encodeURIComponent(DEFAULT_GAME)}`,
    nextPortResponseSchema,
  ).then((r) => r.port);
}

// ---------- players / whitelist ----------

export function listPlayers(serverId: string): Promise<PlayersResponse> {
  return apiGet(`/api/servers/${serverId}/players`, playersResponseSchema);
}

export function addPlayer(
  serverId: string,
  username: string,
): Promise<AddPlayerResponse> {
  return apiSend(
    "POST",
    `/api/servers/${serverId}/players`,
    { username },
    addPlayerResponseSchema,
  );
}

// action ผ่าน console ของ server (op/deop/kick/ban/pardon) — ต้อง running
export function playerAction(
  serverId: string,
  action: "op" | "deop" | "kick" | "ban" | "pardon",
  username: string,
): Promise<void> {
  return apiSendVoid("POST", `/api/servers/${serverId}/players/action`, {
    action,
    username,
  });
}

export function removePlayer(serverId: string, uuid: string): Promise<void> {
  return apiSendVoid(
    "DELETE",
    `/api/servers/${serverId}/players/${encodeURIComponent(uuid)}`,
  );
}

// ---------- profile ของตัวเอง (ไม่ต้องมี capability) ----------

export function updateProfile(displayName: string): Promise<UserResponse> {
  return apiSend(
    "PATCH",
    "/api/auth/me",
    { display_name: displayName },
    userResponseSchema,
  );
}

export function uploadAvatar(file: File): Promise<UserResponse> {
  const form = new FormData();
  form.append("avatar", file);
  return request("/api/auth/me/avatar", { method: "PUT", body: form }).then(
    (body) => userResponseSchema.parse(body),
  );
}

export function deleteAvatar(): Promise<UserResponse> {
  return apiSend("DELETE", "/api/auth/me/avatar", undefined, userResponseSchema);
}

// ---------- users ----------

// ลบ user = soft delete (ไปอยู่ถังขยะ) — กู้กลับด้วย restoreUser ได้พร้อมสิทธิ์ต่อ server เดิม
export function deleteUser(userId: string): Promise<void> {
  return apiSendVoid("DELETE", `/api/users/${userId}`);
}

export function restoreUser(userId: string): Promise<UserResponse> {
  return apiSend(
    "POST",
    `/api/users/${userId}/restore`,
    undefined,
    userResponseSchema,
  );
}

// ---------- server access ของ user คนหนึ่ง (/admin/users/{id}/servers) ----------

export function listUserServers(
  userId: string,
): Promise<ServerPermissionsResponse> {
  return apiGet(
    `/api/users/${userId}/servers`,
    serverPermissionsResponseSchema,
  );
}

export function assignUserServer(
  userId: string,
  body: { server_id: string; role: PermissionRole; capabilities: string[] },
): Promise<void> {
  return apiSendVoid("POST", `/api/users/${userId}/servers`, body);
}

export function unassignUserServer(
  userId: string,
  serverId: string,
): Promise<void> {
  return apiSendVoid("DELETE", `/api/users/${userId}/servers/${serverId}`);
}

// เช็คว่าชื่อนี้ใช้สร้างบัญชีได้ไหม (ซ้ำ/ถูกจองไว้/ผิดรูปแบบ) — ต้องมี cap users.create
export function checkUsername(
  username: string,
): Promise<UsernameCheckResponse> {
  return apiGet(
    `/api/users/check-username?username=${encodeURIComponent(username)}`,
    usernameCheckResponseSchema,
  );
}

// รายชื่อ user ที่ active สำหรับให้เลือกใน access tab (ไม่ใช่ admin-only)
export function listUserDirectory(): Promise<UserDirectoryResponse> {
  return apiGet("/api/users/directory", userDirectoryResponseSchema);
}
