package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type userRepository struct{ Base }

// NewUserRepository builds the user repository.
func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{Base: NewBase(db)}
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	if err := r.DB(ctx).Create(user).Error; err != nil {
		return translate(err, "user")
	}
	return nil
}

func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	// Select-less Save would also write zero values for untouched columns;
	// Updates on a struct skips zero values, which is what callers expect here.
	if err := r.DB(ctx).Model(&domain.User{ID: user.ID}).Updates(user).Error; err != nil {
		return translate(err, "user")
	}
	return nil
}

func (r *userRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.User{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "user")
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	if err := r.DB(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, translate(err, "user")
	}
	return &user, nil
}

func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	var user domain.User
	if err := r.DB(ctx).First(&user, "phone = ?", phone).Error; err != nil {
		return nil, translate(err, "user")
	}
	return &user, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	if err := r.DB(ctx).First(&user, "lower(email) = lower(?)", email).Error; err != nil {
		return nil, translate(err, "user")
	}
	return &user, nil
}

func (r *userRepository) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	var count int64
	err := r.DB(ctx).Model(&domain.User{}).Where("phone = ?", phone).Count(&count).Error
	if err != nil {
		return false, translate(err, "user")
	}
	return count > 0, nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.DB(ctx).Model(&domain.User{}).
		Where("lower(email) = lower(?)", email).Count(&count).Error
	if err != nil {
		return false, translate(err, "user")
	}
	return count > 0, nil
}

func (r *userRepository) List(ctx context.Context, filter domain.UserFilter, offset, limit int, order string) ([]domain.User, int64, error) {
	q := r.DB(ctx).Model(&domain.User{})

	if s := strings.TrimSpace(filter.Search); s != "" {
		like := "%" + s + "%"
		q = q.Where(
			"phone ILIKE ? OR email ILIKE ? OR (first_name || ' ' || last_name) ILIKE ?",
			like, like, like)
	}
	if len(filter.Status) > 0 {
		q = q.Where("status IN ?", filter.Status)
	}
	if filter.Locale != "" {
		q = q.Where("locale = ?", filter.Locale)
	}
	if filter.RoleCode != "" {
		q = q.Where(`EXISTS (
			SELECT 1 FROM user_roles ur
			JOIN roles ro ON ro.id = ur.role_id
			WHERE ur.user_id = users.id AND ro.code = ?)`, filter.RoleCode)
	}
	if filter.Created.From != nil {
		q = q.Where("users.created_at >= ?", *filter.Created.From)
	}
	if filter.Created.To != nil {
		q = q.Where("users.created_at < ?", *filter.Created.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "user")
	}

	var users []domain.User
	if err := applyPage(applyOrder(q, order), offset, limit).Find(&users).Error; err != nil {
		return nil, 0, translate(err, "user")
	}
	return users, total, nil
}

func (r *userRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", id).Delete(&domain.User{}).Error
	return translate(err, "user")
}
