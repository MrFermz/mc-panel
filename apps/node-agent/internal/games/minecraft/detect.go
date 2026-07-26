// detect.go — เดาเวอร์ชันจริงของ server ที่ user นำเข้ามา (jar bundle version.json มาให้)
// best-effort เท่านั้น: เดาไม่ได้ = คืน "" แล้วให้ caller fallback ไปค่าที่ user กรอก
package minecraft

import (
	"archive/zip"
	"encoding/json"
	"io"
	"regexp"
)

var versionTokenRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// detectVersion เดา version จาก jar: version.json ใน jar ก่อน แล้ว fallback ชื่อไฟล์เดิม
func detectVersion(artifactPath, originalName string) string {
	if artifactPath != "" {
		if v := versionFromJarManifest(artifactPath); v != "" {
			return v
		}
	}
	// fallback: token version ในชื่อไฟล์เดิม เช่น paper-1.21.1.jar
	if originalName != "" {
		if v := versionTokenRe.FindString(originalName); v != "" {
			return v
		}
	}
	return ""
}

// versionFromJarManifest เปิด jar เป็น zip อ่าน version.json ที่ root → คืน id/name
// (vanilla/paper/fabric bundle ไฟล์นี้มา) — อ่านไม่ได้/ไม่มีให้คืน ""
func versionFromJarManifest(jarPath string) string {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "version.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		// version.json เล็ก (ไม่กี่ร้อย byte) — จำกัดขนาดกันไฟล์ผิดปกติ
		b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		rc.Close()
		if err != nil {
			return ""
		}
		var vj struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(b, &vj) != nil {
			return ""
		}
		if vj.ID != "" {
			return vj.ID
		}
		return vj.Name
	}
	return ""
}
