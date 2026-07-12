package runner

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// EnsureRuntimeImage ทำให้ image พร้อมใช้บน node นี้: มี cache แล้ว reuse ทันที
// ไม่มี → pull eclipse-temurin ตาม java version ที่อยู่ใน tag ท้าย ':' แล้ว tag ซ้ำ
// เป็นชื่อเดิม เพื่อให้ instance อื่นบน node เดียวกันใช้ cache ร่วมกันโดยไม่ต้อง pull ซ้ำ
func EnsureRuntimeImage(ctx context.Context, cli *client.Client, imageRef string) error {
	if _, err := cli.ImageInspect(ctx, imageRef); err == nil {
		log.Printf("reusing cached runtime image: %s", imageRef)
		return nil
	} else if !client.IsErrNotFound(err) {
		return fmt.Errorf("inspect image %q: %w", imageRef, err)
	}

	idx := strings.LastIndex(imageRef, ":")
	if idx < 0 || idx == len(imageRef)-1 {
		return fmt.Errorf("cannot derive java version from image ref %q: no tag after ':'", imageRef)
	}
	javaVer := imageRef[idx+1:]
	base := fmt.Sprintf("docker.io/library/eclipse-temurin:%s-jre", javaVer)

	rc, err := cli.ImagePull(ctx, base, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull base image %q: %w", base, err)
	}
	// ต้องอ่าน body จนจบเพื่อรอ pull เสร็จจริง — ImagePull คืน reader ทันที
	// แต่ layer ยังโหลดไม่ครบจนกว่า stream progress จะหมด
	if _, copyErr := io.Copy(io.Discard, rc); copyErr != nil {
		rc.Close()
		return fmt.Errorf("pull base image %q: %w", base, copyErr)
	}
	rc.Close()

	if err := cli.ImageTag(ctx, base, imageRef); err != nil {
		return fmt.Errorf("tag base image %q as %q: %w", base, imageRef, err)
	}
	log.Printf("pulled and cached runtime image: %s from eclipse-temurin:%s-jre", imageRef, javaVer)
	return nil
}
