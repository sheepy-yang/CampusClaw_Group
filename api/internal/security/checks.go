package security

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
)

// CheckInput 对讲义、学生输入、MCP 输出做提示注入与内容安全检查。
func CheckInput(_ context.Context, _ string) error {
	return apperrors.ErrNotImplemented
}

// AnonymizeForModel 讲评 / 课堂聚类送模型前去除姓名等直接身份信息。
func AnonymizeForModel(_ context.Context, _ []string) ([]string, error) {
	return nil, apperrors.ErrNotImplemented
}

// AuditEvent 页面 / 接口 / 异步任务 / 工具调用审计。
type AuditEvent struct {
	ActorID string
	ClassID string
	Action  string
	Detail  map[string]any
}

// AuditLogger 审计写入接口。
type AuditLogger interface {
	Log(ctx context.Context, ev AuditEvent) error
}

type StubAuditLogger struct{}

func (StubAuditLogger) Log(_ context.Context, _ AuditEvent) error {
	return apperrors.ErrNotImplemented
}
