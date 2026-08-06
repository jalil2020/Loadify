package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

// ─── complaints ─────────────────────────────────────────────────────

type complaintRepository struct{ Base }

// NewComplaintRepository builds the complaint repository.
func NewComplaintRepository(db *gorm.DB) domain.ComplaintRepository {
	return &complaintRepository{Base: NewBase(db)}
}

func (r *complaintRepository) Create(ctx context.Context, complaint *domain.Complaint) error {
	if complaint.EvidenceFileIDs == nil {
		complaint.EvidenceFileIDs = domain.UUIDArray{}
	}
	return translate(r.DB(ctx).Create(complaint).Error, "complaint")
}

func (r *complaintRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Complaint, error) {
	var complaint domain.Complaint
	err := r.DB(ctx).Preload("Reporter").Preload("Trip").
		First(&complaint, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "complaint")
	}
	return &complaint, nil
}

func (r *complaintRepository) List(ctx context.Context, f domain.ComplaintFilter, offset, limit int, order string) ([]domain.Complaint, int64, error) {
	q := r.DB(ctx).Model(&domain.Complaint{})

	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Priority != "" {
		q = q.Where("priority = ?", f.Priority)
	}
	if f.AssignedTo != nil {
		q = q.Where("assigned_to = ?", *f.AssignedTo)
	}
	if f.ReporterID != nil {
		q = q.Where("reporter_user_id = ?", *f.ReporterID)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != nil {
		q = q.Where("target_id = ?", *f.TargetID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "complaint")
	}

	if order == "" {
		// Critical first, then oldest: the queue is worked by urgency, then age.
		order = `CASE priority
			WHEN 'critical' THEN 0 WHEN 'high' THEN 1
			WHEN 'medium' THEN 2 ELSE 3 END, created_at`
	}
	var complaints []domain.Complaint
	err := applyPage(q.Preload("Reporter").Order(order), offset, limit).Find(&complaints).Error
	if err != nil {
		return nil, 0, translate(err, "complaint")
	}
	return complaints, total, nil
}

func (r *complaintRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Complaint{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "complaint")
}

func (r *complaintRepository) CountByStatus(ctx context.Context, statuses []domain.ComplaintStatus) (int64, error) {
	q := r.DB(ctx).Model(&domain.Complaint{})
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, translate(err, "complaint")
	}
	return count, nil
}

// ─── audit ──────────────────────────────────────────────────────────

type auditReadRepository struct{ Base }

// NewAuditReadRepository builds the read side of the audit trail.
func NewAuditReadRepository(db *gorm.DB) domain.AuditRepository {
	return &auditReadRepository{Base: NewBase(db)}
}

func (r *auditReadRepository) List(ctx context.Context, f domain.AuditFilter, offset, limit int) ([]domain.AuditLog, int64, error) {
	q := r.DB(ctx).Model(&domain.AuditLog{})

	if f.ActorUserID != nil {
		q = q.Where("actor_user_id = ?", *f.ActorUserID)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	if f.ResourceID != nil {
		q = q.Where("resource_id = ?", *f.ResourceID)
	}
	if f.Range.From != nil {
		q = q.Where("created_at >= ?", *f.Range.From)
	}
	if f.Range.To != nil {
		q = q.Where("created_at <= ?", *f.Range.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "audit log")
	}

	var logs []domain.AuditLog
	err := applyPage(q.Preload("Actor").Order("created_at DESC"), offset, limit).Find(&logs).Error
	if err != nil {
		return nil, 0, translate(err, "audit log")
	}
	return logs, total, nil
}

// ListForResource returns the history of one object, which is what an
// investigation actually starts from.
func (r *auditReadRepository) ListForResource(ctx context.Context, resourceType string, resourceID uuid.UUID, limit int) ([]domain.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var logs []domain.AuditLog
	err := r.DB(ctx).Preload("Actor").
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, translate(err, "audit log")
	}
	return logs, nil
}

// ─── analytics ──────────────────────────────────────────────────────

type analyticsRepository struct{ Base }

// NewAnalyticsRepository builds the reporting repository.
func NewAnalyticsRepository(db *gorm.DB) domain.AnalyticsRepository {
	return &analyticsRepository{Base: NewBase(db)}
}

// Dashboard collects the admin home figures.
//
// Every count is a scalar subquery in a single statement rather than a dozen
// round trips, because this endpoint is polled by an always-open dashboard.
func (r *analyticsRepository) Dashboard(ctx context.Context, from, to time.Time) (*domain.DashboardStats, error) {
	var stats domain.DashboardStats
	err := r.DB(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM users WHERE deleted_at IS NULL) AS users,
			(SELECT COUNT(*) FROM drivers WHERE deleted_at IS NULL) AS drivers,
			(SELECT COUNT(*) FROM drivers WHERE verification_status = 'verified' AND deleted_at IS NULL) AS verified_drivers,
			(SELECT COUNT(*) FROM drivers WHERE status = 'online' AND deleted_at IS NULL) AS online_drivers,
			(SELECT COUNT(*) FROM companies WHERE deleted_at IS NULL) AS companies,
			(SELECT COUNT(*) FROM companies WHERE status = 'verified' AND deleted_at IS NULL) AS verified_companies,
			(SELECT COUNT(*) FROM loads WHERE status = 'published' AND deleted_at IS NULL) AS published_loads,
			(SELECT COUNT(*) FROM trips WHERE status IN
				('en_route_to_pickup','at_pickup','loaded','in_transit','at_delivery')) AS active_trips,
			(SELECT COUNT(*) FROM trips WHERE status = 'completed') AS completed_trips,
			(SELECT COUNT(*) FROM complaints WHERE status IN ('open','in_review')) AS open_complaints,
			(SELECT COUNT(*) FROM driver_documents WHERE status = 'pending' AND deleted_at IS NULL) AS pending_documents,
			(SELECT COUNT(*) FROM refunds WHERE status = 'pending') AS pending_refunds,
			COALESCE((SELECT SUM(agreed_price_amount) FROM trips
				WHERE status = 'completed' AND completed_at BETWEEN ? AND ?), 0) AS gmv,
			COALESCE((SELECT SUM(commission_amount) FROM trips
				WHERE status = 'completed' AND completed_at BETWEEN ? AND ?), 0) AS commission_earned,
			(SELECT COUNT(*) FROM users WHERE created_at >= date_trunc('day', now())) AS new_users_today,
			(SELECT COUNT(*) FROM loads WHERE created_at >= date_trunc('day', now())) AS loads_today,
			(SELECT COUNT(*) FROM trips WHERE created_at >= date_trunc('day', now())) AS trips_today
	`, from, to, from, to).Scan(&stats).Error
	if err != nil {
		return nil, translate(err, "dashboard")
	}
	return &stats, nil
}

// Series buckets a metric by day.
//
// generate_series produces every day in the window so a quiet day appears as a
// zero rather than a gap the chart would interpolate over.
func (r *analyticsRepository) Series(ctx context.Context, metric string, from, to time.Time) ([]domain.TimeSeriesPoint, error) {
	// The metric name selects a fixed query; it never reaches SQL as text.
	var inner string
	switch metric {
	case "users":
		inner = `SELECT date_trunc('day', created_at) AS d, COUNT(*) AS c, 0::BIGINT AS v
		         FROM users WHERE created_at BETWEEN ? AND ? GROUP BY 1`
	case "loads":
		inner = `SELECT date_trunc('day', created_at) AS d, COUNT(*) AS c, COALESCE(SUM(price_amount),0) AS v
		         FROM loads WHERE created_at BETWEEN ? AND ? GROUP BY 1`
	case "trips":
		inner = `SELECT date_trunc('day', created_at) AS d, COUNT(*) AS c, COALESCE(SUM(agreed_price_amount),0) AS v
		         FROM trips WHERE created_at BETWEEN ? AND ? GROUP BY 1`
	case "gmv":
		inner = `SELECT date_trunc('day', completed_at) AS d, COUNT(*) AS c, COALESCE(SUM(agreed_price_amount),0) AS v
		         FROM trips WHERE status = 'completed' AND completed_at BETWEEN ? AND ? GROUP BY 1`
	default:
		return nil, translate(gorm.ErrRecordNotFound, "metric")
	}

	query := `
		WITH buckets AS (
			SELECT generate_series(date_trunc('day', ?::timestamptz),
			                       date_trunc('day', ?::timestamptz), '1 day') AS day
		), data AS (` + inner + `)
		SELECT buckets.day AS date,
		       COALESCE(data.c, 0) AS count,
		       COALESCE(data.v, 0) AS value
		FROM buckets LEFT JOIN data ON data.d = buckets.day
		ORDER BY buckets.day`

	var points []domain.TimeSeriesPoint
	if err := r.DB(ctx).Raw(query, from, to, from, to).Scan(&points).Error; err != nil {
		return nil, translate(err, "analytics")
	}
	return points, nil
}

// TopCorridors ranks routes by volume, with the fill rate that says whether
// there are enough trucks on each.
func (r *analyticsRepository) TopCorridors(ctx context.Context, from, to time.Time, limit int) ([]domain.CorridorStat, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var stats []domain.CorridorStat
	err := r.DB(ctx).Raw(`
		SELECT
			pr.name_en AS pickup_region,
			dr.name_en AS delivery_region,
			COUNT(l.id) AS loads,
			COUNT(t.id) AS trips,
			COALESCE(AVG(l.price_amount), 0)::BIGINT AS avg_price,
			CASE WHEN COUNT(l.id) = 0 THEN 0
			     ELSE ROUND(COUNT(t.id)::NUMERIC / COUNT(l.id) * 100, 2) END AS fill_rate
		FROM loads l
		JOIN regions pr ON pr.id = l.pickup_region_id
		JOIN regions dr ON dr.id = l.delivery_region_id
		LEFT JOIN trips t ON t.load_id = l.id AND t.status = 'completed'
		WHERE l.created_at BETWEEN ? AND ? AND l.deleted_at IS NULL
		GROUP BY pr.name_en, dr.name_en
		ORDER BY loads DESC
		LIMIT ?`, from, to, limit).Scan(&stats).Error
	if err != nil {
		return nil, translate(err, "analytics")
	}
	return stats, nil
}

// CompanyReport summarises a shipper's own activity.
func (r *analyticsRepository) CompanyReport(ctx context.Context, companyID uuid.UUID, from, to time.Time) (*domain.CompanyReport, error) {
	var report domain.CompanyReport
	err := r.DB(ctx).Raw(`
		SELECT
			COUNT(DISTINCT l.id) FILTER (WHERE l.published_at IS NOT NULL) AS loads_published,
			COUNT(DISTINCT t.load_id) AS loads_filled,
			CASE WHEN COUNT(DISTINCT l.id) FILTER (WHERE l.published_at IS NOT NULL) = 0 THEN 0
			     ELSE ROUND(COUNT(DISTINCT t.load_id)::NUMERIC
			          / COUNT(DISTINCT l.id) FILTER (WHERE l.published_at IS NOT NULL) * 100, 2)
			END AS fill_rate,
			COUNT(t.id) FILTER (WHERE t.status = 'completed') AS trips_completed,
			COUNT(t.id) FILTER (WHERE t.status = 'cancelled') AS trips_cancelled,
			COALESCE(SUM(t.agreed_price_amount) FILTER (WHERE t.status = 'completed'), 0) AS total_spend,
			COALESCE(AVG(t.agreed_price_amount) FILTER (WHERE t.status = 'completed'), 0)::BIGINT AS avg_price_per_trip,
			COALESCE(
				SUM(t.agreed_price_amount) FILTER (WHERE t.status = 'completed')
				/ NULLIF(SUM(COALESCE(t.distance_actual_km, t.distance_planned_km))
				         FILTER (WHERE t.status = 'completed'), 0), 0)::BIGINT AS avg_price_per_km,
			COALESCE(SUM(COALESCE(t.distance_actual_km, t.distance_planned_km))
			         FILTER (WHERE t.status = 'completed'), 0) AS total_distance_km,
			COALESCE(AVG(EXTRACT(EPOCH FROM (t.created_at - l.published_at)) / 3600)
			         FILTER (WHERE l.published_at IS NOT NULL), 0) AS avg_time_to_fill_hours,
			COALESCE((SELECT rating FROM companies WHERE id = ?), 0) AS rating
		FROM loads l
		LEFT JOIN trips t ON t.load_id = l.id
		WHERE l.company_id = ? AND l.created_at BETWEEN ? AND ? AND l.deleted_at IS NULL`,
		companyID, companyID, from, to).Scan(&report).Error
	if err != nil {
		return nil, translate(err, "analytics")
	}
	return &report, nil
}

// DriverReport summarises a driver's earnings.
func (r *analyticsRepository) DriverReport(ctx context.Context, driverID uuid.UUID, from, to time.Time) (*domain.DriverReport, error) {
	var report domain.DriverReport
	err := r.DB(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'completed') AS trips_completed,
			COUNT(*) FILTER (WHERE status = 'cancelled') AS trips_cancelled,
			COALESCE(SUM(agreed_price_amount - commission_amount)
			         FILTER (WHERE status = 'completed'), 0) AS total_earned,
			COALESCE(SUM(commission_amount) FILTER (WHERE status = 'completed'), 0) AS commission_paid,
			COALESCE(AVG(agreed_price_amount) FILTER (WHERE status = 'completed'), 0)::BIGINT AS avg_price_per_trip,
			COALESCE(SUM(COALESCE(distance_actual_km, distance_planned_km))
			         FILTER (WHERE status = 'completed'), 0) AS total_distance_km,
			COALESCE((SELECT acceptance_rate FROM drivers WHERE id = ?), 0) AS acceptance_rate,
			COALESCE((SELECT rating FROM drivers WHERE id = ?), 0) AS rating
		FROM trips
		WHERE driver_id = ? AND created_at BETWEEN ? AND ?`,
		driverID, driverID, driverID, from, to).Scan(&report).Error
	if err != nil {
		return nil, translate(err, "analytics")
	}
	return &report, nil
}
