// console.go — วิธีอ่าน "สถานะในเกม" ของ Minecraft จาก console (games.ConsoleSpec)
//
// MC ไม่มี API ให้ agent ถาม จึงต้องยิงคำสั่งเข้า console แล้ว parse reply:
//   - `list` = source of truth ของรายชื่อ + max players (ทุก variant รองรับ)
//   - `tps`  = metric ที่มีเฉพาะ Paper/Spigot — variant อื่นตอบ "Unknown command"
//     ซึ่ง tracker จะจำแล้วเลิกถามตลอด session
//   - `Done (...)` = start เสร็จ world พร้อมรับคำสั่งแล้ว
//   - `X joined/left the game` = อัปเดตระหว่างรอบ resync
package minecraft

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/game-manager/node-agent/internal/games"
)

var (
	// "There are 2 of a max of 20 players online: Steve, Alex" (vanilla/paper)
	listReplyRe = regexp.MustCompile(`There are (\d+)(?:/(\d+))? (?:of a max of (\d+) )?players online:?\s*(.*)$`)
	// "TPS from last 1m, 5m, 15m: 19.98, 19.99, 20.0" (paper/spigot; อาจมี § สี ซึ่งถูกตัดไปแล้ว)
	tpsReplyRe = regexp.MustCompile(`TPS from last [^:]*:\s*([\d.]+)`)
	// § + 1 ตัวอักษร = color code ของ MC — ต้องตัดก่อน parse ไม่งั้น regex ไม่ match
	colorRe = regexp.MustCompile("§.")
)

func consoleSpec() games.ConsoleSpec {
	return games.ConsoleSpec{
		RosterCommand: "list",
		MetricCommand: "tps",
		Parse:         parseConsoleLine,
	}
}

// parseConsoleLine อ่าน 1 บรรทัดดิบจาก console แล้วบอกว่าเป็น event อะไร
func parseConsoleLine(line string) games.Event {
	// hot path: ทุกบรรทัดของทุก server วิ่งผ่านที่นี่ — คัดด้วย substring ถูก ๆ ก่อน
	// ไม่งั้น server ที่ log รัว ๆ (modded) จะเสีย CPU ไปกับ regex ที่ไม่มีวันแมตช์
	if !mayBeInteresting(line) {
		return games.Event{}
	}
	msg := stripColor(messageOf(line))
	if msg == "" {
		return games.Event{}
	}

	// "Done (12.345s)! For help, type ..." = start เสร็จ world พร้อมรับคำสั่ง
	if isStartupDone(msg) {
		return games.Event{Kind: games.EventReady}
	}

	if m := listReplyRe.FindStringSubmatch(msg); m != nil {
		ev := games.Event{Kind: games.EventRoster, Names: parseNameList(m[4])}
		if max := firstNonEmpty(m[2], m[3]); max != "" {
			if n, err := strconv.Atoi(max); err == nil {
				ev.MaxPlayers = n
			}
		}
		return ev
	}

	// reply ของ `tps` — เอาเฉพาะค่า 1m (ตัวแรก) ที่สะท้อนสถานะปัจจุบันที่สุด
	if m := tpsReplyRe.FindStringSubmatch(msg); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return games.Event{Kind: games.EventMetric, Metric: v}
		}
		return games.Event{}
	}

	if isUnknownCommand(msg) {
		return games.Event{Kind: games.EventUnknownCommand}
	}

	if name, ok := parseSuffix(msg, " joined the game"); ok {
		return games.Event{Kind: games.EventJoin, Name: name}
	}
	if name, ok := parseSuffix(msg, " left the game"); ok {
		return games.Event{Kind: games.EventLeave, Name: name}
	}
	return games.Event{}
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

// mayBeInteresting คัดกรองหยาบ ๆ ว่าบรรทัดนี้อาจเป็นสิ่งที่ parser สนใจ
// ต้องครอบคลุมกว่าตัว parser จริงเสมอ (false negative = ผู้เล่นหาย/TPS ไม่อัปเดต)
func mayBeInteresting(line string) bool {
	return strings.Contains(line, "the game") ||
		strings.Contains(line, "players online") ||
		strings.Contains(line, "TPS from last") ||
		strings.Contains(line, "Unknown") ||
		strings.Contains(line, "<--[HERE]") ||
		strings.Contains(line, "Done (")
}

// isStartupDone จับบรรทัดที่ vanilla/paper/fabric/forge พิมพ์เมื่อ start เสร็จ:
// `Done (12.345s)! For help, type "help"` — world โหลดเสร็จ ยิงคำสั่งเข้า console ได้แล้ว
func isStartupDone(msg string) bool {
	return strings.HasPrefix(msg, "Done (") && strings.Contains(msg, ")")
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

// parseSuffix — ข้อความ join/leave ของ vanilla ที่ fork ทั้งหมดใช้ตาม
// ต้องเป็นชื่อผู้เล่นล้วน ๆ (ไม่มีช่องว่าง) กันบรรทัดของ plugin ที่ลงท้ายเหมือนกันหลุดเข้ามา
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

func parseNameList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		// paper แปะ suffix บางอย่างต่อท้ายชื่อได้ — เอาเฉพาะ token แรก
		if i := strings.IndexAny(name, " \t"); i >= 0 {
			name = name[:i]
		}
		if name != "" && validName(name) {
			out = append(out, name)
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
