package handler

import (
	"slices"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// FileHandler exposes upload and download endpoints.
type FileHandler struct {
	files     *usecase.FileUsecase
	validator validator.Validator
}

// NewFileHandler builds the file handler.
func NewFileHandler(files *usecase.FileUsecase, v validator.Validator) *FileHandler {
	return &FileHandler{files: files, validator: v}
}

// Upload stores one file and returns its id and URL.
// POST /api/v1/files?purpose=document
func (h *FileHandler) Upload(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}

	purpose := usecase.UploadPurpose(c.Query("purpose"))
	if purpose == "" {
		return apperror.ErrBadRequest.WithMessage("Query parameter 'purpose' is required")
	}

	header, err := c.FormFile("file")
	if err != nil {
		return apperror.ErrBadRequest.WithMessage("Attach a file in the 'file' form field").Wrap(err)
	}

	res, err := h.files.Upload(c.UserContext(), header, purpose, userID)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// UploadMany stores several files in one request, which is what the vehicle and
// load photo pickers need.
// POST /api/v1/files/batch?purpose=vehicle
func (h *FileHandler) UploadMany(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	purpose := usecase.UploadPurpose(c.Query("purpose"))
	if purpose == "" {
		return apperror.ErrBadRequest.WithMessage("Query parameter 'purpose' is required")
	}

	form, err := c.MultipartForm()
	if err != nil {
		return apperror.ErrBadRequest.WithMessage("Expected a multipart form").Wrap(err)
	}
	headers := form.File["files"]
	if len(headers) == 0 {
		return apperror.ErrBadRequest.WithMessage("Attach at least one file in the 'files' form field")
	}
	if len(headers) > 12 {
		return apperror.ErrValidation.WithMessage("At most 12 files per request")
	}

	results := make([]dto.FileResponse, 0, len(headers))
	for _, header := range headers {
		res, err := h.files.Upload(c.UserContext(), header, purpose, userID)
		if err != nil {
			// Fail the whole batch: a half-uploaded photo set is worse than a
			// clear error the client can retry.
			return err
		}
		results = append(results, *res)
	}
	return response.Created(c, results)
}

// Get returns file metadata with a fresh URL.
// GET /api/v1/files/:id
func (h *FileHandler) Get(c *fiber.Ctx) error {
	fileID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	res, err := h.files.Get(c.UserContext(), fileID, claims.UserID, isAdmin(claims.Roles))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Delete removes a file the caller owns.
// DELETE /api/v1/files/:id
func (h *FileHandler) Delete(c *fiber.Ctx) error {
	fileID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	if err := h.files.Delete(c.UserContext(), fileID, claims.UserID, isAdmin(claims.Roles)); err != nil {
		return err
	}
	return response.NoContent(c)
}

// isAdmin reports whether the principal holds a platform-wide staff role.
func isAdmin(roles []string) bool {
	return slices.Contains(roles, domain.RoleAdmin) ||
		slices.Contains(roles, domain.RoleSuperAdmin)
}
