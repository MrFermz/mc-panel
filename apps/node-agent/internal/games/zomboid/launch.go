// launch.go — วิธี "รัน" instance ของเกมนี้: launch script, คำสั่งปิด, env, ไฟล์ config
// ที่ panel เขียนให้ตอน provision
package zomboid

import (
	"fmt"

	"github.com/game-manager/node-agent/internal/games"
)

// adminPasswordFile = ไฟล์ใน PanelDir ที่เก็บรหัส admin ของเกม (provision เป็นคนสร้าง,
// launch script เป็นคนอ่าน) — เกมไม่มีที่เก็บรหัสนี้ในไฟล์ config ของตัวเอง
const adminPasswordFile = "adminpassword"

// stopCommand — PZ เซฟ world แล้วปิดเมื่อพิมพ์ `quit` เข้า console
func stopCommand(string) string { return "quit" }

// launchScript — เกมพก JVM มาเอง (jre64/) ผ่านตัว launcher ของ Steam
// ต้อง exec ให้ start-server.sh เป็น process สุดท้าย เพื่อให้ stdin ของ docker attach
// ไหลถึงคอนโซลของเกมและ SIGTERM ไปถึงตัวเกม
func launchScript(string) string {
	return "#!/bin/sh\n" +
		"cd " + games.ContainerDataDir + "\n" +
		// ไฟล์จาก Steam บางรอบมาแบบไม่มี exec bit — เกมจะ start ไม่ขึ้นแบบเงียบ ๆ
		"chmod +x " + launcherName + " ProjectZomboid64 jre64/bin/java 2>/dev/null\n" +
		// heap ของ JVM อยู่ในไฟล์ของเกมเอง แต่ memory เป็นของที่ panel คุม จึงเขียนทับทุกครั้ง
		"if [ -f ProjectZomboid64.json ]; then\n" +
		"  sed -i \"s/\\\"-Xms[^\\\"]*\\\"/\\\"-Xms${GM_MEMORY_MB}m\\\"/; s/\\\"-Xmx[^\\\"]*\\\"/\\\"-Xmx${GM_MEMORY_MB}m\\\"/\" ProjectZomboid64.json\n" +
		"fi\n" +
		"exec ./" + launcherName +
		" -cachedir=" + games.ContainerDataDir + "/" + cacheDir +
		" -servername " + serverName +
		" -adminpassword \"$(cat " + games.PanelDir + "/" + adminPasswordFile + ")\"\n"
}

// launchEnv — launch script อ่าน heap ที่จะเขียนลงไฟล์ของเกมจาก GM_MEMORY_MB
func launchEnv(memoryMB int) []string {
	return []string{fmt.Sprintf("GM_MEMORY_MB=%d", heapMB(memoryMB))}
}

// heapMB แปลง memory_mb ที่ user จัดสรร (= hard limit ของทั้ง container) เป็น -Xmx ของ JVM
// เหตุผลเดียวกับ Minecraft: JVM กินนอก heap อีกมาก ถ้าตั้ง heap เท่า limit จะโดน OOM kill
// (คนละ definition กันเพราะเป็นความรู้ของเกม — สองเกมปรับค่าอิสระจากกันได้)
func heapMB(memoryMB int) int {
	reserve := memoryMB / 3
	if reserve < 256 {
		reserve = 256
	}
	if reserve > 2048 {
		reserve = 2048
	}
	if reserve > memoryMB/2 {
		reserve = memoryMB / 2
	}
	return memoryMB - reserve
}

// seedFiles — ไฟล์ config ที่ panel เขียนให้ตอน provision
// เกมนี้ไม่มี license ให้ยอมรับ (acceptLicense จึงไม่ถูกใช้) และไฟล์ ini ตัวเต็มเกมเขียนเอง
// ตอน start ครั้งแรก — เรา seed เฉพาะ port ให้ตรงกับ port ใน container เท่านั้น
// (Overwrite=false: ห้ามทับของที่ user/เกมแก้ไว้แล้ว)
func seedFiles(string, bool) []games.SeedFile {
	return []games.SeedFile{{
		Path:      ConfigPath,
		Content:   fmt.Appendf(nil, "DefaultPort=%d\nUDPPort=%d\n", gamePort, udpPort),
		Mode:      0o644,
		Overwrite: false,
	}}
}
