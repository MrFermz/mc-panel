package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/jobs"
	"github.com/mc-panel/control-plane/internal/store"
)

func (a *API) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.st.ListNodes(r.Context())
	if err != nil {
		a.log.Error("list nodes failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	views := make([]nodeView, 0, len(nodes))
	for _, n := range nodes {
		views = append(views, toNodeView(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": views})
}

func (a *API) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "name is required (max 100 characters)")
		return
	}

	// token format: <node_id>.<secret> — DB เก็บ SHA-256 ของ token ทั้งเส้น
	// จึงต้องกำหนด id เองก่อน insert
	nodeID := uuid.New()
	secret, err := auth.GenerateSecretHex(32)
	if err != nil {
		a.log.Error("generate node secret failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	token := nodeID.String() + "." + secret

	node, err := a.st.CreateNode(r.Context(), nodeID, req.Name, auth.HashToken(token))
	if store.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "name_exists", "a node with this name already exists")
		return
	}
	if err != nil {
		a.log.Error("create node failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// agent ไม่มีสิทธิ์สร้าง consumer เอง (NATS ACL) — control-plane ต้องเตรียมให้
	if err := jobs.EnsureNodeConsumer(r.Context(), a.js, nodeID.String()); err != nil {
		a.log.Error("create node consumer failed, rolling back node", "node_id", nodeID, "error", err)
		if derr := a.st.DeleteNode(r.Context(), nodeID); derr != nil {
			a.log.Error("rollback node failed", "node_id", nodeID, "error", derr)
		}
		writeError(w, http.StatusBadGateway, "nats_unavailable", "failed to provision job queue for node")
		return
	}

	a.audit(r, &actor.ID, nil, "node_created",
		map[string]any{"node_id": nodeID.String(), "name": node.Name})

	// token แสดงครั้งเดียว — DB มีแค่ hash
	writeJSON(w, http.StatusCreated, map[string]any{
		"node":  toNodeView(node),
		"token": token,
	})
}

func (a *API) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFrom(r.Context())

	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusNotFound, "node_not_found", "node not found")
		return
	}

	count, err := a.st.CountServersByNode(r.Context(), id)
	if err != nil {
		a.log.Error("count servers by node failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "node_has_servers",
			"node still has servers; delete them first")
		return
	}

	err = a.st.DeleteNode(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "node_not_found", "node not found")
		return
	}
	if err != nil {
		a.log.Error("delete node failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if err := jobs.DeleteNodeConsumer(r.Context(), a.js, id.String()); err != nil {
		// consumer ค้างไม่กระทบความถูกต้อง — แค่ log ไว้
		a.log.Warn("delete node consumer failed", "node_id", id, "error", err)
	}

	a.audit(r, &actor.ID, nil, "node_deleted", map[string]any{"node_id": id.String()})
	w.WriteHeader(http.StatusNoContent)
}
