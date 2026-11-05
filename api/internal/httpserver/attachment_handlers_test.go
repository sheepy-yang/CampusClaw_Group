package httpserver

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type fakeAttachmentStore struct {
	objects map[string][]byte
}

func (s *fakeAttachmentStore) EnsureBucket(context.Context) error { return nil }
func (s *fakeAttachmentStore) Put(_ context.Context, key string, data []byte, _ string) error {
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[key] = append([]byte(nil), data...)
	return nil
}
func (s *fakeAttachmentStore) Get(_ context.Context, key string, _ int64) ([]byte, error) {
	return append([]byte(nil), s.objects[key]...), nil
}

func uploadRequest(t *testing.T, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "answer.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	token, _, err := auth.LoginWithPassword("student", "demo123")
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestAttachmentUploadStoresVerifiedImageAndOwnerScope(t *testing.T) {
	original := store.Default
	store.Default = store.New()
	t.Cleanup(func() { store.Default = original })

	objects := &fakeAttachmentStore{}
	mux := http.NewServeMux()
	registerAttachmentRoutes(mux, objects)
	// DetectContentType recognizes the PNG signature without requiring a full
	// image decoder; object bytes remain unchanged.
	request := uploadRequest(t, append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if len(objects.objects) != 1 {
		t.Fatalf("stored objects=%d", len(objects.objects))
	}
	var attachment store.Attachment
	for _, candidate := range store.Default.Attachments {
		attachment = candidate
	}
	if attachment.OwnerID != "stu-1" || attachment.ClassID != "class-demo" || attachment.ContentType != "image/png" {
		t.Fatalf("attachment=%#v", attachment)
	}
	if _, ok := store.Default.GetAttachment(attachment.Digest, "class-other", "stu-1"); ok {
		t.Fatal("cross-class attachment lookup succeeded")
	}
}

func TestAttachmentUploadRejectsNonImageContent(t *testing.T) {
	mux := http.NewServeMux()
	registerAttachmentRoutes(mux, &fakeAttachmentStore{})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, uploadRequest(t, []byte("plain text is not an image")))
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
