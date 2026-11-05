package platform

import (
	"encoding/json"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
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
			writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		next(w, r, p)
	}
}

func withTeacher(next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return withAuth(func(w http.ResponseWriter, r *http.Request, p auth.Principal) {
		if err := auth.RequireTeacher(p); err != nil {
			writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}
		next(w, r, p)
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
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
		writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  p,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	_ = r
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
	httpapi.WriteJSON(w, http.StatusOK, store.Default.MaterialsByClass(classID))
}

func handleCreateMaterial(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	var body store.Material
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.ID = store.NewID("mat")
	body.ClassID = classID
	store.Default.Lock()
	store.Default.Materials[body.ID] = body
	store.Default.Unlock()
	httpapi.WriteJSON(w, http.StatusCreated, body)
}

func handleListAssignments(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
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
	httpapi.WriteJSON(w, http.StatusOK, out)
}

func handleCreateAssignment(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	classID := r.PathValue("classID")
	if classID != p.ClassID {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "not found")
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
	httpapi.WriteJSON(w, http.StatusCreated, body)
}

func handleAudit(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	httpapi.WriteJSON(w, http.StatusOK, store.Default.ListAudit(p.ClassID))
	_ = r
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	httpapi.WriteError(w, r, status, code, msg)
}
