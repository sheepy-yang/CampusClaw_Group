package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testToken = "test-mcp-token"

func testHandler() http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewHandler(testToken, logger)
}

func callRPC(t *testing.T, method string) *httptest.ResponseRecorder {
	return callRPCWithParams(t, testHandler(), method, nil)
}

func callRPCWithParams(t *testing.T, handler http.Handler, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestHealthDoesNotRequireAuthentication(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	testHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestMCPRequiresBearerToken(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	response := httptest.NewRecorder()

	testHandler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestInitializeAndToolDiscovery(t *testing.T) {
	initialize := callRPC(t, "initialize")
	if initialize.Code != http.StatusOK {
		t.Fatalf("initialize status = %d", initialize.Code)
	}
	var initResponse map[string]any
	if err := json.NewDecoder(initialize.Body).Decode(&initResponse); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if initResponse["jsonrpc"] != "2.0" {
		t.Fatalf("unexpected initialize response: %#v", initResponse)
	}

	list := callRPC(t, "tools/list")
	var listResponse struct {
		Result struct {
			Tools []ToolDesc `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listResponse); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	if len(listResponse.Result.Tools) != 3 {
		t.Fatalf("tool count = %d, want 3", len(listResponse.Result.Tools))
	}
	for _, tool := range listResponse.Result.Tools {
		if tool.Name == "" || len(tool.InputSchema) == 0 {
			t.Errorf("incomplete tool descriptor: %#v", tool)
		}
	}
}

func TestInvalidToolCallAndUnknownMethodReturnJSONRPCErrors(t *testing.T) {
	for method, expectedCode := range map[string]int{
		"tools/call": -32602,
		"unknown":    -32601,
	} {
		response := callRPC(t, method)
		var rpc rpcResponse
		if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
			t.Fatalf("%s decode response: %v", method, err)
		}
		if rpc.Error == nil || rpc.Error.Code != expectedCode {
			t.Errorf("%s error = %#v, want code %d", method, rpc.Error, expectedCode)
		}
	}
}

func TestToolCallExecutesAndReturnsStructuredContent(t *testing.T) {
	response := callRPCWithParams(t, testHandler(), "tools/call", map[string]any{
		"name": "formula_verify",
		"arguments": map[string]any{
			"expression": "(1+2)*3",
			"expected":   "9",
		},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var rpc struct {
		Result struct {
			StructuredContent map[string]any `json:"structuredContent"`
			IsError           bool           `json:"isError"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
		t.Fatal(err)
	}
	if rpc.Error != nil || rpc.Result.IsError || rpc.Result.StructuredContent["verified"] != true {
		t.Fatalf("tool result = %#v", rpc)
	}
}

func TestToolCallRejectsUnknownToolAndInvalidSchema(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		code   string
	}{
		{
			name:   "unknown tool",
			params: map[string]any{"name": "unknown", "arguments": map[string]any{}},
			code:   "TOOL_NOT_FOUND",
		},
		{
			name: "missing argument",
			params: map[string]any{
				"name":      "formula_verify",
				"arguments": map[string]any{"expression": "1"},
			},
			code: "INVALID_TOOL_ARGUMENTS",
		},
		{
			name: "extra argument",
			params: map[string]any{
				"name":      "formula_verify",
				"arguments": map[string]any{"expression": "1", "expected": "1", "student_id": "other"},
			},
			code: "INVALID_TOOL_ARGUMENTS",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := callRPCWithParams(t, testHandler(), "tools/call", test.params)
			var rpc rpcResponse
			if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
				t.Fatal(err)
			}
			if rpc.Error == nil || rpc.Error.Code != -32602 || rpc.Error.Data["code"] != test.code {
				t.Fatalf("error = %#v", rpc.Error)
			}
		})
	}
}

func TestToolCallTimeoutIsMappedWithoutLeakingDetails(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := NewHandlerWithOptions(testToken, logger, HandlerOptions{
		CallTimeout: 5 * time.Millisecond,
		Executor: func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	response := callRPCWithParams(t, handler, "tools/call", map[string]any{
		"name":      "formula_verify",
		"arguments": map[string]any{"expression": "1", "expected": "1"},
	})
	var rpc rpcResponse
	if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
		t.Fatal(err)
	}
	if rpc.Error == nil || rpc.Error.Code != -32002 || rpc.Error.Data["code"] != "TOOL_TIMEOUT" {
		t.Fatalf("timeout error = %#v", rpc.Error)
	}
}

func TestMCPRejectsMalformedEnvelopeAndTrailingJSON(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"1.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":true,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"} {}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		testHandler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d", body, response.Code)
		}
	}
}

func TestExecutorContextCancellationIsAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ExecuteTool(ctx, "formula_verify", map[string]any{"expression": "1", "expected": "1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
