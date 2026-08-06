package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ComplaintTargetType is what a complaint is about.
type ComplaintTargetType string

const (
	ComplaintTargetUser    ComplaintTargetType = "user"
	ComplaintTargetDriver  ComplaintTargetType = "driver"
	ComplaintTargetCompany ComplaintTargetType = "company"
	ComplaintTargetLoad    ComplaintTargetType = "load"
	ComplaintTargetTrip    ComplaintTargetType = "trip"
)

// ComplaintStatus is where a complaint sits in moderation.
type ComplaintStatus string

const (
	ComplaintOpen     ComplaintStatus = "open"
	ComplaintInReview ComplaintStatus = "in_review"
	ComplaintResolved ComplaintStatus = "resolved"
	ComplaintRejected ComplaintStatus = "rejected"
)

// Complaint is a report filed by one party against another.
type Complaint struct {
	ID              uuid.UUID           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ReporterUserID  uuid.UUID           `gorm:"type:uuid;not null;index"`
	TargetType      ComplaintTargetType `gorm:"size:16;not null"`
	TargetID        uuid.UUID           `gorm:"type:uuid;not null"`
	TripID          *uuid.UUID          `gorm:"type:uuid"`
	Category        string              `gorm:"size:24;not null"`
	Description     string              `gorm:"not null"`
	EvidenceFileIDs UUIDArray           `gorm:"type:uuid[]"`
	Status          ComplaintStatus     `gorm:"size:16;not null;default:'open'"`
	Priority        string              `gorm:"size:8;not null;default:'medium'"`
	AssignedTo      *uuid.UUID          `gorm:"type:uuid"`
	Resolution      *string
	ResolvedBy      *uuid.UUID `gorm:"type:uuid"`
	ResolvedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time

	Reporter *User `gorm:"foreignKey:ReporterUserID"`
	Trip     *Trip `gorm:"foreignKey:TripID"`
}

// AuditLog is one recorded privileged or state-changing action.
type AuditLog struct {
	ID           int64      `gorm:"primaryKey"`
	ActorUserID  *uuid.UUID `gorm:"type:uuid"`
	ActorRole    *string    `gorm:"size:32"`
	Action       string     `gorm:"size:64;not null"`
	ResourceType string     `gorm:"size:48;not null"`
	ResourceID   *uuid.UUID `gorm:"type:uuid"`
	OldValues    JSONRaw    `gorm:"type:jsonb"`
	NewValues    JSONRaw    `gorm:"type:jsonb"`
	Description  *string
	IPAddress    *string `gorm:"type:inet"`
	UserAgent    *string
	RequestID    *string `gorm:"size:64"`
	CreatedAt    time.Time

	Actor *User `gorm:"foreignKey:ActorUserID"`
}

// TableName pins the audit table.
func (AuditLog) TableName() string { return "audit_logs" }

// DashboardStats is the live marketplace snapshot the admin home shows.
type DashboardStats struct {
	Users             int64 `json:"users"`
	Drivers           int64 `json:"drivers"`
	VerifiedDrivers   int64 `json:"verified_drivers"`
	OnlineDrivers     int64 `json:"online_drivers"`
	Companies         int64 `json:"companies"`
	VerifiedCompanies int64 `json:"verified_companies"`
	PublishedLoads    int64 `json:"published_loads"`
	ActiveTrips       int64 `json:"active_trips"`
	CompletedTrips    int64 `json:"completed_trips"`
	OpenComplaints    int64 `json:"open_complaints"`
	PendingDocuments  int64 `json:"pending_documents"`
	PendingRefunds    int64 `json:"pending_refunds"`
	// GMV is the value of trips completed in the reporting window.
	GMV              int64 `json:"gmv"`
	CommissionEarned int64 `json:"commission_earned"`
	NewUsersToday    int64 `json:"new_users_today"`
	LoadsToday       int64 `json:"loads_today"`
	TripsToday       int64 `json:"trips_today"`
}

// TimeSeriesPoint is one bucket of a chart.
type TimeSeriesPoint struct {
	Date  time.Time `json:"date"`
	Count int64     `json:"count"`
	Value int64     `json:"value"`
}

// CorridorStat describes traffic on one route, which is what tells operations
// where to recruit drivers.
type CorridorStat struct {
	PickupRegion   string  `json:"pickup_region"`
	DeliveryRegion string  `json:"delivery_region"`
	Loads          int64   `json:"loads"`
	Trips          int64   `json:"trips"`
	AvgPrice       int64   `json:"avg_price"`
	FillRate       float64 `json:"fill_rate"`
}

// ─── ports ──────────────────────────────────────────────────────────

// ComplaintFilter narrows the moderation queue.
type ComplaintFilter struct {
	Statuses   []ComplaintStatus
	Category   string
	Priority   string
	AssignedTo *uuid.UUID
	ReporterID *uuid.UUID
	TargetType ComplaintTargetType
	TargetID   *uuid.UUID
}

// ComplaintRepository persists complaints.
type ComplaintRepository interface {
	Create(ctx context.Context, complaint *Complaint) error
	FindByID(ctx context.Context, id uuid.UUID) (*Complaint, error)
	List(ctx context.Context, filter ComplaintFilter, offset, limit int, order string) ([]Complaint, int64, error)
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	CountByStatus(ctx context.Context, statuses []ComplaintStatus) (int64, error)
}

// AuditFilter narrows an audit search.
type AuditFilter struct {
	ActorUserID  *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Range        TimeRange
}

// AuditRepository reads the audit trail. There is no update or delete: an audit
// log that can be edited is not an audit log.
type AuditRepository interface {
	List(ctx context.Context, filter AuditFilter, offset, limit int) ([]AuditLog, int64, error)
	ListForResource(ctx context.Context, resourceType string, resourceID uuid.UUID, limit int) ([]AuditLog, error)
}

// AnalyticsRepository computes the aggregates the admin dashboard and reports need.
type AnalyticsRepository interface {
	Dashboard(ctx context.Context, from, to time.Time) (*DashboardStats, error)
	// Series buckets a metric by day. metric is one of: users, loads, trips, gmv.
	Series(ctx context.Context, metric string, from, to time.Time) ([]TimeSeriesPoint, error)
	TopCorridors(ctx context.Context, from, to time.Time, limit int) ([]CorridorStat, error)
	CompanyReport(ctx context.Context, companyID uuid.UUID, from, to time.Time) (*CompanyReport, error)
	DriverReport(ctx context.Context, driverID uuid.UUID, from, to time.Time) (*DriverReport, error)
}

// CompanyReport is a shipper's own performance summary.
type CompanyReport struct {
	LoadsPublished     int64   `json:"loads_published"`
	LoadsFilled        int64   `json:"loads_filled"`
	FillRate           float64 `json:"fill_rate"`
	TripsCompleted     int64   `json:"trips_completed"`
	TripsCancelled     int64   `json:"trips_cancelled"`
	TotalSpend         int64   `json:"total_spend"`
	AvgPricePerTrip    int64   `json:"avg_price_per_trip"`
	AvgPricePerKm      int64   `json:"avg_price_per_km"`
	TotalDistanceKm    float64 `json:"total_distance_km"`
	AvgTimeToFillHours float64 `json:"avg_time_to_fill_hours"`
	Rating             float64 `json:"rating"`
}

// DriverReport is a driver's own earnings and performance summary.
type DriverReport struct {
	TripsCompleted  int64   `json:"trips_completed"`
	TripsCancelled  int64   `json:"trips_cancelled"`
	TotalEarned     int64   `json:"total_earned"`
	CommissionPaid  int64   `json:"commission_paid"`
	AvgPricePerTrip int64   `json:"avg_price_per_trip"`
	TotalDistanceKm float64 `json:"total_distance_km"`
	AcceptanceRate  float64 `json:"acceptance_rate"`
	Rating          float64 `json:"rating"`
}
