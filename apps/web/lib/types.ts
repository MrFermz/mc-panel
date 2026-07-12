// zod schemas ของ payload ทุกตัวตาม docs/api.md — แก้ที่นั่นก่อนแล้วค่อยแก้ที่นี่
import { z } from "zod";

export const errorBodySchema = z.object({
  code: z.string(),
  message: z.string(),
});

export const userSchema = z.object({
  id: z.string(),
  email: z.string(),
  username: z.string().nullable().default(null),
  display_name: z.string(),
  is_admin: z.boolean(),
  is_active: z.boolean(),
  must_change_password: z.boolean(),
  capabilities: z.array(z.string()).default([]),
  created_at: z.string(),
});
export type User = z.infer<typeof userSchema>;

// catalog ของ capability keys จาก GET /api/meta/capabilities
export const capabilitySchema = z.object({
  key: z.string(),
  label: z.string(),
  description: z.string(),
});
export type Capability = z.infer<typeof capabilitySchema>;

export const capabilitiesResponseSchema = z.object({
  capabilities: z.array(capabilitySchema),
});

export const nodeStatusSchema = z.enum(["online", "offline"]);

export const nodeSchema = z.object({
  id: z.string(),
  name: z.string(),
  status: nodeStatusSchema,
  agent_version: z.string(),
  os: z.string(),
  arch: z.string(),
  cpu_percent: z.number(),
  memory_used_mb: z.number(),
  memory_total_mb: z.number(),
  disk_used_mb: z.number(),
  disk_total_mb: z.number(),
  last_heartbeat_at: z.string().nullable().default(null),
  created_at: z.string(),
});
export type Node = z.infer<typeof nodeSchema>;

export const serverTypeIdSchema = z.enum([
  "vanilla",
  "paper",
  "fabric",
  "forge",
  "velocity",
]);
export type ServerTypeId = z.infer<typeof serverTypeIdSchema>;

export const serverStatusSchema = z.enum([
  "provisioning",
  "stopped",
  "starting",
  "running",
  "stopping",
  "errored",
  "deleting",
]);
export type ServerStatus = z.infer<typeof serverStatusSchema>;

export const serverStatsSchema = z.object({
  cpu_percent: z.number(),
  memory_used_mb: z.number(),
  memory_limit_mb: z.number(),
  updated_at: z.string(),
});
export type ServerStats = z.infer<typeof serverStatsSchema>;

export const serverSchema = z.object({
  id: z.string(),
  node_id: z.string(),
  owner_id: z.string().nullable().default(null),
  name: z.string(),
  server_type: serverTypeIdSchema,
  mc_version: z.string(),
  memory_mb: z.number(),
  host_port: z.number().nullable().default(null),
  status: serverStatusSchema,
  created_at: z.string(),
  updated_at: z.string(),
  // null เมื่อ server ไม่ได้รันหรือยังไม่มีข้อมูล stats
  stats: serverStatsSchema.nullable().default(null),
});
export type Server = z.infer<typeof serverSchema>;

export const jobTypeSchema = z.enum([
  "create_server",
  "start_server",
  "stop_server",
  "kill_server",
  "delete_server",
]);

export const jobStatusSchema = z.enum([
  "pending",
  "running",
  "succeeded",
  "failed",
]);

export const jobSchema = z.object({
  id: z.string(),
  server_id: z.string().nullable().default(null),
  type: jobTypeSchema,
  status: jobStatusSchema,
  error: z.string().default(""),
  // อีเมลของคนสั่งงาน — null เมื่อ user ถูกลบ (requested_by เป็น SET NULL)
  requested_by_email: z.string().nullable().default(null),
  created_at: z.string(),
  started_at: z.string().nullable().default(null),
  completed_at: z.string().nullable().default(null),
});
export type Job = z.infer<typeof jobSchema>;

export const permissionRoleSchema = z.enum(["owner", "operator", "viewer"]);
export type PermissionRole = z.infer<typeof permissionRoleSchema>;

export const permissionSchema = z.object({
  user_id: z.string(),
  email: z.string(),
  display_name: z.string().default(""),
  role: permissionRoleSchema,
  can_console_write: z.boolean(),
  can_manage_files: z.boolean(),
});
export type Permission = z.infer<typeof permissionSchema>;

// ---------- file manager (docs/api.md หัวข้อ File manager) ----------

export const fileEntrySchema = z.object({
  name: z.string(),
  is_dir: z.boolean(),
  size: z.number(),
  mod_time: z.string(),
});
export type FileEntry = z.infer<typeof fileEntrySchema>;

export const fileListResponseSchema = z.object({
  path: z.string(),
  entries: z.array(fileEntrySchema),
});
export type FileListResponse = z.infer<typeof fileListResponseSchema>;

export const fileContentResponseSchema = z.object({
  path: z.string(),
  content: z.string(),
  truncated: z.boolean().default(false),
});
export type FileContentResponse = z.infer<typeof fileContentResponseSchema>;

// ---------- server.properties (docs/api.md หัวข้อ Server properties) ----------

export const serverPropertyFieldSchema = z.object({
  key: z.string(),
  label: z.string(),
  type: z.enum(["enum", "int", "bool", "string"]),
  options: z.array(z.string()).default([]),
  min: z.number().nullable().default(null),
  max: z.number().nullable().default(null),
});
export type ServerPropertyField = z.infer<typeof serverPropertyFieldSchema>;

export const serverPropertiesResponseSchema = z.object({
  fields: z.array(serverPropertyFieldSchema),
  values: z.record(z.string(), z.string()),
  extra: z.record(z.string(), z.string()).default({}),
});
export type ServerPropertiesResponse = z.infer<
  typeof serverPropertiesResponseSchema
>;

export const metaNodeSchema = z.object({
  id: z.string(),
  name: z.string(),
  status: z.string(),
});
export type MetaNode = z.infer<typeof metaNodeSchema>;

export const metaServerTypeSchema = z.object({
  id: serverTypeIdSchema,
  label: z.string(),
  needs_eula: z.boolean(),
});
export type MetaServerType = z.infer<typeof metaServerTypeSchema>;

// ---------- response wrappers ----------

export const userResponseSchema = z.object({ user: userSchema });
export const usersResponseSchema = z.object({ users: z.array(userSchema) });
export const createUserResponseSchema = z.object({
  user: userSchema,
  initial_password: z.string(),
});
export const resetPasswordResponseSchema = z.object({
  initial_password: z.string(),
});

export const nodesResponseSchema = z.object({ nodes: z.array(nodeSchema) });
export const createNodeResponseSchema = z.object({
  node: nodeSchema,
  token: z.string(),
});

export const serversResponseSchema = z.object({
  servers: z.array(serverSchema),
});
export const serverResponseSchema = z.object({ server: serverSchema });
export const serverDetailResponseSchema = z.object({
  server: serverSchema,
  permissions: z.array(permissionSchema).default([]),
});
export const createServerResponseSchema = z.object({
  server: serverSchema,
  job: jobSchema,
});

export const jobResponseSchema = z.object({ job: jobSchema });
export const jobsResponseSchema = z.object({ jobs: z.array(jobSchema) });

export const permissionsResponseSchema = z.object({
  permissions: z.array(permissionSchema),
});

export const metaNodesResponseSchema = z.object({
  nodes: z.array(metaNodeSchema),
});
export const metaServerTypesResponseSchema = z.object({
  types: z.array(metaServerTypeSchema),
});
export const versionsResponseSchema = z.object({
  versions: z.array(z.string()),
});

// ---------- websocket console (docs/api.md หัวข้อ WebSocket) ----------

export const consoleServerMessageSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("lines"), lines: z.array(z.string()) }),
  z.object({ type: z.literal("status"), status: serverStatusSchema }),
  z.object({
    type: z.literal("error"),
    code: z.string(),
    message: z.string(),
  }),
]);
export type ConsoleServerMessage = z.infer<typeof consoleServerMessageSchema>;

// ---------- websocket events (panel-wide realtime, docs/api.md หัวข้อ WebSocket events) ----------
// server ส่งเฉพาะ event ที่ user มีสิทธิ์เห็น (server access + node event เฉพาะ admin) — client เชื่อได้เลย
export const eventsServerMessageSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("server_stats"),
    server_id: z.string(),
    stats: serverStatsSchema.nullable(),
  }),
  z.object({
    type: z.literal("server_status"),
    server_id: z.string(),
    status: serverStatusSchema,
  }),
  z.object({ type: z.literal("node_stats"), node: nodeSchema }),
  z.object({ type: z.literal("server_jobs"), server_id: z.string() }),
]);
export type EventsServerMessage = z.infer<typeof eventsServerMessageSchema>;
