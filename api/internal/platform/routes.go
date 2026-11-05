package platform

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/metrics"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// RegisterRoutes 平台服务：认证、班级隔离、讲义检索、作业与审计。
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", withAuth(handleLogout))
	mux.HandleFunc("GET /api/v1/auth/me", withAuth(handleMe))

	mux.HandleFunc("GET /api/v1/classes/{classID}/materials", withAuth(handleListMaterials))
	mux.HandleFunc("POST /api/v1/classes/{classID}/materials", withTeacher(handleCreateMaterial))
	mux.HandleFunc("GET /api/v1/materials/{materialID}", withAuth(handleGetMaterial))
	mux.HandleFunc("GET /api/v1/materials/{materialID}/chunks", withAuth(handleMaterialChunks))
	mux.HandleFunc("POST /api/v1/classes/{classID}/search", withAuth(handleSearchMaterials))

	mux.HandleFunc("GET /api/v1/classes/{classID}/assistant", withAuth(handleGetAssistant))
	mux.HandleFunc("PUT /api/v1/classes/{classID}/assistant", withTeacher(handleUpdateAssistant))
	mux.HandleFunc("GET /api/v1/conversations", withAuth(handleListConversations))
	mux.HandleFunc("POST /api/v1/classes/{classID}/conversations", withAuth(handleCreateConversation))
	mux.HandleFunc("GET /api/v1/conversations/{conversationID}", withAuth(handleGetConversation))
	mux.HandleFunc("POST /api/v1/conversations/{conversationID}/messages", withAuth(handleConversationMessage))

	mux.HandleFunc("GET /api/v1/classes/{classID}/assignments", withAuth(handleListAssignments))
	mux.HandleFunc("POST /api/v1/classes/{classID}/assignments", withTeacher(handleCreateAssignment))
	mux.HandleFunc("GET /api/v1/assignments/{assignmentID}", withAuth(handleGetAssignment))
	mux.HandleFunc("POST /api/v1/assignments/{assignmentID}/publish", withTeacher(handlePublishAssignment))
	mux.HandleFunc("POST /api/v1/assignments/{assignmentID}/submissions", withAuth(handleSubmitAssignment))
	mux.HandleFunc("GET /api/v1/submissions/{submissionID}", withAuth(handleGetPlatformSubmission))
	mux.HandleFunc("POST /api/v1/submissions/{submissionID}/score", withTeacher(handleOverridePlatformScore))
	mux.HandleFunc("GET /api/v1/submissions/{submissionID}/mistake-explanation", withAuth(handleMistakeExplanation))
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/retry", withAuth(handleRetryTask))

	mux.HandleFunc("GET /api/v1/audit", withTeacher(handleAudit))
}

func withAuth(next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			auditPlatform(r, auth.Principal{}, "authorization", r.URL.Path, "denied", "", 0, nil)
			writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		next(w, r, p)
	}
}

func withTeacher(next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return withAuth(func(w http.ResponseWriter, r *http.Request, p auth.Principal) {
		if err := auth.RequireTeacher(p); err != nil {
			auditPlatform(r, p, "authorization", r.URL.Path, "denied", "", 0, nil)
			writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}
		next(w, r, p)
	})
}

func auditPlatform(r *http.Request, p auth.Principal, action, object, result, skill string, usage int, detail map[string]any) {
	var durationMS int64
	if detail != nil {
		switch value := detail["durationMs"].(type) {
		case int64:
			durationMS = value
		case int:
			durationMS = int64(value)
		case float64:
			durationMS = int64(value)
		}
	}
	if err := store.Default.AppendAudit(store.AuditEvent{
		ID: store.NewID("audit"), ActorID: p.UserID, ClassID: p.ClassID,
		Action: action, Object: object, Result: result, Skill: skill, Usage: usage,
		DurationMS: durationMS, RequestID: httpapi.RequestID(r.Context()), Detail: detail, CreatedAt: time.Now().UTC(),
	}); err != nil {
		log.Printf("audit persistence failed action=%s object=%s request_id=%s: %v", action, object, httpapi.RequestID(r.Context()), err)
	}
	if skill != "" {
		metrics.ObserveSkill(skill, result, time.Duration(durationMS)*time.Millisecond)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeErr(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
		return false
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "request must contain one JSON object")
		return false
	}
	return true
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body, 16<<10) {
		return
	}

	token, p, err := auth.LoginWithPassword(body.Username, body.Password)
	if err != nil {
		auditPlatform(r, auth.Principal{}, "auth.login", "session", "denied", "", 0, nil)
		writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	auditPlatform(r, p, "auth.login", "session", "succeeded", "", 0, nil)
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  p,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	auth.Logout(r.Header.Get("Authorization"))
	auditPlatform(r, p, "auth.logout", "session", "succeeded", "", 0, nil)
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func handleMe(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	httpapi.WriteJSON(w, http.StatusOK, p)
	_ = r
}

func handleListMaterials(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	materials := store.Default.MaterialsByClass(classID)
	for i := range materials {
		materials[i].Content = ""
		materials[i].Chunks = nil
	}
	httpapi.WriteJSON(w, http.StatusOK, materials)
}

func handleCreateMaterial(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decodeJSON(w, r, &body, 2<<20) {
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	body.Content = strings.TrimSpace(body.Content)
	if body.Title == "" || len([]rune(body.Title)) > 120 || body.Content == "" || len([]rune(body.Content)) > 100000 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "title and content are required and must be within limits")
		return
	}
	id := store.NewID("mat")
	version := "v1"
	material := store.Material{
		ID: id, ClassID: classID, Title: body.Title, Content: body.Content,
		Version: version, ContentHash: store.Digest(body.Content), IndexStatus: "indexing",
		CreatedBy: p.UserID, CreatedAt: time.Now().UTC(),
	}
	material.Chunks = store.ChunkMaterial(id, version, body.Title, body.Content)
	if len(material.Chunks) == 0 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "material produced no searchable text")
		return
	}
	store.Default.SaveMaterial(material)
	if err := store.Default.EnsureMaterialDurable(material.ID); err != nil {
		auditPlatform(r, p, "material.ingest", id, "failed", "rag_search", 0, map[string]any{"reason": "persistence_unavailable"})
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "material could not be durably saved")
		return
	}
	embeddingModel, err := indexMaterial(r.Context(), classID, material.Chunks)
	if err != nil {
		material.IndexStatus = "failed"
		store.Default.SaveMaterial(material)
		_ = store.Default.EnsureMaterialDurable(material.ID)
		auditPlatform(r, p, "material.ingest", id, "failed", "rag_search", 0, map[string]any{"reason": "index_unavailable"})
		writeErr(w, r, http.StatusServiceUnavailable, "INDEX_UNAVAILABLE", "material was saved but could not be indexed")
		return
	}
	material.IndexStatus = "ready"
	material.EmbeddingModel = embeddingModel
	store.Default.SaveMaterial(material)
	if err := store.Default.EnsureMaterialDurable(material.ID); err != nil {
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "material index state could not be durably saved")
		return
	}
	auditPlatform(r, p, "material.ingest", id, "succeeded", "rag_search", len(material.Chunks), map[string]any{"version": version})
	material.Content = ""
	httpapi.WriteJSON(w, http.StatusCreated, material)
}

func handleGetMaterial(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	material, ok := store.Default.GetMaterial(r.PathValue("materialID"))
	if !ok || material.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	material.Chunks = nil
	httpapi.WriteJSON(w, http.StatusOK, material)
}

func handleMaterialChunks(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	material, ok := store.Default.GetMaterial(r.PathValue("materialID"))
	if !ok || material.ClassID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"materialId": material.ID, "version": material.Version, "chunks": material.Chunks,
	})
}

func handleSearchMaterials(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if !decodeJSON(w, r, &body, 64<<10) {
		return
	}
	body.Query = strings.TrimSpace(body.Query)
	if body.Query == "" || len([]rune(body.Query)) > 500 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "query is required and must not exceed 500 characters")
		return
	}
	hits, err := retrieveClass(r.Context(), classID, body.Query, body.Limit)
	if err != nil {
		auditPlatform(r, p, "material.search", classID, "failed", "rag_search", 0, nil)
		writeErr(w, r, http.StatusServiceUnavailable, "RETRIEVAL_UNAVAILABLE", "knowledge retrieval is temporarily unavailable")
		return
	}
	runID := store.NewID("retrieval")
	auditPlatform(r, p, "material.search", runID, "succeeded", "rag_search", len(hits), map[string]any{"hitCount": len(hits)})
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"retrievalRunId": runID, "hits": hits})
}

func handleListAssignments(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	out := store.Default.ListAssignments(classID)
	if p.Role == auth.RoleStudent {
		filtered := out[:0]
		for _, assignment := range out {
			if assignment.Status != "published" {
				continue
			}
			hideAssignmentAnswers(&assignment)
			filtered = append(filtered, assignment)
		}
		out = filtered
	}
	httpapi.WriteJSON(w, http.StatusOK, out)
}

func handleCreateAssignment(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body struct {
		Title       string           `json:"title"`
		Description string           `json:"description"`
		MaxScore    float64          `json:"maxScore"`
		Items       []store.Question `json:"items"`
	}
	if !decodeJSON(w, r, &body, 1<<20) {
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" || len([]rune(body.Title)) > 160 || len(body.Items) == 0 || len(body.Items) > 100 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "title and 1-100 items are required")
		return
	}
	if body.MaxScore == 0 {
		body.MaxScore = 10
	}
	if math.IsNaN(body.MaxScore) || math.IsInf(body.MaxScore, 0) || body.MaxScore <= 0 || body.MaxScore > 10_000 {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "maxScore must be between 0 and 10000")
		return
	}
	for i := range body.Items {
		body.Items[i].ID = strings.TrimSpace(body.Items[i].ID)
		body.Items[i].Stem = strings.TrimSpace(body.Items[i].Stem)
		body.Items[i].Answer = strings.TrimSpace(body.Items[i].Answer)
		body.Items[i].KnowledgePoint = strings.TrimSpace(body.Items[i].KnowledgePoint)
		if body.Items[i].ID == "" {
			body.Items[i].ID = store.NewID("q")
		}
		if body.Items[i].KnowledgePoint == "" {
			body.Items[i].KnowledgePoint = body.Title
		}
		if len([]rune(body.Items[i].KnowledgePoint)) > 100 {
			writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "knowledgePoint must not exceed 100 characters")
			return
		}
		if body.Items[i].Stem == "" || body.Items[i].Answer == "" {
			writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "every item requires a stem and answer")
			return
		}
	}
	assignment := store.Assignment{
		ID: store.NewID("asg"), ClassID: classID, Title: body.Title, Description: strings.TrimSpace(body.Description),
		Status: "draft", MaxScore: body.MaxScore, Items: body.Items, CreatedBy: p.UserID, CreatedAt: time.Now().UTC(),
	}
	store.Default.SaveAssignment(assignment)
	if err := store.Default.EnsureAssignmentDurable(assignment.ID); err != nil {
		auditPlatform(r, p, "assignment.create", assignment.ID, "failed", "", 0, nil)
		writeErr(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "assignment could not be durably saved")
		return
	}
	auditPlatform(r, p, "assignment.create", assignment.ID, "succeeded", "", len(assignment.Items), nil)
	httpapi.WriteJSON(w, http.StatusCreated, assignment)
}

func handleAudit(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	httpapi.WriteJSON(w, http.StatusOK, store.Default.ListAudit(p.ClassID))
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	httpapi.WriteError(w, r, status, code, msg)
}
