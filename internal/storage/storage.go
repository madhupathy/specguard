package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/specguard/specguard/internal/config"
)

type Storage interface {
	Store(ctx context.Context, key string, data io.Reader, contentType string) error
	Retrieve(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
	GetURL(ctx context.Context, key string) (string, error)
}

type S3Storage struct {
	client *s3.S3
	bucket string
	region string
}

type LocalStorage struct {
	basePath string
}

func New(cfg config.StorageConfig) (Storage, error) {
	switch cfg.Type {
	case "s3":
		return newS3Storage(cfg)
	case "local":
		return newLocalStorage(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

func newS3Storage(cfg config.StorageConfig) (*S3Storage, error) {
	awsConfig := &aws.Config{
		Region: aws.String(cfg.Region),
		Credentials: credentials.NewStaticCredentials(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
	}

	// For S3-compatible storage (like MinIO, R2)
	if cfg.Endpoint != "" {
		awsConfig.Endpoint = aws.String(cfg.Endpoint)
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &S3Storage{
		client: s3.New(sess),
		bucket: cfg.Bucket,
		region: cfg.Region,
	}, nil
}

func newLocalStorage(cfg config.StorageConfig) (*LocalStorage, error) {
	if err := os.MkdirAll(cfg.LocalPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create local storage directory: %w", err)
	}

	return &LocalStorage{
		basePath: cfg.LocalPath,
	}, nil
}

// S3 Storage Implementation

func (s *S3Storage) Store(ctx context.Context, key string, data io.Reader, contentType string) error {
	_, err := s.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        aws.ReadSeekCloser(data),
		ContentType: aws.String(contentType),
	})
	return err
}

func (s *S3Storage) Retrieve(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	result, err := s.client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, obj := range result.Contents {
		keys = append(keys, *obj.Key)
	}
	return keys, nil
}

func (s *S3Storage) GetURL(ctx context.Context, key string) (string, error) {
	req, _ := s.client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return req.Presign(15 * time.Minute) // 15 minute presigned URL
}

// Local Storage Implementation

func (l *LocalStorage) Store(ctx context.Context, key string, data io.Reader, contentType string) error {
	fullPath := filepath.Join(l.basePath, key)

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data
	_, err = io.Copy(file, data)
	return err
}

func (l *LocalStorage) Retrieve(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := filepath.Join(l.basePath, key)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(l.basePath, key)
	return os.Remove(fullPath)
}

func (l *LocalStorage) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	baseDir := filepath.Join(l.basePath, prefix)

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(l.basePath, path)
			if err != nil {
				return err
			}
			keys = append(keys, relPath)
		}
		return nil
	})

	return keys, err
}

func (l *LocalStorage) GetURL(ctx context.Context, key string) (string, error) {
	// For local storage, return file path
	fullPath := filepath.Join(l.basePath, key)
	if _, err := os.Stat(fullPath); err != nil {
		return "", err
	}
	return "file://" + fullPath, nil
}
