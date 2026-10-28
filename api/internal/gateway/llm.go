package gateway

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
)

type ContentPartType string

const (
	ContentText     ContentPartType = "text"
	ContentImageURL ContentPartType = "image_url"
)

type ContentPart struct {
	Type     ContentPartType
	Text     string
	ImageURL string
}

// ChatMessage supports text and image parts without exposing provider wire types.
type ChatMessage struct {
	Role    string
	Content []ContentPart
}

// ChatRequest / ChatResponse are provider-neutral model contracts.
type ChatRequest struct {
	Model    string
	Messages []ChatMessage
	TraceID  string
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

type StreamEventType string

const (
	StreamMeta  StreamEventType = "meta"
	StreamTool  StreamEventType = "tool"
	StreamDelta StreamEventType = "delta"
	StreamDone  StreamEventType = "done"
	StreamError StreamEventType = "error"
)

type StreamEvent struct {
	Type      StreamEventType
	Delta     string
	ToolName  string
	ErrorCode string
	Metadata  map[string]string
}

// Client 模型网关：密钥仅存服务端环境变量。
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}

// StubClient 骨架。
type StubClient struct{}

func (StubClient) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	return nil, apperrors.ErrNotImplemented
}
func (StubClient) StreamChat(_ context.Context, _ ChatRequest) (<-chan StreamEvent, error) {
	return nil, apperrors.ErrNotImplemented
}
func (StubClient) Embed(_ context.Context, _ EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, apperrors.ErrNotImplemented
}
