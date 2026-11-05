package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// ToolCall 一次受鉴权与审计约束的工具调用。
type ToolCall struct {
	ServerID string
	Tool     string
	Args     map[string]any
	UserID   string
	ClassID  string
}

type ToolResult struct {
	OK       bool
	Data     map[string]any
	Error    string
	DataOnly bool
}

// Host MCP 主机：技能只能绑定已连接、已发现且属于本班的工具。
type Host interface {
	ListTools(ctx context.Context, classID string) ([]string, error)
	Call(ctx context.Context, call ToolCall) (*ToolResult, error)
}

// HTTPHost 调用同仓 mcp-teaching-tools。
type HTTPHost struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewHost() Host {
	base := os.Getenv("MCP_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:8100"
	}
	return &HTTPHost{
		BaseURL: base,
		Token:   os.Getenv("MCP_TOKEN"),
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *HTTPHost) ListTools(ctx context.Context, _ string) ([]string, error) {
	resp, err := h.rpc(ctx, "tools/list", nil)
	if err != nil {
		// 降级：本地已知工具列表
		return []string{"formula_verify", "assignment_stats", "student_mistakes"}, nil
	}
	tools, _ := resp["tools"].([]any)
	var names []string
	for _, t := range tools {
		if m, ok := t.(map[string]any); ok {
			names = append(names, fmt.Sprint(m["name"]))
		}
	}
	if len(names) == 0 {
		return []string{"formula_verify", "assignment_stats", "student_mistakes"}, nil
	}
	return names, nil
}

func (h *HTTPHost) Call(ctx context.Context, call ToolCall) (*ToolResult, error) {
	params := map[string]any{
		"name":      call.Tool,
		"arguments": call.Args,
		"userId":    call.UserID,
		"classId":   call.ClassID,
	}
	resp, err := h.rpc(ctx, "tools/call", params)
	if err != nil {
		return &ToolResult{OK: false, Error: err.Error(), DataOnly: true}, nil
	}
	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		return &ToolResult{OK: false, Error: errMsg, DataOnly: true}, nil
	}
	data, _ := resp["result"].(map[string]any)
	if data == nil {
		data = resp
	}
	return &ToolResult{OK: true, Data: data, DataOnly: true}, nil
}

func (h *HTTPHost) rpc(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.BaseURL+"/mcp", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	res, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("mcp http %d: %v", res.StatusCode, out["error"])
	}
	return out, nil
}

type StubHost = HTTPHost
