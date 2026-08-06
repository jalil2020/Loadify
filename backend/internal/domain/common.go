// Package domain holds the entities and the ports (interfaces) the usecase
// layer depends on. It imports no framework, no driver and no transport.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Locale is a supported interface language.
type Locale string

const (
	LocaleUZ Locale = "uz"
	LocaleRU Locale = "ru"
	LocaleEN Locale = "en"
)

// Currency codes handled by the platform. Amounts are always minor units.
const (
	CurrencyUZS = "UZS"
	CurrencyUSD = "USD"
)

// Platform is the client type a session was created from.
type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIOS     Platform = "ios"
	PlatformWeb     Platform = "web"
	PlatformUnknown Platform = "unknown"
)

// ParsePlatform maps arbitrary client input onto a known platform.
func ParsePlatform(s string) Platform {
	switch Platform(s) {
	case PlatformAndroid, PlatformIOS, PlatformWeb:
		return Platform(s)
	default:
		return PlatformUnknown
	}
}

// VerificationStatus is the moderation state shared by drivers, companies,
// vehicles and every uploaded document.
type VerificationStatus string

const (
	VerificationPending  VerificationStatus = "pending"
	VerificationApproved VerificationStatus = "approved"
	VerificationRejected VerificationStatus = "rejected"
	VerificationVerified VerificationStatus = "verified"
)

// TxManager runs a function inside a database transaction. Usecases depend on
// this port rather than on *gorm.DB, so multi-repository writes stay atomic
// without leaking the ORM upward.
type TxManager interface {
	// WithinTransaction runs fn with a context carrying the transaction. Any
	// repository called with that context joins the same transaction.
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

// AuditEntry is a single row written to audit_logs.
type AuditEntry struct {
	ActorUserID  *uuid.UUID
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	OldValues    map[string]any
	NewValues    map[string]any
	Description  string
	IPAddress    string
	UserAgent    string
	RequestID    string
}

// AuditLogger records privileged and state-changing actions.
type AuditLogger interface {
	Record(ctx context.Context, entry AuditEntry) error
}

// TimeRange is an inclusive-exclusive period used by filters and reports.
type TimeRange struct {
	From *time.Time
	To   *time.Time
}
