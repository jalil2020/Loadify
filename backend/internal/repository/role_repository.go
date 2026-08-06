package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/loadify/backend/internal/domain"
)

type roleRepository struct{ Base }

// NewRoleRepository builds the role and permission repository.
func NewRoleRepository(db *gorm.DB) domain.RoleRepository {
	return &roleRepository{Base: NewBase(db)}
}

func (r *roleRepository) FindByCode(ctx context.Context, code string) (*domain.Role, error) {
	var role domain.Role
	if err := r.DB(ctx).First(&role, "code = ?", code).Error; err != nil {
		return nil, translate(err, "role")
	}
	return &role, nil
}

func (r *roleRepository) ListRoles(ctx context.Context) ([]domain.Role, error) {
	var roles []domain.Role
	err := r.DB(ctx).Preload("Permissions").Order("level DESC").Find(&roles).Error
	if err != nil {
		return nil, translate(err, "role")
	}
	return roles, nil
}

func (r *roleRepository) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	var perms []domain.Permission
	err := r.DB(ctx).Order("resource, action").Find(&perms).Error
	if err != nil {
		return nil, translate(err, "permission")
	}
	return perms, nil
}

func (r *roleRepository) AssignRole(ctx context.Context, userID, roleID uuid.UUID, companyID, grantedBy *uuid.UUID) error {
	grant := domain.UserRole{
		UserID:    userID,
		RoleID:    roleID,
		CompanyID: companyID,
		GrantedBy: grantedBy,
	}
	// The partial unique index on (user_id, role_id, company_id) makes a repeat
	// grant a no-op rather than an error.
	err := r.DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error
	return translate(err, "role assignment")
}

func (r *roleRepository) RevokeRole(ctx context.Context, userID, roleID uuid.UUID, companyID *uuid.UUID) error {
	q := r.DB(ctx).Where("user_id = ? AND role_id = ?", userID, roleID)
	if companyID != nil {
		q = q.Where("company_id = ?", *companyID)
	} else {
		q = q.Where("company_id IS NULL")
	}
	return translate(q.Delete(&domain.UserRole{}).Error, "role assignment")
}

func (r *roleRepository) RolesOfUser(ctx context.Context, userID uuid.UUID) ([]domain.UserRole, error) {
	var grants []domain.UserRole
	err := r.DB(ctx).
		Preload("Role").
		Where("user_id = ?", userID).
		Where("expires_at IS NULL OR expires_at > now()").
		Find(&grants).Error
	if err != nil {
		return nil, translate(err, "role assignment")
	}
	return grants, nil
}

func (r *roleRepository) PermissionCodesOfUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var codes []string
	err := r.DB(ctx).
		Table("permissions p").
		Distinct("p.code").
		Joins("JOIN role_permissions rp ON rp.permission_id = p.id").
		Joins("JOIN user_roles ur ON ur.role_id = rp.role_id").
		Where("ur.user_id = ?", userID).
		Where("ur.expires_at IS NULL OR ur.expires_at > now()").
		Order("p.code").
		Pluck("p.code", &codes).Error
	if err != nil {
		return nil, translate(err, "permission")
	}
	return codes, nil
}

func (r *roleRepository) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	db := r.DB(ctx)
	if err := db.Exec(`DELETE FROM role_permissions WHERE role_id = ?`, roleID).Error; err != nil {
		return translate(err, "role permission")
	}
	if len(permissionIDs) == 0 {
		return nil
	}

	type rolePermission struct {
		RoleID       uuid.UUID `gorm:"column:role_id"`
		PermissionID uuid.UUID `gorm:"column:permission_id"`
	}
	rows := make([]rolePermission, 0, len(permissionIDs))
	for _, pid := range permissionIDs {
		rows = append(rows, rolePermission{RoleID: roleID, PermissionID: pid})
	}
	err := db.Table("role_permissions").
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
	return translate(err, "role permission")
}
