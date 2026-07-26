// keyvalue.go — ConfigFormat สำเร็จรูปสำหรับไฟล์ `key=value` บรรทัดละคู่ (java .properties,
// ini แบบไม่มี section) ซึ่งเป็นรูปแบบที่เกมหลายเกมใช้ตรงกัน
//
// นี่คือ **ไวยากรณ์ของไฟล์** ไม่ใช่ความรู้ของเกม — เกมเป็นคนบอกว่าไฟล์ชื่ออะไรและมี key อะไรบ้าง
// (ConfigSpec ของ definition) แล้วหยิบ format ตัวนี้ไปใช้ถ้าไวยากรณ์ตรงกัน
package games

import "strings"

// KeyValueFormat = ไฟล์ text ที่แต่ละบรรทัดเป็น `key=value` และ comment ขึ้นต้นด้วย # หรือ !
type KeyValueFormat struct{}

// Parse แยก key=value จาก text (ข้าม comment/บรรทัดว่าง). split ตัว `=` แรกเท่านั้น
func (KeyValueFormat) Parse(text string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		if isCommentOrBlank(line) {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(line[i+1:])
	}
	return out
}

// Merge รวมค่าที่จะเปลี่ยนกลับเข้าไฟล์ โดยรักษา comment/บรรทัดว่าง/ลำดับ key เดิม
// (key ที่มีอยู่แล้ว → แทนค่าในบรรทัดนั้น; catalog key ที่ยังไม่มี → append ต่อท้าย)
// ส่วนที่เหลือ byte-identical
func (KeyValueFormat) Merge(text string, values map[string]string, order []ConfigField) string {
	lines := strings.Split(text, "\n")
	seen := make(map[string]bool)

	for idx, line := range lines {
		if isCommentOrBlank(line) {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		if v, ok := values[key]; ok {
			lines[idx] = key + "=" + v
			seen[key] = true
		}
	}

	// append catalog key ที่ยังไม่มีในไฟล์ ตามลำดับ catalog (deterministic)
	var appended []string
	for _, f := range order {
		if v, ok := values[f.Key]; ok && !seen[f.Key] {
			appended = append(appended, f.Key+"="+v)
		}
	}

	if len(appended) > 0 {
		// ตัด trailing empty line ที่เกิดจากไฟล์ลงท้ายด้วย "\n" ก่อน append เพื่อไม่ให้เกิดบรรทัดว่างคั่น
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, appended...)
	}

	return strings.Join(lines, "\n")
}

func isCommentOrBlank(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!")
}
