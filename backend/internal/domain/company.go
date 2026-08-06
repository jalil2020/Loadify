package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CompanyStatus is the moderation state of a logistics company.
type CompanyStatus string

const (
	CompanyStatusPending  CompanyStatus = "pending"
	CompanyStatusVerified CompanyStatus = "verified"
	CompanyStatusRejected CompanyStatus = "rejected"
	CompanyStatusBlocked  CompanyStatus = "blocked"
)

// CompanyDocumentType enumerates the papers a company must supply.
type CompanyDocumentType string

const (
	CompanyDocRegistration   CompanyDocumentType = "registration"
	CompanyDocLicense        CompanyDocumentType = "license"
	CompanyDocTaxCertificate CompanyDocumentType = "tax_certificate"
	CompanyDocBankDetails    CompanyDocumentType = "bank_details"
	CompanyDocOther          CompanyDocumentType = "other"
)

// RequiredCompanyDocuments must all be approved before a company is verified.
var RequiredCompanyDocuments = []CompanyDocumentType{CompanyDocRegistration}

// MemberRole is a person's role inside a company.
type MemberRole string

const (
	MemberRoleManager    MemberRole = "company_manager"
	MemberRoleDispatcher MemberRole = "dispatcher"
)

// Company is a logistics company publishing loads.
type Company struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OwnerUserID        uuid.UUID `gorm:"type:uuid;not null;index"`
	LegalName          string    `gorm:"size:255;not null"`
	BrandName          string    `gorm:"size:255;not null"`
	TIN                *string   `gorm:"column:tin;size:20"`
	RegistrationNumber *string   `gorm:"size:64"`
	Description        *string
	LogoFileID         *uuid.UUID `gorm:"type:uuid"`
	AddressID          *uuid.UUID `gorm:"type:uuid"`
	Email              *string    `gorm:"size:255"`
	Phone              string     `gorm:"size:20;not null"`
	Website            *string    `gorm:"size:255"`
	EmployeesCount     *int
	FleetSize          *int
	FoundedYear        *int16
	Status             CompanyStatus `gorm:"size:16;not null;default:'pending'"`
	RejectionReason    *string
	VerifiedAt         *time.Time
	VerifiedBy         *uuid.UUID `gorm:"type:uuid"`
	Rating             float64    `gorm:"type:numeric(3,2);not null;default:0"`
	RatingCount        int        `gorm:"not null;default:0"`
	CompletedTrips     int        `gorm:"not null;default:0"`
	CancelledTrips     int        `gorm:"not null;default:0"`
	PublishedLoads     int        `gorm:"not null;default:0"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`

	Owner   *User             `gorm:"foreignKey:OwnerUserID"`
	Address *Address          `gorm:"foreignKey:AddressID"`
	Logo    *File             `gorm:"foreignKey:LogoFileID"`
	Members []CompanyMember   `gorm:"foreignKey:CompanyID"`
	Docs    []CompanyDocument `gorm:"foreignKey:CompanyID"`
}

// IsVerified reports whether the company may publish loads.
func (c *Company) IsVerified() bool { return c.Status == CompanyStatusVerified }

// CompanyMember links a user to a company with a role.
type CompanyMember struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	MemberRole MemberRole `gorm:"size:24;not null;default:'dispatcher'"`
	Position   *string    `gorm:"size:100"`
	IsActive   bool       `gorm:"not null;default:true"`
	InvitedBy  *uuid.UUID `gorm:"type:uuid"`
	InvitedAt  *time.Time
	JoinedAt   *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`

	User    *User    `gorm:"foreignKey:UserID"`
	Company *Company `gorm:"foreignKey:CompanyID"`
}

// CompanyDocument is one uploaded and moderated company paper.
type CompanyDocument struct {
	ID              uuid.UUID           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       uuid.UUID           `gorm:"type:uuid;not null;index"`
	Type            CompanyDocumentType `gorm:"size:32;not null"`
	FileID          uuid.UUID           `gorm:"type:uuid;not null"`
	Number          *string             `gorm:"size:64"`
	IssuedAt        *time.Time          `gorm:"type:date"`
	ExpiresAt       *time.Time          `gorm:"type:date"`
	Status          VerificationStatus  `gorm:"size:16;not null;default:'pending'"`
	RejectionReason *string
	ReviewedBy      *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	File *File `gorm:"foreignKey:FileID"`
}

// ─── ports ──────────────────────────────────────────────────────────

// CompanyFilter narrows a company listing.
type CompanyFilter struct {
	Search    string
	Statuses  []CompanyStatus
	RegionID  *uuid.UUID
	MinRating float64
}

// CompanyRepository persists companies and their membership.
type CompanyRepository interface {
	Create(ctx context.Context, company *Company) error
	Update(ctx context.Context, company *Company) error
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	FindByID(ctx context.Context, id uuid.UUID) (*Company, error)
	FindWithRelations(ctx context.Context, id uuid.UUID) (*Company, error)
	FindByOwner(ctx context.Context, ownerUserID uuid.UUID) (*Company, error)
	FindByTIN(ctx context.Context, tin string) (*Company, error)
	List(ctx context.Context, filter CompanyFilter, offset, limit int, order string) ([]Company, int64, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	RecalculateRating(ctx context.Context, companyID uuid.UUID) error

	AddMember(ctx context.Context, member *CompanyMember) error
	UpdateMember(ctx context.Context, member *CompanyMember) error
	FindMember(ctx context.Context, companyID, userID uuid.UUID) (*CompanyMember, error)
	FindMemberByID(ctx context.Context, id uuid.UUID) (*CompanyMember, error)
	ListMembers(ctx context.Context, companyID uuid.UUID, activeOnly bool) ([]CompanyMember, error)
	RemoveMember(ctx context.Context, id uuid.UUID) error
	CountMembers(ctx context.Context, companyID uuid.UUID, role MemberRole) (int64, error)
	ActiveMemberUserIDs(ctx context.Context, companyID uuid.UUID) ([]uuid.UUID, error)
}

// CompanyMemberLookup resolves a company to its active members' user ids.
//
// The realtime hub needs this for company-scoped fan-out but has no business
// knowing about the rest of the company repository, so it depends on this
// one-method port instead.
type CompanyMemberLookup interface {
	ActiveMemberUserIDs(ctx context.Context, companyID uuid.UUID) ([]uuid.UUID, error)
}

// CompanyDocumentRepository persists company papers.
type CompanyDocumentRepository interface {
	Upsert(ctx context.Context, doc *CompanyDocument) error
	ListByCompany(ctx context.Context, companyID uuid.UUID) ([]CompanyDocument, error)
	FindByID(ctx context.Context, id uuid.UUID) (*CompanyDocument, error)
	Review(ctx context.Context, id uuid.UUID, status VerificationStatus, reason *string, reviewedBy uuid.UUID) error
	ListPending(ctx context.Context, offset, limit int) ([]CompanyDocument, int64, error)
	CountApprovedByTypes(ctx context.Context, companyID uuid.UUID, types []CompanyDocumentType) (int64, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}
