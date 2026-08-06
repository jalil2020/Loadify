package domain

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FileVisibility controls whether an object is reachable without a signed URL.
type FileVisibility string

const (
	FileVisibilityPrivate FileVisibility = "private"
	FileVisibilityPublic  FileVisibility = "public"
)

// File is an object stored in S3/MinIO plus its metadata row.
type File struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OwnerUserID    *uuid.UUID     `gorm:"type:uuid;index"`
	Bucket         string         `gorm:"size:64;not null"`
	ObjectKey      string         `gorm:"size:512;not null"`
	OriginalName   string         `gorm:"size:255;not null"`
	MimeType       string         `gorm:"size:128;not null"`
	SizeBytes      int64          `gorm:"not null"`
	ChecksumSHA256 *string        `gorm:"size:64"`
	Visibility     FileVisibility `gorm:"size:16;not null;default:'private'"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// UploadInput describes an object being written to storage.
type UploadInput struct {
	Reader      io.Reader
	Size        int64
	ObjectKey   string
	ContentType string
	Public      bool
}

// Storage is the object-storage port.
type Storage interface {
	Upload(ctx context.Context, in UploadInput) error
	Download(ctx context.Context, objectKey string) (io.ReadCloser, error)
	Delete(ctx context.Context, objectKey string) error
	PresignedGetURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error)
	PresignedPutURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error)
	PublicURL(objectKey string) string
	Exists(ctx context.Context, objectKey string) (bool, error)
}

// FileRepository persists file metadata.
type FileRepository interface {
	Create(ctx context.Context, file *File) error
	FindByID(ctx context.Context, id uuid.UUID) (*File, error)
	FindByIDs(ctx context.Context, ids []uuid.UUID) ([]File, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	ListOrphans(ctx context.Context, olderThan time.Time, limit int) ([]File, error)
}
