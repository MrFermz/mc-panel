// meta.go — .gamemanager/meta.json: บันทึกว่า instance บน disk ตัวนี้เป็นเกม/variant อะไร
//
// จำเป็นเพราะ job start/stop ไม่ได้พก game/variant มาด้วย (payload มีแค่ memory/port/image)
// runner จึงอ่านจากไฟล์นี้เพื่อรู้ว่าต้องใช้ definition ตัวไหน — provision เป็นคนเขียน
// ทุกครั้งที่ provision/import สำเร็จ
package games

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ContainerDataDir = ที่ที่ jail ของ instance ถูก bind เข้าไปใน container เสมอ
	// (runner เป็นคน bind, launch script ของแต่ละเกมต้องอ้างอิงค่านี้ไม่ใช่ hardcode เอง)
	ContainerDataDir = "/data"
	// PanelDir = directory ที่ panel เป็นเจ้าของใน jail ของแต่ละ server
	PanelDir = ".gamemanager"
	// MetaFileName = ชื่อไฟล์ metadata ใน PanelDir
	MetaFileName = "meta.json"
	// LaunchScriptName = script ที่ container รัน (provision เขียนจาก Definition.LaunchScript)
	LaunchScriptName = "launch.sh"
)

// InstanceMeta = เนื้อของ .gamemanager/meta.json — สัญญาระหว่าง provision กับ runner
type InstanceMeta struct {
	Game        string `json:"game"`
	Variant     string `json:"variant"`
	GameVersion string `json:"game_version"`
	// StopCommand ซ้ำกับ Definition.StopCommand(variant) โดยตั้งใจ — instance ที่ provision
	// ไว้ก่อนมี field `game` ยังหยุดได้ถูกต้องจากไฟล์นี้ตัวเดียว
	StopCommand string `json:"stop_command"`
}

// WriteInstanceMeta เขียนทับเสมอ — panel เป็นเจ้าของไฟล์นี้ ไม่ว่า zip ที่ import จะมี
// .gamemanager เดิมติดมาหรือไม่
func WriteInstanceMeta(dir string, meta InstanceMeta) error {
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, PanelDir, MetaFileName), append(b, '\n'), 0o644)
}

// InstanceLookup = "instance id บน disk ตัวนี้เป็นเกมอะไร" — ตัวเชื่อมระหว่าง registry
// กับ meta.json ที่ provision เขียนไว้ ใช้โดย runner และ tracker ที่ได้มาแค่ server id
type InstanceLookup struct {
	Registry *Registry
	DataDir  string
}

// DefinitionFor คืน definition + meta ของ instance
// meta อ่านไม่ได้ (instance เก่า/ไฟล์หาย) = เกม default; registry ไม่รู้จักเกมนั้น = ok=false
func (l InstanceLookup) DefinitionFor(serverID string) (*Definition, InstanceMeta, bool) {
	// serverID มาจาก job/label — ห้ามให้พาออกนอก DataDir
	if serverID == "" || strings.ContainsAny(serverID, `/\`) || serverID == "." || serverID == ".." {
		return nil, InstanceMeta{}, false
	}
	meta := ReadInstanceMeta(filepath.Join(l.DataDir, serverID))
	def, ok := l.Registry.Resolve(meta.Game)
	if !ok {
		return nil, meta, false
	}
	return def, meta, true
}

// ConsoleSpecFor = console spec ของ instance (gamestate.GameLookup)
func (l InstanceLookup) ConsoleSpecFor(serverID string) (ConsoleSpec, bool) {
	def, _, ok := l.DefinitionFor(serverID)
	if !ok {
		return ConsoleSpec{}, false
	}
	return def.Console, true
}

// ReadInstanceMeta อ่าน meta ของ instance — อ่านไม่ได้/ไฟล์เก่าที่ยังไม่มี field ไหน
// จะได้ zero value ของ field นั้น (caller ต้อง resolve ค่าว่างเป็น default เอง)
func ReadInstanceMeta(dir string) InstanceMeta {
	var meta InstanceMeta
	b, err := os.ReadFile(filepath.Join(dir, PanelDir, MetaFileName))
	if err != nil {
		return meta
	}
	if json.Unmarshal(b, &meta) != nil {
		return InstanceMeta{}
	}
	return meta
}
