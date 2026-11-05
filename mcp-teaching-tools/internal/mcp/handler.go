package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const protocolVersion = "2025-06-18"
const maxRPCBodyBytes = 1 << 20

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type ToolExecutor func(context.Context, string, map[string]any) (map[string]any, error)

type HandlerOptions struct {
	CallTimeout time.Duration
	Executor    ToolExecutor
}

type rpcServer struct {
	logger      *slog.Logger
	callTimeout time.Duration
	execute     ToolExecutor
}

type requestIDKey struct{}

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

// NewHandler provides authenticated MCP discovery and deterministic tool execution.
func NewHandler(token string, logger *slog.Logger) http.Handler {
	return NewHandlerWithOptions(token, logger, HandlerOptions{})
}

func NewHandlerWithOptions(token string, logger *slog.Logger, options HandlerOptions) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if options.CallTimeout <= 0 {
		options.CallTimeout = 3 * time.Second
	}
	if options.Executor == nil {
		options.Executor = ExecuteTool
	}
	server := &rpcServer{
		logger:      logger,
		callTimeout: options.CallTimeout,
		execute:     options.Executor,
	}
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
		server.handleMCP(w, r)
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

func (s *rpcServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRPCBodyBytes)
	var req rpcRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, -32700, "Parse error")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, -32700, "Parse error")
		return
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" || !validRPCID(req.ID) {
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
				"version": "1.0.0",
			},
		})
	case "tools/list":
		writeRPCResult(w, req.ID, map[string]any{"tools": ListTools()})
	case "tools/call":
		s.handleToolCall(w, r, req)
	default:
		writeRPCError(w, http.StatusOK, req.ID, -32601, "Method not found")
	}
}

func (s *rpcServer) handleToolCall(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := decodeStrict(req.Params, &params); err != nil ||
		!toolNamePattern.MatchString(params.Name) ||
		params.Arguments == nil {
		writeRPCErrorData(w, http.StatusOK, req.ID, -32602, "Invalid params", "INVALID_PARAMS")
		return
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), s.callTimeout)
	defer cancel()
	result, err := s.execute(ctx, params.Name, params.Arguments)
	duration := time.Since(started)
	status, errorCode := "succeeded", ""
	defer func() {
		s.logger.InfoContext(
			r.Context(),
			"mcp tool call completed",
			"event", "mcp_tool_call",
			"request_id", requestIDFromContext(r.Context()),
			"tool", params.Name,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"error_code", errorCode,
		)
	}()
	if err != nil {
		status = "failed"
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			errorCode = "TOOL_TIMEOUT"
			writeRPCErrorData(w, http.StatusOK, req.ID, -32002, "Tool execution timed out", errorCode)
		case errors.Is(err, ErrUnknownTool):
			errorCode = "TOOL_NOT_FOUND"
			writeRPCErrorData(w, http.StatusOK, req.ID, -32602, "Unknown tool", errorCode)
		case errors.Is(err, ErrInvalidArgs):
			errorCode = "INVALID_TOOL_ARGUMENTS"
			writeRPCErrorData(w, http.StatusOK, req.ID, -32602, "Invalid tool arguments", errorCode)
		default:
			errorCode = "TOOL_EXECUTION_FAILED"
			writeRPCErrorData(w, http.StatusOK, req.ID, -32000, "Tool execution failed", errorCode)
		}
		return
	}

	text, err := json.Marshal(result)
	if err != nil {
		status = "failed"
		errorCode = "TOOL_RESULT_INVALID"
		writeRPCErrorData(w, http.StatusOK, req.ID, -32000, "Tool result is invalid", errorCode)
		return
	}
	writeRPCResult(w, req.ID, map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(text)}},
		"structuredContent": result,
		"isError":           false,
	})
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

func writeRPCErrorData(w http.ResponseWriter, status int, id json.RawMessage, code int, message, stableCode string) {
	writeJSON(w, status, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
			Data:    map[string]any{"code": stableCode},
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func validBearer(header, token string) bool {
	if token == "" {
		return false
	}
	expected := "Bearer " + token
	if len(header) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(expected)) == 1
}

func validRPCID(id json.RawMessage) bool {
	if len(id) == 0 || bytes.Equal(bytes.TrimSpace(id), []byte("null")) {
		return false
	}
	var value any
	if err := json.Unmarshal(id, &value); err != nil {
		return false
	}
	switch value.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}

func decodeStrict(raw json.RawMessage, destination any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("params are required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
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
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
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

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(raw[:])
}
