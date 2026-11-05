package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
)

func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		p, err := auth.Authenticate(r.Context(), h)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}
}

func withTeacher(next http.HandlerFunc) http.HandlerFunc {
	return withAuth(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.FromContext(r.Context())
		if err := auth.RequireTeacher(p); err != nil {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	})
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

func decodeBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
