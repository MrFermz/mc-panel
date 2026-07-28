package httpapi

import (
	"slices"

	"github.com/game-manager/control-plane/internal/store"
)

// capabilityMeta คือ 1 entry ใน catalog global capability (ดู docs/api.md).
// catalog เป็น source of truth ฝั่ง control-plane — ห้ามรับ/เก็บ key นอกนี้
// group/action ให้ web จัดกลุ่ม + แปลเองโดยไม่ต้องพึ่ง label อังกฤษจาก API
type capabilityMeta struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Action      string `json:"action"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// capability key — key = "{group}.{action}" ทุกตัว, ใช้ const แทน string literal
// ที่จุด enforce เพื่อกันพิมพ์ผิดเงียบ ๆ
//
// เพิ่ม feature ใหม่ = เพิ่ม const + entry ใน capabilityCatalog + ผูกกับ route
// ใน api.go + เพิ่ม i18n ฝั่ง web (ดู CLAUDE.md หัวข้อ "เพิ่ม feature ใหม่")
const (
	capUsersView          = "users.view"
	capUsersCreate        = "users.create"
	capUsersEdit          = "users.edit"
	capUsersDelete        = "users.delete"
	capUsersRestore       = "users.restore"
	capUsersResetPassword = "users.reset_password"

	capNodesView   = "nodes.view"
	capNodesCreate = "nodes.create"
	capNodesDelete = "nodes.delete"

	capServersViewAll = "servers.view_all"
	capServersCreate  = "servers.create"
	capServersEdit    = "servers.edit"
	capServersDelete  = "servers.delete"
	capServersRestore = "servers.restore"
	capServersPurge   = "servers.purge"
	capServersPower   = "servers.power"

	capConsoleView  = "console.view"
	capConsoleWrite = "console.write"

	capFilesView   = "files.view"
	capFilesWrite  = "files.write"
	capFilesDelete = "files.delete"

	capPlayersView     = "players.view"
	capPlayersManage   = "players.manage"
	capPlayersModerate = "players.moderate"

	capSettingsView = "settings.view"
	capSettingsEdit = "settings.edit"

	capAccessView   = "access.view"
	capAccessManage = "access.manage"
)

// capGamesGroup = กลุ่ม capability "สร้าง server ของเกมนี้ได้ไหม" — action คือ **id ของเกม**
// จึงเป็นกลุ่มเดียวใน catalog ที่รายการไม่คงที่: มาจาก registry ตอน runtime
// (เพิ่มเกมใหม่ = ได้ capability ใหม่เองโดยไม่ต้องแก้ catalog — ดู CLAUDE.md กฎข้อ 0)
const capGamesGroup = "games"

// gameCapKey คืน key ของสิทธิ์สร้าง server สำหรับเกมหนึ่ง (เช่น "games.minecraft")
func gameCapKey(gameID string) string { return capGamesGroup + "." + gameID }

// baseCapabilityCatalog = capability ที่ไม่ผูกกับเกม (ลำดับคงที่สำหรับ UI)
// catalog ตัวเต็มที่ใช้ validate/ส่งให้ web คือ API.capabilityCatalog() ซึ่งต่อท้ายด้วย
// capability ต่อเกมจาก registry
var baseCapabilityCatalog = []capabilityMeta{
	{capUsersView, "users", "view", "View users", "Open the Users page and see panel accounts"},
	{capUsersCreate, "users", "create", "Create users", "Create new panel accounts"},
	{capUsersEdit, "users", "edit", "Edit users", "Change role, permissions and status of accounts"},
	{capUsersDelete, "users", "delete", "Delete users", "Move a panel account to the trash, keeping its server access"},
	{capUsersRestore, "users", "restore", "Restore users", "Bring a deleted account back with the access it had"},
	{capUsersResetPassword, "users", "reset_password", "Reset passwords", "Issue a new password for an account"},

	{capNodesView, "nodes", "view", "View nodes", "Open the Nodes page and see node health"},
	{capNodesCreate, "nodes", "create", "Register nodes", "Register a new node and issue its agent token"},
	{capNodesDelete, "nodes", "delete", "Remove nodes", "Remove a node from the panel"},

	{capServersViewAll, "servers", "view_all", "View all servers", "See every server, not only the shared ones"},
	{capServersCreate, "servers", "create", "Create servers", "Create new server instances"},
	{capServersEdit, "servers", "edit", "Edit servers", "Rename a server and change memory or host port"},
	{capServersDelete, "servers", "delete", "Delete servers", "Move a server to the trash, keeping its data and files"},
	{capServersRestore, "servers", "restore", "Restore servers", "Bring a deleted server back from the trash"},
	{capServersPurge, "servers", "purge", "Purge servers", "Permanently erase a deleted server with its data and files"},
	{capServersPower, "servers", "power", "Power controls", "Start, stop, restart and kill servers"},

	{capConsoleView, "console", "view", "View console", "Stream live console output and read history"},
	{capConsoleWrite, "console", "write", "Run commands", "Send commands to the server console"},

	{capFilesView, "files", "view", "Browse files", "List directories and read file contents"},
	{capFilesWrite, "files", "write", "Edit files", "Write files, create directories and rename entries"},
	{capFilesDelete, "files", "delete", "Delete files", "Delete files and directories"},

	{capPlayersView, "players", "view", "View players", "See the player list, allowlist and bans"},
	{capPlayersManage, "players", "manage", "Manage allowlist", "Add and remove allowlisted players"},
	{capPlayersModerate, "players", "moderate", "Moderate players", "Op, deop, kick, ban and pardon players"},

	{capSettingsView, "settings", "view", "View settings", "Read the game config file and server settings"},
	{capSettingsEdit, "settings", "edit", "Edit settings", "Change values in the game config file"},

	{capAccessView, "access", "view", "View access", "See who a server is shared with"},
	{capAccessManage, "access", "manage", "Manage access", "Share a server with users and revoke access"},
}

// capabilityCatalog = catalog ตัวเต็มของ instance นี้ = base + 1 entry ต่อเกมที่ลงทะเบียนไว้
// (label/description ของแถวเกมมาจาก Definition — ไม่มีชื่อเกมฝังอยู่ใน httpapi)
func (a *API) capabilityCatalog() []capabilityMeta {
	defs := a.games.All()
	out := make([]capabilityMeta, 0, len(baseCapabilityCatalog)+len(defs))
	out = append(out, baseCapabilityCatalog...)
	for _, d := range defs {
		out = append(out, capabilityMeta{
			Key:         gameCapKey(d.ID),
			Group:       capGamesGroup,
			Action:      d.ID,
			Label:       d.Label,
			Description: "Create " + d.Label + " servers",
		})
	}
	return out
}

// serverScopedCaps คือ subset ของ catalog ที่มีความหมาย "ต่อ server ตัวหนึ่ง" —
// ชั้น server (server_permissions.capabilities) grant ได้เฉพาะ key พวกนี้ แล้ว enforce
// แบบ AND กับ global capability (ดู effectiveServerCap). global-only cap (users.*, nodes.*,
// servers.view_all/create, access.*) ไม่อยู่ในนี้: users/nodes ไม่ผูก server, view_all/create
// เป็นสิทธิ์ระดับ panel, ส่วน access (แชร์ server ให้คนอื่น) เป็นของ owner เท่านั้น
var serverScopedCaps = map[string]bool{
	capServersEdit:     true,
	capServersDelete:   true,
	capServersRestore:  true,
	capServersPurge:    true,
	capServersPower:    true,
	capConsoleView:     true,
	capConsoleWrite:    true,
	capFilesView:       true,
	capFilesWrite:      true,
	capFilesDelete:     true,
	capPlayersView:     true,
	capPlayersManage:   true,
	capPlayersModerate: true,
	capSettingsView:    true,
	capSettingsEdit:    true,
}

// validateServerCapabilities: ทุก key ต้องเป็น server-scoped cap ไม่งั้น reject
func validateServerCapabilities(keys []string) bool {
	for _, k := range keys {
		if !serverScopedCaps[k] {
			return false
		}
	}
	return true
}

// dedupStrings คืน slice ใหม่ที่ตัดค่าซ้ำ (คงลำดับที่เจอครั้งแรก) — ใช้ normalize grant list
func dedupStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (a *API) isKnownCapability(key string) bool {
	for _, c := range a.capabilityCatalog() {
		if c.Key == key {
			return true
		}
	}
	return false
}

// validateCapabilities: ทุก key ต้องอยู่ใน catalog ไม่งั้น reject (400 invalid_capability)
func (a *API) validateCapabilities(keys []string) bool {
	for _, k := range keys {
		if !a.isKnownCapability(k) {
			return false
		}
	}
	return true
}

// hasCapability: is_admin ครอบทุก capability โดยปริยาย ; ไม่งั้นเช็คใน list ของ user
func hasCapability(u *store.User, key string) bool {
	return u.IsAdmin || slices.Contains(u.Capabilities, key)
}
