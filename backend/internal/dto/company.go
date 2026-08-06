package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateCompanyRequest registers a logistics company for the signed-in user.
type CreateCompanyRequest struct {
	LegalName          string          `json:"legal_name" validate:"required,min=2,max=255"`
	BrandName          string          `json:"brand_name" validate:"required,min=2,max=255"`
	TIN                string          `json:"tin" validate:"omitempty,min=9,max=20"`
	RegistrationNumber string          `json:"registration_number" validate:"omitempty,max=64"`
	Description        string          `json:"description" validate:"omitempty,max=2000"`
	Phone              string          `json:"phone" validate:"required,uzphone"`
	Email              string          `json:"email" validate:"omitempty,email,max=255"`
	Website            string          `json:"website" validate:"omitempty,url,max=255"`
	LogoFileID         *uuid.UUID      `json:"logo_file_id" validate:"omitempty"`
	Address            *AddressRequest `json:"address" validate:"omitempty"`
	EmployeesCount     *int            `json:"employees_count" validate:"omitempty,gte=0"`
	FleetSize          *int            `json:"fleet_size" validate:"omitempty,gte=0"`
	FoundedYear        *int            `json:"founded_year" validate:"omitempty,gte=1900,lte=2100"`
}

// UpdateCompanyRequest edits an existing company.
type UpdateCompanyRequest struct {
	LegalName      string          `json:"legal_name" validate:"omitempty,min=2,max=255"`
	BrandName      string          `json:"brand_name" validate:"omitempty,min=2,max=255"`
	Description    string          `json:"description" validate:"omitempty,max=2000"`
	Phone          string          `json:"phone" validate:"omitempty,uzphone"`
	Email          string          `json:"email" validate:"omitempty,email,max=255"`
	Website        string          `json:"website" validate:"omitempty,url,max=255"`
	LogoFileID     *uuid.UUID      `json:"logo_file_id" validate:"omitempty"`
	Address        *AddressRequest `json:"address" validate:"omitempty"`
	EmployeesCount *int            `json:"employees_count" validate:"omitempty,gte=0"`
	FleetSize      *int            `json:"fleet_size" validate:"omitempty,gte=0"`
	FoundedYear    *int            `json:"founded_year" validate:"omitempty,gte=1900,lte=2100"`
}

// UploadCompanyDocumentRequest attaches a company paper.
type UploadCompanyDocumentRequest struct {
	Type      string     `json:"type" validate:"required,oneof=registration license tax_certificate bank_details other"`
	FileID    uuid.UUID  `json:"file_id" validate:"required"`
	Number    string     `json:"number" validate:"omitempty,max=64"`
	IssuedAt  *time.Time `json:"issued_at" validate:"omitempty"`
	ExpiresAt *time.Time `json:"expires_at" validate:"omitempty"`
}

// InviteMemberRequest adds a dispatcher or manager to the company.
type InviteMemberRequest struct {
	Phone      string `json:"phone" validate:"required,uzphone"`
	MemberRole string `json:"member_role" validate:"required,oneof=company_manager dispatcher"`
	Position   string `json:"position" validate:"omitempty,max=100"`
}

// UpdateMemberRequest changes a member's role or activation.
type UpdateMemberRequest struct {
	MemberRole string `json:"member_role" validate:"omitempty,oneof=company_manager dispatcher"`
	Position   string `json:"position" validate:"omitempty,max=100"`
	IsActive   *bool  `json:"is_active" validate:"omitempty"`
}

// ReviewCompanyRequest is the admin verification decision.
type ReviewCompanyRequest struct {
	Status string `json:"status" validate:"required,oneof=verified rejected"`
	Reason string `json:"reason" validate:"required_if=Status rejected,omitempty,max=500"`
}

// CompanyResponse is a company as returned to clients.
type CompanyResponse struct {
	ID                 uuid.UUID          `json:"id"`
	OwnerUserID        uuid.UUID          `json:"owner_user_id"`
	LegalName          string             `json:"legal_name"`
	BrandName          string             `json:"brand_name"`
	TIN                *string            `json:"tin,omitempty"`
	RegistrationNumber *string            `json:"registration_number,omitempty"`
	Description        *string            `json:"description"`
	LogoURL            *string            `json:"logo_url"`
	Address            *AddressResponse   `json:"address"`
	Phone              string             `json:"phone,omitempty"`
	Email              *string            `json:"email,omitempty"`
	Website            *string            `json:"website"`
	EmployeesCount     *int               `json:"employees_count"`
	FleetSize          *int               `json:"fleet_size"`
	FoundedYear        *int16             `json:"founded_year"`
	Status             string             `json:"status"`
	RejectionReason    *string            `json:"rejection_reason,omitempty"`
	VerifiedAt         *time.Time         `json:"verified_at"`
	Rating             float64            `json:"rating"`
	RatingCount        int                `json:"rating_count"`
	CompletedTrips     int                `json:"completed_trips"`
	CancelledTrips     int                `json:"cancelled_trips"`
	PublishedLoads     int                `json:"published_loads"`
	Members            []MemberResponse   `json:"members,omitempty"`
	Documents          []DocumentResponse `json:"documents,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
}

// MemberResponse is one company member.
type MemberResponse struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	FullName   string     `json:"full_name"`
	Phone      string     `json:"phone"`
	AvatarURL  *string    `json:"avatar_url"`
	MemberRole string     `json:"member_role"`
	Position   *string    `json:"position"`
	IsActive   bool       `json:"is_active"`
	JoinedAt   *time.Time `json:"joined_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// InviteMemberResponse reports the outcome of an invitation.
type InviteMemberResponse struct {
	Member  MemberResponse `json:"member"`
	Message string         `json:"message"`
}

// CompanySearchQuery filters the public company directory.
type CompanySearchQuery struct {
	Search    string  `query:"q" validate:"omitempty,max=100"`
	RegionID  string  `query:"region_id" validate:"omitempty,uuid"`
	MinRating float64 `query:"min_rating" validate:"omitempty,gte=0,lte=5"`
	Status    string  `query:"status" validate:"omitempty,oneof=pending verified rejected blocked"`
}

// ─── reference data ─────────────────────────────────────────────────

// RegionResponse is a localised region.
type RegionResponse struct {
	ID        uuid.UUID      `json:"id"`
	CountryID uuid.UUID      `json:"country_id"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	NameUZ    string         `json:"name_uz"`
	NameRU    string         `json:"name_ru"`
	NameEN    string         `json:"name_en"`
	Center    *PointResponse `json:"center"`
}

// CityResponse is a localised city.
type CityResponse struct {
	ID        uuid.UUID      `json:"id"`
	RegionID  uuid.UUID      `json:"region_id"`
	Name      string         `json:"name"`
	NameUZ    string         `json:"name_uz"`
	NameRU    string         `json:"name_ru"`
	NameEN    string         `json:"name_en"`
	Location  *PointResponse `json:"location"`
	IsCapital bool           `json:"is_capital"`
}

// CountryResponse is a localised country.
type CountryResponse struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	PhoneCode    string    `json:"phone_code"`
	CurrencyCode string    `json:"currency_code"`
	FlagEmoji    *string   `json:"flag_emoji"`
}

// VehicleTypeResponse is one entry of the truck-type vocabulary.
type VehicleTypeResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
