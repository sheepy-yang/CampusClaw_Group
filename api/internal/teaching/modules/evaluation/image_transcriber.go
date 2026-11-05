package evaluation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/gateway"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/objectstore"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type GatewayImageTranscriber struct {
	Objects objectstore.Store
	Model   gateway.Client
}

func (t *GatewayImageTranscriber) Transcribe(
	ctx context.Context,
	classID, studentID, imageDigest string,
) (TranscriptionResult, error) {
	if t == nil || t.Objects == nil || t.Model == nil {
		return TranscriptionResult{}, errors.New("image transcription dependencies are unavailable")
	}
	attachment, ok := store.Default.GetAttachment(imageDigest, classID, studentID)
	if !ok {
		return TranscriptionResult{}, ErrNotFound
	}
	data, err := t.Objects.Get(ctx, attachment.ObjectKey, objectstore.MaxAttachmentBytes)
	if err != nil {
		return TranscriptionResult{}, err
	}
	imageURL := "data:" + attachment.ContentType + ";base64," + base64.StdEncoding.EncodeToString(data)
	response, err := t.Model.Chat(ctx, gateway.ChatRequest{
		Messages: []gateway.ChatMessage{
			{
				Role: "system",
				Content: "你是手写作答转写器。忠实转写图片中的作答，不补全、不批改。仅输出 JSON：" +
					`{"text":"逐字转写结果","confidence":0到1之间的小数}。无法辨认的片段用[无法辨认]标注。`,
			},
			{Role: "user", Content: "请转写这张作答图片。"},
		},
		MultimodalURLs: []string{imageURL},
	})
	if err != nil {
		return TranscriptionResult{}, err
	}
	if strings.HasPrefix(response.Model, "offline-demo") {
		return TranscriptionResult{}, errors.New("configured model does not support image transcription")
	}
	text, _ := response.RawJSON["text"].(string)
	confidence, ok := numberValue(response.RawJSON["confidence"])
	if strings.TrimSpace(text) == "" || !ok || confidence <= 0 || confidence > 1 {
		return TranscriptionResult{}, errors.New("image model returned an invalid transcription")
	}
	return TranscriptionResult{Text: strings.TrimSpace(text), Confidence: confidence}, nil
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	case float32:
		result := float64(number)
		return result, !math.IsNaN(result) && !math.IsInf(result, 0)
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}

func NewGatewayImageTranscriberFromEnv() (*GatewayImageTranscriber, error) {
	objects, err := objectstore.NewFromEnv()
	if err != nil {
		return nil, fmt.Errorf("object store: %w", err)
	}
	return &GatewayImageTranscriber{Objects: objects, Model: gateway.NewClient()}, nil
}

var _ ImageTranscriber = (*GatewayImageTranscriber)(nil)
