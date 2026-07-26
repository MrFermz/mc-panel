// Package games นิยาม "เกม" ที่ panel รองรับ (game definition)
//
// ความรู้เฉพาะเกมทุกอย่างที่ control-plane ต้องใช้ — ชนิดของ server (variant), กติกาเวอร์ชัน,
// runtime image, catalog ของไฟล์ config, กติกาผู้เล่น/allowlist/คำสั่ง moderation — อยู่ใน
// Definition ตัวเดียว ส่วนที่เหลือของ control-plane ทำงานผ่าน Registry นี้เท่านั้น
// **ห้ามเขียน switch ตามชื่อเกมหรือชื่อ variant กระจายอยู่ในโค้ดอื่นอีก** — เพิ่มเกมใหม่ต้อง
// แปลว่า "เพิ่ม Definition หนึ่งตัว" ไม่ใช่ไล่แก้ handler ทีละเส้น
//
// คู่ขนานกับ package ชื่อเดียวกันฝั่ง node-agent ซึ่งถือความรู้ฝั่งรันจริง (provision/launch/
// console parsing) — สอง app แยก module กันจึงไม่ import ข้ามกัน (ดู CLAUDE.md เรื่อง submodules)
// สิ่งที่ต้องตรงกันสองฝั่งคือ **id ของเกมและ variant** เท่านั้น ซึ่งเดินทางผ่าน job payload
package games

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// DefaultID = เกมที่ใช้เมื่อ caller ไม่ระบุ (row/job ที่สร้างไว้ก่อนมี field `game`)
const DefaultID = "minecraft"

var (
	// ErrUnknownVariant: variant ที่ขอไม่มีอยู่ในเกมนี้
	ErrUnknownVariant = errors.New("games: unknown variant")
	// ErrPlayerNotFound: identity service ของเกมยืนยันว่าไม่มีผู้เล่นชื่อนี้
	ErrPlayerNotFound = errors.New("games: player not found")
	// ErrNoAvatar: ผู้เล่นคนนี้ไม่มีรูปประจำตัวให้แสดง (uuid offline-mode / ไม่มี texture)
	ErrNoAvatar = errors.New("games: no avatar for player")
)

// Definition = ทุกอย่างที่ control-plane ต้องรู้เกี่ยวกับเกมหนึ่งเกม
type Definition struct {
	// ID เดินทางไปกับ job payload และเก็บใน servers.game — ต้องตรงกับ id ฝั่ง agent
	ID string
	// Label ชื่อที่โผล่ใน UI และในข้อความ error ของ API
	Label string
	// LicenseName ชื่อข้อตกลงที่ user ต้องยอมรับก่อนสร้าง server ของเกมนี้
	// (minecraft = "Minecraft EULA") — โผล่ในข้อความ 400 license_required
	LicenseName string

	// Variants = ชนิดของ server ในเกมนี้ เรียงตามลำดับที่อยากให้ UI แสดง
	Variants []Variant

	// DefaultHostPort = port เริ่มต้นที่ใช้ไล่หา host port ว่างให้ฟอร์มสร้าง server
	DefaultHostPort int
	// MinMemoryMB = เพดานล่างของ memory_mb ที่ยอมให้ตั้ง
	MinMemoryMB int

	Version VersionSpec
	Config  ConfigSpec
	Players PlayerSpec
}

// Variant = ชนิดของ server ภายในเกมหนึ่ง (minecraft: vanilla/paper/fabric/forge/velocity)
// เดินทางใน API/DB ในชื่อเดิม `variant`
type Variant struct {
	ID    string
	Label string
	// RequiresLicense = variant นี้ต้องให้ user ติ๊กยอมรับ license ของเกมเองก่อนสร้าง
	// (ห้าม default true เด็ดขาด — user ต้องติ๊กเอง)
	RequiresLicense bool
}

// Variant คืน variant ตาม id — ไม่รู้จัก = ok=false (caller ตอบ 400 invalid_variant)
func (d *Definition) Variant(id string) (Variant, bool) {
	for _, v := range d.Variants {
		if v.ID == id {
			return v, true
		}
	}
	return Variant{}, false
}

// HasVariant = shorthand ของ Variant() เมื่อสนใจแค่ว่ามีจริงไหม
func (d *Definition) HasVariant(id string) bool {
	_, ok := d.Variant(id)
	return ok
}

// RequiresLicense = variant นี้บังคับให้ยอมรับ EULA ไหม (variant ที่ไม่รู้จักถือว่าไม่บังคับ —
// caller เช็ค HasVariant ก่อนอยู่แล้ว จึงไปไม่ถึงตรงนี้)
func (d *Definition) RequiresLicense(variantID string) bool {
	v, ok := d.Variant(variantID)
	return ok && v.RequiresLicense
}

// VariantList = รายชื่อ variant คั่นด้วย ", " สำหรับข้อความ error ("must be one of: ...")
func (d *Definition) VariantList() string {
	ids := make([]string, 0, len(d.Variants))
	for _, v := range d.Variants {
		ids = append(ids, v.ID)
	}
	return strings.Join(ids, ", ")
}

// ---------- เวอร์ชันของ server ----------

// VersionSpec = กติกาเรื่องเวอร์ชันของเกม (field `game_version` ใน API/DB — ชื่อเดิมที่คงไว้
// เพื่อไม่ break contract, ความหมายคือ "เวอร์ชันของเกม")
type VersionSpec struct {
	// MaxLen = ความยาวสูงสุดที่ยอมรับจาก user
	MaxLen int
	// List ดึงรายการเวอร์ชันของ variant จาก upstream official (nil = เกมนี้ไม่มี catalog)
	// คืน ErrUnknownVariant เมื่อ variant ไม่มีจริง
	List func(ctx context.Context, variant string) ([]string, error)
	// RuntimeImage = image ที่ job start_server สั่งให้ agent ใช้กับ variant/version นี้
	RuntimeImage func(variant, version string) string
}

// ---------- ไฟล์ config ที่แก้ผ่าน UI ----------

// ConfigSpec = ไฟล์ config หลักของ server ที่ panel เปิดให้แก้ผ่านหน้า settings
type ConfigSpec struct {
	// FileName = ไฟล์ที่ root ของ server dir (อ่าน/เขียนผ่าน file stream ของ agent)
	FileName string
	// EditableWhileRunning=false → PUT ตอบ 409 invalid_state ตอน server ไม่ได้หยุด
	// (เกมที่เขียนทับไฟล์ตอน shutdown เช่น Minecraft ต้องเป็น false)
	EditableWhileRunning bool
	// Fields = curated set ของ key ที่ให้แก้ผ่าน UI (key อื่นในไฟล์เก็บ verbatim ไม่แตะ)
	Fields []ConfigField
	// Format = วิธี parse/merge ไฟล์ โดยต้องรักษา comment/ลำดับ/key นอก catalog ไว้ครบ
	Format ConfigFormat
}

// ConfigField อธิบาย 1 key ใน catalog ให้ web render form ได้ + ให้ server validate ค่าที่ส่งมา
type ConfigField struct {
	Key     string
	Label   string
	Type    string // enum | int | bool | string
	Options []string
	Min     *int
	Max     *int
	Default string
}

// ConfigFormat = ไวยากรณ์ของไฟล์ config (Minecraft ใช้ java .properties)
type ConfigFormat interface {
	// Parse แยก key=value ออกจากเนื้อไฟล์
	Parse(text string) map[string]string
	// Merge เขียนค่าที่เปลี่ยนกลับเข้าไฟล์เดิม โดยส่วนที่ไม่เกี่ยวต้อง byte-identical
	// order = catalog ใช้จัดลำดับ key ที่ต้อง append เพิ่ม (ให้ผลลัพธ์ deterministic)
	Merge(text string, values map[string]string, order []ConfigField) string
}

// Field คืน field ใน catalog ตาม key
func (c ConfigSpec) Field(key string) (ConfigField, bool) {
	for _, f := range c.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return ConfigField{}, false
}

// Valid เช็คค่าที่ client ส่งมาเทียบกับชนิด/ช่วงของ field
// (ชนิดเป็นของกลาง ไม่ผูกเกม — เกมกำหนดแค่ว่า key ไหนเป็นชนิดอะไร)
func (f ConfigField) Valid(val string) bool {
	switch f.Type {
	case "enum":
		for _, opt := range f.Options {
			if val == opt {
				return true
			}
		}
		return false
	case "int":
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		if f.Min != nil && n < *f.Min {
			return false
		}
		if f.Max != nil && n > *f.Max {
			return false
		}
		return true
	case "bool":
		return val == "true" || val == "false"
	case "string":
		return true
	default:
		return false
	}
}

// Defaults = ค่า default ของทุก key ใน catalog (ใช้ตอนไฟล์ยังไม่มี/ยังไม่มี server)
func (c ConfigSpec) Defaults() map[string]string {
	out := make(map[string]string, len(c.Fields))
	for _, f := range c.Fields {
		out[f.Key] = f.Default
	}
	return out
}

// ---------- ผู้เล่น ----------

// PlayerSpec = กติกาผู้เล่นของเกม: identity, ไฟล์รายชื่อ, คำสั่ง moderation, สถิติเวลาเล่น
type PlayerSpec struct {
	// IdentityService ชื่อบริการที่ใช้ verify ผู้เล่น (โผล่ในข้อความ error: "could not reach X")
	IdentityService string
	// UsernameRule คำอธิบายกติกาชื่อ สำหรับข้อความ 400 invalid_username
	UsernameRule string
	// ValidateUsername กติกาชื่อของเกม — เช็คก่อนยิง identity service เสมอ
	ValidateUsername func(username string) bool
	// ConsoleSafeUsername ชื่อที่ปลอดภัยพอจะต่อเข้าไปในคำสั่ง console ได้ตรง ๆ
	// (ห้ามมี whitespace/newline — ไม่งั้นเป็น command injection เข้า server console)
	ConsoleSafeUsername func(username string) bool
	// Lookup verify ชื่อกับ identity service → uuid + ชื่อในรูป canonical
	// คืน ErrPlayerNotFound เมื่อไม่มีตัวตน, error อื่น = upstream ไม่พร้อม
	Lookup func(ctx context.Context, username string) (Profile, error)
	// Avatar รูปประจำตัวผู้เล่นเป็น PNG (nil = เกมนี้ไม่มีรูป) — คืน ErrNoAvatar เมื่อไม่มีรูป
	// ปกติ definition จะเสียบ avatarcache.Cache.Avatar เข้ามา ไม่ยิง upstream ตรง ๆ
	Avatar AvatarFetcher

	// Allowlist ไฟล์รายชื่อที่ panel rebuild จาก DB ทุกครั้งที่รายชื่อเปลี่ยน
	Allowlist AllowlistSpec
	// StateFiles ไฟล์ของเกมเองที่อ่านมา merge เข้ารายชื่อผู้เล่น (เห็นแล้ว/op/แบน)
	StateFiles []StateFile
	// Actions = allow-list ของ action → คำสั่ง console (ห้ามรับคำสั่งดิบจาก client)
	Actions map[string]string
	// Playtime ที่อยู่ของสถิติเวลาเล่น (nil = เกมนี้ไม่มี)
	Playtime *PlaytimeSpec
}

// AvatarFetcher คืน PNG รูปประจำตัวของผู้เล่นหนึ่งคน — คืน ErrNoAvatar เมื่อไม่มีรูป
type AvatarFetcher func(ctx context.Context, id uuid.UUID) ([]byte, error)

// Profile = ผลของการ verify ชื่อกับ identity service
type Profile struct {
	UUID     uuid.UUID
	Username string
}

// AllowlistEntry = 1 แถวใน DB ที่จะถูกเขียนลงไฟล์ allowlist
type AllowlistEntry struct {
	UUID     uuid.UUID
	Username string
}

// AllowlistSpec = ไฟล์ที่ panel เป็นเจ้าของ (DB คือ source of truth, ไฟล์ rebuild ทุกครั้ง)
type AllowlistSpec struct {
	FileName string
	// EnabledKey = key ใน ConfigSpec ที่บอกว่ารายชื่อนี้ถูกบังคับใช้จริงไหม
	// (ค่า "true" = เปิด) — "" แปลว่าเกมนี้บังคับใช้เสมอ
	EnabledKey string
	// Encode สร้างเนื้อไฟล์จากรายชื่อใน DB
	Encode func(entries []AllowlistEntry) ([]byte, error)
	// ReloadCommand คำสั่ง console ที่ทำให้ไฟล์มีผลทันทีโดยไม่ต้อง restart ("" = ไม่มี)
	ReloadCommand string
}

// PlayerFlag = สถานะที่ติดให้ผู้เล่นเมื่อเจอชื่อในไฟล์ state หนึ่งไฟล์
type PlayerFlag string

const (
	FlagSeen   PlayerFlag = "seen"
	FlagOp     PlayerFlag = "op"
	FlagBanned PlayerFlag = "banned"
)

// StateFile = ไฟล์ของเกมที่ panel อ่านอย่างเดียวเพื่อ merge เข้ารายชื่อผู้เล่น
type StateFile struct {
	// Path relative ต่อ root ของ server dir
	Path string
	Flag PlayerFlag
	// Decode แปลงเนื้อไฟล์เป็นรายชื่อ — parse ไม่ได้ให้คืน error (caller ถือเป็น best-effort)
	Decode func(content []byte) ([]PlayerRef, error)
}

// PlayerRef = ผู้เล่นหนึ่งคนที่อ่านได้จากไฟล์ state (uuid ดิบตามที่อยู่ในไฟล์)
type PlayerRef struct {
	UUID string
	Name string
}

// PlaytimeSpec = ที่อยู่ของสถิติเวลาเล่นต่อผู้เล่น (ไฟล์ละคน)
type PlaytimeSpec struct {
	// SaveNameKey = key ใน ConfigSpec ที่บอกชื่อ save/world ปัจจุบัน (path ของไฟล์อิงชื่อนี้)
	SaveNameKey string
	// DefaultSaveName ใช้เมื่ออ่าน config ไม่ได้/ไม่มีค่า
	DefaultSaveName string
	// Path ที่อยู่ของไฟล์สถิติของผู้เล่นคนหนึ่ง (relative ต่อ jail)
	Path func(saveName string, playerUUID uuid.UUID) string
	// Decode แปลงเนื้อไฟล์เป็นวินาที — อ่านไม่ได้/ไม่รู้ให้คืน 0
	Decode func(content []byte) int64
}

// ---------- registry ----------

// Registry = เกมทั้งหมดที่ instance นี้รองรับ (immutable หลังสร้าง — อ่านพร้อมกันได้)
type Registry struct {
	byID  map[string]*Definition
	order []*Definition
}

// NewRegistry สร้าง registry จาก definition ตามลำดับที่ให้มา (ลำดับนี้คือลำดับที่ UI เห็น)
func NewRegistry(defs ...*Definition) *Registry {
	r := &Registry{byID: make(map[string]*Definition, len(defs)), order: defs}
	for _, d := range defs {
		r.byID[d.ID] = d
	}
	return r
}

// Get คืน definition ตาม id แบบตรงตัว
func (r *Registry) Get(id string) (*Definition, bool) {
	d, ok := r.byID[id]
	return d, ok
}

// Resolve เหมือน Get แต่ id ว่างแปลว่า DefaultID — ใช้กับ row/job ที่สร้างก่อนมี field `game`
// และกับ query param ที่ client ไม่ได้ส่งมา
func (r *Registry) Resolve(id string) (*Definition, bool) {
	if id == "" {
		id = DefaultID
	}
	return r.Get(id)
}

// All คืนทุกเกมตามลำดับที่ลงทะเบียนไว้
func (r *Registry) All() []*Definition { return r.order }

// Default คืนเกม DefaultID — nil ถ้าไม่ได้ลงทะเบียนไว้ (ไม่ควรเกิดขึ้นใน production)
func (r *Registry) Default() *Definition { return r.byID[DefaultID] }
