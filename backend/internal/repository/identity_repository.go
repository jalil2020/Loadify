package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type identityRepository struct{ Base }

// NewIdentityRepository builds the tenancy resolver.
func NewIdentityRepository(db *gorm.DB) domain.IdentityRepository {
	return &identityRepository{Base: NewBase(db)}
}

// Resolve looks up the driver row and the active company membership of a user.
// Both are optional: an admin has neither, a driver employed by a company has both.
func (r *identityRepository) Resolve(ctx context.Context, userID uuid.UUID) (domain.UserContext, error) {
	var out domain.UserContext

	var driver struct {
		ID        uuid.UUID  `gorm:"column:id"`
		CompanyID *uuid.UUID `gorm:"column:company_id"`
	}
	err := r.DB(ctx).Table("drivers").
		Select("id, company_id").
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Take(&driver).Error
	switch {
	case err == nil:
		id := driver.ID
		out.DriverID = &id
		out.CompanyID = driver.CompanyID
	case isNotFound(err):
		// Not a driver; carry on.
	default:
		return out, translate(err, "driver")
	}

	var membership struct {
		CompanyID  uuid.UUID `gorm:"column:company_id"`
		MemberRole string    `gorm:"column:member_role"`
	}
	err = r.DB(ctx).Table("company_members").
		Select("company_id, member_role").
		Where("user_id = ? AND is_active AND deleted_at IS NULL", userID).
		Order("CASE member_role WHEN 'company_manager' THEN 0 ELSE 1 END").
		Take(&membership).Error
	switch {
	case err == nil:
		id := membership.CompanyID
		out.CompanyID = &id
		out.CompanyRole = membership.MemberRole
	case isNotFound(err):
		// Not a company member; carry on.
	default:
		return out, translate(err, "company membership")
	}

	return out, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
