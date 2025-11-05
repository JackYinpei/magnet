package storage

import (
	"context"
	"time"
)

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified *time.Time
}

// UploadOptions conveys upload destination metadata.
type UploadOptions struct {
	Bucket           string
	KeyPrefix        string
	ProgressCallback func(done, total int64)
}

// Service uploads completed downloads to remote object storage.
type Service interface {
	UploadDirectory(ctx context.Context, localPath string, opts UploadOptions) (string, error)
	ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)
	DeletePrefix(ctx context.Context, bucket, prefix string) error
	GetObjectURL(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
}
