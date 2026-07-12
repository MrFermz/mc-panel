// Package httpapi คือ HTTP API ทั้งหมดของ control-plane ตาม docs/api.md
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/mc-panel/control-plane/internal/agenthub"
	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/console"
	"github.com/mc-panel/control-plane/internal/events"
	"github.com/mc-panel/control-plane/internal/jobs"
	"github.com/mc-panel/control-plane/internal/serverstats"
	"github.com/mc-panel/control-plane/internal/store"
	"github.com/mc-panel/control-plane/internal/versions"
)

type API struct {
	st       *store.Store
	auth     *auth.Manager
	disp     *jobs.Dispatcher
	versions *versions.Service
	rings    *console.Registry
	stats    *serverstats.Cache
	hub      *agenthub.Hub
	events   *events.Hub
	js       jetstream.JetStream
	log      *slog.Logger
}

func New(st *store.Store, am *auth.Manager, disp *jobs.Dispatcher, vs *versions.Service, rings *console.Registry, stats *serverstats.Cache, hub *agenthub.Hub, ev *events.Hub, js jetstream.JetStream, log *slog.Logger) *API {
	return &API{
		st:       st,
		auth:     am,
		disp:     disp,
		versions: vs,
		rings:    rings,
		stats:    stats,
		hub:      hub,
		events:   ev,
		js:       js,
		log:      log,
	}
}

func (a *API) Router(consoleWS, eventsWS http.HandlerFunc) http.Handler {
	r := chi.NewRouter()

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", a.handleLogin)
		// logout ไม่บังคับ session ที่ valid — cookie หมดอายุแล้วก็ต้อง logout ได้
		api.Post("/auth/logout", a.handleLogout)

		api.Group(func(pr chi.Router) {
			pr.Use(a.requireAuth)

			pr.Get("/auth/me", a.handleMe)
			pr.Post("/auth/change-password", a.handleChangePassword)

			// user directory: authed ทุกคน (ไม่ใช่ users.manage) — owner ใช้เลือก collaborator
			pr.Get("/users/directory", a.handleUserDirectory)

			pr.Group(func(um chi.Router) {
				um.Use(a.requireCap(capManageUsers))
				um.Get("/users", a.handleListUsers)
				um.Post("/users", a.handleCreateUser)
				um.Patch("/users/{id}", a.handleUpdateUser)
				um.Delete("/users/{id}", a.handleDeleteUser)
				um.Post("/users/{id}/reset-password", a.handleResetPassword)
			})

			pr.Group(func(nm chi.Router) {
				nm.Use(a.requireCap(capManageNodes))
				nm.Get("/nodes", a.handleListNodes)
				nm.Post("/nodes", a.handleCreateNode)
				nm.Delete("/nodes/{id}", a.handleDeleteNode)
			})

			pr.Get("/servers", a.handleListServers)
			pr.With(a.requireCap(capCreateServers)).Post("/servers", a.handleCreateServer)
			pr.With(a.requireCap(capCreateServers)).Post("/servers/import", a.handleImportServer)
			pr.Get("/servers/{id}", a.handleGetServer)
			pr.Patch("/servers/{id}", a.handleUpdateServer)
			pr.Delete("/servers/{id}", a.handleDeleteServer)
			pr.Post("/servers/{id}/actions", a.handleServerAction)
			pr.Get("/servers/{id}/jobs", a.handleListServerJobs)
			pr.Get("/servers/{id}/console/history", a.handleConsoleHistory)
			pr.Get("/servers/{id}/permissions", a.handleListPermissions)
			pr.Post("/servers/{id}/permissions", a.handleUpsertPermission)
			pr.Delete("/servers/{id}/permissions/{user_id}", a.handleDeletePermission)

			pr.Get("/servers/{id}/files", a.handleListFiles)
			pr.Get("/servers/{id}/files/content", a.handleReadFile)
			pr.Put("/servers/{id}/files/content", a.handleWriteFile)
			pr.Post("/servers/{id}/files/dir", a.handleMakeDir)
			pr.Post("/servers/{id}/files/rename", a.handleRenameFile)
			pr.Delete("/servers/{id}/files", a.handleDeleteFile)

			pr.Get("/servers/{id}/properties", a.handleGetProperties)
				pr.Put("/servers/{id}/properties", a.handleUpdateProperties)

				pr.Get("/servers/{id}/players", a.handleListPlayers)
				pr.Post("/servers/{id}/players", a.handleAddPlayer)
				pr.Delete("/servers/{id}/players/{uuid}", a.handleRemovePlayer)

				pr.Get("/jobs/{id}", a.handleGetJob)

			pr.Get("/meta/server-types", a.handleServerTypes)
			pr.Get("/meta/versions", a.handleVersions)
			pr.Get("/meta/nodes", a.handleMetaNodes)
			pr.Get("/meta/next-port", a.handleMetaNextPort)
			pr.Get("/meta/capabilities", a.handleCapabilities)
		})
	})

	r.Get("/ws/servers/{id}/console", consoleWS)
	// events WS: browser เปิดเส้นเดียว รับ push realtime (server/node/stats/jobs)
	// อยู่นอก /api group เหมือน console (WS ต่างชั้น middleware กับ REST)
	r.Get("/ws/events", eventsWS)

	return r
}

// audit ห้ามทำให้ request ล้ม — log อย่างเดียวถ้าพลาด
func (a *API) audit(r *http.Request, userID, serverID *uuid.UUID, action string, detail map[string]any) {
	if err := a.st.InsertAudit(r.Context(), userID, serverID, action, detail, clientIP(r)); err != nil {
		a.log.Error("audit insert failed", "action", action, "error", err)
	}
}
