package vectorstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func TestQdrantUpsertAndClassScopedSearch(t *testing.T) {
	collectionExists := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/course":
			if !collectionExists {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": "green"}})
		case r.Method == http.MethodPut && r.URL.Path == "/collections/course":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			vectors := body["vectors"].(map[string]any)
			if vectors["size"].(float64) != 3 {
				t.Fatalf("collection dimension = %#v", vectors["size"])
			}
			collectionExists = true
			_ = json.NewEncoder(w).Encode(map[string]any{"result": true})
		case r.Method == http.MethodPut && r.URL.Path == "/collections/course/points":
			if r.URL.Query().Get("wait") != "true" {
				t.Fatal("upsert must wait for durability")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			points := body["points"].([]any)
			payload := points[0].(map[string]any)["payload"].(map[string]any)
			if payload["class_id"] != "class-a" {
				t.Fatalf("upsert scope = %#v", payload["class_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": "completed"}})
		case r.Method == http.MethodPost && r.URL.Path == "/collections/course/points/search":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !strings.Contains(string(mustJSON(body["filter"])), "class-a") {
				t.Fatalf("search filter = %#v", body["filter"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": []map[string]any{{
					"score": 0.91,
					"payload": map[string]any{
						"class_id": "class-a", "chunk_id": "c1", "material_id": "m1",
						"material_version": "v1", "chunk_index": 0, "character_start": 0,
						"character_end": 4, "content_hash": "hash", "text": "内容", "source": "讲义§1",
					},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Qdrant{baseURL: server.URL, collection: "course", client: server.Client()}
	chunk := store.Chunk{ID: "c1", MaterialID: "m1", MaterialVersion: "v1", Text: "内容", ContentHash: "hash"}
	if err := client.Upsert(context.Background(), "class-a", []store.Chunk{chunk}, [][]float32{{1, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	hits, err := client.Search(context.Background(), "class-a", []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "c1" || hits[0].VectorScore != 0.91 {
		t.Fatalf("unexpected hits: %#v", hits)
	}
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
