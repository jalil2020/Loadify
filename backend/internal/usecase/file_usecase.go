package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// UploadPurpose scopes an upload, deciding the allowed types, the size ceiling
// and where the object lands in the bucket.
type UploadPurpose string

const (
	UploadAvatar         UploadPurpose = "avatar"
	UploadDocument       UploadPurpose = "document"
	UploadVehiclePhoto   UploadPurpose = "vehicle"
	UploadLoadPhoto      UploadPurpose = "load"
	UploadLoadDocument   UploadPurpose = "load_document"
	UploadTripDocument   UploadPurpose = "trip_document"
	UploadChatAttachment UploadPurpose = "chat"
	UploadCompanyLogo    UploadPurpose = "logo"
)

// uploadRule is the per-purpose policy.
type uploadRule struct {
	prefix    string
	maxBytes  int64
	mimeTypes []string
	public    bool
}

// Images stay small because they are viewed on phones over mobile data;
// documents get more room because scans of a passport page are large.
var uploadRules = map[UploadPurpose]uploadRule{
	UploadAvatar:         {prefix: "avatars", maxBytes: 5 << 20, mimeTypes: imageTypes, public: true},
	UploadCompanyLogo:    {prefix: "logos", maxBytes: 5 << 20, mimeTypes: imageTypes, public: true},
	UploadVehiclePhoto:   {prefix: "vehicles", maxBytes: 8 << 20, mimeTypes: imageTypes, public: false},
	UploadLoadPhoto:      {prefix: "loads", maxBytes: 8 << 20, mimeTypes: imageTypes, public: false},
	UploadDocument:       {prefix: "documents", maxBytes: 15 << 20, mimeTypes: documentTypes, public: false},
	UploadLoadDocument:   {prefix: "load-documents", maxBytes: 15 << 20, mimeTypes: documentTypes, public: false},
	UploadTripDocument:   {prefix: "trip-documents", maxBytes: 15 << 20, mimeTypes: documentTypes, public: false},
	UploadChatAttachment: {prefix: "chat", maxBytes: 10 << 20, mimeTypes: documentTypes, public: false},
}

var (
	imageTypes    = []string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif"}
	documentTypes = []string{
		"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif", "application/pdf",
	}
)

// signedURLTTL is how long a presigned download link stays valid.
const signedURLTTL = 30 * time.Minute

// FileUsecase handles uploads and URL resolution.
type FileUsecase struct {
	files   domain.FileRepository
	storage domain.Storage
	cfg     *config.Config
	log     logger.Logger
}

// NewFileUsecase wires the file usecase.
func NewFileUsecase(files domain.FileRepository, storage domain.Storage, cfg *config.Config, log logger.Logger) *FileUsecase {
	return &FileUsecase{files: files, storage: storage, cfg: cfg, log: log}
}

// Upload validates and stores one uploaded file.
//
// The declared Content-Type is not trusted: the first 512 bytes are sniffed and
// the sniffed type decides. Otherwise a renamed executable with an image
// Content-Type would be accepted.
func (u *FileUsecase) Upload(ctx context.Context, header *multipart.FileHeader, purpose UploadPurpose, ownerID uuid.UUID) (*dto.FileResponse, error) {
	if u.storage == nil {
		return nil, apperror.ErrUnavailable.WithMessage("File storage is not available right now")
	}

	rule, ok := uploadRules[purpose]
	if !ok {
		return nil, apperror.ErrBadRequest.WithMessage("Unknown upload purpose")
	}
	if header.Size <= 0 {
		return nil, apperror.ErrValidation.WithMessage("The uploaded file is empty")
	}
	if header.Size > rule.maxBytes {
		return nil, apperror.ErrPayload.WithMessage(
			fmt.Sprintf("Maximum size for this upload is %d MB", rule.maxBytes>>20))
	}

	src, err := header.Open()
	if err != nil {
		return nil, apperror.ErrBadRequest.WithMessage("Could not read the uploaded file").Wrap(err)
	}
	defer func() { _ = src.Close() }()

	// Read the whole file: the size ceiling above keeps this bounded, and both
	// the checksum and the MinIO writer need a rewindable source.
	data, err := io.ReadAll(io.LimitReader(src, rule.maxBytes+1))
	if err != nil {
		return nil, apperror.ErrInternal.Wrap(err)
	}
	if int64(len(data)) > rule.maxBytes {
		return nil, apperror.ErrPayload.WithMessage(
			fmt.Sprintf("Maximum size for this upload is %d MB", rule.maxBytes>>20))
	}

	detected := detectContentType(data)
	if !slices.Contains(rule.mimeTypes, detected) {
		return nil, apperror.ErrValidation.WithMessage(
			"Unsupported file type. Allowed: " + strings.Join(rule.mimeTypes, ", "))
	}

	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])

	objectKey := buildObjectKey(rule.prefix, ownerID, header.Filename, detected)

	err = u.storage.Upload(ctx, domain.UploadInput{
		Reader:      bytes.NewReader(data),
		Size:        int64(len(data)),
		ObjectKey:   objectKey,
		ContentType: detected,
		Public:      rule.public,
	})
	if err != nil {
		return nil, apperror.ErrInternal.WithMessage("Upload failed, try again").Wrap(err)
	}

	visibility := domain.FileVisibilityPrivate
	if rule.public {
		visibility = domain.FileVisibilityPublic
	}

	file := &domain.File{
		ID:             uuid.New(),
		OwnerUserID:    &ownerID,
		Bucket:         u.cfg.S3.Bucket,
		ObjectKey:      objectKey,
		OriginalName:   sanitiseFilename(header.Filename),
		MimeType:       detected,
		SizeBytes:      int64(len(data)),
		ChecksumSHA256: &checksum,
		Visibility:     visibility,
	}
	if err := u.files.Create(ctx, file); err != nil {
		// The object is already in the bucket; remove it so the sweep job does
		// not have to. A failure here is logged, not surfaced.
		if delErr := u.storage.Delete(ctx, objectKey); delErr != nil {
			u.log.Warn("orphaned object cleanup failed",
				logger.String("object_key", objectKey), logger.Err(delErr))
		}
		return nil, err
	}

	return u.toResponse(ctx, file), nil
}

// ResolveURL returns a URL for a file: public objects get their permanent URL,
// private ones a short-lived presigned link.
func (u *FileUsecase) ResolveURL(ctx context.Context, file *domain.File) string {
	if file == nil || u.storage == nil {
		return ""
	}
	if file.Visibility == domain.FileVisibilityPublic {
		return u.storage.PublicURL(file.ObjectKey)
	}
	url, err := u.storage.PresignedGetURL(ctx, file.ObjectKey, signedURLTTL)
	if err != nil {
		u.log.Warn("presigning download url failed",
			logger.String("object_key", file.ObjectKey), logger.Err(err))
		return ""
	}
	return url
}

// ResolveURLByID looks a file up and resolves its URL. Returns an empty string
// for a missing file rather than an error, because a broken avatar must not
// fail the profile request that contains it.
func (u *FileUsecase) ResolveURLByID(ctx context.Context, id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	file, err := u.files.FindByID(ctx, *id)
	if err != nil {
		return nil
	}
	url := u.ResolveURL(ctx, file)
	if url == "" {
		return nil
	}
	return &url
}

// Get returns file metadata with a resolved URL, enforcing ownership for
// private objects.
func (u *FileUsecase) Get(ctx context.Context, id, requesterID uuid.UUID, isAdmin bool) (*dto.FileResponse, error) {
	file, err := u.files.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if file.Visibility == domain.FileVisibilityPrivate && !isAdmin {
		if file.OwnerUserID == nil || *file.OwnerUserID != requesterID {
			return nil, apperror.ErrForbidden
		}
	}
	return u.toResponse(ctx, file), nil
}

// AssertOwnership rejects an attempt to attach a file the caller did not upload.
// Every endpoint that accepts a file_id must call this: file ids are UUIDs, but
// they travel through responses and a leaked one must not be reusable.
func (u *FileUsecase) AssertOwnership(ctx context.Context, userID, fileID uuid.UUID) error {
	file, err := u.files.FindByID(ctx, fileID)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrValidation.
				WithDetails(apperror.FieldError{Field: "file_id", Message: "File not found"})
		}
		return err
	}
	if file.OwnerUserID == nil || *file.OwnerUserID != userID {
		return apperror.ErrForbidden.WithMessage("This file belongs to another account")
	}
	return nil
}

// Delete removes a file the requester owns.
func (u *FileUsecase) Delete(ctx context.Context, id, requesterID uuid.UUID, isAdmin bool) error {
	file, err := u.files.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !isAdmin && (file.OwnerUserID == nil || *file.OwnerUserID != requesterID) {
		return apperror.ErrForbidden
	}
	if err := u.files.SoftDelete(ctx, id); err != nil {
		return err
	}
	// The object itself is removed by the retention sweep, so a restore is
	// still possible during the grace period.
	return nil
}

// PurgeOrphans deletes uploads that were never attached to anything.
func (u *FileUsecase) PurgeOrphans(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	if u.storage == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	orphans, err := u.files.ListOrphans(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, file := range orphans {
		if err := u.storage.Delete(ctx, file.ObjectKey); err != nil {
			u.log.Warn("purging object failed",
				logger.String("object_key", file.ObjectKey), logger.Err(err))
			continue
		}
		if err := u.files.SoftDelete(ctx, file.ID); err != nil {
			u.log.Warn("purging file record failed",
				logger.String("file_id", file.ID.String()), logger.Err(err))
			continue
		}
		purged++
	}
	return purged, nil
}

func (u *FileUsecase) toResponse(ctx context.Context, file *domain.File) *dto.FileResponse {
	return &dto.FileResponse{
		ID:           file.ID,
		URL:          u.ResolveURL(ctx, file),
		OriginalName: file.OriginalName,
		MimeType:     file.MimeType,
		SizeBytes:    file.SizeBytes,
		CreatedAt:    file.CreatedAt,
	}
}

// ─── helpers ────────────────────────────────────────────────────────

// detectContentType sniffs the magic bytes. net/http.DetectContentType does not
// know HEIC, which every recent iPhone produces, so that case is handled here.
func detectContentType(data []byte) string {
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "hevx", "mif1":
			return "image/heic"
		case "heim", "heis", "hevm", "hevs", "msf1":
			return "image/heif"
		}
	}
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) >= 5 && string(data[:5]) == "%PDF-":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// buildObjectKey produces a collision-free, non-guessable key. The owner id is
// in the path for operational triage; the random component is what makes the
// key unguessable, since private objects are served by presigned URL anyway.
func buildObjectKey(prefix string, ownerID uuid.UUID, filename, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" || len(ext) > 6 {
		ext = extensionFor(mimeType)
	}
	now := time.Now().UTC()
	return fmt.Sprintf("%s/%s/%04d/%02d/%s%s",
		prefix, ownerID.String(), now.Year(), int(now.Month()), uuid.NewString(), ext)
}

func extensionFor(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	case "image/heif":
		return ".heif"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

// sanitiseFilename keeps the display name harmless: no path components, no
// control characters, bounded length.
func sanitiseFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "upload"
	}
	if len(cleaned) > 200 {
		cleaned = cleaned[:200]
	}
	return cleaned
}
