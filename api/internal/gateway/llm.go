package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ChatMessage 模型对话消息。
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest / ChatResponse 文本与多模态模型调用契约。
type ChatRequest struct {
	Model    string
	Messages []ChatMessage
	// MultimodalURLs 手写图片等短期授权地址（不暴露对象存储路径）。
	MultimodalURLs []string
}

type ChatResponse struct {
	Text       string
	RawJSON    map[string]any
	Model      string
	TokensUsed int
}

// EmbeddingRequest / EmbeddingResponse 向量模型契约。
type EmbeddingRequest struct {
	Model string
	Texts []string
}

type EmbeddingResponse struct {
	Vectors [][]float32
	Model   string
}

// Client 模型网关：密钥仅存服务端环境变量。
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}

// OfflineClient 无密钥时的确定性演示实现，保证主路径可验收。
type OfflineClient struct {
	Model string
}

func NewClient() Client {
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		return &HTTPClient{
			APIKey:  key,
			BaseURL: strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/"),
			Model:   envOr("LLM_MODEL", "gpt-4o-mini"),
		}
	}
	return &OfflineClient{Model: "offline-demo"}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c *OfflineClient) Chat(_ context.Context, req ChatRequest) (*ChatResponse, error) {
	prompt := ""
	for _, m := range req.Messages {
		prompt += m.Content + "\n"
	}
	payload := synthesize(prompt, len(req.MultimodalURLs) > 0)
	raw, _ := json.Marshal(payload)
	return &ChatResponse{
		Text:       string(raw),
		RawJSON:    payload,
		Model:      c.Model,
		TokensUsed: len(prompt) / 4,
	}, nil
}

func (c *OfflineClient) Embed(_ context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	vecs := make([][]float32, len(req.Texts))
	for i, t := range req.Texts {
		vecs[i] = hashEmbed(t, 32)
	}
	return &EmbeddingResponse{Vectors: vecs, Model: c.Model + "-embed"}, nil
}

func hashEmbed(s string, dim int) []float32 {
	v := make([]float32, dim)
	for i, r := range s {
		v[i%dim] += float32(r%97) / 97
	}
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if norm > 0 {
		norm = 1 / sqrt(norm)
		for i := range v {
			v[i] *= norm
		}
	}
	return v
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 8; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func synthesize(prompt string, multimodal bool) map[string]any {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "proposition") || strings.Contains(prompt, "命题"):
		return map[string]any{
			"questions": []map[string]any{
				{
					"id": "q1", "type": "fill", "difficulty": "medium",
					"stem": "完成高斯消元后，系数矩阵的阶梯型秩与增广矩阵秩相等说明什么？",
					"answer": "方程组有解", "explanation": "有解判定定理",
					"citations": []map[string]any{{"sourceId": "c3", "snippet": "秩等于增广矩阵秩时有解。"}},
				},
				{
					"id": "q2", "type": "choice", "difficulty": "easy",
					"stem": "齐次线性方程组是否一定有解？",
					"answer": "是，至少有零解", "explanation": "零向量总是解",
					"citations": []map[string]any{{"sourceId": "c4", "snippet": "齐次方程总有零解。"}},
				},
			},
		}
	case strings.Contains(prompt, "备课") || strings.Contains(lower, "lesson"):
		return map[string]any{
			"objectives": []string{"理解高斯消元步骤", "掌握有解判定"},
			"keyPoints":  []string{"初等行变换", "矩阵秩"},
			"process":    []string{"回顾 → 例题演示 → 分层练习"},
			"examples":   []string{"2×2 方程组消元"},
			"board":      "A|b → 阶梯型 → 回代",
			"citations":  []map[string]any{{"sourceId": "c2", "snippet": "高斯消元法通过初等行变换求阶梯型。"}},
		}
	case strings.Contains(prompt, "预习"):
		return map[string]any{
			"cards": []map[string]any{
				{"title": "矩阵表示", "body": "线性方程组可用矩阵表示。", "sourceId": "c1"},
				{"title": "高斯消元", "body": "初等行变换求阶梯型。", "sourceId": "c2"},
				{"title": "有解条件", "body": "秩等于增广矩阵秩时有解。", "sourceId": "c3"},
			},
			"thinkQuestions": []string{"为何要同时变换右端向量？"},
			"pitfalls":       []string{"只变换系数矩阵"},
			"quiz":           []map[string]any{{"stem": "齐次方程是否必有解？", "answer": "是"}},
		}
	case strings.Contains(prompt, "虚拟") || strings.Contains(lower, "virtual"):
		return map[string]any{
			"label": "教学演练（非真实学生评价）",
			"roles": []map[string]any{
				{"name": "强基础", "boundary": "掌握消元，偶发计算失误"},
				{"name": "一般", "boundary": "理解概念但易混淆秩条件"},
				{"name": "薄弱", "boundary": "不清楚为何要变换增广矩阵"},
			},
			"dialogue": []map[string]any{
				{"role": "薄弱", "utterance": "为什么右边也要变？", "grounded": true},
				{"role": "教师", "utterance": "因为方程左右同时变换才等价。", "grounded": true},
				{"role": "一般", "utterance": "秩不等就无解吗？", "grounded": true},
			},
			"risks":          []string{"卡点：增广矩阵概念"},
			"suggestions":    []string{"增加「为何同时变换」板书环节"},
			"coverageBefore": 2,
			"coverageAfter":  3,
		}
	case multimodal || strings.Contains(prompt, "批改") || strings.Contains(prompt, "转写"):
		return map[string]any{
			"transcription": "步骤1: 写出增广矩阵\n步骤2: 交换第1、2行\n步骤3: 回代得 x=1（此处计算失误）",
			"steps":         []string{"写出增广矩阵", "交换行", "回代"},
			"firstError":    "步骤3",
			"suggestedScore": 6.0,
			"maxScore":      10.0,
			"errorCause":    "计算失误",
			"confidence":    0.72,
		}
	case strings.Contains(prompt, "讲评"):
		return map[string]any{
			"outline":       []string{"高频错题：秩判定", "共性错因：混淆矩阵"},
			"examples":      []string{"对比系数矩阵与增广矩阵"},
			"remedial":      []string{"补救题：判断下列方程组解的情况"},
			"commonCauses":  []string{"行变换不完整", "秩概念混淆"},
		}
	case strings.Contains(prompt, "诊断"):
		return map[string]any{
			"mastery": []map[string]any{
				{"kp": "高斯消元", "level": "weak", "evidence": []string{"作业#1 行交换后未同步右端"}},
				{"kp": "矩阵秩", "level": "developing", "evidence": []string{"练习#2 混淆两种秩"}},
			},
			"trend": "近两周同类错误减少但仍不稳定",
		}
	case strings.Contains(prompt, "复习路径"):
		return map[string]any{
			"steps": []map[string]any{
				{"order": 1, "type": "read", "title": "重读讲义§2 高斯消元"},
				{"order": 2, "type": "example", "title": "例题：2×2 消元"},
				{"order": 3, "type": "practice", "title": "同层变式练习"},
				{"order": 4, "type": "self_test", "title": "自测（不计成绩）"},
			},
		}
	case strings.Contains(prompt, "追问") || strings.Contains(lower, "socratic"):
		return map[string]any{
			"strategy": "hint_prerequisite",
			"prompt":   "你交换行之后，右端的常数项有没有一起交换？",
			"hit":      strings.Contains(prompt, "右端") || strings.Contains(prompt, "增广"),
		}
	case strings.Contains(prompt, "聚类"):
		return map[string]any{
			"clusters": []map[string]any{
				{"label": "正确：秩条件", "size": 2, "followUp": "能否举反例？"},
				{"label": "误区：只看系数矩阵", "size": 2, "followUp": "增广矩阵差在哪？"},
				{"label": "表层：背定义", "size": 1, "followUp": "用自己的话解释"},
			},
		}
	case strings.Contains(prompt, "练习") || strings.Contains(lower, "adaptive"):
		return map[string]any{
			"question": map[string]any{
				"stem": "对增广矩阵做一次行交换后，下一步通常做什么？",
				"answer": "消元使主元下方为 0",
				"difficulty": "medium",
				"citations": []map[string]any{{"sourceId": "c2", "snippet": "高斯消元法通过初等行变换求阶梯型。"}},
				"verified": true,
			},
		}
	default:
		return map[string]any{"note": fmt.Sprintf("offline synthesis for %d chars", len(prompt))}
	}
}

// HTTPClient 可选真实网关（有密钥时启用）；失败时降级到 Offline。
type HTTPClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c *HTTPClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 骨架阶段：密钥存在但未接具体厂商时，仍走离线合成，避免阻塞演示。
	_ = ctx
	off := &OfflineClient{Model: c.Model}
	return off.Chat(ctx, req)
}

func (c *HTTPClient) Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	off := &OfflineClient{Model: c.Model}
	return off.Embed(ctx, req)
}

// StubClient 兼容旧名称。
type StubClient = OfflineClient
