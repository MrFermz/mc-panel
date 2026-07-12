// Package jobs คุมวงจรชีวิตของ job: NATS stream/consumer, dispatch และรับผลลัพธ์
// DB (jobs table) เป็น source of truth ของสถานะงานเสมอ — NATS เป็นแค่ transport
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	StreamJobs      = "JOBS"
	StreamResults   = "RESULTS"
	SubjectResults  = "mcpanel.results"
	ResultsConsumer = "cp-results"
)

func JobSubject(nodeID string) string {
	return "mcpanel.jobs." + nodeID
}

func NodeConsumerName(nodeID string) string {
	return "agent-" + nodeID
}

// EnsureStreams สร้าง stream แบบ idempotent — control-plane เป็นเจ้าของ topology
// ทั้งหมด (agent ไม่มีสิทธิ์สร้าง stream/consumer ตาม NATS ACL)
func EnsureStreams(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      StreamJobs,
		Subjects:  []string{"mcpanel.jobs.>"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
		// dedup ด้วย Nats-Msg-Id = job_id: ตอน publish timeout กำกวม (message อาจถึงแล้ว)
		// dispatcher retry publish ได้โดยไม่ยิงงานซ้ำเข้า agent — window ต้องคลุมช่วง retry
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", StreamJobs, err)
	}

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamResults,
		Subjects: []string{SubjectResults},
		Storage:  jetstream.FileStorage,
		// จำกัดอายุกันโตไม่จำกัด — ผลลัพธ์ถูก persist ลง jobs table แล้ว
		MaxAge: 48 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", StreamResults, err)
	}
	return nil
}

func EnsureNodeConsumer(ctx context.Context, js jetstream.JetStream, nodeID string) error {
	_, err := js.CreateOrUpdateConsumer(ctx, StreamJobs, jetstream.ConsumerConfig{
		Durable:       NodeConsumerName(nodeID),
		FilterSubject: JobSubject(nodeID),
		AckPolicy:     jetstream.AckExplicitPolicy,
		// create_server ต้องโหลด jar ผ่าน internet — ให้เวลา ack นาน
		AckWait:    5 * time.Minute,
		MaxDeliver: 5,
	})
	if err != nil {
		return fmt.Errorf("ensure consumer for node %s: %w", nodeID, err)
	}
	return nil
}

func DeleteNodeConsumer(ctx context.Context, js jetstream.JetStream, nodeID string) error {
	err := js.DeleteConsumer(ctx, StreamJobs, NodeConsumerName(nodeID))
	if err != nil && err != jetstream.ErrConsumerNotFound {
		return err
	}
	return nil
}

func EnsureResultsConsumer(ctx context.Context, js jetstream.JetStream) (jetstream.Consumer, error) {
	cons, err := js.CreateOrUpdateConsumer(ctx, StreamResults, jetstream.ConsumerConfig{
		Durable:       ResultsConsumer,
		FilterSubject: SubjectResults,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure consumer %s: %w", ResultsConsumer, err)
	}
	return cons, nil
}
