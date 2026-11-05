package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClientUsesOpenAICompatibleChatContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "course-chat" {
			t.Fatalf("model = %#v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "course-chat",
			"choices": []map[string]any{{"message": map[string]any{"content": "```json\n{\"answer\":\"有解\"}\n```"}}},
			"usage":   map[string]any{"total_tokens": 17},
		})
	}))
	defer server.Close()

	client := &HTTPClient{APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "course-chat", Client: server.Client()}
	response, err := client.Chat(context.Background(), ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "问题"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.RawJSON["answer"] != "有解" || response.TokensUsed != 17 {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestHTTPClientEmbeddingContractAndDimension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, included := body["dimensions"]; included {
			t.Fatal("request must not send dimensions")
		}
		input, _ := body["input"].([]any)
		if len(input) != 2 {
			t.Fatalf("input = %#v", body["input"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "course-embedding",
			"data": []map[string]any{
				{"index": 1, "embedding": []float64{0, 1, 0}},
				{"index": 0, "embedding": []float64{1, 0, 0}},
			},
		})
	}))
	defer server.Close()

	client := &HTTPClient{
		EmbeddingAPIKey: "embed-key", EmbeddingBaseURL: server.URL + "/v1",
		EmbeddingModel: "course-embedding", EmbeddingDim: 3, Client: server.Client(),
	}
	response, err := client.Embed(context.Background(), EmbeddingRequest{Texts: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Vectors) != 2 || response.Vectors[0][0] != 1 || response.Vectors[1][1] != 1 {
		t.Fatalf("vectors were not returned in index order: %#v", response.Vectors)
	}

	client.EmbeddingDim = 2048
	if _, err := client.Embed(context.Background(), EmbeddingRequest{Texts: []string{"a", "b"}}); err == nil {
		t.Fatal("expected dimension mismatch")
	}
}

func TestGatewayErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"secret provider detail"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	client := &HTTPClient{APIKey: "key", BaseURL: server.URL, Model: "course-chat", Client: server.Client()}
	_, err := client.Chat(context.Background(), ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err == nil || strings.Contains(err.Error(), "secret provider detail") {
		t.Fatalf("unsafe error = %v", err)
	}
}

func TestNewClientOnlyAllowsOfflineFallbackInDemoEnvironments(t *testing.T) {
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "EMBEDDING_API_KEY", "EMBEDDING_BASE_URL"} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "test")
	if _, ok := NewClient().(*OfflineClient); !ok {
		t.Fatal("test environment should use the deterministic offline client")
	}
	t.Setenv("APP_ENV", "production")
	client := NewClient()
	if _, ok := client.(*unavailableClient); !ok {
		t.Fatalf("production client = %T, want unavailableClient", client)
	}
	if _, err := client.Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("production without model configuration must fail explicitly")
	}
}
