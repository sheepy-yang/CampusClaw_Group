package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
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

type demoCredential struct {
	Password  string
	Principal Principal
}

type configuredCredential struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	UserID       string `json:"userId"`
	ClassID      string `json:"classId"`
	Role         Role   `json:"role"`
	Name         string `json:"name"`
}

// 演示账号仅用于本地验收；登录后仍签发随机、可撤销且有过期时间的会话令牌。
var demoCredentials = map[string]demoCredential{
	"teacher":       {Password: "demo123", Principal: Principal{UserID: "tea-1", ClassID: "class-demo", Role: RoleTeacher, Name: "张老师"}},
	"student":       {Password: "demo123", Principal: Principal{UserID: "stu-1", ClassID: "class-demo", Role: RoleStudent, Name: "李同学"}},
	"student2":      {Password: "demo123", Principal: Principal{UserID: "stu-2", ClassID: "class-demo", Role: RoleStudent, Name: "王同学"}},
	"student3":      {Password: "demo123", Principal: Principal{UserID: "stu-3", ClassID: "class-demo", Role: RoleStudent, Name: "赵同学"}},
	"student4":      {Password: "demo123", Principal: Principal{UserID: "stu-4", ClassID: "class-demo", Role: RoleStudent, Name: "钱同学"}},
	"student5":      {Password: "demo123", Principal: Principal{UserID: "stu-5", ClassID: "class-demo", Role: RoleStudent, Name: "孙同学"}},
	"teacher_other": {Password: "demo123", Principal: Principal{UserID: "tea-2", ClassID: "class-other", Role: RoleTeacher, Name: "其他班教师"}},
	"student_other": {Password: "demo123", Principal: Principal{UserID: "stu-other", ClassID: "class-other", Role: RoleStudent, Name: "其他班学生"}},
}

type session struct {
	Principal Principal
	ExpiresAt time.Time
}

var sessions = struct {
	sync.RWMutex
	values map[string]session
}{values: map[string]session{}}

const sessionTTL = 12 * time.Hour

var processSessionSecret = func() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		sum := sha256.Sum256([]byte(time.Now().UTC().String()))
		return sum[:]
	}
	return secret
}()

var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("invalid-password"), bcrypt.DefaultCost)

func sessionSecret() []byte {
	if configured := strings.TrimSpace(os.Getenv("SESSION_SECRET")); configured != "" {
		return []byte(configured)
	}
	return processSessionSecret
}

func signedToken(sessionID string) string {
	mac := hmac.New(sha256.New, sessionSecret())
	_, _ = mac.Write([]byte(sessionID))
	return sessionID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifiedSessionID(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, sessionSecret())
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", false
	}
	return parts[0], true
}

func demoLoginEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "", "local", "development", "dev", "test", "demo":
		return true
	default:
		return false
	}
}

// Authenticate 校验 Bearer Token、撤销状态和过期时间并返回 Principal。
func Authenticate(_ context.Context, header string) (Principal, error) {
	if !strings.HasPrefix(header, "Bearer ") {
		return Principal{}, ErrUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	sessionID, valid := verifiedSessionID(token)
	if !valid {
		return Principal{}, ErrUnauthorized
	}
	sessions.RLock()
	current, ok := sessions.values[sessionID]
	sessions.RUnlock()
	if !ok || time.Now().After(current.ExpiresAt) {
		if ok {
			sessions.Lock()
			delete(sessions.values, sessionID)
			sessions.Unlock()
		}
		return Principal{}, ErrUnauthorized
	}
	return current.Principal, nil
}

// LoginWithPassword 校验演示账号密码并返回 Token。
func LoginWithPassword(username, password string) (token string, p Principal, err error) {
	username = strings.TrimSpace(username)
	if demoLoginEnabled() {
		cred, ok := demoCredentials[username]
		if !ok || subtle.ConstantTimeCompare([]byte(cred.Password), []byte(password)) != 1 {
			return "", Principal{}, ErrUnauthorized
		}
		p = cred.Principal
	} else {
		credentials, parseErr := configuredCredentials()
		if parseErr != nil {
			return "", Principal{}, ErrUnauthorized
		}
		found := false
		passwordHash := dummyPasswordHash
		for _, credential := range credentials {
			if credential.Username != username {
				continue
			}
			found = true
			passwordHash = []byte(credential.PasswordHash)
			p = Principal{
				UserID: credential.UserID, ClassID: credential.ClassID,
				Role: credential.Role, Name: credential.Name,
			}
			break
		}
		if bcrypt.CompareHashAndPassword(passwordHash, []byte(password)) != nil || !found {
			return "", Principal{}, ErrUnauthorized
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Principal{}, ErrUnauthorized
	}
	sessionID := base64.RawURLEncoding.EncodeToString(raw)
	token = signedToken(sessionID)
	sessions.Lock()
	sessions.values[sessionID] = session{Principal: p, ExpiresAt: time.Now().Add(sessionTTL)}
	sessions.Unlock()
	return token, p, nil
}

func configuredCredentials() ([]configuredCredential, error) {
	var credentials []configuredCredential
	if err := json.Unmarshal([]byte(strings.TrimSpace(os.Getenv("AUTH_USERS_JSON"))), &credentials); err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, ErrUnauthorized
	}
	seen := map[string]bool{}
	for _, credential := range credentials {
		if strings.TrimSpace(credential.Username) == "" ||
			strings.TrimSpace(credential.PasswordHash) == "" ||
			strings.TrimSpace(credential.UserID) == "" ||
			strings.TrimSpace(credential.ClassID) == "" ||
			strings.TrimSpace(credential.Name) == "" ||
			(credential.Role != RoleTeacher && credential.Role != RoleStudent) ||
			seen[credential.Username] {
			return nil, ErrUnauthorized
		}
		if _, err := bcrypt.Cost([]byte(credential.PasswordHash)); err != nil {
			return nil, ErrUnauthorized
		}
		seen[credential.Username] = true
	}
	return credentials, nil
}

// Login 按角色发放演示 Token（兼容一键角色登录）。
func Login(role Role) (token string, p Principal, err error) {
	switch role {
	case RoleTeacher:
		return LoginWithPassword("teacher", "demo123")
	case RoleStudent:
		return LoginWithPassword("student", "demo123")
	default:
		return "", Principal{}, ErrUnauthorized
	}
}

func Logout(header string) {
	if !strings.HasPrefix(header, "Bearer ") {
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	sessionID, valid := verifiedSessionID(token)
	if !valid {
		return
	}
	sessions.Lock()
	delete(sessions.values, sessionID)
	sessions.Unlock()
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
