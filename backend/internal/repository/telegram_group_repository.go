package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type telegramGroupRepository struct{ Base }

// NewTelegramGroupRepository builds the group repository.
func NewTelegramGroupRepository(db *gorm.DB) domain.TelegramGroupRepository {
	return &telegramGroupRepository{Base: NewBase(db)}
}

// Upsert stores a group the person picked out of their own dialogs, or
// refreshes what Telegram now says about one already stored.
func (r *telegramGroupRepository) Upsert(ctx context.Context, group *domain.TelegramGroup) error {
	err := r.DB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}, {Name: "tg_peer_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "username", "kind", "member_count", "slowmode_seconds",
			"can_send", "access_hash", "last_synced_at", "updated_at",
		}),
	}).Create(group).Error
	if err != nil {
		return translate(err, "telegram group")
	}
	return nil
}

func (r *telegramGroupRepository) FindByID(ctx context.Context, companyID, id uuid.UUID) (*domain.TelegramGroup, error) {
	var group domain.TelegramGroup
	err := r.DB(ctx).Where("id = ? AND company_id = ?", id, companyID).First(&group).Error
	if err != nil {
		return nil, translate(err, "telegram group")
	}
	return &group, nil
}

func (r *telegramGroupRepository) ListByAccount(ctx context.Context, companyID, accountID uuid.UUID) ([]domain.TelegramGroup, error) {
	var groups []domain.TelegramGroup
	err := r.DB(ctx).
		Where("company_id = ? AND account_id = ?", companyID, accountID).
		Order("title").Find(&groups).Error
	if err != nil {
		return nil, translate(err, "telegram group")
	}
	return groups, nil
}

func (r *telegramGroupRepository) ListByCompany(ctx context.Context, companyID uuid.UUID) ([]domain.TelegramGroup, error) {
	var groups []domain.TelegramGroup
	err := r.DB(ctx).Where("company_id = ?", companyID).Order("title").Find(&groups).Error
	if err != nil {
		return nil, translate(err, "telegram group")
	}
	return groups, nil
}

// candidateSQL is the deterministic filter that runs before anything is ranked
// and long before any model is asked anything.
//
// Two conditions carry most of the meaning. A group whose rules name body types
// or cargo types is only offered when the advert matches one of them, while a
// group that names none is offered for anything — the logistician narrowed it
// or they did not, and an empty list is not the same as "nothing". And a group
// with corridors must share at least one end with the advert; a group with no
// corridors at all is a general board, so it stays.
const candidateSQL = `
SELECT g.id
FROM telegram_groups g
JOIN telegram_group_rules r ON r.group_id = g.id
WHERE g.company_id = @company
  AND g.account_id = @account
  AND g.deleted_at IS NULL
  AND g.active AND g.posting_enabled AND g.can_send
  AND (cardinality(r.vehicle_types) = 0 OR @vehicle::text IS NULL OR @vehicle::text = ANY(r.vehicle_types))
  AND (cardinality(r.cargo_type_codes) = 0 OR @cargo::text IS NULL OR @cargo::text = ANY(r.cargo_type_codes))
  AND (r.min_weight_kg IS NULL OR @weight::int IS NULL OR @weight::int >= r.min_weight_kg)
  AND (r.max_weight_kg IS NULL OR @weight::int IS NULL OR @weight::int <= r.max_weight_kg)
  AND (
    NOT EXISTS (SELECT 1 FROM telegram_group_routes tr WHERE tr.group_id = g.id)
    OR EXISTS (
      SELECT 1 FROM telegram_group_routes tr
      WHERE tr.group_id = g.id
        AND (
          (tr.origin_region_id IS NOT NULL AND tr.origin_region_id = @origin_region)
          OR (tr.origin_region_id IS NULL AND tr.origin_country = @origin_country)
          OR (tr.dest_region_id IS NOT NULL AND tr.dest_region_id = @dest_region)
          OR (tr.dest_region_id IS NULL AND tr.dest_country = @dest_country)
        )
    )
  )`

// Candidates returns the groups worth ranking, with their rules and corridors.
func (r *telegramGroupRepository) Candidates(
	ctx context.Context,
	companyID uuid.UUID,
	query domain.GroupCandidateQuery,
) ([]domain.GroupCandidate, error) {
	var ids []uuid.UUID
	err := r.DB(ctx).Raw(candidateSQL, map[string]any{
		"company":        companyID,
		"account":        query.AccountID,
		"vehicle":        query.VehicleType,
		"cargo":          query.CargoTypeCode,
		"weight":         query.WeightKg,
		"origin_region":  query.OriginRegionID,
		"origin_country": query.OriginCountry,
		"dest_region":    query.DestRegionID,
		"dest_country":   query.DestCountry,
	}).Scan(&ids).Error
	if err != nil {
		return nil, translate(err, "telegram group")
	}
	if len(ids) == 0 {
		return nil, nil
	}

	var groups []domain.TelegramGroup
	if err := r.DB(ctx).Where("id IN ?", ids).Order("title").Find(&groups).Error; err != nil {
		return nil, translate(err, "telegram group")
	}

	rules, err := r.rulesByGroup(ctx, ids)
	if err != nil {
		return nil, err
	}
	routes, err := r.routesByGroup(ctx, ids)
	if err != nil {
		return nil, err
	}

	candidates := make([]domain.GroupCandidate, 0, len(groups))
	for _, group := range groups {
		candidates = append(candidates, domain.GroupCandidate{
			Group:  group,
			Rules:  rules[group.ID],
			Routes: routes[group.ID],
		})
	}
	return candidates, nil
}

func (r *telegramGroupRepository) rulesByGroup(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.TelegramGroupRules, error) {
	var rows []domain.TelegramGroupRules
	if err := r.DB(ctx).Where("group_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, translate(err, "telegram group rules")
	}
	byGroup := make(map[uuid.UUID]domain.TelegramGroupRules, len(rows))
	for _, row := range rows {
		byGroup[row.GroupID] = row
	}
	return byGroup, nil
}

func (r *telegramGroupRepository) routesByGroup(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.TelegramGroupRoute, error) {
	var rows []domain.TelegramGroupRoute
	if err := r.DB(ctx).Where("group_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, translate(err, "telegram group routes")
	}
	byGroup := make(map[uuid.UUID][]domain.TelegramGroupRoute, len(ids))
	for _, row := range rows {
		byGroup[row.GroupID] = append(byGroup[row.GroupID], row)
	}
	return byGroup, nil
}

// SaveRules replaces a group's rules and corridors together.
//
// Together because they are one statement by the person: "this group takes
// tents from Tashkent to Russia". Writing the rules and leaving yesterday's
// corridors behind would post into groups nobody chose any more.
func (r *telegramGroupRepository) SaveRules(
	ctx context.Context,
	rules *domain.TelegramGroupRules,
	routes []domain.TelegramGroupRoute,
) error {
	return r.DB(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "group_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"cargo_type_codes", "vehicle_types", "min_weight_kg", "max_weight_kg",
				"hourly_limit", "daily_limit", "cooldown_minutes",
				"quiet_from_hour", "quiet_to_hour", "notes", "updated_at",
			}),
		}).Create(rules).Error
		if err != nil {
			return translate(err, "telegram group rules")
		}

		if err := tx.Where("group_id = ?", rules.GroupID).
			Delete(&domain.TelegramGroupRoute{}).Error; err != nil {
			return translate(err, "telegram group routes")
		}
		if len(routes) == 0 {
			return nil
		}
		if err := tx.Create(&routes).Error; err != nil {
			return translate(err, "telegram group routes")
		}
		return nil
	})
}

func (r *telegramGroupRepository) Rules(
	ctx context.Context,
	companyID, groupID uuid.UUID,
) (*domain.TelegramGroupRules, []domain.TelegramGroupRoute, error) {
	var rules domain.TelegramGroupRules
	err := r.DB(ctx).Where("group_id = ? AND company_id = ?", groupID, companyID).First(&rules).Error
	if err != nil {
		return nil, nil, translate(err, "telegram group rules")
	}

	var routes []domain.TelegramGroupRoute
	if err := r.DB(ctx).Where("group_id = ?", groupID).Find(&routes).Error; err != nil {
		return nil, nil, translate(err, "telegram group routes")
	}
	return &rules, routes, nil
}

// Deactivate marks a group the account can no longer reach: left, removed, or
// deleted. It is not an error state, it is a fact about the world.
func (r *telegramGroupRepository) Deactivate(ctx context.Context, companyID, id uuid.UUID, reason string) error {
	return r.flag(ctx, companyID, id, map[string]any{
		"active":          false,
		"inactive_reason": reason,
	})
}

// DisablePosting keeps the group but stops sending to it, which is what a
// refusal from Telegram means: the group is there, this account may not write.
func (r *telegramGroupRepository) DisablePosting(ctx context.Context, companyID, id uuid.UUID, reason string) error {
	return r.flag(ctx, companyID, id, map[string]any{
		"posting_enabled": false,
		"can_send":        false,
		"inactive_reason": reason,
	})
}

func (r *telegramGroupRepository) flag(ctx context.Context, companyID, id uuid.UUID, columns map[string]any) error {
	res := r.DB(ctx).Model(&domain.TelegramGroup{}).
		Where("id = ? AND company_id = ?", id, companyID).
		Updates(columns)
	if res.Error != nil {
		return translate(res.Error, "telegram group")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("telegram group not found")
	}
	return nil
}

// MarkPosted records that something went out, which is what the ranking reads
// when it prefers a group that has not heard from us lately.
func (r *telegramGroupRepository) MarkPosted(ctx context.Context, id uuid.UUID, at time.Time) error {
	err := r.DB(ctx).Model(&domain.TelegramGroup{}).
		Where("id = ?", id).
		UpdateColumn("last_posted_at", at).Error
	if err != nil {
		return translate(err, "telegram group")
	}
	return nil
}

// Remove retires a group the person disconnected. The row is kept for the same
// reason a disconnected account is: the posts that went out point at it.
func (r *telegramGroupRepository) Remove(ctx context.Context, companyID, id uuid.UUID) error {
	res := r.DB(ctx).Where("id = ? AND company_id = ?", id, companyID).
		Delete(&domain.TelegramGroup{})
	if res.Error != nil {
		return translate(res.Error, "telegram group")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("telegram group not found")
	}
	return nil
}

// CountActive backs the tariff limit on connected groups.
func (r *telegramGroupRepository) CountActive(ctx context.Context, companyID uuid.UUID) (int64, error) {
	var total int64
	err := r.DB(ctx).Model(&domain.TelegramGroup{}).
		Where("company_id = ? AND active", companyID).
		Count(&total).Error
	if err != nil {
		return 0, translate(err, "telegram group")
	}
	return total, nil
}
