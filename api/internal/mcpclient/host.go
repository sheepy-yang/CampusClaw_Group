package mcpclient

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
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
	OK    bool
	Data  map[string]any
	Error string
	// DataOnly 调用结果仅作为模型上下文数据，不作系统指令。
	DataOnly bool
}

// Host MCP 主机：技能只能绑定已连接、已发现且属于本班的工具。
type Host interface {
	ListTools(ctx context.Context, classID string) ([]string, error)
	Call(ctx context.Context, call ToolCall) (*ToolResult, error)
}

// StubHost 骨架。
type StubHost struct{}

func (StubHost) ListTools(_ context.Context, _ string) ([]string, error) {
	return nil, apperrors.ErrNotImplemented
}
func (StubHost) Call(_ context.Context, _ ToolCall) (*ToolResult, error) {
	return nil, apperrors.ErrNotImplemented
}
