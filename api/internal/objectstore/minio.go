package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const MaxAttachmentBytes int64 = 10 << 20

type Store interface {
	EnsureBucket(context.Context) error
	Put(context.Context, string, []byte, string) error
	Get(context.Context, string, int64) ([]byte, error)
}

type MinIOStore struct {
	client *minio.Client
	bucket string
}

func NewFromEnv() (*MinIOStore, error) {
	rawEndpoint := strings.TrimSpace(os.Getenv("MINIO_ENDPOINT"))
	if rawEndpoint == "" {
		rawEndpoint = strings.TrimSpace(os.Getenv("MINIO_URL"))
	}
	if rawEndpoint == "" {
		return nil, errors.New("MINIO_ENDPOINT or MINIO_URL is required")
	}
	secure := false
	if parsed, err := url.Parse(rawEndpoint); err == nil && parsed.Host != "" {
		secure = parsed.Scheme == "https"
		rawEndpoint = parsed.Host
	}
	accessKey := firstNonEmpty(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_ROOT_USER"))
	secretKey := firstNonEmpty(os.Getenv("MINIO_SECRET_KEY"), os.Getenv("MINIO_ROOT_PASSWORD"))
	bucket := strings.TrimSpace(os.Getenv("MINIO_BUCKET"))
	if accessKey == "" || secretKey == "" || bucket == "" {
		return nil, errors.New("MinIO credentials and MINIO_BUCKET are required")
	}
	client, err := minio.New(rawEndpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}
	return &MinIOStore{client: client, bucket: bucket}, nil
}

func (s *MinIOStore) EnsureBucket(ctx context.Context) error {
	if s == nil || s.client == nil || s.bucket == "" {
		return errors.New("object store is not configured")
	}
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check attachment bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create attachment bucket: %w", err)
	}
	return nil
}

func (s *MinIOStore) Put(ctx context.Context, objectKey string, data []byte, contentType string) error {
	if s == nil || s.client == nil || strings.TrimSpace(objectKey) == "" {
		return errors.New("object store is not configured")
	}
	_, err := s.client.PutObject(
		ctx, s.bucket, objectKey, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return fmt.Errorf("upload attachment: %w", err)
	}
	return nil
}

func (s *MinIOStore) Get(ctx context.Context, objectKey string, maxBytes int64) ([]byte, error) {
	if s == nil || s.client == nil || strings.TrimSpace(objectKey) == "" {
		return nil, errors.New("object store is not configured")
	}
	if maxBytes <= 0 {
		maxBytes = MaxAttachmentBytes
	}
	object, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("open attachment: %w", err)
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read attachment: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("attachment exceeds size limit")
	}
	return data, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

var _ Store = (*MinIOStore)(nil)
