package auth

import (
	"context"
	"strings"
)

// Role 服务端认定的角色，不采信客户端提交值。
type Role string

const (
	RoleTeacher Role = "teacher"
	RoleStudent Role = "student"
)

// Principal 从 Token 解析出的认证上下文。
type Principal struct {
	UserID  string `json:"userId"`
	ClassID string `json:"classId"`
	Role    Role   `json:"role"`
	Name    string `json:"name"`
}

type ctxKey struct{}

// FromContext 读取服务端认证上下文。
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// WithPrincipal 注入认证上下文。
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// 演示账号表：Token → Principal。班级与角色仅来自服务端。
var demoAccounts = map[string]Principal{
	"teacher-demo":   {UserID: "tea-1", ClassID: "class-demo", Role: RoleTeacher, Name: "张老师"},
	"student-demo":   {UserID: "stu-1", ClassID: "class-demo", Role: RoleStudent, Name: "李同学"},
	"student-demo-2": {UserID: "stu-2", ClassID: "class-demo", Role: RoleStudent, Name: "王同学"},
}

// 演示登录：账号/密码 → Token（仅用于本地与验收测试）。
var demoCredentials = map[string]struct {
	Password string
	Token    string
}{
	"teacher":  {Password: "demo123", Token: "teacher-demo"},
	"student":  {Password: "demo123", Token: "student-demo"},
	"student2": {Password: "demo123", Token: "student-demo-2"},
}

// Authenticate 校验 Bearer Token 并返回 Principal。
func Authenticate(_ context.Context, token string) (Principal, error) {
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if token == "" {
		return Principal{}, ErrUnauthorized
	}
	p, ok := demoAccounts[token]
	if !ok {
		return Principal{}, ErrUnauthorized
	}
	return p, nil
}

// LoginWithPassword 校验演示账号密码并返回 Token。
func LoginWithPassword(username, password string) (token string, p Principal, err error) {
	username = strings.TrimSpace(username)
	cred, ok := demoCredentials[username]
	if !ok || cred.Password != password {
		return "", Principal{}, ErrUnauthorized
	}
	p = demoAccounts[cred.Token]
	return cred.Token, p, nil
}

// Login 按角色发放演示 Token（兼容一键角色登录）。
func Login(role Role) (token string, p Principal, err error) {
	switch role {
	case RoleTeacher:
		return "teacher-demo", demoAccounts["teacher-demo"], nil
	case RoleStudent:
		return "student-demo", demoAccounts["student-demo"], nil
	default:
		return "", Principal{}, ErrUnauthorized
	}
}

// RequireTeacher 学生调用教师专属能力时返回 403。
func RequireTeacher(p Principal) error {
	if p.Role != RoleTeacher {
		return ErrForbidden
	}
	return nil
}

// SameClass 校验资源班级归属；不一致统一表现为 not found。
func SameClass(p Principal, classID string) error {
	if p.ClassID != classID {
		return ErrNotFound
	}
	return nil
}
