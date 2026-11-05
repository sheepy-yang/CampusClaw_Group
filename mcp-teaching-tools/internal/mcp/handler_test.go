package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testToken = "test-mcp-token"

func testHandler() http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewHandler(testToken, logger)
}

func callRPC(t *testing.T, method string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	testHandler().ServeHTTP(response, request)
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

func TestToolCallAndUnknownMethodReturnJSONRPCErrors(t *testing.T) {
	for method, expectedCode := range map[string]int{
		"tools/call": -32001,
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
