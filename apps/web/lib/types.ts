// zod schemas ของ payload ทุกตัวตาม docs/api.md — แก้ที่นั่นก่อนแล้วค่อยแก้ที่นี่
import { z } from "zod";

export const errorBodySchema = z.object({
  code: z.string(),
  message: z.string(),
});

export const userSchema = z.object({
  id: z.string(),
  username: z.string(),
  display_name: z.string().default(""),
  // avatar_url ชี้ /api/users/{id}/avatar?v=... (null = ยังไม่ตั้งรูป — UI ตกไปใช้ตัวอักษรย่อ)
  avatar_url: z.string().nullable().default(null),
  is_admin: z.boolean(),
  is_active: z.boolean(),
  must_change_password: z.boolean(),
  capabilities: z.array(z.string()).default([]),
  created_at: z.string(),
  // ไม่ null = อยู่ในถังขยะ (โผล่เฉพาะ /api/users?status=deleted ของหน้า admin)
  deleted_at: z.string().nullable().default(null),
});
export type User = z.infer<typeof userSchema>;

// catalog ของ capability keys จาก GET /api/meta/capabilities
export const capabilitySchema = z.object({
  key: z.string(),
  // group/action ใช้จัดกลุ่ม + หา i18n key ฝั่ง web (label/description เป็น fallback อังกฤษ)
  group: z.string(),
  action: z.string(),
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
  // network throughput ของทั้ง node (bytes/sec) — default 0 เผื่อ payload เก่ายังไม่มี field นี้
  net_rx_bps: z.number().default(0),
  net_tx_bps: z.number().default(0),
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
  // network + disk I/O rate ของ container (bytes/sec) — default 0 เผื่อ payload เก่ายังไม่มี
  net_rx_bps: z.number().default(0),
  net_tx_bps: z.number().default(0),
  disk_read_bps: z.number().default(0),
  disk_write_bps: z.number().default(0),
  // เวลาที่ container เริ่มรันรอบนี้ — null เมื่อ agent ไม่รู้ (payload เก่า/เพิ่งเริ่ม)
  started_at: z.string().nullable().default(null),
  // สถานะในเกมที่ agent อ่านจาก console (คนละแหล่งกับ container stats)
  // online_players ว่าง = ยังไม่ได้ resync รอบแรก หรือไม่มีใครออนไลน์
  online_players: z.array(z.string()).default([]),
  max_players: z.number().default(0),
  // tps 0 = server type ไม่มีคำสั่ง `tps` (vanilla/fabric/forge) — มีเฉพาะ paper/spigot
  tps: z.number().default(0),
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
  // ไม่ null = อยู่ในถังขยะ (โผล่เฉพาะ /api/servers?scope=all ของหน้า admin)
  deleted_at: z.string().nullable().default(null),
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
  "import_server",
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
  // ชื่อคนสั่งงาน — null เมื่อ user ถูกลบ (requested_by เป็น SET NULL)
  // web ประกอบเป็น userTitle เอง (display_name → username)
  requested_by_name: z.string().nullable().default(null),
  requested_by_username: z.string().nullable().default(null),
  created_at: z.string(),
  started_at: z.string().nullable().default(null),
  completed_at: z.string().nullable().default(null),
});
export type Job = z.infer<typeof jobSchema>;

// owner = superuser ต่อ server (ได้ทุก server-scoped cap + จัดการ access), member = grant ราย cap
export const permissionRoleSchema = z.enum(["owner", "member"]);
export type PermissionRole = z.infer<typeof permissionRoleSchema>;

export const permissionSchema = z.object({
  user_id: z.string(),
  username: z.string(),
  display_name: z.string().default(""),
  avatar_url: z.string().nullable().default(null),
  role: permissionRoleSchema,
  // server-scoped capability ที่ grant ให้ member (owner จะว่าง = ได้ทุกอย่างโดยปริยาย)
  capabilities: z.array(z.string()).default([]),
});
export type Permission = z.infer<typeof permissionSchema>;

// grant เดียวกันมองจากฝั่ง user แทนฝั่ง server — GET /api/users/{id}/servers
// (หน้า /admin/users/{id}/servers assign server ให้ user ทีเดียวหลายตัว)
export const serverPermissionSchema = z.object({
  server_id: z.string(),
  server_name: z.string(),
  server_status: serverStatusSchema,
  node_id: z.string(),
  role: permissionRoleSchema,
  capabilities: z.array(z.string()).default([]),
});
export type ServerPermission = z.infer<typeof serverPermissionSchema>;

export const serverPermissionsResponseSchema = z.object({
  permissions: z.array(serverPermissionSchema).default([]),
});
export type ServerPermissionsResponse = z.infer<
  typeof serverPermissionsResponseSchema
>;

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

// GET /api/meta/next-port — free host port ที่แนะนำสำหรับ node ที่เลือก
export const nextPortResponseSchema = z.object({ port: z.number() });

// ---------- players / whitelist (docs/api.md หัวข้อ Players) ----------

// unified player list — union ของ whitelist ∪ joined(usercache) ∪ ops ∪ banned
// boolean flag ทั้งชุด default false เผื่อ response บางเส้น (เช่น add) ส่งมาไม่ครบ
export const serverPlayerSchema = z.object({
  uuid: z.string().default(""),
  username: z.string(),
  whitelisted: z.boolean().default(false),
  seen: z.boolean().default(false),
  op: z.boolean().default(false),
  banned: z.boolean().default(false),
  // online มาจาก stats ที่ agent อ่านจาก console (ไม่ใช่ไฟล์) — false เมื่อ server ไม่ได้รัน
  online: z.boolean().default(false),
  // 0 = ไม่รู้ (ยังไม่เคยเล่น / อ่าน world stats ไม่ได้ / เกิน cap ฝั่ง backend)
  playtime_seconds: z.number().default(0),
});
export type ServerPlayer = z.infer<typeof serverPlayerSchema>;

export const playersResponseSchema = z.object({
  whitelist_enabled: z.boolean().default(false),
  players: z.array(serverPlayerSchema),
});
export type PlayersResponse = z.infer<typeof playersResponseSchema>;

export const addPlayerResponseSchema = z.object({
  player: serverPlayerSchema,
});
export type AddPlayerResponse = z.infer<typeof addPlayerResponseSchema>;

// ---------- response wrappers ----------

export const userResponseSchema = z.object({ user: userSchema });
export type UserResponse = z.infer<typeof userResponseSchema>;
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
export type CreateServerResponse = z.infer<typeof createServerResponseSchema>;

export const jobResponseSchema = z.object({ job: jobSchema });
export const jobsResponseSchema = z.object({ jobs: z.array(jobSchema) });

export const permissionsResponseSchema = z.object({
  permissions: z.array(permissionSchema),
});

// GET /api/users/directory — รายชื่อ user ที่ active สำหรับเลือกใน access tab
export const directoryUserSchema = z.object({
  id: z.string(),
  username: z.string(),
  display_name: z.string().default(""),
  avatar_url: z.string().nullable().default(null),
});
export type DirectoryUser = z.infer<typeof directoryUserSchema>;

// GET /api/users/check-username — reason ว่าง = ใช้ได้
export const usernameCheckResponseSchema = z.object({
  username: z.string(),
  available: z.boolean(),
  reason: z.enum(["", "invalid", "reserved", "taken"]).default(""),
});
export type UsernameCheckResponse = z.infer<typeof usernameCheckResponseSchema>;

export const userDirectoryResponseSchema = z.object({
  users: z.array(directoryUserSchema),
});
export type UserDirectoryResponse = z.infer<typeof userDirectoryResponseSchema>;

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
  // ความคืบหน้าของ lifecycle job ตัวหนึ่ง — pending/running ตอน dispatch,
  // succeeded/failed ตอนจบ (error มีข้อความจริงจาก agent). restart = ขา stop ของ restart
  z.object({
    type: z.literal("job_update"),
    server_id: z.string(),
    job_id: z.string(),
    job_type: jobTypeSchema,
    status: jobStatusSchema,
    error: z.string().default(""),
    restart: z.boolean().default(false),
  }),
  // list เปลี่ยน (สร้าง/import/ลบ server) — web invalidate ["servers"] ให้ dashboard สดโดยไม่ต้อง refresh
  z.object({ type: z.literal("server_added"), server_id: z.string() }),
  z.object({ type: z.literal("server_removed"), server_id: z.string() }),
]);
export type EventsServerMessage = z.infer<typeof eventsServerMessageSchema>;
