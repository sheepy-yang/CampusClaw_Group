package auth

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
)

// Role 服务端认定的角色，不采信客户端提交值。
type Role string

const (
	RoleTeacher Role = "teacher"
	RoleStudent Role = "student"
)

// Principal 从 Token 解析出的认证上下文。
type Principal struct {
	UserID  string
	ClassID string
	Role    Role
}

type ctxKey struct{}

// FromContext 读取服务端认证上下文。
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// WithPrincipal 注入认证上下文（骨架：中间件待实现）。
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// Authenticate 校验 Token 并返回 Principal。
// The authentication change will provide the adapter; class and role must come from the server.
func Authenticate(_ context.Context, _ string) (Principal, error) {
	return Principal{}, apperrors.ErrNotImplemented
}

// RequireTeacher 学生调用教师专属能力时返回 403。
func RequireTeacher(p Principal) error {
	if p.Role != RoleTeacher {
		return ErrForbidden
	}
	return nil
}
