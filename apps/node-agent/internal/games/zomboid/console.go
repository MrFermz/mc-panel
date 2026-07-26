// console.go — วิธีอ่าน "สถานะในเกม" ของ Project Zomboid จาก console (games.ConsoleSpec)
//
// เฟสนี้อ่านได้แค่ **ready** เท่านั้น (บรรทัด `*** SERVER STARTED ***` ที่เกมพิมพ์เมื่อ
// โหลด world เสร็จ) — ยังไม่มีรายชื่อผู้เล่นออนไลน์/metric ให้ dashboard:
//
//	คำสั่ง `players` ของ PZ ตอบเป็น **หลายบรรทัด** (บรรทัดหัวข้อ + ชื่อละบรรทัด)
//	แต่ games.ConsoleSpec.Parse ตีความทีละบรรทัดอิสระ (EventRoster = 1 บรรทัด = ทั้ง roster)
//	จึงยัดลง model ปัจจุบันไม่ได้ ต้องรอรอบที่แก้ console model ให้ parser มี state ได้ก่อน
//	ระหว่างนี้ RosterCommand เว้นว่าง = tracker ไม่ยิงคำสั่งอะไรเลย และ online_players เป็น 0
//	(0 ที่แปลว่า "ยังอ่านไม่ได้" ดีกว่าเดาจาก join/left ที่ไม่มีตัว resync ให้ถูกต้อง)
package zomboid

import (
	"strings"

	"github.com/game-manager/node-agent/internal/games"
)

func consoleSpec() games.ConsoleSpec {
	return games.ConsoleSpec{
		RosterCommand: "",
		MetricCommand: "",
		Parse:         parseConsoleLine,
	}
}

// readyLine = บรรทัดที่ PZ พิมพ์เมื่อ server พร้อมรับ connection แล้ว
const readyLine = "SERVER STARTED"

func parseConsoleLine(line string) games.Event {
	if strings.Contains(line, readyLine) {
		return games.Event{Kind: games.EventReady}
	}
	return games.Event{}
}
