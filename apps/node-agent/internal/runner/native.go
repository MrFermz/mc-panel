package runner

import (
	"context"
	"errors"
	"io"
)

// NativeRunner (fork/exec process ตรง ๆ บน Linux) อยู่นอก scope ปัจจุบัน —
// ระบบตัดสินใจเป็น full docker แล้ว (ทุก instance รันเป็น sibling container ผ่าน DockerRunner)
// คง stub ไว้ตาม Runner interface เผื่อกลับมาทำ native mode ในอนาคต
type NativeRunner struct{}

var errNativeNotImplemented = errors.New("native runner is out of scope: the system runs full docker")

func NewNativeRunner() *NativeRunner {
	return &NativeRunner{}
}

func (r *NativeRunner) Start(ctx context.Context, cfg ServerConfig) error {
	return errNativeNotImplemented
}

func (r *NativeRunner) Stop(id string, graceful bool) error {
	return errNativeNotImplemented
}

func (r *NativeRunner) Kill(id string) error {
	return errNativeNotImplemented
}

func (r *NativeRunner) AttachConsole(id string) (io.ReadWriteCloser, error) {
	return nil, errNativeNotImplemented
}

func (r *NativeRunner) Stats(id string) (ResourceStats, error) {
	return ResourceStats{}, errNativeNotImplemented
}
