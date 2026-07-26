// Package gamestate ติดตามสถานะ "ภายในเกม" ของแต่ละ instance (ผู้เล่นออนไลน์, metric ประจำเกม)
// ที่ container stats มองไม่เห็น — เกมส่วนใหญ่ไม่มี API ให้ agent ถาม จึงต้องอ่านจาก console
//
// package นี้เป็นเครื่องจักรล้วน ๆ ไม่รู้จักเกมใด ๆ: คำสั่งที่ยิงและการแปลบรรทัดมาจาก
// games.ConsoleSpec ของ instance นั้น (ดู internal/games)
//
// วิธีทำงาน: tracker เกาะกับ console session (attach = server รันอยู่ = เขียน stdin ได้)
//   - ตั้งต้น/resync ด้วย RosterCommand เป็นระยะ → ได้รายชื่อ + max players ที่เชื่อถือได้
//   - ระหว่างรอบ resync อัปเดตทันทีจาก event join/left
//   - MetricCommand ของเกมนั้น ยิงไปพร้อมกันทุกรอบ (ค่าต้อง refresh) ถ้ารอบแรก
//     server ตอบว่าไม่รู้จักคำสั่ง = variant นี้ไม่รองรับ จำไว้แล้วเลิกถามตลอด session
//
// reply ของคำสั่งที่ tracker ยิงเองจะถูกกรองออกจาก console ที่ user เห็น (ดู ObserveLine)
package gamestate

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/game-manager/node-agent/internal/games"
)

const (
	// resyncInterval — roster command เป็น source of truth; join/left คอยอัปเดตระหว่างรอบ
	// ไม่ถี่กว่านี้เพราะทุกครั้งคือคำสั่งจริงที่วิ่งเข้า server thread
	resyncInterval = 30 * time.Second
	// replyWindow — ช่วงที่ถือว่าบรรทัดที่เข้ามาเป็น reply ของคำสั่งที่ tracker เพิ่งยิง
	// (จึงกรองออกจาก console ของ user). กว้างพอสำหรับ server ที่ tick ช้า
	replyWindow = 3 * time.Second
	// readyFallback — ยิงคำสั่งแรกช้าสุดเท่านี้ถ้ายังไม่เห็น event ready
	// จำเป็นสำหรับเคส agent re-attach เข้า server ที่ start เสร็จไปนานแล้ว (ไม่มีบรรทัด ready
	// ให้เห็นอีก) — server ที่รันอยู่แล้วยิงคำสั่งได้ปลอดภัย ส่วน fresh start จะรอ ready จริงก่อนเสมอ
	readyFallback = 60 * time.Second
)

// CommandWriter คือส่วนของ console.Manager ที่ tracker ต้องใช้ (แยก interface ตัดวงจร import)
type CommandWriter interface {
	WriteInput(serverID, command string) error
}

// GameLookup คือส่วนของ games.Registry ที่ tracker ต้องใช้ — หา console spec ของ instance
type GameLookup interface {
	ConsoleSpecFor(serverID string) (games.ConsoleSpec, bool)
}

// Snapshot คือค่าที่ serverstats อ่านไปแนบกับ ServerStats แต่ละรอบ
type Snapshot struct {
	Online     []string
	MaxPlayers int
	// Metric = ค่าประจำเกมที่วัดจาก console (minecraft = TPS) — เดินทางไปเป็น field tick_rate
	// 0 เมื่อ variant นี้ไม่มีคำสั่งนั้น หรือยัง probe ไม่เสร็จ
	Metric float64
}

// serverState = สถานะต่อ 1 server ที่ console attach อยู่
type serverState struct {
	console    games.ConsoleSpec
	online     map[string]struct{}
	maxPlayers int
	metric     float64
	// metricUnsupported = probe แล้วพบว่า server ไม่รู้จัก MetricCommand — ไม่ยิงซ้ำ
	metricUnsupported bool
	// metricSeen = เคยอ่านค่าได้จริงอย่างน้อยครั้งหนึ่ง = server นี้รองรับแน่นอน
	metricSeen bool
	// เวลาที่ยิงคำสั่งล่าสุด ใช้ตัดสินว่าบรรทัดที่เข้ามาเป็น reply ของเรา (กรองทิ้ง) หรือของ user
	rosterSentAt time.Time
	metricSentAt time.Time
	stop         chan struct{}
	// ready ปิดเมื่อเห็น event ready ของเกม = โหลดเสร็จ ยิงคำสั่งเข้า console ได้ปลอดภัย —
	// ยิงก่อนหน้านั้นเกมอาจ crash/สแปม error (minecraft: NPE ใน CommandSourceStack). ปิดครั้งเดียว
	ready       chan struct{}
	readyClosed bool
}

type Tracker struct {
	mu      sync.Mutex
	writer  CommandWriter
	games   GameLookup
	servers map[string]*serverState
}

// NewTracker — games ใช้หา console spec ของแต่ละ instance (nil ไม่ได้)
func NewTracker(gl GameLookup) *Tracker {
	return &Tracker{games: gl, servers: make(map[string]*serverState)}
}

// SetWriter ผูก console.Manager เข้ากับ tracker — แยกจาก constructor เพราะสองตัวอ้างถึงกัน
// (Manager ต้องมี tracker เป็น observer ตั้งแต่สร้าง, tracker ต้องเขียน stdin ผ่าน Manager)
func (t *Tracker) SetWriter(w CommandWriter) {
	t.mu.Lock()
	t.writer = w
	t.mu.Unlock()
}

// OnAttach เริ่มติดตาม server ตัวนี้ — console.Manager เรียกเมื่อ attach สำเร็จ
// server ที่หา console spec ไม่ได้ (เกมที่ agent ไม่รู้จัก) จะไม่ถูกติดตามเลย
func (t *Tracker) OnAttach(serverID string) {
	spec, ok := t.games.ConsoleSpecFor(serverID)
	if !ok || spec.Parse == nil {
		log.Printf("gamestate: no console spec for server=%s, in-game state disabled", serverID)
		return
	}

	t.mu.Lock()
	if _, ok := t.servers[serverID]; ok {
		t.mu.Unlock()
		return
	}
	st := &serverState{
		console: spec,
		online:  make(map[string]struct{}),
		stop:    make(chan struct{}),
		ready:   make(chan struct{}),
	}
	t.servers[serverID] = st
	t.mu.Unlock()

	go t.pollLoop(serverID, st.stop, st.ready)
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
	return Snapshot{Online: online, MaxPlayers: st.maxPlayers, Metric: st.metric}
}

// pollLoop ยิง roster/metric command จนกว่า console จะ detach
func (t *Tracker) pollLoop(serverID string, stop <-chan struct{}, ready <-chan struct{}) {
	// รอ server ประกาศ start เสร็จก่อนยิงคำสั่งแรก — ยิงตอนเกมยังโหลดไม่เสร็จ
	// ทำให้บางเกม (minecraft: Paper/vanilla) โยน error สแปม console ทุกรอบ
	// fallback เผื่อ re-attach เข้า server ที่ start ไปนานแล้ว (ไม่มีบรรทัด ready ให้เห็นอีก)
	select {
	case <-stop:
		return
	case <-ready:
	case <-time.After(readyFallback):
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
	roster := st.console.RosterCommand
	metric := st.console.MetricCommand
	now := time.Now()
	if roster != "" {
		st.rosterSentAt = now
	}
	if metric != "" && st.metricUnsupported {
		metric = ""
	}
	if metric != "" {
		st.metricSentAt = now
	}
	t.mu.Unlock()

	if roster != "" {
		if err := writer.WriteInput(serverID, roster); err != nil {
			log.Printf("gamestate roster command failed: server=%s err=%v", serverID, err)
			return
		}
	}
	if metric != "" {
		if err := writer.WriteInput(serverID, metric); err != nil {
			log.Printf("gamestate metric command failed: server=%s err=%v", serverID, err)
		}
	}
}

// ObserveLine อ่าน 1 บรรทัดจาก console ของ server แล้วอัปเดตสถานะ
// คืน false = ให้ console.Manager ทิ้งบรรทัดนี้ ไม่ต้องส่งให้ user เห็น
// (reply ของคำสั่งที่ tracker ยิงเอง — user ไม่ได้สั่งจึงไม่ควรเห็น)
func (t *Tracker) ObserveLine(serverID, line string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.servers[serverID]
	if !ok {
		return true
	}

	ev := st.console.Parse(line)
	if ev.Kind == games.EventNone {
		return true
	}
	now := time.Now()

	switch ev.Kind {
	case games.EventReady:
		// บรรทัดนี้ยังให้ user เห็นตามปกติ — เป็น log จริงของ server ไม่ใช่ reply ของ tracker
		if !st.readyClosed {
			st.readyClosed = true
			close(st.ready)
		}
		return true

	case games.EventRoster:
		// ยึดเป็น source of truth แทน set เดิมทั้งชุด
		if ev.MaxPlayers > 0 {
			st.maxPlayers = ev.MaxPlayers
		}
		st.online = make(map[string]struct{}, len(ev.Names))
		for _, name := range ev.Names {
			st.online[name] = struct{}{}
		}
		// user สั่งคำสั่งนี้เองก็เห็น reply ตามปกติ — ซ่อนเฉพาะรอบที่ tracker ยิง
		return now.Sub(st.rosterSentAt) > replyWindow

	case games.EventMetric:
		st.metric = ev.Metric
		st.metricSeen = true
		st.metricUnsupported = false
		return now.Sub(st.metricSentAt) > replyWindow

	case games.EventUnknownCommand:
		// variant ที่ไม่มี metric command — จำไว้แล้วเลิกถาม (กันสแปม error ทุกรอบ resync)
		// เช็ค metricSeen ด้วย: หน้าต่างนี้อิงเวลา ถ้า user พิมพ์คำสั่งผิดพอดีในช่วง replyWindow
		// หลัง probe จะถูกเข้าใจผิดว่า server ไม่รองรับ — server ที่เคยรายงานค่าได้แล้ว
		// ห้ามถูกปิดด้วยเหตุนี้
		if st.console.MetricCommand != "" && !st.metricSeen && now.Sub(st.metricSentAt) <= replyWindow {
			st.metricUnsupported = true
			st.metric = 0
			return false
		}
		return true

	case games.EventJoin:
		st.online[ev.Name] = struct{}{}
		return true

	case games.EventLeave:
		delete(st.online, ev.Name)
		return true
	}
	return true
}

// sortNames เรียงแบบ case-insensitive ให้ลำดับใน UI นิ่ง (map iteration สุ่มลำดับ)
func sortNames(names []string) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && strings.ToLower(names[j]) < strings.ToLower(names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}
