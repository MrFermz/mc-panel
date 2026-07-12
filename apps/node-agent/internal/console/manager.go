// Package console จัดการ attach session กับ stdin/stdout ของ MC container
// แล้ว stream บรรทัด log เป็น batch ขึ้น control plane ผ่าน gRPC
package console

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

const (
	flushInterval = 100 * time.Millisecond
	flushMaxLines = 50
	// flushMaxBytes จำกัดขนาด batch เป็น byte — 50 บรรทัด × maxLineBytes อาจทะลุ gRPC MaxRecvMsgSize
	// ฝั่ง server ได้ จึง flush เมื่อรวม byte เกิน ~1MB (อยู่ใต้ limit แน่นอน)
	flushMaxBytes = 1024 * 1024
	// MC log ปกติบรรทัดสั้น แต่ stacktrace ของ modded server ยาวได้มาก
	maxLineBytes = 1024 * 1024
)

// Attacher คือส่วนของ Runner ที่ console ต้องใช้ (แยก interface ตัดวงจร import)
type Attacher interface {
	AttachConsole(id string) (io.ReadWriteCloser, error)
}

// Sender คือส่วนของ grpc client ที่ console ต้องใช้
type Sender interface {
	SendConsoleOutput(serverID string, lines []string) error
}

type Manager struct {
	attacher Attacher
	sender   Sender

	mu       sync.Mutex
	sessions map[string]io.ReadWriteCloser
}

func NewManager(attacher Attacher, sender Sender) *Manager {
	return &Manager{
		attacher: attacher,
		sender:   sender,
		sessions: make(map[string]io.ReadWriteCloser),
	}
}

// Attach เปิด session กับ container ของ server (no-op ถ้า attach อยู่แล้ว)
// เรียกได้จากหลายทาง (start job, docker start event, reconcile) — ต้อง idempotent
func (m *Manager) Attach(serverID string) error {
	m.mu.Lock()
	if _, ok := m.sessions[serverID]; ok {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	rwc, err := m.attacher.AttachConsole(serverID)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if _, ok := m.sessions[serverID]; ok {
		// แพ้ race กับ Attach อีกตัว — ปิดของตัวเองทิ้ง
		m.mu.Unlock()
		rwc.Close()
		return nil
	}
	m.sessions[serverID] = rwc
	m.mu.Unlock()

	go m.pump(serverID, rwc)
	log.Printf("console attached: server=%s", serverID)
	return nil
}

// PushSystemLine ส่ง console line เดียวที่มาจากระบบ (ไม่ใช่ output ของ container)
// ผ่าน sender ตัวเดียวกับ pump — เข้า ring buffer/WS ของ control plane ให้ user เห็น
// ใช้แจ้งเหตุการณ์ที่ container ไม่ได้พิมพ์เอง เช่น crash cleanup
func (m *Manager) PushSystemLine(serverID, text string) {
	if err := m.sender.SendConsoleOutput(serverID, []string{text}); err != nil {
		log.Printf("system line dropped: server=%s err=%v", serverID, err)
	}
}

func (m *Manager) Detach(serverID string) {
	m.mu.Lock()
	rwc, ok := m.sessions[serverID]
	delete(m.sessions, serverID)
	m.mu.Unlock()
	if ok {
		rwc.Close()
		log.Printf("console detached: server=%s", serverID)
	}
}

// detachIf ลบ session เฉพาะเมื่อยังเป็น rwc ตัวเดิม (compare-and-delete)
// pump ต้องใช้ตัวนี้แทน Detach — กัน pump เก่าลบ session ใหม่ที่เพิ่ง attach
// หลัง container restart เร็ว ๆ (Detach ธรรมดาจะลบอะไรก็ตามที่อยู่ใน map ตอนนั้น)
func (m *Manager) detachIf(serverID string, rwc io.ReadWriteCloser) {
	m.mu.Lock()
	cur, ok := m.sessions[serverID]
	if !ok || cur != rwc {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, serverID)
	m.mu.Unlock()
	rwc.Close()
	log.Printf("console detached: server=%s", serverID)
}

func (m *Manager) DetachAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Detach(id)
	}
}

// WriteInput เขียนคำสั่งจาก user เข้า stdin ของ container
// ถ้ายังไม่มี session (เช่น agent เพิ่ง reconnect) ลอง attach ให้ก่อน
func (m *Manager) WriteInput(serverID, command string) error {
	m.mu.Lock()
	rwc, ok := m.sessions[serverID]
	m.mu.Unlock()
	if !ok {
		if err := m.Attach(serverID); err != nil {
			return fmt.Errorf("no console session for server %s: %w", serverID, err)
		}
		m.mu.Lock()
		rwc, ok = m.sessions[serverID]
		m.mu.Unlock()
		if !ok {
			return errors.New("console session closed while attaching")
		}
	}
	_, err := rwc.Write([]byte(command + "\n"))
	return err
}

// pump อ่าน output แตกเป็นบรรทัดแล้ว flush เป็น batch ทุก 100ms หรือครบ 50 บรรทัด
// gRPC stream หลุด → ทิ้ง batch นั้นเลย ไม่ buffer สะสม (ring buffer อยู่ฝั่ง control plane)
func (m *Manager) pump(serverID string, rwc io.ReadWriteCloser) {
	defer m.detachIf(serverID, rwc)

	lines := make(chan string, 256)
	go m.readLines(serverID, rwc, lines)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var batch []string
	var batchBytes int
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := m.sender.SendConsoleOutput(serverID, batch); err != nil {
			log.Printf("console batch dropped: server=%s lines=%d err=%v", serverID, len(batch), err)
		}
		batch = nil
		batchBytes = 0
	}
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				// container ตาย/ถูก detach — flush ที่เหลือแล้วจบ session
				flush()
				return
			}
			batch = append(batch, line)
			batchBytes += len(line)
			// flush เมื่อครบจำนวนบรรทัด หรือรวม byte เกินเพดาน (กัน message ใหญ่เกิน gRPC limit)
			if len(batch) >= flushMaxLines || batchBytes >= flushMaxBytes {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// readLines อ่าน output แตกเป็นบรรทัดส่งเข้า channel
// ใช้ bufio.Reader (ไม่ใช่ Scanner) เพื่อไม่ให้บรรทัดยาวเกิน maxLineBytes ทำ console หยุดถาวร —
// บรรทัดที่ยาวเกินจะถูกตัดเป็นชิ้น ๆ ชิ้นละไม่เกิน maxLineBytes แทนการตาย
// (Scanner จะคืน ErrTooLong แล้วปิด channel ทำให้ pump เข้าใจผิดว่า container ตาย)
func (m *Manager) readLines(serverID string, rwc io.ReadWriteCloser, lines chan<- string) {
	defer close(lines)
	br := bufio.NewReaderSize(rwc, 64*1024)
	var line []byte // สะสมชิ้นของบรรทัดข้าม ErrBufferFull — append เป็น copy ไม่ alias buffer ของ br
	for {
		frag, err := br.ReadSlice('\n')
		line = append(line, frag...)

		// ตัดชิ้นเมื่อยาวเกินเพดาน กันบรรทัดยาวผิดปกติทำ message/หน่วยความจำโตไม่จำกัด
		for len(line) >= maxLineBytes {
			lines <- string(line[:maxLineBytes])
			line = line[maxLineBytes:]
		}

		switch {
		case err == nil:
			// เจอ '\n' — line ตอนนี้ลงท้ายด้วย '\n' (อาจมี '\r' นำหน้า) ตัดออกก่อนส่ง
			lines <- string(trimEOL(line))
			line = line[:0]
		case errors.Is(err, bufio.ErrBufferFull):
			// บรรทัดยาวกว่า buffer ของ br — อ่านต่อจนเจอ '\n' หรือชนเพดาน maxLineBytes
			continue
		default:
			// EOF หรือ read error — ส่งเศษที่ค้าง แล้วจบ (log สาเหตุถ้าไม่ใช่ EOF ปกติ)
			if len(line) > 0 {
				lines <- string(trimEOL(line))
			}
			if !errors.Is(err, io.EOF) {
				log.Printf("console reader stopped: server=%s err=%v", serverID, err)
			}
			return
		}
	}
}

// trimEOL ตัด '\n' และ '\r' ท้ายบรรทัด (ตาม behavior เดิมของ bufio.ScanLines)
func trimEOL(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}
