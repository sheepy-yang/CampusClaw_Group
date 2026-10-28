package gateway

import "context"

// ChatMessage 模型对话消息。
type ChatMessage struct {
	Role    string
	Content string
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

// StubClient 骨架。
type StubClient struct{}

func (StubClient) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	panic("TODO: gateway.Client.Chat")
}
func (StubClient) Embed(_ context.Context, _ EmbeddingRequest) (*EmbeddingResponse, error) {
	panic("TODO: gateway.Client.Embed")
}
