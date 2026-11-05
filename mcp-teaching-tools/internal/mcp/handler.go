package mcp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

const protocolVersion = "2025-06-18"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}

// NewHandler provides the authenticated JSON-RPC protocol shell without tool business logic.
func NewHandler(token string, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		if !validBearer(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": "valid bearer token required",
			})
			return
		}
		handleMCP(w, r)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "up", "service": "mcp"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code":    "NOT_FOUND",
			"message": "resource not found",
		})
	})
	return requestLogging(logger, mux)
}

func handleMCP(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, -32700, "Parse error")
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" || len(req.ID) == 0 {
		writeRPCError(w, http.StatusBadRequest, req.ID, -32600, "Invalid Request")
		return
	}

	switch req.Method {
	case "initialize":
		writeRPCResult(w, req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]string{
				"name":    "mcp-teaching-tools",
				"version": "0.0.1-skeleton",
			},
		})
	case "tools/list":
		writeRPCResult(w, req.ID, map[string]any{"tools": ListTools()})
	case "tools/call":
		writeRPCError(w, http.StatusOK, req.ID, -32001, "Tool execution is not implemented in the skeleton")
	default:
		writeRPCError(w, http.StatusOK, req.ID, -32601, "Method not found")
	}
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, status int, id json.RawMessage, code int, message string) {
	writeJSON(w, status, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func validBearer(header, token string) bool {
	expected := "Bearer " + token
	if len(header) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(expected)) == 1
}

func requestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(requestID) {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		recorder := &statusWriter{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		logger.Info(
			"http request completed",
			"event", "http_request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(raw[:])
}
