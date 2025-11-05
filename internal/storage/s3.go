package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Service uploads task data to Amazon S3 (or compatible APIs).
type S3Service struct {
	client   *s3.Client
	uploader *manager.Uploader
}

func NewS3Service(client *s3.Client) *S3Service {
	return &S3Service{
		client:   client,
		uploader: manager.NewUploader(client),
	}
}

func (s *S3Service) UploadDirectory(ctx context.Context, localPath string, opts UploadOptions) (string, error) {
	if opts.Bucket == "" {
		return "", fmt.Errorf("storage bucket is required")
	}

	root := filepath.Clean(localPath)
	if fi, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("stat local path: %w", err)
	} else if !fi.IsDir() {
		return "", fmt.Errorf("local path must be a directory")
	}

	type uploadFile struct {
		path string
		rel  string
		size int64
	}

	var files []uploadFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		files = append(files, uploadFile{
			path: path,
			rel:  filepath.ToSlash(rel),
			size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return "", err
	}

	var totalSize int64
	for _, file := range files {
		totalSize += file.size
	}

	progress := newProgressReporter(totalSize, opts.ProgressCallback)
	if progress != nil {
		progress.report(0)
	}

	keyPrefix := strings.Trim(opts.KeyPrefix, "/")
	if keyPrefix == "" {
		keyPrefix = fmt.Sprintf("task-%d", os.Getpid())
	}

	for _, file := range files {
		key := keyPrefix
		if file.rel != "" && file.rel != "." {
			key = strings.TrimSuffix(keyPrefix, "/")
			if key != "" {
				key += "/"
			}
			key += file.rel
		}
		if key == "" {
			key = filepath.ToSlash(filepath.Base(file.path))
		}

		f, err := os.Open(file.path)
		if err != nil {
			return "", fmt.Errorf("open file %s: %w", file.path, err)
		}
		var reader io.Reader = f
		if progress != nil {
			reader = io.TeeReader(f, progress)
		}
		_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(opts.Bucket),
			Key:    aws.String(key),
			Body:   reader,
			ACL:    types.ObjectCannedACLPrivate,
		})
		closeErr := f.Close()
		if err != nil {
			return "", fmt.Errorf("upload %s: %w", file.path, err)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close file %s: %w", file.path, closeErr)
		}
	}

	if progress != nil {
		progress.flush()
	}

	return fmt.Sprintf("s3://%s/%s", opts.Bucket, keyPrefix), nil
}

func (s *S3Service) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	if bucket == "" {
		return nil, fmt.Errorf("storage bucket is required")
	}

	var objects []ObjectInfo
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if strings.TrimSpace(prefix) != "" {
		input.Prefix = aws.String(prefix)
	}

	for {
		output, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}

		for _, obj := range output.Contents {
			objects = append(objects, ObjectInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: obj.LastModified,
			})
		}

		if !aws.ToBool(output.IsTruncated) || output.NextContinuationToken == nil {
			break
		}
		input.ContinuationToken = output.NextContinuationToken
	}

	return objects, nil
}

func (s *S3Service) DeletePrefix(ctx context.Context, bucket, prefix string) error {
	if bucket == "" {
		return fmt.Errorf("storage bucket is required")
	}
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return fmt.Errorf("prefix is required")
	}

	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(trimmed),
	}

	for {
		output, err := s.client.ListObjectsV2(ctx, listInput)
		if err != nil {
			return fmt.Errorf("list objects for delete: %w", err)
		}

		if len(output.Contents) == 0 {
			if !aws.ToBool(output.IsTruncated) {
				break
			}
		} else {
			identifiers := make([]types.ObjectIdentifier, 0, len(output.Contents))
			for _, obj := range output.Contents {
				identifiers = append(identifiers, types.ObjectIdentifier{Key: obj.Key})
			}
			if len(identifiers) > 0 {
				_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
					Bucket: aws.String(bucket),
					Delete: &types.Delete{
						Objects: identifiers,
						Quiet:   aws.Bool(true),
					},
				})
				if err != nil {
					return fmt.Errorf("delete objects: %w", err)
				}
			}
		}

		if !aws.ToBool(output.IsTruncated) || output.NextContinuationToken == nil {
			break
		}
		listInput.ContinuationToken = output.NextContinuationToken
	}

	return nil
}

var _ Service = (*S3Service)(nil)

type progressReporter struct {
	total    int64
	done     int64
	cb       func(done, total int64)
	mu       sync.Mutex
	lastFire time.Time
}

func newProgressReporter(total int64, cb func(done, total int64)) *progressReporter {
	if cb == nil {
		return nil
	}
	return &progressReporter{
		total: total,
		cb:    cb,
	}
}

func (p *progressReporter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.done += int64(len(b))
	now := time.Now()
	if now.Sub(p.lastFire) >= 200*time.Millisecond || p.done == p.total {
		p.lastFire = now
		p.cb(p.done, p.total)
	}

	return len(b), nil
}

func (p *progressReporter) report(done int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done = done
	p.lastFire = time.Now()
	p.cb(p.done, p.total)
}

func (p *progressReporter) flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cb(p.done, p.total)
}
