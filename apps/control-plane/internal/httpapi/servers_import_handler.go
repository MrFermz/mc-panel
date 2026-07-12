package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	agentv1 "github.com/mc-panel/proto/gen/go/mcpanel/agent/v1"

	"github.com/mc-panel/control-plane/internal/agenthub"
	"github.com/mc-panel/control-plane/internal/auth"
	"github.com/mc-panel/control-plane/internal/store"
)

const (
	// maxImportUpload = เพดานขนาด zip ที่ยอมรับ (กัน DoS จาก body ยักษ์) — MaxBytesReader
	// ตัดที่ชั้น body ก่อนถึง handler ไม่ buffer ทั้งก้อน
	maxImportUpload = 2 << 30 // 2 GiB
	// importChunkSize = ขนาดก้อนที่ stream ต่อ FileWriteChunk ไป agent (bounded memory:
	// buffer สองก้อนสลับกัน look-ahead เพื่อ set last ให้ตรง ไม่เคยถือทั้ง zip ใน memory)
	importChunkSize = 768 * 1024
	// maxImportTextField = เพดานของ text field แต่ละอันใน multipart (name/version ฯลฯ)
	maxImportTextField = 4096
	// importArchivePath = ที่ staged zip ใน jail ของ server (relative) — agent แตกด้วย SafeJoin
	importArchivePath = ".mcpanel/import.zip"
)

// importMeta = text fields ที่มากับ multipart (นอกจาก part `archive`)
type importMeta struct {
	name       string
	nodeID     string
	serverType string
	mcVersion  string
	memoryMB   string
	hostPort   string
	acceptEula string
}

func (m *importMeta) set(field, value string) {
	switch field {
	case "name":
		m.name = value
	case "node_id":
		m.nodeID = value
	case "server_type":
		m.serverType = value
	case "mc_version":
		m.mcVersion = value
	case "memory_mb":
		m.memoryMB = value
	case "host_port":
		m.hostPort = value
	case "accept_eula":
		m.acceptEula = value
	}
}

// validatedImport = ผลของการ validate metadata (เหมือน handleCreateServer ทุกกฎ)
type validatedImport struct {
	name       string
	nodeID     uuid.UUID
	serverType string
	mcVersion  string
	memoryMB   int
	hostPort   *int
	acceptEula bool
}

// validateImportMeta ตรวจ field ให้เหมือน create ทุกข้อ — คืน (code, message) เมื่อไม่ผ่าน
// (status เป็น 400 ทั้งหมด ยกเว้น node_not_found = 404 จัดการแยกใน handler)
func validateImportMeta(m *importMeta) (*validatedImport, string, string) {
	name := strings.TrimSpace(m.name)
	if name == "" || len(name) > 100 {
		return nil, "invalid_name", "name is required (max 100 characters)"
	}
	if !validServerTypes[m.serverType] {
		return nil, "invalid_server_type", "server_type must be one of: vanilla, paper, fabric, forge, velocity"
	}
	mcVersion := strings.TrimSpace(m.mcVersion)
	if mcVersion == "" || len(mcVersion) > 50 {
		return nil, "invalid_mc_version", "mc_version is required (max 50 characters)"
	}
	memoryMB, err := strconv.Atoi(strings.TrimSpace(m.memoryMB))
	if err != nil || memoryMB < 256 {
		return nil, "invalid_memory", "memory_mb must be at least 256"
	}
	var hostPort *int
	if hp := strings.TrimSpace(m.hostPort); hp != "" {
		p, err := strconv.Atoi(hp)
		if err != nil {
			return nil, "invalid_host_port", "host_port must be between 1024 and 65535"
		}
		hostPort = &p
	}
	if !validateHostPort(hostPort) {
		return nil, "invalid_host_port", "host_port must be between 1024 and 65535"
	}
	acceptEula := parseImportBool(m.acceptEula)
	// velocity เป็น proxy ไม่รัน Mojang jar — ไม่มี EULA ให้ยอมรับ
	if !acceptEula && m.serverType != "velocity" {
		return nil, "eula_required", "you must accept the Minecraft EULA to import this server"
	}
	nodeID, err := uuid.Parse(strings.TrimSpace(m.nodeID))
	if err != nil {
		return nil, "invalid_request", "node_id must be a valid UUID"
	}
	return &validatedImport{
		name:       name,
		nodeID:     nodeID,
		serverType: m.serverType,
		mcVersion:  mcVersion,
		memoryMB:   memoryMB,
		hostPort:   hostPort,
		acceptEula: acceptEula,
	}, "", ""
}

func parseImportBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "on", "yes":
		return true
	}
	return false
}

// handleImportServer รับ zip ของ server เดิมแบบ streaming multipart แล้ว stage เข้า agent เป็น
// chunked FileWriteChunk (.mcpanel/import.zip ใน jail) ก่อน dispatch NATS job `import_server`
// — lifecycle command เป็น job ตามกฎ #2, ส่วน bytes-over-gRPC เป็น file I/O ปกติ (file manager
// ก็ทำแบบนี้). ห้าม buffer ทั้ง zip: อ่านทีละก้อน look-ahead หนึ่งก้อนพอให้รู้ก้อนสุดท้าย
func (a *API) handleImportServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFrom(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUpload)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "expected multipart/form-data body")
		return
	}

	var meta importMeta
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			// ครบทุก part แล้วแต่ไม่เจอ `archive` — zip เป็น field บังคับ
			writeError(w, http.StatusBadRequest, "invalid_request", "archive file part is required")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed multipart body")
			return
		}

		if part.FormName() != "archive" {
			val, err := io.ReadAll(io.LimitReader(part, maxImportTextField))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "malformed multipart field")
				return
			}
			meta.set(part.FormName(), string(val))
			continue
		}

		// ถึง part `archive` แล้ว — metadata ต้องมาก่อนหน้านี้ทั้งหมด (client ส่ง archive เป็น part สุดท้าย)
		a.streamImportArchive(w, r, user, &meta, part)
		return
	}
}

// streamImportArchive: validate metadata → สร้าง server row → stream zip เข้า agent → dispatch job
// ทุก failure หลังสร้าง row ทำ cleanup (staging ล้ม = ลบ row ที่เพิ่งสร้าง, dispatch ล้ม = mark errored
// เหมือน create) ก่อนตอบ error
func (a *API) streamImportArchive(w http.ResponseWriter, r *http.Request, user *store.User, meta *importMeta, archive io.Reader) {
	v, code, msg := validateImportMeta(meta)
	if code != "" {
		writeError(w, http.StatusBadRequest, code, msg)
		return
	}

	node, err := a.st.GetNodeByID(r.Context(), v.nodeID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node_not_found", "node not found")
			return
		}
		a.log.Error("load node failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// admission control ก่อน stage/สร้าง row — กัน RAM overcommit เหมือน create
	if !a.checkNodeMemory(w, r, node, v.memoryMB, 0) {
		return
	}

	// อ่านก้อนแรกก่อนสร้าง row เพื่อจับ empty archive โดยยังไม่ทิ้ง phantom row ไว้
	buf := make([]byte, importChunkSize)
	other := make([]byte, importChunkSize)
	hold := buf
	next := other
	holdN, holdEOF, err := fillImportChunk(archive, hold)
	if err != nil {
		a.writeImportReadError(w, err)
		return
	}
	if holdN == 0 && holdEOF {
		writeError(w, http.StatusBadRequest, "empty_archive", "archive is empty")
		return
	}

	srv, err := a.st.CreateServerWithOwner(r.Context(), v.nodeID, user.ID,
		v.name, v.serverType, v.mcVersion, v.memoryMB, v.hostPort)
	if store.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "host_port_taken", "host_port is already used on this node")
		return
	}
	if err != nil {
		a.log.Error("create server for import failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// stage zip: look-ahead หนึ่งก้อน เพื่อ set last=true เฉพาะก้อนที่มีข้อมูลจริงก้อนสุดท้าย
	// (กันเคส size หาร importChunkSize ลงตัวพอดี ที่ io.ReadFull ยังไม่เห็น EOF)
	first := true
	var total int64
	for {
		if holdEOF {
			if status, code, msg := a.sendImportChunk(r.Context(), srv, hold[:holdN], first, true); status != 0 {
				a.cleanupImportStaging(r.Context(), srv.ID)
				writeError(w, status, code, msg)
				return
			}
			total += int64(holdN)
			break
		}

		nextN, nextEOF, err := fillImportChunk(archive, next)
		if err != nil {
			a.cleanupImportStaging(r.Context(), srv.ID)
			a.writeImportReadError(w, err)
			return
		}
		// ก้อนถัดไปว่างและถึง EOF = hold คือก้อนข้อมูลสุดท้ายจริง ๆ
		lastChunk := nextN == 0 && nextEOF
		if status, code, msg := a.sendImportChunk(r.Context(), srv, hold[:holdN], first, lastChunk); status != 0 {
			a.cleanupImportStaging(r.Context(), srv.ID)
			writeError(w, status, code, msg)
			return
		}
		total += int64(holdN)
		first = false
		if lastChunk {
			break
		}
		hold, next = next, hold
		holdN, holdEOF = nextN, nextEOF
	}

	if total == 0 {
		// ป้องกันไว้อีกชั้น (ไม่ควรถึงเพราะเช็ค empty ก่อนสร้าง row แล้ว)
		a.cleanupImportStaging(r.Context(), srv.ID)
		writeError(w, http.StatusBadRequest, "empty_archive", "archive is empty")
		return
	}

	a.audit(r, &user.ID, &srv.ID, "server_import", map[string]any{
		"name": srv.Name, "server_type": srv.ServerType, "mc_version": srv.MCVersion,
		"node_id": srv.NodeID.String(), "archive_bytes": total,
	})

	// staging สำเร็จ row คงอยู่แน่ (cleanup path ที่ลบ row อยู่ก่อนหน้านี้แล้ว)
	// → แจ้ง browser refetch server list
	a.events.ServerAdded(srv.ID)

	job, err := a.disp.ImportServer(r.Context(), srv, v.acceptEula, importArchivePath, user.ID)
	if err != nil {
		// zip staged แล้วแต่ dispatch ล้ม — mark errored เหมือน create (row เป็น server ที่ provision ไม่สำเร็จ)
		if serr := a.st.UpdateServerStatus(r.Context(), srv.ID, "errored"); serr != nil {
			a.log.Error("mark server errored failed", "server_id", srv.ID, "error", serr)
		}
		writeError(w, http.StatusBadGateway, "dispatch_failed", "failed to dispatch import job")
		return
	}

	job.RequestedByEmail = &user.Email
	writeJSON(w, http.StatusCreated, map[string]any{
		"server": toServerView(srv, a.statsViewFor(srv)),
		"job":    toJobView(job),
	})
}

// sendImportChunk ส่งก้อนหนึ่งไป agent แล้ว map error เหมือน sendFileRequest — คืน status 0 เมื่อสำเร็จ
func (a *API) sendImportChunk(ctx context.Context, srv *store.Server, content []byte, first, last bool) (int, string, string) {
	req := &agentv1.FileRequest{
		ServerId: srv.ID.String(),
		Op: &agentv1.FileRequest_WriteChunk{WriteChunk: &agentv1.FileWriteChunk{
			Path:    importArchivePath,
			Content: content,
			First:   first,
			Last:    last,
		}},
	}
	resp, err := a.hub.SendFileRequest(ctx, srv.NodeID, req)
	switch {
	case errors.Is(err, agenthub.ErrNodeNotConnected), errors.Is(err, agenthub.ErrSendTimeout):
		return http.StatusServiceUnavailable, "node_offline", "node agent is offline"
	case errors.Is(err, agenthub.ErrAgentTimeout):
		return http.StatusGatewayTimeout, "agent_timeout", "node agent did not respond in time"
	case err != nil:
		a.log.Error("import chunk send failed", "server_id", srv.ID, "error", err)
		return http.StatusBadGateway, "import_failed", "failed to stage archive to node agent"
	}
	if !resp.Success {
		a.log.Error("import chunk rejected", "server_id", srv.ID, "error", resp.Error)
		return http.StatusBadGateway, "import_failed", "node agent rejected archive chunk: "+resp.Error
	}
	return 0, "", ""
}

// cleanupImportStaging ลบ row ที่เพิ่งสร้างเมื่อ staging ล้ม (best-effort) — ไม่มีอะไรบน agent ที่ค้าง
// ต้องกู้ (import.zip ที่เขียนไปบางส่วนอยู่ใน jail และหายไปตอน delete/import รอบหน้า)
func (a *API) cleanupImportStaging(ctx context.Context, serverID uuid.UUID) {
	if err := a.st.DeleteServerRow(ctx, serverID); err != nil {
		a.log.Error("cleanup import row failed", "server_id", serverID, "error", err)
	}
}

// writeImportReadError map error ตอนอ่าน body: เกินเพดาน = 413, อื่น ๆ = 400
func (a *API) writeImportReadError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "archive_too_large", "archive exceeds the maximum upload size")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_request", "failed to read archive body")
}

// fillImportChunk อ่านเต็มบัฟเฟอร์ให้มากที่สุด คืน (n, eofReached, err)
// eofReached=true เมื่อถึงท้าย stream แล้ว (n อาจ >0 หรือ 0)
func fillImportChunk(r io.Reader, buf []byte) (int, bool, error) {
	n, err := io.ReadFull(r, buf)
	switch {
	case err == nil:
		return n, false, nil
	case errors.Is(err, io.ErrUnexpectedEOF):
		return n, true, nil
	case errors.Is(err, io.EOF):
		return 0, true, nil
	default:
		return n, false, err
	}
}
