// Package seed สร้างข้อมูลเริ่มต้นตอน boot: admin คนแรก + node "local" จาก NODE_TOKEN
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

func Run(ctx context.Context, st *store.Store, log *slog.Logger, adminEmail, nodeToken string) error {
	if err := seedAdmin(ctx, st, log, adminEmail); err != nil {
		return err
	}
	return seedLocalNode(ctx, st, log, nodeToken)
}

func seedAdmin(ctx context.Context, st *store.Store, log *slog.Logger, adminEmail string) error {
	n, err := st.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return nil
	}

	password, err := auth.GeneratePassword()
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	u, err := st.CreateUser(ctx, adminEmail, nil, hash, "Admin", true, nil)
	if err != nil {
		return fmt.Errorf("create initial admin: %w", err)
	}

	// พิมพ์เป็น block ตรง ๆ (ไม่ใช่ structured log) เพราะ Makefile ใช้
	// `grep -A4 "INITIAL ADMIN"` ดึง credentials จาก docker logs
	fmt.Fprintf(os.Stderr,
		"\n==================================================\n"+
			"INITIAL ADMIN credentials (shown only once)\n"+
			"  email:    %s\n"+
			"  password: %s\n"+
			"Save this password now — it will NOT be shown again.\n"+
			"==================================================\n\n",
		u.Email, password)
	log.Info("initial admin created", "email", u.Email, "user_id", u.ID)
	return nil
}

func seedLocalNode(ctx context.Context, st *store.Store, log *slog.Logger, nodeToken string) error {
	if nodeToken == "" {
		return nil
	}
	n, err := st.CountNodes(ctx)
	if err != nil {
		return fmt.Errorf("count nodes: %w", err)
	}
	if n > 0 {
		return nil
	}

	node, err := st.CreateNode(ctx, uuid.New(), "local", auth.HashToken(nodeToken))
	if err != nil {
		return fmt.Errorf("create local node: %w", err)
	}
	log.Info("seeded local node from NODE_TOKEN", "node_id", node.ID)
	return nil
}
