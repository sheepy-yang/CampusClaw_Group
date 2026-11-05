package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// NewHandler 实现 initialize / tools/list / tools/call，Bearer Token 鉴权。
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", handleMCP)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}

func handleMCP(w http.ResponseWriter, r *http.Request) {
	token := expectedToken()
	if token != "" {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	var req struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	switch req.Method {
	case "initialize":
		writeJSON(w, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "mcp-teaching-tools", "version": "0.1.0"},
		})
	case "tools/list":
		writeJSON(w, map[string]any{"tools": ListTools()})
	case "tools/call":
		handleCall(w, r, req.Params)
	default:
		http.Error(w, `{"error":"unknown method"}`, http.StatusBadRequest)
	}
}

func handleCall(w http.ResponseWriter, r *http.Request, params map[string]any) {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	// 透传调用方身份（由主机注入，不采信浏览器）
	if uid, ok := params["userId"]; ok {
		args["userId"] = uid
	}
	if cid, ok := params["classId"]; ok {
		args["classId"] = cid
	}
	if role, ok := params["role"]; ok {
		args["role"] = role
	}

	var (
		result map[string]any
		err    error
	)
	switch name {
	case "formula_verify":
		result, err = FormulaVerify(r.Context(), args)
	case "assignment_stats":
		result, err = AssignmentStats(r.Context(), args)
	case "student_mistakes":
		result, err = StudentMistakes(r.Context(), args)
	default:
		http.Error(w, fmt.Sprintf(`{"error":"unknown tool %s"}`, name), http.StatusBadRequest)
		return
	}
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"result": result})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
