package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"path"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/objectstore"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func registerAttachmentRoutes(mux *http.ServeMux, objects objectstore.Store) {
	mux.HandleFunc("POST /api/v1/attachments", func(w http.ResponseWriter, r *http.Request) {
		handleAttachmentUpload(w, r, objects)
	})
}

func handleAttachmentUpload(w http.ResponseWriter, r *http.Request, objects objectstore.Store) {
	principal, err := auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, objectstore.MaxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_MULTIPART", "a multipart file upload is required")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if len(r.MultipartForm.Value) != 0 || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_MULTIPART", "exactly one file field is required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_MULTIPART", "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, objectstore.MaxAttachmentBytes+1))
	if err != nil || len(data) == 0 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "INVALID_FILE", "file cannot be read or is empty")
		return
	}
	if int64(len(data)) > objectstore.MaxAttachmentBytes {
		httpapi.WriteError(w, r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds 10 MiB")
		return
	}
	contentType := http.DetectContentType(data)
	extension := ""
	switch contentType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	default:
		httpapi.WriteError(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE_TYPE", "only JPEG, PNG and WebP images are accepted")
		return
	}
	sum := sha256.Sum256(data)
	hexDigest := hex.EncodeToString(sum[:])
	digest := "sha256:" + hexDigest
	objectKey := path.Join("attachments", hexDigest[:2], hexDigest+extension)
	if err := objects.EnsureBucket(r.Context()); err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "OBJECT_STORE_UNAVAILABLE", "attachment bucket is unavailable")
		return
	}
	if err := objects.Put(r.Context(), objectKey, data, contentType); err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "OBJECT_STORE_UNAVAILABLE", "attachment could not be stored")
		return
	}
	attachment := store.Attachment{
		Digest: digest, ObjectKey: objectKey, ClassID: principal.ClassID, OwnerID: principal.UserID,
		ContentType: contentType, Size: int64(len(data)), CreatedAt: time.Now().UTC(),
	}
	if err := store.Default.SaveAttachment(attachment); err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "attachment metadata could not be stored")
		return
	}
	if err := store.Default.AppendAudit(store.AuditEvent{
		ID: store.NewID("audit"), ActorID: principal.UserID, ClassID: principal.ClassID,
		Action: "attachment.upload", Object: digest, Result: "succeeded",
		RequestID: httpapi.RequestID(r.Context()), CreatedAt: time.Now().UTC(),
		Detail: map[string]any{"contentType": contentType, "size": len(data)},
	}); err != nil {
		log.Printf("attachment audit persistence failed object=%s request_id=%s: %v", digest, httpapi.RequestID(r.Context()), err)
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{
		"digest": digest, "contentType": contentType, "size": len(data),
	})
}
