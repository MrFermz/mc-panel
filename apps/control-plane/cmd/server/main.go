package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/agenthub"
	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/config"
	"github.com/mc-panel/control-plane/internal/console"
	"github.com/mc-panel/control-plane/internal/events"
	"github.com/mc-panel/control-plane/internal/httpapi"
	"github.com/mc-panel/control-plane/internal/jobs"
	"github.com/mc-panel/control-plane/internal/seed"
	"github.com/mc-panel/control-plane/internal/serverstats"
	"github.com/mc-panel/control-plane/internal/store"
	"github.com/mc-panel/control-plane/internal/versions"
	"github.com/mc-panel/control-plane/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false,
		"probe /healthz of the local server and exit 0/1 (used by docker HEALTHCHECK)")
	resetAdmin := flag.Bool("reset-admin-password", false,
		"reset (or create) the admin account's password, print a new one-time password, then exit")
	resetUsername := flag.String("username", "",
		"username of the account to reset with -reset-admin-password (default: ADMIN_USERNAME)")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config failed", "error", err)
		os.Exit(1)
	}

	if *resetAdmin {
		os.Exit(runResetAdminPassword(cfg, log, *resetUsername))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log); err != nil {
		log.Error("control-plane exited with error", "error", err)
		os.Exit(1)
	}
	log.Info("control-plane stopped")
}

// runHealthcheck ใช้ใน docker HEALTHCHECK — image เป็น distroless ไม่มี shell/curl
// จึงให้ binary ตัวเองเป็น probe
func runHealthcheck() int {
	port := config.HTTPPort(os.Getenv("HTTP_ADDR"))
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

// runResetAdminPassword กู้กรณีลืม password admin — ต่อแค่ Postgres (password อยู่ที่นี่ที่เดียว
// ไม่เกี่ยว Redis/NATS) สุ่ม password ใหม่ + must_change_password + bump token_version (session เก่าตายหมด)
// รันผ่าน container ที่มี DATABASE_URL อยู่แล้ว ไม่ต้อง exec เข้า postgres เอง
func runResetAdminPassword(cfg *config.Config, log *slog.Logger, username string) int {
	if username == "" {
		username = cfg.AdminUsername
	}
	// `-username=Admin` ต้องเจอบัญชี `admin` และถ้าต้องสร้างใหม่ต้องผ่าน CHECK ของ DB (00018)
	username = strings.ToLower(strings.TrimSpace(username))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reset-admin-password: connect postgres: %v\n", err)
		return 1
	}
	defer pool.Close()
	st := store.New(pool)

	password, err := auth.GeneratePassword()
	if err != nil {
		fmt.Fprintf(os.Stderr, "reset-admin-password: generate password: %v\n", err)
		return 1
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reset-admin-password: hash password: %v\n", err)
		return 1
	}

	u, err := st.GetUserByUsername(ctx, username)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// ไม่มี account นี้ — สร้างใหม่เป็น admin (เคส users ถูกลบหมด/พิมพ์ username ผิดตอน seed)
		u, err = st.CreateUser(ctx, username, hash, true, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reset-admin-password: create admin: %v\n", err)
			return 1
		}
	case err != nil:
		fmt.Fprintf(os.Stderr, "reset-admin-password: lookup %s: %v\n", username, err)
		return 1
	default:
		if _, err := st.SetUserPassword(ctx, u.ID, hash, true); err != nil {
			fmt.Fprintf(os.Stderr, "reset-admin-password: set password: %v\n", err)
			return 1
		}
	}

	fmt.Fprintf(os.Stderr,
		"\n==================================================\n"+
			"ADMIN PASSWORD RESET (shown only once)\n"+
			"  username: %s\n"+
			"  password: %s\n"+
			"Log in with this password — you will be forced to change it.\n"+
			"All existing sessions for this account are now invalid.\n"+
			"==================================================\n\n",
		u.Username, password)
	log.Info("admin password reset", "username", u.Username, "user_id", u.ID)
	return 0
}

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if err := runMigrations(ctx, cfg.DatabaseURL, log); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create postgres pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	log.Info("connected to postgres")

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		// ไม่ fatal — redis ใช้แค่ login rate limit ซึ่งออกแบบให้ fail-open
		log.Warn("redis ping failed, login rate limit will fail open", "error", err)
	} else {
		log.Info("connected to redis")
	}

	nc, err := connectNATS(ctx, cfg.NATSURL, log)
	if err != nil {
		return err
	}
	defer nc.Drain()
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("init jetstream: %w", err)
	}
	if err := jobs.EnsureStreams(ctx, js); err != nil {
		return err
	}
	log.Info("connected to NATS, streams ensured")

	st := store.New(pool)

	if err := seed.Run(ctx, st, log, cfg.AdminUsername, cfg.NodeToken); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	// consumer ต่อ node เป็นของ control-plane (agent ไม่มีสิทธิ์สร้างตาม NATS ACL)
	// ensure ตอน boot เผื่อ node ที่สร้างไว้ก่อน / seed เมื่อกี้
	nodes, err := st.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	for _, n := range nodes {
		if err := jobs.EnsureNodeConsumer(ctx, js, n.ID.String()); err != nil {
			return err
		}
	}

	rings := console.NewRegistry()
	wsHub := console.NewHub()
	// stats cache ตัวเดียว: agenthub เขียน (จาก gRPC), httpapi อ่าน (จาก HTTP)
	statsCache := serverstats.NewCache()
	// events hub: fan-out realtime ไป browser (push แทน REST poll) — agenthub/jobs เป็นคน emit
	eventsHub := events.NewHub()
	am := auth.NewManager(st, rdb, cfg.JWTSecret, cfg.CookieSecure, log)
	hub := agenthub.NewHub(st, rings, wsHub, statsCache, eventsHub, log)
	disp := jobs.NewDispatcher(st, js, eventsHub, log)
	vs := versions.New()

	resultConsumer := jobs.NewResultConsumer(st, rings, wsHub, eventsHub, log)
	consumeCtx, err := resultConsumer.Start(ctx, js)
	if err != nil {
		return fmt.Errorf("start result consumer: %w", err)
	}
	defer consumeCtx.Stop()

	wsHandler := &console.WSHandler{
		Auth:              am,
		Store:             st,
		Rings:             rings,
		Hub:               wsHub,
		Sender:            hub,
		AllowedOrigins:    cfg.AllowedOrigins,
		TrustedProxyCount: cfg.TrustedProxyCount,
		Log:               log,
	}
	eventsHandler := &events.WSHandler{
		Auth:           am,
		Store:          st,
		Hub:            eventsHub,
		AllowedOrigins: cfg.AllowedOrigins,
		Log:            log,
	}
	// httpapi.clientIP อ่านค่านี้ — set ครั้งเดียวก่อน server รับ request (ไม่มี race)
	httpapi.SetTrustedProxyCount(cfg.TrustedProxyCount)
	api := httpapi.New(st, am, disp, vs, rings, statsCache, hub, eventsHub, js, log)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Router(wsHandler.HandleConsole, eventsHandler.HandleEvents),
		ReadHeaderTimeout: 10 * time.Second,
	}

	agentSvc := agenthub.NewService(hub)
	grpcSrv := grpc.NewServer(
		grpc.ChainStreamInterceptor(agentSvc.StreamAuthInterceptor),
		// default 4MB เล็กเกิน — ConsoleOutput batch ใหญ่ทำ stream หลุด
		// (agent cap batch ~1MB; 16MB เผื่อ margin)
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	agentv1.RegisterAgentServiceServer(grpcSrv, agentSvc)

	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", cfg.GRPCAddr, err)
	}

	errCh := make(chan error, 2)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()
	go func() {
		log.Info("grpc server listening", "addr", cfg.GRPCAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()
	go hub.RunOfflineChecker(ctx)

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown failed", "error", err)
	}

	grpcStopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-time.After(5 * time.Second):
		grpcSrv.Stop()
	}
	return nil
}

func runMigrations(ctx context.Context, dsn string, log *slog.Logger) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	// compose มี depends_on healthy อยู่แล้ว แต่ตอนรัน dev นอก docker
	// postgres อาจยังไม่พร้อม — retry สั้น ๆ ดีกว่า crash loop
	for attempt := 1; ; attempt++ {
		err = db.PingContext(ctx)
		if err == nil {
			break
		}
		if attempt >= 30 {
			return fmt.Errorf("postgres not reachable after %d attempts: %w", attempt, err)
		}
		log.Info("waiting for postgres", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return err
	}
	log.Info("migrations applied")
	return nil
}

func connectNATS(ctx context.Context, url string, log *slog.Logger) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("control-plane"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	for attempt := 1; ; attempt++ {
		nc, err := nats.Connect(url, opts...)
		if err == nil {
			return nc, nil
		}
		if attempt >= 30 {
			return nil, fmt.Errorf("NATS not reachable after %d attempts: %w", attempt, err)
		}
		log.Info("waiting for NATS", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
