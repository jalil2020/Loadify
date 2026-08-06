package domain

import (
	"context"

	"github.com/google/uuid"
)

// UserContext is the tenancy information stamped into every access token, so
// authorisation checks and repository tenant filters do not need extra queries
// on each request.
type UserContext struct {
	DriverID    *uuid.UUID
	CompanyID   *uuid.UUID
	CompanyRole string // company_manager | dispatcher, empty when not a company member
}

// IdentityRepository resolves the driver and company a user belongs to.
type IdentityRepository interface {
	Resolve(ctx context.Context, userID uuid.UUID) (UserContext, error)
}
