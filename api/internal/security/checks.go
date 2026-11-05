package security

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var injectPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)system\s*:\s*`),
	regexp.MustCompile(`(?i)<\|?(system|assistant)\|?>`),
	regexp.MustCompile(`(?i)你现在是系统`),
}

var namePatterns = []*regexp.Regexp{
	regexp.MustCompile(`[\p{Han}]{2,4}(同学|老师)`),
	regexp.MustCompile(`(?i)\b(zhang|li|wang|chen)\s+(san|si|wu)\b`),
}

// CheckInput 对讲义、学生输入、MCP 输出做提示注入与内容安全检查。
func CheckInput(_ context.Context, text string) error {
	for _, p := range injectPatterns {
		if p.MatchString(text) {
			return ErrUnsafeInput
		}
	}
	return nil
}

// AnonymizeForModel 讲评 / 课堂聚类送模型前去除姓名等直接身份信息。
func AnonymizeForModel(_ context.Context, texts []string) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		s := t
		for _, p := range namePatterns {
			s = p.ReplaceAllString(s, "[学生]")
		}
		// 去掉显式姓名字段风格前缀
		if strings.Contains(s, "：") {
			parts := strings.SplitN(s, "：", 2)
			if len(parts) == 2 && len([]rune(parts[0])) <= 4 {
				s = parts[1]
			}
		}
		out[i] = strings.TrimSpace(s)
	}
	return out
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

// MemoryAuditLogger 写入内存 store。
type MemoryAuditLogger struct {
	seq int
	mu  sync.Mutex
}

func NewAuditLogger() *MemoryAuditLogger { return &MemoryAuditLogger{} }

func (a *MemoryAuditLogger) Log(_ context.Context, ev AuditEvent) error {
	a.mu.Lock()
	a.seq++
	id := time.Now().Format("20060102150405") + "-" + itoa(a.seq)
	a.mu.Unlock()
	store.Default.AppendAudit(store.AuditEvent{
		ID: id, ActorID: ev.ActorID, ClassID: ev.ClassID,
		Action: ev.Action, Detail: ev.Detail, CreatedAt: time.Now(),
	})
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// StubAuditLogger 兼容旧名称。
type StubAuditLogger = MemoryAuditLogger
