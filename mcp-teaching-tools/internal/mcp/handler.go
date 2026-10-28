package mcp

import (
	"encoding/json"
	"net/http"
)

// NewHandler 实现 initialize / tools/list / tools/call（骨架），Bearer Token 鉴权待补。
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", handleMCP)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}

func handleMCP(w http.ResponseWriter, r *http.Request) {
	// TODO: 校验 Authorization: Bearer <MCP_TOKEN>
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	method, _ := req["method"].(string)
	switch method {
	case "initialize":
		writeJSON(w, map[string]any{"protocolVersion": "skeleton", "serverInfo": map[string]string{"name": "mcp-teaching-tools"}})
	case "tools/list":
		writeJSON(w, map[string]any{"tools": ListTools()})
	case "tools/call":
		http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
	default:
		http.Error(w, `{"error":"unknown method"}`, http.StatusBadRequest)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
