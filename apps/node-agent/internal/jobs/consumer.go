// Package jobs ดึงงานจาก NATS JetStream (stream JOBS) มาประมวลผล
// แล้วรายงานผลกลับทาง mcpanel.results
//
// stream/consumer ทั้งหมดถูกสร้างโดย control-plane — agent มีสิทธิ์แค่ attach
// (ดู ACL ใน infra/nats/nats-server.conf)
package jobs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	jobv1 "github.com/mc-panel/proto/gen/go/mcpanel/job/v1"
	"google.golang.org/protobuf/proto"
)

const (
	streamName     = "JOBS"
	resultsSubject = "mcpanel.results"

	// InProgress ต้องถี่กว่า AckWait ของ consumer (control-plane เป็นคนตั้ง)
	// เพื่อกัน redelivery ระหว่างงานยาว (provision) หรือระหว่างรอคิวของ server เดียวกัน
	keepaliveInterval = 10 * time.Second

	attachBackoffMax = 30 * time.Second
)

type Consumer struct {
	js      jetstream.JetStream
	nodeID  string
	handler *Handler

	workers map[string]chan *task

	// lastSeq เก็บ stream sequence ของ job ล่าสุดที่ประมวลผลไปแล้วต่อ server —
	// ใช้ fence redelivery ที่ผิดลำดับ (เช่น stop เก่าย้อนมาหลัง start ใหม่)
	// workers เข้าถึงจากคนละ goroutine ต่อ server จึงต้องล็อก mu
	mu      sync.Mutex
	lastSeq map[string]uint64
}

type task struct {
	msg           jetstream.Msg
	env           *jobv1.JobEnvelope
	stopKeepalive chan struct{}
}

func NewConsumer(nc *nats.Conn, nodeID string, handler *Handler) (*Consumer, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	return &Consumer{
		js:      js,
		nodeID:  nodeID,
		handler: handler,
		workers: make(map[string]chan *task),
		lastSeq: make(map[string]uint64),
	}, nil
}

// Run ค้างจนกว่า ctx จะถูกยกเลิก — attach consumer แล้วดึง message เข้า dispatcher
// consumer หาย/หลุด (เช่น control-plane restart แล้วยังสร้างไม่เสร็จ) → retry backoff
func (c *Consumer) Run(ctx context.Context) error {
	consumerName := "agent-" + c.nodeID
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		cons, err := c.js.Consumer(ctx, streamName, consumerName)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("attach jetstream consumer %s failed: %v (retrying in %s)", consumerName, err, backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > attachBackoffMax {
				backoff = attachBackoffMax
			}
			continue
		}
		backoff = time.Second
		log.Printf("jetstream consumer attached: %s", consumerName)

		if err := c.consume(ctx, cons); err != nil && ctx.Err() == nil {
			log.Printf("jetstream consume loop ended: %v (reattaching)", err)
		}
	}
}

func (c *Consumer) consume(ctx context.Context, cons jetstream.Consumer) error {
	it, err := cons.Messages(jetstream.PullMaxMessages(8))
	if err != nil {
		return err
	}
	quit := make(chan struct{})
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		select {
		case <-ctx.Done():
		case <-quit: // iterator พังเองระหว่าง ctx ยังไม่จบ — ปลุก goroutine นี้ให้จบด้วย
		}
		it.Stop()
	}()
	defer func() {
		close(quit)
		<-stopDone // อย่าปล่อย goroutine ค้างข้าม reattach รอบใหม่
	}()

	for {
		msg, err := it.Next()
		if err != nil {
			return err
		}
		c.dispatch(ctx, msg)
	}
}

// dispatch จัดคิวงานแบบ serial ต่อ server (ลำดับงานของ server เดียวกันสำคัญ)
// แต่คนละ server ประมวลผลขนานกันได้ — worker หนึ่ง goroutine ต่อ server
func (c *Consumer) dispatch(ctx context.Context, msg jetstream.Msg) {
	env := &jobv1.JobEnvelope{}
	if err := proto.Unmarshal(msg.Data(), env); err != nil {
		// decode ไม่ได้ redeliver ไปก็ไม่มีวันหาย — ack ทิ้ง
		log.Printf("dropping malformed job message: %v", err)
		if err := msg.Ack(); err != nil {
			log.Printf("ack malformed job message failed: %v", err)
		}
		return
	}

	key := taskKey(env)

	t := &task{msg: msg, env: env, stopKeepalive: make(chan struct{})}
	go keepalive(t)

	ch, ok := c.workers[key]
	if !ok {
		ch = make(chan *task, 32)
		c.workers[key] = ch
		go c.worker(ctx, ch)
	}
	select {
	case ch <- t:
	case <-ctx.Done():
		close(t.stopKeepalive)
	}
}

func (c *Consumer) worker(ctx context.Context, ch <-chan *task) {
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ch:
			c.process(ctx, t)
		}
	}
}

func (c *Consumer) process(ctx context.Context, t *task) {
	defer close(t.stopKeepalive)

	seq := jobSeq(t.msg)
	// fence: job นี้เก่ากว่างานที่ทำไปแล้วของ server เดียวกัน (redeliver ผิดลำดับ
	// หลัง publishResult fail แล้ว worker เดินงานใหม่กว่าไปก่อน) — รันซ้ำจะย้อน state
	// เช่น stop เก่ามาหยุด server ที่ job ใหม่กว่าเพิ่ง start; ทิ้งแล้ว ack เลย
	if c.superseded(t.env, seq) {
		log.Printf("job superseded by newer job, dropping: job_id=%s server_id=%s seq=%d", t.env.JobId, t.env.ServerId, seq)
		if err := t.msg.Ack(); err != nil {
			log.Printf("ack superseded job failed: job_id=%s err=%v", t.env.JobId, err)
		}
		return
	}

	log.Printf("job started: job_id=%s server_id=%s type=%s", t.env.JobId, t.env.ServerId, jobType(t.env))
	started := time.Now()
	detail, err := c.handler.Process(ctx, t.env)

	if err != nil && ctx.Err() != nil {
		// กำลัง shutdown — งานโดนตัดกลางคัน อย่ารายงาน fail อย่า ack
		// ให้ redeliver หลัง boot รอบหน้า (งาน idempotent อยู่แล้ว)
		return
	}

	// บันทึก seq ที่เดินผ่านก่อน publish/ack — ถ้า publish fail แล้ว job นี้ค้างไม่ ack
	// การ redeliver ของ job ที่ "เก่ากว่า" seq นี้จะถูก fence ทิ้งด้านบน (idempotent ต่อ ordering)
	// ตัว job นี้เอง (seq เท่าเดิม) ยัง redeliver มา publish ผลซ้ำได้เพราะเงื่อนไขเป็น strict-less
	c.markProcessed(t.env, seq)

	res := &jobv1.JobResult{
		JobId:    t.env.JobId,
		ServerId: t.env.ServerId,
		Success:  err == nil,
		Detail:   detail,
	}
	if err != nil {
		res.Error = err.Error()
		log.Printf("job failed: job_id=%s took=%s err=%v", t.env.JobId, time.Since(started).Round(time.Millisecond), err)
	} else {
		log.Printf("job succeeded: job_id=%s took=%s", t.env.JobId, time.Since(started).Round(time.Millisecond))
	}

	// publish ผลก่อนแล้วค่อย ack — ถ้า crash คั่นกลางจะได้ redeliver+รายงานซ้ำ
	// (control-plane เห็นผลซ้ำได้ ไม่เป็นไร) ดีกว่า ack แล้วผลหายเงียบ
	if !c.publishResult(res) {
		return
	}
	if err := t.msg.Ack(); err != nil {
		log.Printf("ack job failed: job_id=%s err=%v", t.env.JobId, err)
	}
}

func (c *Consumer) publishResult(res *jobv1.JobResult) bool {
	data, err := proto.Marshal(res)
	if err != nil {
		log.Printf("marshal job result failed: job_id=%s err=%v", res.JobId, err)
		return false
	}
	// ใช้ context แยกจาก run loop — ผลของงานที่เสร็จแล้วควรถูกส่งแม้กำลัง shutdown
	for attempt := 1; attempt <= 3; attempt++ {
		pctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := c.js.Publish(pctx, resultsSubject, data)
		cancel()
		if err == nil {
			return true
		}
		log.Printf("publish job result failed: job_id=%s attempt=%d err=%v", res.JobId, attempt, err)
		time.Sleep(time.Second)
	}
	return false
}

// keepalive ยืดเวลา ack ระหว่างงานยังไม่เสร็จ (ทั้งตอนรอคิวและตอนประมวลผล)
func keepalive(t *task) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.stopKeepalive:
			return
		case <-ticker.C:
			if err := t.msg.InProgress(); err != nil {
				log.Printf("job in-progress signal failed: job_id=%s err=%v", t.env.JobId, err)
			}
		}
	}
}

// taskKey จัดกลุ่มงานแบบ serial ต่อ server (fallback เป็น job id เมื่อไม่มี server id
// — งานเช่นนั้นไม่มีลำดับข้ามกัน key จึง match แค่ตัวเอง ไม่ถูก fence)
func taskKey(env *jobv1.JobEnvelope) string {
	if env.ServerId != "" {
		return env.ServerId
	}
	return env.JobId
}

// jobSeq คืน stream sequence ของ message (ลำดับที่ control-plane publish เข้า stream)
// 0 = อ่าน metadata ไม่ได้ → ปิด fence ทิ้งไป ปลอดภัยกว่าเสี่ยงทิ้งงานผิด
func jobSeq(msg jetstream.Msg) uint64 {
	md, err := msg.Metadata()
	if err != nil {
		log.Printf("read job metadata failed: err=%v (skipping ordering fence)", err)
		return 0
	}
	return md.Sequence.Stream
}

// superseded บอกว่างานนี้เก่ากว่างานล่าสุดที่ประมวลผลไปแล้วของ server เดียวกันหรือไม่
// strict-less: seq เท่าเดิม (job เดิม redeliver เพราะ publish/ack fail) ยังให้เดินต่อได้
func (c *Consumer) superseded(env *jobv1.JobEnvelope, seq uint64) bool {
	if seq == 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return seq < c.lastSeq[taskKey(env)]
}

func (c *Consumer) markProcessed(env *jobv1.JobEnvelope, seq uint64) {
	if seq == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if seq > c.lastSeq[taskKey(env)] {
		c.lastSeq[taskKey(env)] = seq
	}
}

func jobType(env *jobv1.JobEnvelope) string {
	switch env.Payload.(type) {
	case *jobv1.JobEnvelope_CreateServer:
		return "create_server"
	case *jobv1.JobEnvelope_StartServer:
		return "start_server"
	case *jobv1.JobEnvelope_StopServer:
		return "stop_server"
	case *jobv1.JobEnvelope_KillServer:
		return "kill_server"
	case *jobv1.JobEnvelope_DeleteServer:
		return "delete_server"
	default:
		return fmt.Sprintf("%T", env.Payload)
	}
}
