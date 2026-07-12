// Package grpcclient คุมการ connect ออกไปหา control plane ผ่าน gRPC bidirectional stream
// stream นี้ใช้เฉพาะข้อมูล realtime (hello/heartbeat/console/status) — คำสั่งงานผ่าน NATS เท่านั้น
package grpcclient

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

const (
	backoffInitial = 1 * time.Second
	backoffMax     = 30 * time.Second

	// outboxSize จำกัด message ที่ค้างรอส่งบน stream — realtime data เต็มแล้วทิ้งรอบนั้นได้
	outboxSize = 256
	// consoleInputQueueSize จำกัดคำสั่ง console ที่ค้างต่อ server — เต็มแล้วทิ้ง กัน recv loop ค้าง
	consoleInputQueueSize = 32
)

var (
	// ErrNotConnected คืนจาก send ระหว่างที่ stream หลุด — ผู้เรียกทิ้งข้อมูลรอบนั้นได้
	// (heartbeat/console เป็นข้อมูล realtime ไม่ต้อง buffer ยาว)
	ErrNotConnected = errors.New("grpc stream not connected")
	// ErrSendQueueFull คืนเมื่อ outbox เต็ม (stream ส่งไม่ทัน) — ทิ้ง message รอบนั้น
	ErrSendQueueFull = errors.New("grpc send queue full")
)

type Client struct {
	addr  string
	token string
	hello *agentv1.Hello

	// mu คุม outbox — สลับตอน stream ต่อ/หลุด; ตัว Send จริงมี writer goroutine เดียวต่อ stream
	mu     sync.Mutex
	outbox chan *agentv1.AgentMessage

	handlerMu      sync.RWMutex
	onConsoleInput func(serverID, command string)
	onFileRequest  func(*agentv1.FileRequest) *agentv1.FileResponse

	// per-server queue ของ ConsoleInput — กัน handler ที่ block (attach/stdin write) ไม่ให้ค้าง recv loop
	inputMu     sync.Mutex
	inputQueues map[string]chan string

	welcomeOnce sync.Once
	nodeIDCh    chan string
}

func New(addr, token string, hello *agentv1.Hello) *Client {
	return &Client{
		addr:        addr,
		token:       token,
		hello:       hello,
		inputQueues: make(map[string]chan string),
		nodeIDCh:    make(chan string, 1),
	}
}

// OnConsoleInput ตั้ง handler สำหรับ ConsoleInput จาก control plane
// ต้องเรียกก่อน Run เพื่อไม่ให้พลาด input ช่วงแรก
func (c *Client) OnConsoleInput(fn func(serverID, command string)) {
	c.handlerMu.Lock()
	c.onConsoleInput = fn
	c.handlerMu.Unlock()
}

// OnFileRequest ตั้ง handler สำหรับ FileRequest จาก control plane
// handler จะถูกเรียกใน goroutine แยก (ไม่ block recv loop) — file op อาจช้า (อ่าน/เขียน disk)
// ต้องเรียกก่อน Run
func (c *Client) OnFileRequest(fn func(*agentv1.FileRequest) *agentv1.FileResponse) {
	c.handlerMu.Lock()
	c.onFileRequest = fn
	c.handlerMu.Unlock()
}

// WaitForNodeID block จนกว่าจะได้ Welcome แรกจาก control plane
// — เป็นทางเดียวที่ agent รู้ identity ตัวเอง (config มีแค่ token)
func (c *Client) WaitForNodeID(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case id := <-c.nodeIDCh:
		return id, nil
	}
}

// Run เปิด stream ค้างไว้ตลอดอายุ agent — reconnect ด้วย exponential backoff
// และส่ง Hello ใหม่ทุกครั้งที่ต่อใหม่ (control plane ต้องการ Hello เป็น message แรกเสมอ)
func (c *Client) Run(ctx context.Context) error {
	conn, err := grpc.NewClient(c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// keepalive ให้ตรวจ dead connection ได้ใน ~40s แม้ไม่มี stream ค้าง
		// (control plane ตั้ง EnforcementPolicy MinTime 10s รองรับความถี่นี้)
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()
	svc := agentv1.NewAgentServiceClient(conn)

	backoff := backoffInitial
	for {
		welcomed, err := c.runStream(ctx, svc)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if welcomed {
			backoff = backoffInitial
		}
		log.Printf("grpc stream disconnected: %v (reconnecting in %s)", err, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > backoffMax {
			backoff = backoffMax
		}
	}
}

func (c *Client) runStream(ctx context.Context, svc agentv1.AgentServiceClient) (welcomed bool, err error) {
	sctx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
	stream, err := svc.Connect(sctx)
	if err != nil {
		return false, err
	}

	// Hello ต้องเป็น message แรกเสมอ — ส่งตรงบน stream ก่อนเปิด writer/outbox
	// (send() ยังคืน ErrNotConnected อยู่จนกว่า c.outbox จะถูก publish)
	helloMsg := &agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Hello{Hello: c.hello}}
	if err := stream.Send(helloMsg); err != nil {
		return false, err
	}

	// writer goroutine เดียวเป็นคน Send จริง — sender อื่นแค่ push เข้า outbox แบบ non-blocking
	// กัน stream.Send ที่ block (flow-control window เต็ม) ไม่ให้ heartbeat/console/status ค้างกันเอง
	// (gRPC ห้าม Send พร้อมกันหลาย goroutine — มี writer เดียวจึงยังปลอดภัย)
	outbox := make(chan *agentv1.AgentMessage, outboxSize)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for msg := range outbox {
			if err := stream.Send(msg); err != nil {
				// stream หลุด — ปล่อยให้ recv loop เป็นคนคืน error แล้ว reconnect
				return
			}
		}
	}()

	c.mu.Lock()
	c.outbox = outbox
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.outbox = nil
		c.mu.Unlock()
		// ปิด outbox หลังตัดออกจาก c.outbox แล้ว — sender ที่กำลัง push ถือ c.mu อยู่
		// จึงเห็น nil ก่อน close เสมอ ไม่มีทาง send-on-closed-channel
		close(outbox)
		<-writerDone
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			return welcomed, err
		}
		switch p := msg.Payload.(type) {
		case *agentv1.ControlMessage_Welcome:
			welcomed = true
			nodeID := p.Welcome.NodeId
			log.Printf("control plane welcome received: node_id=%s", nodeID)
			c.welcomeOnce.Do(func() { c.nodeIDCh <- nodeID })
		case *agentv1.ControlMessage_ConsoleInput:
			c.dispatchConsoleInput(p.ConsoleInput.ServerId, p.ConsoleInput.Command)
		case *agentv1.ControlMessage_FileRequest:
			c.dispatchFileRequest(p.FileRequest)
		}
	}
}

// dispatchConsoleInput ส่งคำสั่งเข้า per-server queue แบบ non-blocking
// recv loop ต้องไม่ค้าง — handler (WriteInput) ข้างในมี attach/stdin write ที่ block ได้
// per-server เพื่อรักษาลำดับคำสั่งของ server เดียวกัน และไม่ให้ server หนึ่งค้างข้าม server อื่น
func (c *Client) dispatchConsoleInput(serverID, command string) {
	c.handlerMu.RLock()
	hasHandler := c.onConsoleInput != nil
	c.handlerMu.RUnlock()
	if !hasHandler {
		return
	}
	q := c.inputQueue(serverID)
	select {
	case q <- command:
	default:
		// คิวเต็ม (handler ค้าง) — ทิ้งคำสั่ง ดีกว่าให้ recv loop ของทุก server ค้างตาม
		log.Printf("console input dropped, queue full: server=%s", serverID)
	}
}

// inputQueue คืน queue ของ server (ครั้งแรกที่เจอ server นั้นจะสร้าง worker goroutine ให้)
func (c *Client) inputQueue(serverID string) chan string {
	c.inputMu.Lock()
	defer c.inputMu.Unlock()
	if q, ok := c.inputQueues[serverID]; ok {
		return q
	}
	q := make(chan string, consoleInputQueueSize)
	c.inputQueues[serverID] = q
	go func() {
		for cmd := range q {
			c.handlerMu.RLock()
			fn := c.onConsoleInput
			c.handlerMu.RUnlock()
			if fn != nil {
				fn(serverID, cmd)
			}
		}
	}()
	return q
}

// dispatchFileRequest รัน handler ใน goroutine แยก — file op (อ่าน/เขียน disk) block ได้
// ต้องไม่ค้าง recv loop เหมือน ConsoleInput; ไม่ต้อง per-server queue เพราะแต่ละ request
// มี request_id ของตัวเองและ control-plane รอ response แบบ async อยู่แล้ว
func (c *Client) dispatchFileRequest(req *agentv1.FileRequest) {
	c.handlerMu.RLock()
	fn := c.onFileRequest
	c.handlerMu.RUnlock()
	if fn == nil {
		return
	}
	go func() {
		resp := fn(req)
		if resp == nil {
			return
		}
		if err := c.SendFileResponse(resp); err != nil {
			log.Printf("file response dropped: request_id=%s err=%v", req.GetRequestId(), err)
		}
	}()
}

// send push message เข้า outbox แบบ non-blocking แล้วให้ writer goroutine เป็นคน Send จริง
// ถือ c.mu แค่ตอน push (ไม่ block) — ไม่ถือข้าม stream.Send ที่ block ได้
func (c *Client) send(msg *agentv1.AgentMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.outbox == nil {
		return ErrNotConnected
	}
	select {
	case c.outbox <- msg:
		return nil
	default:
		return ErrSendQueueFull
	}
}

func (c *Client) SendConsoleOutput(serverID string, lines []string) error {
	return c.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_ConsoleOutput{
		ConsoleOutput: &agentv1.ConsoleOutput{ServerId: serverID, Lines: lines},
	}})
}

func (c *Client) SendServerStatus(serverID string, state agentv1.ServerState, exitCode int32) error {
	return c.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_ServerStatus{
		ServerStatus: &agentv1.ServerStatus{ServerId: serverID, State: state, ExitCode: exitCode},
	}})
}

func (c *Client) SendHeartbeat(hb *agentv1.Heartbeat) error {
	return c.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Heartbeat{Heartbeat: hb}})
}

func (c *Client) SendServerStats(st *agentv1.ServerStats) error {
	return c.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_ServerStats{ServerStats: st}})
}

func (c *Client) SendFileResponse(resp *agentv1.FileResponse) error {
	return c.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_FileResponse{FileResponse: resp}})
}
