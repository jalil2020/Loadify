// Package storage implements the object-storage port over any S3-compatible
// service. MinIO is used in development and on the launch droplet; the same
// adapter works unchanged against AWS S3 or DigitalOcean Spaces.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

type minioStorage struct {
	client *minio.Client
	cfg    config.S3Config
	log    logger.Logger
}

// New connects to the object store and makes sure the bucket exists.
func New(cfg config.S3Config, log logger.Logger) (domain.Storage, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("connect object storage: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket %s: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("create bucket %s: %w", cfg.Bucket, err)
		}
		log.Info("object storage bucket created", logger.String("bucket", cfg.Bucket))
	}

	log.Info("object storage connected",
		logger.String("endpoint", endpoint),
		logger.String("bucket", cfg.Bucket))

	return &minioStorage{client: client, cfg: cfg, log: log}, nil
}

func (s *minioStorage) Upload(ctx context.Context, in domain.UploadInput) error {
	opts := minio.PutObjectOptions{ContentType: in.ContentType}
	if in.Public {
		opts.UserMetadata = map[string]string{"x-amz-acl": "public-read"}
	}
	if _, err := s.client.PutObject(ctx, s.cfg.Bucket, in.ObjectKey, in.Reader, in.Size, opts); err != nil {
		return fmt.Errorf("upload %s: %w", in.ObjectKey, err)
	}
	return nil
}

func (s *minioStorage) Download(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.cfg.Bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", objectKey, err)
	}
	// GetObject is lazy; stat here so a missing object fails now, not mid-stream.
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("stat %s: %w", objectKey, err)
	}
	return obj, nil
}

func (s *minioStorage) Delete(ctx context.Context, objectKey string) error {
	if err := s.client.RemoveObject(ctx, s.cfg.Bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %s: %w", objectKey, err)
	}
	return nil
}

func (s *minioStorage) PresignedGetURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.cfg.Bucket, objectKey, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign get %s: %w", objectKey, err)
	}
	return s.rewriteHost(u.String()), nil
}

func (s *minioStorage) PresignedPutURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, s.cfg.Bucket, objectKey, ttl)
	if err != nil {
		return "", fmt.Errorf("presign put %s: %w", objectKey, err)
	}
	return s.rewriteHost(u.String()), nil
}

func (s *minioStorage) PublicURL(objectKey string) string {
	return strings.TrimRight(s.cfg.PublicURL, "/") + "/" + s.cfg.Bucket + "/" + objectKey
}

func (s *minioStorage) Exists(ctx context.Context, objectKey string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.cfg.Bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", objectKey, err)
	}
	return true, nil
}

// rewriteHost swaps the internal endpoint for the publicly reachable one, so
// URLs signed inside the Docker network still work from a phone.
func (s *minioStorage) rewriteHost(signed string) string {
	if s.cfg.PublicURL == "" || s.cfg.PublicURL == s.cfg.Endpoint {
		return signed
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		return signed
	}
	public, err := url.Parse(s.cfg.PublicURL)
	if err != nil {
		return signed
	}
	parsed.Scheme, parsed.Host = public.Scheme, public.Host
	return parsed.String()
}
