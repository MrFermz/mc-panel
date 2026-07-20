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
	"github.com/mc-panel/control-plane/internal/playerface"
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
	faces    *playerface.Cache
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
		faces:    playerface.NewCache(st),
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

			// profile ของตัวเอง: ไม่ผูก capability — เจ้าของบัญชีแก้ข้อมูลตัวเองได้เสมอ
			// (เส้นเดียวกับ change-password) และไม่มีทางแตะ user คนอื่นเพราะยึด id จาก session
			pr.Patch("/auth/me", a.handleUpdateProfile)
			pr.Put("/auth/me/avatar", a.handleUploadAvatar)
			pr.Delete("/auth/me/avatar", a.handleDeleteAvatar)
			// avatar ของ user คนไหนก็อ่านได้เมื่อ login แล้ว (เหมือน /users/directory)
			pr.Get("/users/{id}/avatar", a.handleGetAvatar)

			// user directory: authed ทุกคน (ไม่ใช่ users.view) — owner ใช้เลือก collaborator
			pr.Get("/users/directory", a.handleUserDirectory)

			// ตารางนี้คือ map "endpoint → capability" ที่เดียวของระบบ —
			// route ใหม่ทุกเส้นต้องผูก capability ที่นี่ (ดู CLAUDE.md กฎข้อ 3)
			pr.With(a.requireCap(capUsersView)).Get("/users", a.handleListUsers)
			pr.With(a.requireCap(capUsersView)).Get("/users/{id}", a.handleGetUser)
			pr.With(a.requireCap(capUsersCreate)).Post("/users", a.handleCreateUser)
			// เช็คชื่อซ้ำสด ๆ ตอนกรอกฟอร์ม — cap เดียวกับการสร้าง (กัน user enumeration)
			pr.With(a.requireCap(capUsersCreate)).
				Get("/users/check-username", a.handleCheckUsername)
			pr.With(a.requireCap(capUsersEdit)).Patch("/users/{id}", a.handleUpdateUser)
			pr.With(a.requireCap(capUsersDelete)).Delete("/users/{id}", a.handleDeleteUser)
			pr.With(a.requireCap(capUsersRestore)).Post("/users/{id}/restore", a.handleRestoreUser)
			pr.With(a.requireCap(capUsersResetPassword)).
				Post("/users/{id}/reset-password", a.handleResetPassword)

			// access list มองจากฝั่ง user — เช็ค owner ของ server ตัวนั้นซ้ำในตัว handler
			pr.With(a.requireCap(capAccessView)).Get("/users/{id}/servers", a.handleListUserServers)
			pr.With(a.requireCap(capAccessManage)).
				Post("/users/{id}/servers", a.handleUpsertUserServer)
			pr.With(a.requireCap(capAccessManage)).
				Delete("/users/{id}/servers/{server_id}", a.handleDeleteUserServer)

			pr.With(a.requireCap(capNodesView)).Get("/nodes", a.handleListNodes)
			pr.With(a.requireCap(capNodesCreate)).Post("/nodes", a.handleCreateNode)
			pr.With(a.requireCap(capNodesDelete)).Delete("/nodes/{id}", a.handleDeleteNode)

			pr.Get("/servers", a.handleListServers)
			pr.With(a.requireCap(capServersCreate)).Post("/servers", a.handleCreateServer)
			pr.With(a.requireCap(capServersCreate)).Post("/servers/import", a.handleImportServer)
			pr.Get("/servers/{id}", a.handleGetServer)
			pr.With(a.requireCap(capServersEdit)).Patch("/servers/{id}", a.handleUpdateServer)
			pr.With(a.requireCap(capServersDelete)).Delete("/servers/{id}", a.handleDeleteServer)
			// restore/purge ทำกับ server ที่อยู่ในถังขยะเท่านั้น (deleted_at ไม่ null)
			pr.With(a.requireCap(capServersRestore)).Post("/servers/{id}/restore", a.handleRestoreServer)
			pr.With(a.requireCap(capServersPurge)).Post("/servers/{id}/purge", a.handlePurgeServer)
			pr.With(a.requireCap(capServersPower)).Post("/servers/{id}/actions", a.handleServerAction)
			pr.Get("/servers/{id}/jobs", a.handleListServerJobs)
			pr.With(a.requireCap(capConsoleView)).
				Get("/servers/{id}/console/history", a.handleConsoleHistory)
			pr.With(a.requireCap(capAccessView)).
				Get("/servers/{id}/permissions", a.handleListPermissions)
			pr.With(a.requireCap(capAccessManage)).
				Post("/servers/{id}/permissions", a.handleUpsertPermission)
			pr.With(a.requireCap(capAccessManage)).
				Delete("/servers/{id}/permissions/{user_id}", a.handleDeletePermission)

			pr.With(a.requireCap(capFilesView)).Get("/servers/{id}/files", a.handleListFiles)
			pr.With(a.requireCap(capFilesView)).Get("/servers/{id}/files/content", a.handleReadFile)
			pr.With(a.requireCap(capFilesWrite)).Put("/servers/{id}/files/content", a.handleWriteFile)
			pr.With(a.requireCap(capFilesWrite)).Post("/servers/{id}/files/dir", a.handleMakeDir)
			pr.With(a.requireCap(capFilesWrite)).Post("/servers/{id}/files/rename", a.handleRenameFile)
			pr.With(a.requireCap(capFilesDelete)).Delete("/servers/{id}/files", a.handleDeleteFile)

			pr.With(a.requireCap(capSettingsView)).
				Get("/servers/{id}/properties", a.handleGetProperties)
			pr.With(a.requireCap(capSettingsEdit)).
				Put("/servers/{id}/properties", a.handleUpdateProperties)

			pr.With(a.requireCap(capPlayersView)).Get("/servers/{id}/players", a.handleListPlayers)
			pr.With(a.requireCap(capPlayersView)).
				Get("/servers/{id}/players/{uuid}/face", a.handlePlayerFace)
			pr.With(a.requireCap(capPlayersManage)).Post("/servers/{id}/players", a.handleAddPlayer)
			pr.With(a.requireCap(capPlayersManage)).
				Delete("/servers/{id}/players/{uuid}", a.handleRemovePlayer)
			pr.With(a.requireCap(capPlayersModerate)).
				Post("/servers/{id}/players/action", a.handlePlayerAction)

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
