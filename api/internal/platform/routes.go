package platform

import (
	"encoding/json"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// RegisterRoutes 平台服务：认证、班级隔离、讲义检索、作业与审计。
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", handleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", withAuth(handleMe))

	mux.HandleFunc("GET /api/v1/classes/{classID}/materials", withAuth(handleListMaterials))
	mux.HandleFunc("POST /api/v1/classes/{classID}/materials", withTeacher(handleCreateMaterial))

	mux.HandleFunc("GET /api/v1/classes/{classID}/assignments", withAuth(handleListAssignments))
	mux.HandleFunc("POST /api/v1/classes/{classID}/assignments", withTeacher(handleCreateAssignment))

	mux.HandleFunc("GET /api/v1/audit", withTeacher(handleAudit))
}

func withAuth(next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r, p)
	}
}

func withTeacher(next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return withAuth(func(w http.ResponseWriter, r *http.Request, p auth.Principal) {
		if err := auth.RequireTeacher(p); err != nil {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r, p)
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"` // 兼容：仅传 role 时一键登录
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	var (
		token string
		p     auth.Principal
		err   error
	)
	if body.Username != "" || body.Password != "" {
		token, p, err = auth.LoginWithPassword(body.Username, body.Password)
	} else {
		role := auth.RoleStudent
		if body.Role == "teacher" {
			role = auth.RoleTeacher
		}
		token, p, err = auth.Login(role)
	}
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  p,
	})
}

func handleLogout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func handleMe(w http.ResponseWriter, _ *http.Request, p auth.Principal) {
	writeJSON(w, http.StatusOK, p)
}

func handleListMaterials(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, store.Default.MaterialsByClass(classID))
}

func handleCreateMaterial(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var body store.Material
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.ID = store.NewID("mat")
	body.ClassID = classID
	store.Default.Lock()
	store.Default.Materials[body.ID] = body
	store.Default.Unlock()
	writeJSON(w, http.StatusCreated, body)
}

func handleListAssignments(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	store.Default.Lock()
	defer store.Default.Unlock()
	var out []store.Assignment
	for _, a := range store.Default.Assignments {
		if a.ClassID != classID {
			continue
		}
		cp := a
		if p.Role == auth.RoleStudent {
			for i := range cp.Items {
				cp.Items[i].Answer = ""
				cp.Items[i].Explanation = ""
			}
		}
		out = append(out, cp)
	}
	writeJSON(w, http.StatusOK, out)
}

func handleCreateAssignment(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var body store.Assignment
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.ID = store.NewID("asg")
	body.ClassID = classID
	if body.Status == "" {
		body.Status = "draft"
	}
	store.Default.Lock()
	store.Default.Assignments[body.ID] = body
	store.Default.Unlock()
	writeJSON(w, http.StatusCreated, body)
}

func handleAudit(w http.ResponseWriter, _ *http.Request, p auth.Principal) {
	writeJSON(w, http.StatusOK, store.Default.ListAudit(p.ClassID))
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
