package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

type Object struct {
	Body        io.ReadCloser
	ContentType string
}

type Store interface {
	Put(ctx context.Context, key, contentType string, body io.Reader) (int64, error)
	Get(ctx context.Context, key string) (Object, error)
	Delete(ctx context.Context, key string) error
	Driver() string
}

func New(ctx context.Context, cfg config.Config) (Store, error) {
	if cfg.StorageDriver == "s3" {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.S3Region))
		if err != nil {
			return nil, fmt.Errorf("load AWS configuration: %w", err)
		}
		client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
			if cfg.S3Endpoint != "" {
				options.BaseEndpoint = aws.String(cfg.S3Endpoint)
				options.UsePathStyle = true
			}
		})
		return &s3Store{client: client, bucket: cfg.S3Bucket}, nil
	}
	return &localStore{root: cfg.UploadDirectory}, nil
}

type localStore struct{ root string }

func (s *localStore) Driver() string { return "local" }

func (s *localStore) target(key string) (string, error) {
	base, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(key)))
	if err != nil {
		return "", err
	}
	if target != base && !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("object key escapes storage root")
	}
	return target, nil
}

func (s *localStore) Put(_ context.Context, key, _ string, body io.Reader) (int64, error) {
	target, err := s.target(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(target)
		if copyErr != nil {
			return written, copyErr
		}
		return written, closeErr
	}
	return written, nil
}

func (s *localStore) Get(_ context.Context, key string) (Object, error) {
	target, err := s.target(key)
	if err != nil {
		return Object{}, err
	}
	file, err := os.Open(target)
	if err != nil {
		return Object{}, err
	}
	return Object{Body: file}, nil
}

func (s *localStore) Delete(_ context.Context, key string) error {
	target, err := s.target(key)
	if err != nil {
		return err
	}
	return os.Remove(target)
}

type s3Store struct {
	client *s3.Client
	bucket string
}

func (s *s3Store) Driver() string { return "s3" }

func (s *s3Store) Put(ctx context.Context, key, contentType string, body io.Reader) (int64, error) {
	counter := &countingReader{reader: body}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(filepath.ToSlash(key)),
		Body: counter, ContentType: aws.String(contentType),
	})
	return counter.count, err
}

func (s *s3Store) Get(ctx context.Context, key string) (Object, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(filepath.ToSlash(key))})
	if err != nil {
		return Object{}, err
	}
	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	return Object{Body: result.Body, ContentType: contentType}, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(filepath.ToSlash(key))})
	return err
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.count += int64(n)
	return n, err
}
