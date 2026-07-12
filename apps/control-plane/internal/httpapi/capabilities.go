package httpapi

import (
	"slices"

	"github.com/mc-panel/control-plane/internal/store"
)

// capabilityMeta คือ 1 entry ใน catalog global capability (ดู docs/api.md).
// catalog เป็น source of truth ฝั่ง control-plane — ห้ามรับ/เก็บ key นอกนี้
type capabilityMeta struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// capability key — ใช้ const แทน string literal ที่จุด enforce เพื่อกันพิมพ์ผิดเงียบ ๆ
const (
	capManageUsers    = "users.manage"
	capManageNodes    = "nodes.manage"
	capCreateServers  = "servers.create"
	capViewAllServers = "servers.view_all"
)

// capabilityCatalog รายการ capability ที่ระบบรู้จักทั้งหมด (ลำดับคงที่สำหรับ UI)
var capabilityCatalog = []capabilityMeta{
	{Key: capManageUsers, Label: "Manage users", Description: "View the Users page and create/edit/reset users"},
	{Key: capManageNodes, Label: "Manage nodes", Description: "View the Nodes page and register/remove nodes"},
	{Key: capCreateServers, Label: "Create servers", Description: "Create new server instances"},
	{Key: capViewAllServers, Label: "View all servers", Description: "See every server, not only ones shared with the user"},
}

func isKnownCapability(key string) bool {
	for _, c := range capabilityCatalog {
		if c.Key == key {
			return true
		}
	}
	return false
}

// validateCapabilities: ทุก key ต้องอยู่ใน catalog ไม่งั้น reject (400 invalid_capability)
func validateCapabilities(keys []string) bool {
	for _, k := range keys {
		if !isKnownCapability(k) {
			return false
		}
	}
	return true
}

// hasCapability: is_admin ครอบทุก capability โดยปริยาย ; ไม่งั้นเช็คใน list ของ user
func hasCapability(u *store.User, key string) bool {
	return u.IsAdmin || slices.Contains(u.Capabilities, key)
}
