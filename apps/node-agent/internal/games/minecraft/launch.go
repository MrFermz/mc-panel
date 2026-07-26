// launch.go — วิธี "รัน" instance ของเกมนี้: launch script, คำสั่งปิด, env, ไฟล์ config
// ที่ panel เขียนให้ตอน provision และชื่อ artifact หลักที่ launch script คาดหวัง
package minecraft

import (
	"fmt"

	"github.com/game-manager/node-agent/internal/games"
)

// stopCommand — velocity ใช้ `end`, ที่เหลือใช้ `stop`
func stopCommand(variant string) string {
	if variant == "velocity" {
		return "end"
	}
	return "stop"
}

// launchScript — java ต้องเป็น process สุดท้ายผ่าน exec เสมอ เพื่อให้เป็น PID 1
// (รับ stdin จาก docker attach และ SIGTERM ตรง ไม่ผ่าน shell)
func launchScript(variant string) string {
	header := "#!/bin/sh\ncd " + games.ContainerDataDir + "\n"
	const mem = "${GM_MEMORY_MB:-1024}"
	switch variant {
	case "velocity":
		return header + "exec java -Xms" + mem + "M -Xmx" + mem + "M -jar velocity.jar\n"
	case "forge":
		// forge ใหม่ (>=1.17) ได้ run.sh + อ่าน jvm args จาก user_jvm_args.txt
		// forge เก่าได้ jar ชื่อ forge-{mc}-{build}.jar รันตรง ๆ
		return header +
			"if [ -f run.sh ]; then\n" +
			"  echo \"-Xms" + mem + "M -Xmx" + mem + "M\" > user_jvm_args.txt\n" +
			"  exec sh run.sh nogui\n" +
			"fi\n" +
			"exec java -Xms" + mem + "M -Xmx" + mem + "M -jar forge-*.jar nogui\n"
	default: // vanilla / paper / fabric
		return header + "exec java -Xms" + mem + "M -Xmx" + mem + "M -jar server.jar nogui\n"
	}
}

// launchEnv — launch script อ่าน heap ที่จะส่งให้ JVM จาก GM_MEMORY_MB
func launchEnv(memoryMB int) []string {
	return []string{fmt.Sprintf("GM_MEMORY_MB=%d", HeapMB(memoryMB))}
}

// HeapMB แปลง memory_mb ที่ user จัดสรร (= hard limit ของทั้ง container) เป็น -Xmx ของ JVM
// JVM กินนอก heap อีกมาก (metaspace, code cache, thread stacks, direct buffers, GC overhead)
// วัดจริง: Paper 1.21 heap 1G โดน OOM kill ที่ limit 1.25x ตอน world-gen เลยกันไว้ ~1/3
// (limit ≈ 1.5x heap) แต่ไม่เกิน 2GB เพื่อไม่ให้เครื่องใหญ่เสียเปล่า และไม่เกินครึ่งของ limit
// เพื่อให้ instance เล็ก (256MB ซึ่งเป็นขั้นต่ำที่ API ยอม) ยังเหลือ heap พอ start ได้
func HeapMB(memoryMB int) int {
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

// seedFiles — ไฟล์ที่ panel เขียนให้ตอน provision
//   - eula.txt เขียนเฉพาะเมื่อ user ติ๊กยอมรับเอง (ระบบห้าม default eula ให้เด็ดขาด)
//   - server.properties เขียนเฉพาะเมื่อยังไม่มี (ห้ามทับของเดิม) และไม่ใช่ velocity ที่เป็น proxy
func seedFiles(variant string, acceptLicense bool) []games.SeedFile {
	var files []games.SeedFile
	if acceptLicense {
		files = append(files, games.SeedFile{
			Path: "eula.txt", Content: []byte("eula=true\n"), Mode: 0o644, Overwrite: true,
		})
	}
	if variant != "velocity" {
		// port ใน container ตายตัว 25565 เสมอ — host port ไปกำหนดที่ PortBindings ตอน start
		files = append(files, games.SeedFile{
			Path:      "server.properties",
			Content:   fmt.Appendf(nil, "server-port=%d\n", containerPort),
			Mode:      0o644,
			Overwrite: false,
		})
	}
	return files
}

// mainArtifact = ชื่อ jar ที่ launch script รัน — forge ใช้ run.sh/forge-*.jar จึงไม่ normalize
func mainArtifact(variant string) string {
	switch variant {
	case "forge":
		return ""
	case "velocity":
		return "velocity.jar"
	default:
		return "server.jar"
	}
}
