package games

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// imageBuildTimeout — build ของ runtime image ต้องโหลด package จาก distro + upstream
// บน connection ช้าใช้เวลาได้หลายนาที แต่ต้องไม่ค้างตลอดกาลจนงาน provision ไม่จบ
const imageBuildTimeout = 20 * time.Minute

// EnsureRuntimeImage ทำให้ image พร้อมใช้บน node นี้: มี cache แล้ว reuse ทันที
// ไม่มี → เตรียมตามที่ game definition บอกผ่าน ImageSource (pull image ของ upstream
// แล้ว tag ซ้ำเป็นชื่อของเรา หรือ build เองจาก Dockerfile ของ definition)
// การ tag/build เป็นชื่อเดียวกันเสมอทำให้ instance อื่นบน node เดียวกันใช้ cache ร่วมกัน
func EnsureRuntimeImage(ctx context.Context, cli *client.Client, imageRef string, src ImageSource) error {
	if insp, err := cli.ImageInspect(ctx, imageRef); err == nil {
		// image ที่ cache ไว้ต้องเป็น platform ที่เกมต้องการจริง ๆ ไม่งั้นมันจะไปพัง
		// ตอน start แทน (binary ของเกมรันไม่ได้) ซึ่งไล่หาสาเหตุยากกว่ามาก
		if got := insp.Os + "/" + insp.Architecture; src.Platform != "" && got != src.Platform {
			return fmt.Errorf("cached runtime image %q is %s but this game needs %s: remove it (docker rmi %s) and prepare it again",
				imageRef, got, src.Platform, imageRef)
		}
		log.Printf("reusing cached runtime image: %s", imageRef)
		return nil
	} else if !client.IsErrNotFound(err) {
		return fmt.Errorf("inspect image %q: %w", imageRef, err)
	}

	switch {
	case src.PullFrom != "":
		return pullAndTag(ctx, cli, src.PullFrom, imageRef, src.Platform)
	case src.Dockerfile != "":
		if err := checkBuildPlatform(ctx, cli, imageRef, src.Platform); err != nil {
			return err
		}
		return buildImage(ctx, cli, src.Dockerfile, imageRef, src.Platform)
	default:
		return fmt.Errorf("runtime image %q is not present on this node and this game cannot prepare it automatically: build it first (make runtime-images)", imageRef)
	}
}

// checkBuildPlatform กันเคส cross-build ที่ทำไม่ได้จริง: builder ของ docker daemon
// (classic builder ซึ่งเป็นตัวเดียวที่ Engine API เรียกได้ — BuildKit ต้องใช้ buildx ที่เป็น
// CLI plugin) **ไม่สนใจ platform ที่ส่งไป** มันจะ build เป็น arch ของ node เสมอ
// ปล่อยผ่าน = ได้ image ผิด arch แล้วไปพังด้วย error ที่ไม่บอกอะไร (เช่น apt ไม่มี repo i386
// บน arm) — บอกให้ตรงว่าเกมนี้ต้องใช้ node arch ไหนดีกว่า
//
// การ build image ข้าม arch ไว้ล่วงหน้าด้วย CLI (buildx cross-build ได้) ทำให้ผ่านด่านนี้
// ก็จริง แต่ไม่ใช่ทางแก้ที่แนะนำ: เครื่องมือของเกมที่ต้องใช้ platform pin (SteamCMD เป็น
// binary 32-bit x86) มักรันใต้ emulation ไม่ได้อยู่ดี
func checkBuildPlatform(ctx context.Context, cli *client.Client, imageRef, want string) error {
	if want == "" {
		return nil
	}
	ver, err := cli.ServerVersion(ctx)
	if err != nil {
		// ถามไม่ได้ = ปล่อยให้ build ไปตามเดิม (ดีกว่าบล็อก node ที่ปกติดี)
		log.Printf("could not read docker server version, skipping platform check: %v", err)
		return nil
	}
	if got := ver.Os + "/" + ver.Arch; got != want {
		return fmt.Errorf("this game needs a %s node but this one is %s: the docker engine cannot cross-build runtime image %q, and the game's tooling does not run under emulation — create this server on a %s node",
			want, got, imageRef, want)
	}
	return nil
}

func pullAndTag(ctx context.Context, cli *client.Client, base, imageRef, platform string) error {
	rc, err := cli.ImagePull(ctx, base, image.PullOptions{Platform: platform})
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
	log.Printf("pulled and cached runtime image: %s from %s", imageRef, base)
	return nil
}

// buildImage build runtime image จาก Dockerfile ที่ definition ถือไว้ — context เป็น tar
// ในหน่วยความจำที่มีแค่ Dockerfile (ไม่มีไฟล์จาก host เข้าไปใน image เลย)
func buildImage(ctx context.Context, cli *client.Client, dockerfile, imageRef, platform string) error {
	ctx, cancel := context.WithTimeout(ctx, imageBuildTimeout)
	defer cancel()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Mode: 0o644,
		Size: int64(len(dockerfile)),
	}); err != nil {
		return fmt.Errorf("build context for %q: %w", imageRef, err)
	}
	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		return fmt.Errorf("build context for %q: %w", imageRef, err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("build context for %q: %w", imageRef, err)
	}

	log.Printf("building runtime image: %s", imageRef)
	resp, err := cli.ImageBuild(ctx, &buf, build.ImageBuildOptions{
		Tags:       []string{imageRef},
		Dockerfile: "Dockerfile",
		// Platform ต้องส่งต่อจาก definition — เกมที่เครื่องมือมีแค่ x86 build บน node ARM
		// ไม่ผ่าน (apt ไม่มี repo i386 บน arm) ต้องบังคับ linux/amd64 แล้วรันผ่าน emulation
		Platform:    platform,
		Remove:      true,
		ForceRemove: true,
		Labels:      map[string]string{"project": "game-manager"},
	})
	if err != nil {
		return fmt.Errorf("build runtime image %q: %w", imageRef, err)
	}
	defer resp.Body.Close()

	// daemon รายงาน error ของ build ผ่าน stream ไม่ใช่ status code — ต้องอ่านทุกบรรทัด
	// จนจบ (เป็นตัวรอ build เสร็จด้วย) แล้วเช็ค field errorDetail
	dec := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("build runtime image %q: %w", imageRef, err)
		}
		if msg.Error != "" || msg.ErrorDetail.Message != "" {
			detail := msg.ErrorDetail.Message
			if detail == "" {
				detail = msg.Error
			}
			return fmt.Errorf("build runtime image %q: %s", imageRef, detail)
		}
	}

	if _, err := cli.ImageInspect(ctx, imageRef); err != nil {
		return fmt.Errorf("build runtime image %q finished but the image is missing: %w", imageRef, err)
	}
	log.Printf("built and cached runtime image: %s", imageRef)
	return nil
}
