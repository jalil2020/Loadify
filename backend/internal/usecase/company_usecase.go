package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/logger"
)

// CompanyUsecase owns company profiles, membership and verification.
type CompanyUsecase struct {
	companies domain.CompanyRepository
	docs      domain.CompanyDocumentRepository
	users     domain.UserRepository
	roles     domain.RoleRepository
	geo       domain.GeoRepository
	files     *FileUsecase
	tx        domain.TxManager
	audit     domain.AuditLogger
	cache     cache.Cache
	log       logger.Logger
}

// NewCompanyUsecase wires the company usecase.
func NewCompanyUsecase(
	companies domain.CompanyRepository,
	docs domain.CompanyDocumentRepository,
	users domain.UserRepository,
	roles domain.RoleRepository,
	geoRepo domain.GeoRepository,
	files *FileUsecase,
	tx domain.TxManager,
	audit domain.AuditLogger,
	c cache.Cache,
	log logger.Logger,
) *CompanyUsecase {
	return &CompanyUsecase{
		companies: companies, docs: docs, users: users, roles: roles,
		geo: geoRepo, files: files, tx: tx, audit: audit, cache: c, log: log,
	}
}

// Create registers a company and makes the caller its owner and manager.
func (u *CompanyUsecase) Create(ctx context.Context, userID uuid.UUID, req dto.CreateCompanyRequest) (*dto.CompanyResponse, error) {
	// One company per owner keeps tenancy unambiguous: the access token carries
	// a single company_id, and every scoped query relies on that.
	if _, err := u.companies.FindByOwner(ctx, userID); err == nil {
		return nil, apperror.ErrConflict.WithMessage("You already own a company")
	} else if !apperror.Is(err, apperror.ErrNotFound.Code) {
		return nil, err
	}

	if req.TIN != "" {
		if _, err := u.companies.FindByTIN(ctx, req.TIN); err == nil {
			return nil, apperror.ErrConflict.WithMessage("A company with this TIN is already registered").
				WithDetails(apperror.FieldError{Field: "tin", Message: "Already registered"})
		} else if !apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, err
		}
	}
	if req.LogoFileID != nil {
		if err := u.files.AssertOwnership(ctx, userID, *req.LogoFileID); err != nil {
			return nil, err
		}
	}

	role, err := u.roles.FindByCode(ctx, domain.RoleCompanyManager)
	if err != nil {
		return nil, err
	}

	company := &domain.Company{
		ID:                 uuid.New(),
		OwnerUserID:        userID,
		LegalName:          strings.TrimSpace(req.LegalName),
		BrandName:          strings.TrimSpace(req.BrandName),
		TIN:                nilIfBlank(req.TIN),
		RegistrationNumber: nilIfBlank(req.RegistrationNumber),
		Description:        nilIfBlank(req.Description),
		LogoFileID:         req.LogoFileID,
		Phone:              normalisePhone(req.Phone),
		Email:              nilIfBlank(strings.ToLower(req.Email)),
		Website:            nilIfBlank(req.Website),
		EmployeesCount:     req.EmployeesCount,
		FleetSize:          req.FleetSize,
		Status:             domain.CompanyStatusPending,
	}
	if req.FoundedYear != nil {
		y := int16(*req.FoundedYear)
		company.FoundedYear = &y
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if req.Address != nil {
			address, err := u.createAddress(txCtx, *req.Address)
			if err != nil {
				return err
			}
			company.AddressID = &address.ID
		}
		if err := u.companies.Create(txCtx, company); err != nil {
			return err
		}
		now := time.Now().UTC()
		member := &domain.CompanyMember{
			ID:         uuid.New(),
			CompanyID:  company.ID,
			UserID:     userID,
			MemberRole: domain.MemberRoleManager,
			IsActive:   true,
			JoinedAt:   &now,
		}
		if err := u.companies.AddMember(txCtx, member); err != nil {
			return err
		}
		// The role grant is scoped to this company so a manager of one company
		// gains nothing inside another.
		return u.roles.AssignRole(txCtx, userID, role.ID, &company.ID, &userID)
	})
	if err != nil {
		return nil, err
	}

	u.invalidateUser(ctx, userID)
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "company.created",
		ResourceType: "company",
		ResourceID:   &company.ID,
		NewValues:    map[string]any{"brand_name": company.BrandName, "tin": req.TIN},
	})

	return u.Get(ctx, company.ID, true)
}

// Update edits a company the caller manages.
func (u *CompanyUsecase) Update(ctx context.Context, userID, companyID uuid.UUID, req dto.UpdateCompanyRequest) (*dto.CompanyResponse, error) {
	if err := u.assertManager(ctx, userID, companyID); err != nil {
		return nil, err
	}
	if req.LogoFileID != nil {
		if err := u.files.AssertOwnership(ctx, userID, *req.LogoFileID); err != nil {
			return nil, err
		}
	}

	columns := map[string]any{}
	if req.LegalName != "" {
		columns["legal_name"] = strings.TrimSpace(req.LegalName)
	}
	if req.BrandName != "" {
		columns["brand_name"] = strings.TrimSpace(req.BrandName)
	}
	if req.Description != "" {
		columns["description"] = req.Description
	}
	if req.Phone != "" {
		columns["phone"] = normalisePhone(req.Phone)
	}
	if req.Email != "" {
		columns["email"] = strings.ToLower(req.Email)
	}
	if req.Website != "" {
		columns["website"] = req.Website
	}
	if req.LogoFileID != nil {
		columns["logo_file_id"] = *req.LogoFileID
	}
	if req.EmployeesCount != nil {
		columns["employees_count"] = *req.EmployeesCount
	}
	if req.FleetSize != nil {
		columns["fleet_size"] = *req.FleetSize
	}
	if req.FoundedYear != nil {
		columns["founded_year"] = int16(*req.FoundedYear)
	}

	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if req.Address != nil {
			address, err := u.createAddress(txCtx, *req.Address)
			if err != nil {
				return err
			}
			columns["address_id"] = address.ID
		}
		return u.companies.UpdateColumns(txCtx, companyID, columns)
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "company.updated",
		ResourceType: "company",
		ResourceID:   &companyID,
		NewValues:    columns,
	})
	return u.Get(ctx, companyID, true)
}

// Get returns a company. Contact details and documents are private.
func (u *CompanyUsecase) Get(ctx context.Context, companyID uuid.UUID, includePrivate bool) (*dto.CompanyResponse, error) {
	company, err := u.companies.FindWithRelations(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return u.toResponse(ctx, company, includePrivate), nil
}

// GetMine returns the company the caller belongs to.
func (u *CompanyUsecase) GetMine(ctx context.Context, companyID *uuid.UUID) (*dto.CompanyResponse, error) {
	if companyID == nil {
		return nil, apperror.ErrNotFound.WithMessage("You do not belong to a company yet")
	}
	return u.Get(ctx, *companyID, true)
}

// List serves the public company directory and the admin listing.
func (u *CompanyUsecase) List(ctx context.Context, q dto.CompanySearchQuery, offset, limit int, order string, includePrivate bool) ([]dto.CompanyResponse, int64, error) {
	filter := domain.CompanyFilter{Search: q.Search, MinRating: q.MinRating}
	if q.Status != "" {
		filter.Statuses = []domain.CompanyStatus{domain.CompanyStatus(q.Status)}
	}
	if q.RegionID != "" {
		if id, err := uuid.Parse(q.RegionID); err == nil {
			filter.RegionID = &id
		}
	}

	companies, total, err := u.companies.List(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.CompanyResponse, 0, len(companies))
	for i := range companies {
		out = append(out, *u.toResponse(ctx, &companies[i], includePrivate))
	}
	return out, total, nil
}

// ─── documents & verification ───────────────────────────────────────

// UploadDocument attaches a company paper and re-opens verification.
func (u *CompanyUsecase) UploadDocument(ctx context.Context, userID, companyID uuid.UUID, req dto.UploadCompanyDocumentRequest) (*dto.DocumentResponse, error) {
	if err := u.assertManager(ctx, userID, companyID); err != nil {
		return nil, err
	}
	if err := u.files.AssertOwnership(ctx, userID, req.FileID); err != nil {
		return nil, err
	}

	doc := &domain.CompanyDocument{
		ID:        uuid.New(),
		CompanyID: companyID,
		Type:      domain.CompanyDocumentType(req.Type),
		FileID:    req.FileID,
		Number:    nilIfBlank(req.Number),
		IssuedAt:  req.IssuedAt,
		ExpiresAt: req.ExpiresAt,
		Status:    domain.VerificationPending,
	}
	if err := u.docs.Upsert(ctx, doc); err != nil {
		return nil, err
	}

	company, err := u.companies.FindByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if company.Status == domain.CompanyStatusRejected {
		if err := u.companies.UpdateColumns(ctx, companyID, map[string]any{
			"status":           domain.CompanyStatusPending,
			"rejection_reason": nil,
		}); err != nil {
			u.log.Warn("resetting company verification failed", logger.Err(err))
		}
	}

	stored, err := u.docs.FindByID(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	return u.documentResponse(ctx, stored), nil
}

// ListDocuments returns the company's papers.
func (u *CompanyUsecase) ListDocuments(ctx context.Context, userID, companyID uuid.UUID) ([]dto.DocumentResponse, error) {
	if err := u.assertMember(ctx, userID, companyID); err != nil {
		return nil, err
	}
	docs, err := u.docs.ListByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DocumentResponse, 0, len(docs))
	for i := range docs {
		out = append(out, *u.documentResponse(ctx, &docs[i]))
	}
	return out, nil
}

// ReviewDocument records an admin decision and verifies the company once every
// required paper is approved.
func (u *CompanyUsecase) ReviewDocument(ctx context.Context, adminID, docID uuid.UUID, req dto.ReviewDocumentRequest) error {
	doc, err := u.docs.FindByID(ctx, docID)
	if err != nil {
		return err
	}
	status := domain.VerificationStatus(req.Status)
	reason := nilIfBlank(req.Reason)

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.docs.Review(txCtx, docID, status, reason, adminID); err != nil {
			return err
		}
		if status == domain.VerificationRejected {
			return u.companies.UpdateColumns(txCtx, doc.CompanyID, map[string]any{
				"status":           domain.CompanyStatusRejected,
				"rejection_reason": reason,
			})
		}
		approved, err := u.docs.CountApprovedByTypes(txCtx, doc.CompanyID, domain.RequiredCompanyDocuments)
		if err != nil {
			return err
		}
		if int(approved) < len(domain.RequiredCompanyDocuments) {
			return nil
		}
		return u.companies.UpdateColumns(txCtx, doc.CompanyID, map[string]any{
			"status":           domain.CompanyStatusVerified,
			"verified_at":      time.Now().UTC(),
			"verified_by":      adminID,
			"rejection_reason": nil,
		})
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "company.document_reviewed",
		ResourceType: "company_document",
		ResourceID:   &docID,
		NewValues:    map[string]any{"status": req.Status, "reason": req.Reason},
	})
	return nil
}

// Review lets an admin verify or reject a company directly.
func (u *CompanyUsecase) Review(ctx context.Context, adminID, companyID uuid.UUID, req dto.ReviewCompanyRequest) error {
	columns := map[string]any{"status": domain.CompanyStatus(req.Status)}
	if req.Status == string(domain.CompanyStatusVerified) {
		columns["verified_at"] = time.Now().UTC()
		columns["verified_by"] = adminID
		columns["rejection_reason"] = nil
	} else {
		columns["rejection_reason"] = nilIfBlank(req.Reason)
	}
	if err := u.companies.UpdateColumns(ctx, companyID, columns); err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "company.reviewed",
		ResourceType: "company",
		ResourceID:   &companyID,
		NewValues:    map[string]any{"status": req.Status, "reason": req.Reason},
	})
	return nil
}

// ListPendingDocuments serves the admin company verification queue.
func (u *CompanyUsecase) ListPendingDocuments(ctx context.Context, offset, limit int) ([]dto.DocumentResponse, int64, error) {
	docs, total, err := u.docs.ListPending(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.DocumentResponse, 0, len(docs))
	for i := range docs {
		out = append(out, *u.documentResponse(ctx, &docs[i]))
	}
	return out, total, nil
}

// ─── membership ─────────────────────────────────────────────────────

// InviteMember adds an existing account to the company as manager or dispatcher.
//
// The invited person must already have a Loadify account. Creating one on their
// behalf would mean choosing a password for them, which we will not do.
func (u *CompanyUsecase) InviteMember(ctx context.Context, actorID, companyID uuid.UUID, req dto.InviteMemberRequest) (*dto.InviteMemberResponse, error) {
	if err := u.assertManager(ctx, actorID, companyID); err != nil {
		return nil, err
	}

	phone := normalisePhone(req.Phone)
	user, err := u.users.FindByPhone(ctx, phone)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, apperror.ErrNotFound.WithMessage(
				"No Loadify account uses this number. Ask them to register first, then invite them again")
		}
		return nil, err
	}
	if user.ID == actorID {
		return nil, apperror.ErrValidation.WithMessage("You are already a member of this company")
	}
	if existing, err := u.companies.FindMember(ctx, companyID, user.ID); err == nil {
		if existing.IsActive {
			return nil, apperror.ErrConflict.WithMessage("This person is already a member")
		}
		// Re-activating beats creating a duplicate row: the unique index would
		// reject it anyway, and the membership history stays intact.
		now := time.Now().UTC()
		existing.IsActive = true
		existing.MemberRole = domain.MemberRole(req.MemberRole)
		existing.Position = nilIfBlank(req.Position)
		existing.JoinedAt = &now
		if err := u.companies.UpdateMember(ctx, existing); err != nil {
			return nil, err
		}
		if err := u.grantCompanyRole(ctx, user.ID, companyID, domain.MemberRole(req.MemberRole), actorID); err != nil {
			return nil, err
		}
		u.invalidateUser(ctx, user.ID)
		return &dto.InviteMemberResponse{
			Member:  *u.memberResponse(ctx, existing),
			Message: "Membership reactivated",
		}, nil
	} else if !apperror.Is(err, apperror.ErrNotFound.Code) {
		return nil, err
	}

	now := time.Now().UTC()
	member := &domain.CompanyMember{
		ID:         uuid.New(),
		CompanyID:  companyID,
		UserID:     user.ID,
		MemberRole: domain.MemberRole(req.MemberRole),
		Position:   nilIfBlank(req.Position),
		IsActive:   true,
		InvitedBy:  &actorID,
		InvitedAt:  &now,
		JoinedAt:   &now,
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.companies.AddMember(txCtx, member); err != nil {
			return err
		}
		return u.grantCompanyRole(txCtx, user.ID, companyID, member.MemberRole, actorID)
	})
	if err != nil {
		return nil, err
	}

	u.invalidateUser(ctx, user.ID)
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "company.member_invited",
		ResourceType: "company_member",
		ResourceID:   &member.ID,
		NewValues:    map[string]any{"user_id": user.ID, "role": req.MemberRole},
	})

	member.User = user
	return &dto.InviteMemberResponse{
		Member:  *u.memberResponse(ctx, member),
		Message: "Member added",
	}, nil
}

// ListMembers returns the company roster.
func (u *CompanyUsecase) ListMembers(ctx context.Context, userID, companyID uuid.UUID, activeOnly bool) ([]dto.MemberResponse, error) {
	if err := u.assertMember(ctx, userID, companyID); err != nil {
		return nil, err
	}
	members, err := u.companies.ListMembers(ctx, companyID, activeOnly)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MemberResponse, 0, len(members))
	for i := range members {
		out = append(out, *u.memberResponse(ctx, &members[i]))
	}
	return out, nil
}

// UpdateMember changes a member's role, position or activation.
func (u *CompanyUsecase) UpdateMember(ctx context.Context, actorID, companyID, memberID uuid.UUID, req dto.UpdateMemberRequest) (*dto.MemberResponse, error) {
	if err := u.assertManager(ctx, actorID, companyID); err != nil {
		return nil, err
	}
	member, err := u.companies.FindMemberByID(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if member.CompanyID != companyID {
		return nil, apperror.ErrForbidden
	}

	company, err := u.companies.FindByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	// The owner's own membership is structural: demoting or deactivating it
	// would leave the company with no one able to manage it.
	if member.UserID == company.OwnerUserID {
		if req.MemberRole != "" && req.MemberRole != string(domain.MemberRoleManager) {
			return nil, apperror.ErrForbidden.WithMessage("The company owner must remain a manager")
		}
		if req.IsActive != nil && !*req.IsActive {
			return nil, apperror.ErrForbidden.WithMessage("The company owner cannot be deactivated")
		}
	}

	previousRole := member.MemberRole
	if req.MemberRole != "" {
		member.MemberRole = domain.MemberRole(req.MemberRole)
	}
	if req.Position != "" {
		member.Position = nilIfBlank(req.Position)
	}
	if req.IsActive != nil {
		member.IsActive = *req.IsActive
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.companies.UpdateMember(txCtx, member); err != nil {
			return err
		}
		if previousRole != member.MemberRole {
			if err := u.revokeCompanyRole(txCtx, member.UserID, companyID, previousRole); err != nil {
				return err
			}
		}
		if member.IsActive {
			return u.grantCompanyRole(txCtx, member.UserID, companyID, member.MemberRole, actorID)
		}
		return u.revokeCompanyRole(txCtx, member.UserID, companyID, member.MemberRole)
	})
	if err != nil {
		return nil, err
	}

	u.invalidateUser(ctx, member.UserID)
	return u.memberResponse(ctx, member), nil
}

// RemoveMember revokes a person's membership and company-scoped role.
func (u *CompanyUsecase) RemoveMember(ctx context.Context, actorID, companyID, memberID uuid.UUID) error {
	if err := u.assertManager(ctx, actorID, companyID); err != nil {
		return err
	}
	member, err := u.companies.FindMemberByID(ctx, memberID)
	if err != nil {
		return err
	}
	if member.CompanyID != companyID {
		return apperror.ErrForbidden
	}
	company, err := u.companies.FindByID(ctx, companyID)
	if err != nil {
		return err
	}
	if member.UserID == company.OwnerUserID {
		return apperror.ErrForbidden.WithMessage("The company owner cannot be removed")
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.companies.RemoveMember(txCtx, memberID); err != nil {
			return err
		}
		return u.revokeCompanyRole(txCtx, member.UserID, companyID, member.MemberRole)
	})
	if err != nil {
		return err
	}

	u.invalidateUser(ctx, member.UserID)
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "company.member_removed",
		ResourceType: "company_member",
		ResourceID:   &memberID,
	})
	return nil
}

// ─── internals ──────────────────────────────────────────────────────

// assertMember allows any active member of the company.
func (u *CompanyUsecase) assertMember(ctx context.Context, userID, companyID uuid.UUID) error {
	member, err := u.companies.FindMember(ctx, companyID, userID)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrForbidden.WithMessage("You are not a member of this company")
		}
		return err
	}
	if !member.IsActive {
		return apperror.ErrForbidden.WithMessage("Your membership is not active")
	}
	return nil
}

// assertManager allows only managers — dispatchers cannot change the company
// itself or its roster.
func (u *CompanyUsecase) assertManager(ctx context.Context, userID, companyID uuid.UUID) error {
	member, err := u.companies.FindMember(ctx, companyID, userID)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrForbidden.WithMessage("You are not a member of this company")
		}
		return err
	}
	if !member.IsActive || member.MemberRole != domain.MemberRoleManager {
		return apperror.ErrForbidden.WithMessage("Only a company manager can do this")
	}
	return nil
}

func (u *CompanyUsecase) grantCompanyRole(ctx context.Context, userID, companyID uuid.UUID, memberRole domain.MemberRole, grantedBy uuid.UUID) error {
	code := domain.RoleDispatcher
	if memberRole == domain.MemberRoleManager {
		code = domain.RoleCompanyManager
	}
	role, err := u.roles.FindByCode(ctx, code)
	if err != nil {
		return err
	}
	return u.roles.AssignRole(ctx, userID, role.ID, &companyID, &grantedBy)
}

func (u *CompanyUsecase) revokeCompanyRole(ctx context.Context, userID, companyID uuid.UUID, memberRole domain.MemberRole) error {
	code := domain.RoleDispatcher
	if memberRole == domain.MemberRoleManager {
		code = domain.RoleCompanyManager
	}
	role, err := u.roles.FindByCode(ctx, code)
	if err != nil {
		return err
	}
	return u.roles.RevokeRole(ctx, userID, role.ID, &companyID)
}

func (u *CompanyUsecase) createAddress(ctx context.Context, req dto.AddressRequest) (*domain.Address, error) {
	point := geo.NewPoint(req.Location.Lat, req.Location.Lng)
	if !point.Valid() {
		return nil, apperror.ErrValidation.WithMessage("Invalid address coordinates")
	}
	address := &domain.Address{
		ID:            uuid.New(),
		RegionID:      req.RegionID,
		CityID:        req.CityID,
		Line1:         req.Line1,
		Line2:         nilIfBlank(req.Line2),
		PostalCode:    nilIfBlank(req.PostalCode),
		Formatted:     req.Formatted,
		GooglePlaceID: nilIfBlank(req.GooglePlaceID),
		Location:      point,
		ContactName:   nilIfBlank(req.ContactName),
		ContactPhone:  nilIfBlank(req.ContactPhone),
		Notes:         nilIfBlank(req.Notes),
	}
	if address.Formatted == "" {
		address.Formatted = req.Line1
	}
	if err := u.geo.CreateAddress(ctx, address); err != nil {
		return nil, err
	}
	return address, nil
}

func (u *CompanyUsecase) invalidateUser(ctx context.Context, userID uuid.UUID) {
	if err := u.cache.Delete(ctx, cache.UserKey(userID), cache.UserPermissionsKey(userID)); err != nil {
		u.log.Warn("user cache invalidation failed", logger.Err(err))
	}
}

func (u *CompanyUsecase) toResponse(ctx context.Context, c *domain.Company, includePrivate bool) *dto.CompanyResponse {
	res := &dto.CompanyResponse{
		ID:             c.ID,
		OwnerUserID:    c.OwnerUserID,
		LegalName:      c.LegalName,
		BrandName:      c.BrandName,
		Description:    c.Description,
		Website:        c.Website,
		EmployeesCount: c.EmployeesCount,
		FleetSize:      c.FleetSize,
		FoundedYear:    c.FoundedYear,
		Status:         string(c.Status),
		VerifiedAt:     c.VerifiedAt,
		Rating:         c.Rating,
		RatingCount:    c.RatingCount,
		CompletedTrips: c.CompletedTrips,
		CancelledTrips: c.CancelledTrips,
		PublishedLoads: c.PublishedLoads,
		CreatedAt:      c.CreatedAt,
	}
	if c.Logo != nil {
		url := u.files.ResolveURL(ctx, c.Logo)
		res.LogoURL = &url
	} else {
		res.LogoURL = u.files.ResolveURLByID(ctx, c.LogoFileID)
	}
	res.Address = addressResponse(c.Address)

	if includePrivate {
		res.TIN = c.TIN
		res.RegistrationNumber = c.RegistrationNumber
		res.Phone = c.Phone
		res.Email = c.Email
		res.RejectionReason = c.RejectionReason
		for i := range c.Members {
			res.Members = append(res.Members, *u.memberResponse(ctx, &c.Members[i]))
		}
		for i := range c.Docs {
			res.Documents = append(res.Documents, *u.documentResponse(ctx, &c.Docs[i]))
		}
	}
	return res
}

func (u *CompanyUsecase) memberResponse(ctx context.Context, m *domain.CompanyMember) *dto.MemberResponse {
	res := &dto.MemberResponse{
		ID:         m.ID,
		UserID:     m.UserID,
		MemberRole: string(m.MemberRole),
		Position:   m.Position,
		IsActive:   m.IsActive,
		JoinedAt:   m.JoinedAt,
		CreatedAt:  m.CreatedAt,
	}
	if m.User != nil {
		res.FullName = m.User.FullName()
		res.Phone = m.User.Phone
		res.AvatarURL = u.files.ResolveURLByID(ctx, m.User.AvatarFileID)
	}
	return res
}

func (u *CompanyUsecase) documentResponse(ctx context.Context, d *domain.CompanyDocument) *dto.DocumentResponse {
	res := &dto.DocumentResponse{
		ID:              d.ID,
		Type:            string(d.Type),
		Side:            "single",
		FileID:          d.FileID,
		Number:          d.Number,
		IssuedAt:        d.IssuedAt,
		ExpiresAt:       d.ExpiresAt,
		Status:          string(d.Status),
		RejectionReason: d.RejectionReason,
		ReviewedAt:      d.ReviewedAt,
		CreatedAt:       d.CreatedAt,
	}
	if d.File != nil {
		res.URL = u.files.ResolveURL(ctx, d.File)
	} else if url := u.files.ResolveURLByID(ctx, &d.FileID); url != nil {
		res.URL = *url
	}
	return res
}
