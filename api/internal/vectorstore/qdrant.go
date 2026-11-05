package vectorstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var collectionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Client interface {
	Upsert(ctx context.Context, classID string, chunks []store.Chunk, vectors [][]float32) error
	Search(ctx context.Context, classID string, vector []float32, limit int) ([]store.Chunk, error)
}

type Qdrant struct {
	baseURL    string
	collection string
	client     *http.Client
	mu         sync.Mutex
	dimension  int
}

func NewFromEnv() Client {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("QDRANT_URL")), "/")
	collection := strings.TrimSpace(os.Getenv("QDRANT_COLLECTION"))
	if baseURL == "" || !collectionPattern.MatchString(collection) {
		return nil
	}
	return &Qdrant{
		baseURL: baseURL, collection: collection,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (q *Qdrant) Upsert(ctx context.Context, classID string, chunks []store.Chunk, vectors [][]float32) error {
	if len(chunks) == 0 || len(chunks) != len(vectors) || len(vectors[0]) == 0 {
		return fmt.Errorf("invalid qdrant upsert batch")
	}
	dimension := len(vectors[0])
	for _, vector := range vectors {
		if len(vector) != dimension {
			return fmt.Errorf("inconsistent vector dimensions")
		}
	}
	if err := q.ensureCollection(ctx, dimension); err != nil {
		return err
	}
	points := make([]map[string]any, 0, len(chunks))
	for i, chunk := range chunks {
		points = append(points, map[string]any{
			"id":     pointID(chunk.ID),
			"vector": vectors[i],
			"payload": map[string]any{
				"class_id": classID, "chunk_id": chunk.ID, "material_id": chunk.MaterialID,
				"material_version": chunk.MaterialVersion, "chunk_index": chunk.Index,
				"character_start": chunk.CharacterStart, "character_end": chunk.CharacterEnd,
				"content_hash": chunk.ContentHash, "text": chunk.Text, "source": chunk.Source,
			},
		})
	}
	return q.request(ctx, http.MethodPut, "/collections/"+url.PathEscape(q.collection)+"/points?wait=true", map[string]any{"points": points}, nil, http.StatusOK)
}

func (q *Qdrant) Search(ctx context.Context, classID string, vector []float32, limit int) ([]store.Chunk, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	var response struct {
		Result []struct {
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	body := map[string]any{
		"vector": vector, "limit": limit, "with_payload": true,
		"filter": map[string]any{"must": []map[string]any{{
			"key": "class_id", "match": map[string]any{"value": classID},
		}}},
	}
	if err := q.request(ctx, http.MethodPost, "/collections/"+url.PathEscape(q.collection)+"/points/search", body, &response, http.StatusOK); err != nil {
		return nil, err
	}
	hits := make([]store.Chunk, 0, len(response.Result))
	for _, point := range response.Result {
		payload := point.Payload
		if fmt.Sprint(payload["class_id"]) != classID {
			return nil, fmt.Errorf("qdrant returned a cross-class payload")
		}
		hits = append(hits, store.Chunk{
			ID: fmt.Sprint(payload["chunk_id"]), MaterialID: fmt.Sprint(payload["material_id"]),
			MaterialVersion: fmt.Sprint(payload["material_version"]), Index: intNumber(payload["chunk_index"]),
			CharacterStart: intNumber(payload["character_start"]), CharacterEnd: intNumber(payload["character_end"]),
			ContentHash: fmt.Sprint(payload["content_hash"]), Text: fmt.Sprint(payload["text"]),
			Source: fmt.Sprint(payload["source"]), VectorScore: point.Score, FusionScore: point.Score,
		})
	}
	return hits, nil
}

func (q *Qdrant) ensureCollection(ctx context.Context, dimension int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.dimension == dimension {
		return nil
	}
	status, err := q.requestStatus(ctx, http.MethodGet, "/collections/"+url.PathEscape(q.collection), nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		body := map[string]any{"vectors": map[string]any{"size": dimension, "distance": "Cosine"}}
		if err := q.request(ctx, http.MethodPut, "/collections/"+url.PathEscape(q.collection), body, nil, http.StatusOK); err != nil {
			return err
		}
	} else if status != http.StatusOK {
		return fmt.Errorf("qdrant collection check returned HTTP %d", status)
	}
	q.dimension = dimension
	return nil
}

func (q *Qdrant) request(ctx context.Context, method, path string, body any, dst any, expected int) error {
	status, err := q.requestStatus(ctx, method, path, body, dst)
	if err != nil {
		return err
	}
	if status != expected {
		return fmt.Errorf("qdrant returned HTTP %d", status)
	}
	return nil
}

func (q *Qdrant) requestStatus(ctx context.Context, method, path string, body any, dst any) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := q.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("qdrant unavailable: %w", err)
	}
	defer response.Body.Close()
	if dst != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		if err := json.NewDecoder(io.LimitReader(response.Body, 20<<20)).Decode(dst); err != nil {
			return response.StatusCode, err
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	}
	return response.StatusCode, nil
}

func pointID(value string) string {
	sum := sha256.Sum256([]byte(value))
	raw := hex.EncodeToString(sum[:16])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func intNumber(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}
