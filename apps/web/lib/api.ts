import type { z } from "zod";
import {
  addPlayerResponseSchema,
  createServerResponseSchema,
  errorBodySchema,
  fileContentResponseSchema,
  fileListResponseSchema,
  nextPortResponseSchema,
  playersResponseSchema,
  serverPropertiesResponseSchema,
  userDirectoryResponseSchema,
  userResponseSchema,
  type AddPlayerResponse,
  type CreateServerResponse,
  type FileContentResponse,
  type FileListResponse,
  type PlayersResponse,
  type ServerPropertiesResponse,
  type UserDirectoryResponse,
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

// ---------- meta ----------

export function getNextPort(nodeId: string): Promise<number> {
  return apiGet(
    `/api/meta/next-port?node_id=${encodeURIComponent(nodeId)}`,
    nextPortResponseSchema,
  ).then((r) => r.port);
}

// ---------- import existing server (multipart upload) ----------

// ใช้ XHR ไม่ใช่ fetch เพราะต้องการ upload progress (archive อาจใหญ่ถึง ~2 GiB)
// ห้าม set Content-Type เอง — browser จะใส่ multipart boundary ให้ FormData เอง
export function importServer(
  form: FormData,
  onProgress?: (pct: number) => void,
): Promise<CreateServerResponse> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/servers/import");
    xhr.withCredentials = true; // cookie auth (same-origin)

    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      };
    }

    xhr.onload = () => {
      let body: unknown;
      try {
        body = xhr.responseText ? JSON.parse(xhr.responseText) : undefined;
      } catch {
        body = undefined;
      }

      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(createServerResponseSchema.parse(body));
        } catch (err) {
          reject(err);
        }
        return;
      }

      const parsed = errorBodySchema.safeParse(body);
      const code = parsed.success ? parsed.data.code : "unknown_error";
      const message = parsed.success
        ? parsed.data.message
        : `Request failed (HTTP ${xhr.status})`;
      void handleAuthRedirect(code);
      reject(new ApiError(xhr.status, code, message));
    };

    xhr.onerror = () =>
      reject(new ApiError(0, "network_error", "Upload failed."));
    xhr.onabort = () =>
      reject(new ApiError(0, "aborted", "Upload aborted."));

    xhr.send(form);
  });
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

export function deleteUser(userId: string): Promise<void> {
  return apiSendVoid("DELETE", `/api/users/${userId}`);
}

// รายชื่อ user ที่ active สำหรับให้เลือกใน access tab (ไม่ใช่ admin-only)
export function listUserDirectory(): Promise<UserDirectoryResponse> {
  return apiGet("/api/users/directory", userDirectoryResponseSchema);
}
