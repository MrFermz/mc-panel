// Package mcstate ติดตามสถานะ "ภายในเกม" ของแต่ละ instance (ผู้เล่นออนไลน์, TPS)
// ที่ container stats มองไม่เห็น — Minecraft ไม่มี API ให้ agent ถาม จึงต้องอ่านจาก console
//
// วิธีทำงาน: tracker เกาะกับ console session (attach = server รันอยู่ = เขียน stdin ได้)
//   - ตั้งต้น/resync ด้วยคำสั่ง `list` เป็นระยะ → ได้รายชื่อ + max players ที่เชื่อถือได้ทุก server type
//   - ระหว่างรอบ resync อัปเดตทันทีจากบรรทัด "X joined the game" / "X left the game"
//   - TPS มีเฉพาะ Paper/Spigot — ยิงคำสั่ง `tps` ไปพร้อม `list` ทุกรอบ resync (ค่าต้อง refresh)
//     ถ้ารอบแรกเจอ "Unknown command" = server type นี้ไม่มีคำสั่งนี้ จำไว้แล้วเลิกถามตลอด session
//
// reply ของคำสั่งที่ tracker ยิงเองจะถูกกรองออกจาก console ที่ user เห็น (ดู ObserveLine)
package mcstate

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// resyncInterval — `list` เป็น source of truth; join/left คอยอัปเดตระหว่างรอบ
	// ไม่ถี่กว่านี้เพราะทุกครั้งคือคำสั่งจริงที่วิ่งเข้า server thread
	resyncInterval = 30 * time.Second
	// replyWindow — ช่วงที่ถือว่าบรรทัดที่เข้ามาเป็น reply ของคำสั่งที่ tracker เพิ่งยิง
	// (จึงกรองออกจาก console ของ user). กว้างพอสำหรับ server ที่ tick ช้า
	replyWindow = 3 * time.Second
)

// CommandWriter คือส่วนของ console.Manager ที่ tracker ต้องใช้ (แยก interface ตัดวงจร import)
type CommandWriter interface {
	WriteInput(serverID, command string) error
}

// Snapshot คือค่าที่ serverstats อ่านไปแนบกับ ServerStats แต่ละรอบ
type Snapshot struct {
	Online     []string
	MaxPlayers int
	// TPS = 0 เมื่อ server type ไม่มีคำสั่ง `tps` หรือยัง probe ไม่เสร็จ
	TPS float64
}

// serverState = สถานะต่อ 1 server ที่ console attach อยู่
type serverState struct {
	online     map[string]struct{}
	maxPlayers int
	tps        float64
	// tpsUnsupported = probe แล้วพบว่าไม่มีคำสั่ง `tps` (vanilla/fabric/forge) — ไม่ยิงซ้ำ
	tpsUnsupported bool
	// tpsSeen = เคยอ่านค่า TPS ได้จริงอย่างน้อยครั้งหนึ่ง = server นี้รองรับแน่นอน
	tpsSeen bool
	// เวลาที่ยิงคำสั่งล่าสุด ใช้ตัดสินว่าบรรทัดที่เข้ามาเป็น reply ของเรา (กรองทิ้ง) หรือของ user
	listSentAt time.Time
	tpsSentAt  time.Time
	stop       chan struct{}
}

type Tracker struct {
	mu      sync.Mutex
	writer  CommandWriter
	servers map[string]*serverState
}

func NewTracker() *Tracker {
	return &Tracker{servers: make(map[string]*serverState)}
}

// SetWriter ผูก console.Manager เข้ากับ tracker — แยกจาก constructor เพราะสองตัวอ้างถึงกัน
// (Manager ต้องมี tracker เป็น observer ตั้งแต่สร้าง, tracker ต้องเขียน stdin ผ่าน Manager)
func (t *Tracker) SetWriter(w CommandWriter) {
	t.mu.Lock()
	t.writer = w
	t.mu.Unlock()
}

// OnAttach เริ่มติดตาม server ตัวนี้ — console.Manager เรียกเมื่อ attach สำเร็จ
func (t *Tracker) OnAttach(serverID string) {
	t.mu.Lock()
	if _, ok := t.servers[serverID]; ok {
		t.mu.Unlock()
		return
	}
	st := &serverState{online: make(map[string]struct{}), stop: make(chan struct{})}
	t.servers[serverID] = st
	t.mu.Unlock()

	go t.pollLoop(serverID, st.stop)
}

// OnDetach ล้างสถานะทิ้ง — server หยุด/console หลุด = ไม่มีใครออนไลน์อีกต่อไป
// (ปล่อยค่าเก่าค้างไว้จะทำให้ dashboard โชว์ผู้เล่นของ session ที่จบไปแล้ว)
func (t *Tracker) OnDetach(serverID string) {
	t.mu.Lock()
	st, ok := t.servers[serverID]
	delete(t.servers, serverID)
	t.mu.Unlock()
	if ok {
		close(st.stop)
	}
}

// Snapshot คืนสถานะล่าสุดของ server — ไม่รู้จัก/ไม่ได้ attach = zero value
func (t *Tracker) Snapshot(serverID string) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.servers[serverID]
	if !ok {
		return Snapshot{}
	}
	online := make([]string, 0, len(st.online))
	for name := range st.online {
		online = append(online, name)
	}
	sortNames(online)
	return Snapshot{Online: online, MaxPlayers: st.maxPlayers, TPS: st.tps}
}

// pollLoop ยิง `list` (และ probe `tps` ครั้งแรก) จนกว่า console จะ detach
func (t *Tracker) pollLoop(serverID string, stop <-chan struct{}) {
	// หน่วงรอบแรก — server ที่เพิ่ง start ยังรับคำสั่งไม่ได้จนกว่า world จะโหลดเสร็จ
	select {
	case <-stop:
		return
	case <-time.After(5 * time.Second):
	}

	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()
	for {
		t.poll(serverID)
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func (t *Tracker) poll(serverID string) {
	t.mu.Lock()
	writer := t.writer
	st, ok := t.servers[serverID]
	if !ok || writer == nil {
		t.mu.Unlock()
		return
	}
	st.listSentAt = time.Now()
	probeTPS := !st.tpsUnsupported
	if probeTPS {
		st.tpsSentAt = time.Now()
	}
	t.mu.Unlock()

	if err := writer.WriteInput(serverID, "list"); err != nil {
		log.Printf("mcstate list failed: server=%s err=%v", serverID, err)
		return
	}
	if probeTPS {
		if err := writer.WriteInput(serverID, "tps"); err != nil {
			log.Printf("mcstate tps failed: server=%s err=%v", serverID, err)
		}
	}
}

// ---------- การอ่านบรรทัด ----------

var (
	// "There are 2 of a max of 20 players online: Steve, Alex" (vanilla/paper)
	listReplyRe = regexp.MustCompile(`There are (\d+)(?:/(\d+))? (?:of a max of (\d+) )?players online:?\s*(.*)$`)
	// "TPS from last 1m, 5m, 15m: 19.98, 19.99, 20.0" (paper/spigot; อาจมี § สี ซึ่งถูกตัดไปแล้ว)
	tpsReplyRe = regexp.MustCompile(`TPS from last [^:]*:\s*([\d.]+)`)
	// § + 1 ตัวอักษร = color code ของ MC — ต้องตัดก่อน parse ไม่งั้น regex ไม่ match
	colorRe = regexp.MustCompile("§.")
)

// ObserveLine อ่าน 1 บรรทัดจาก console ของ server แล้วอัปเดตสถานะ
// คืน false = ให้ console.Manager ทิ้งบรรทัดนี้ ไม่ต้องส่งให้ user เห็น
// (reply ของคำสั่งที่ tracker ยิงเอง — user ไม่ได้สั่งจึงไม่ควรเห็น)
func (t *Tracker) ObserveLine(serverID, line string) bool {
	// hot path: ทุกบรรทัดของทุก server วิ่งผ่านที่นี่ — คัดด้วย substring ถูก ๆ ก่อน
	// ไม่งั้น server ที่ log รัว ๆ (modded) จะเสีย CPU ไปกับ regex ที่ไม่มีวันแมตช์
	if !mayBeInteresting(line) {
		return true
	}
	msg := stripColor(messageOf(line))
	if msg == "" {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.servers[serverID]
	if !ok {
		return true
	}
	now := time.Now()

	// reply ของ `list` — ยึดเป็น source of truth แทน set เดิมทั้งชุด
	if m := listReplyRe.FindStringSubmatch(msg); m != nil {
		if max := firstNonEmpty(m[2], m[3]); max != "" {
			if n, err := strconv.Atoi(max); err == nil {
				st.maxPlayers = n
			}
		}
		st.online = parseNameList(m[4])
		// user สั่ง `list` เองก็เห็น reply ตามปกติ — ซ่อนเฉพาะรอบที่ tracker ยิง
		return now.Sub(st.listSentAt) > replyWindow
	}

	// reply ของ `tps` — เอาเฉพาะค่า 1m (ตัวแรก) ที่สะท้อนสถานะปัจจุบันที่สุด
	if m := tpsReplyRe.FindStringSubmatch(msg); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			st.tps = v
			st.tpsSeen = true
			st.tpsUnsupported = false
		}
		return now.Sub(st.tpsSentAt) > replyWindow
	}

	// vanilla/fabric/forge ไม่มีคำสั่ง `tps` — จำไว้แล้วเลิกถาม (กันสแปม error ทุกรอบ resync)
	// เช็ค tpsSeen ด้วย: หน้าต่างนี้อิงเวลา ถ้า user พิมพ์คำสั่งผิดพอดีในช่วง 3 วิหลัง probe
	// จะถูกเข้าใจผิดว่า server ไม่รองรับ — server ที่เคยรายงาน TPS ได้แล้วห้ามถูกปิดด้วยเหตุนี้
	if isUnknownCommand(msg) && !st.tpsSeen && now.Sub(st.tpsSentAt) <= replyWindow {
		st.tpsUnsupported = true
		st.tps = 0
		return false
	}

	if name, ok := parseJoin(msg); ok {
		st.online[name] = struct{}{}
		return true
	}
	if name, ok := parseLeave(msg); ok {
		delete(st.online, name)
		return true
	}
	return true
}

// messageOf ตัด prefix ของ log ("[12:34:56] [Server thread/INFO]: ") ออกให้เหลือข้อความจริง
// ใช้ "]: " ตัวแรก — ครอบคลุมทั้ง vanilla ([time] [thread/LEVEL]:) และ paper ([time LEVEL]:)
func messageOf(line string) string {
	i := strings.Index(line, "]: ")
	if i < 0 {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(line[i+len("]: "):])
}

// mayBeInteresting คัดกรองหยาบ ๆ ว่าบรรทัดนี้อาจเป็นสิ่งที่ ObserveLine สนใจ
// ต้องครอบคลุมกว่าตัว parser จริงเสมอ (false negative = ผู้เล่นหาย/TPS ไม่อัปเดต)
func mayBeInteresting(line string) bool {
	return strings.Contains(line, "the game") ||
		strings.Contains(line, "players online") ||
		strings.Contains(line, "TPS from last") ||
		strings.Contains(line, "Unknown") ||
		strings.Contains(line, "<--[HERE]")
}

// stripColor ตัด § color code — ข้ามไปเลยถ้าไม่มี (เลี่ยง alloc ของ regex ที่ไม่ได้แทนอะไร)
func stripColor(s string) string {
	if !strings.Contains(s, "§") {
		return s
	}
	return colorRe.ReplaceAllString(s, "")
}

func isUnknownCommand(msg string) bool {
	return strings.HasPrefix(msg, "Unknown or incomplete command") ||
		strings.HasPrefix(msg, "Unknown command") ||
		strings.Contains(msg, "<--[HERE]")
}

// parseJoin/parseLeave — ข้อความ join/leave ของ vanilla ที่ fork ทั้งหมดใช้ตาม
// ต้องเป็นชื่อผู้เล่นล้วน ๆ (ไม่มีช่องว่าง) กันบรรทัดของ plugin ที่ลงท้ายเหมือนกันหลุดเข้ามา
func parseJoin(msg string) (string, bool) {
	return parseSuffix(msg, " joined the game")
}

func parseLeave(msg string) (string, bool) {
	return parseSuffix(msg, " left the game")
}

func parseSuffix(msg, suffix string) (string, bool) {
	if !strings.HasSuffix(msg, suffix) {
		return "", false
	}
	name := strings.TrimSuffix(msg, suffix)
	if name == "" || strings.ContainsAny(name, " \t") || !validName(name) {
		return "", false
	}
	return name, true
}

// validName กันบรรทัดแชท/plugin หลุดมาเป็นชื่อผู้เล่น — เช็คหลวม ๆ พอให้ไม่ false positive
// ไม่บังคับ 16 ตัวแบบ Mojang เพราะ offline-mode ตั้งชื่อยาวกว่านั้นได้ และ Geyser/Bedrock
// เติม prefix (`.`/`*`) ให้ชื่อ — เข้มเกินไปจะทำผู้เล่นจริงหายจากรายชื่อแบบเงียบ ๆ
func validName(s string) bool {
	if len(s) > 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '.', r == '*', r == '-':
		default:
			return false
		}
	}
	return true
}

func parseNameList(s string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		// paper แปะ suffix บางอย่างต่อท้ายชื่อได้ — เอาเฉพาะ token แรก
		if i := strings.IndexAny(name, " \t"); i >= 0 {
			name = name[:i]
		}
		if name != "" && validName(name) {
			out[name] = struct{}{}
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// sortNames เรียงแบบ case-insensitive ให้ลำดับใน UI นิ่ง (map iteration สุ่มลำดับ)
func sortNames(names []string) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && strings.ToLower(names[j]) < strings.ToLower(names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}
