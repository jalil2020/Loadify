package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// defaultReportWindow is the period a report covers when none is given.
const defaultReportWindow = 30 * 24 * time.Hour

// AdminUsecase owns moderation, platform administration and reporting.
type AdminUsecase struct {
	users      domain.UserRepository
	roles      domain.RoleRepository
	drivers    domain.DriverRepository
	companies  domain.CompanyRepository
	trips      domain.TripRepository
	complaints domain.ComplaintRepository
	auditRead  domain.AuditRepository
	analytics  domain.AnalyticsRepository
	settings   domain.SettingsRepository
	sessions   domain.SessionRepository
	files      *FileUsecase
	settler    EscrowSettler
	tx         domain.TxManager
	audit      domain.AuditLogger
	notifier   domain.Notifier
	log        logger.Logger
}

// NewAdminUsecase wires the admin usecase.
func NewAdminUsecase(
	users domain.UserRepository,
	roles domain.RoleRepository,
	drivers domain.DriverRepository,
	companies domain.CompanyRepository,
	trips domain.TripRepository,
	complaints domain.ComplaintRepository,
	auditRead domain.AuditRepository,
	analytics domain.AnalyticsRepository,
	settings domain.SettingsRepository,
	sessions domain.SessionRepository,
	files *FileUsecase,
	tx domain.TxManager,
	audit domain.AuditLogger,
	notifier domain.Notifier,
	log logger.Logger,
) *AdminUsecase {
	return &AdminUsecase{
		users: users, roles: roles, drivers: drivers, companies: companies,
		trips: trips, complaints: complaints, auditRead: auditRead,
		analytics: analytics, settings: settings, sessions: sessions,
		files: files, tx: tx, audit: audit, notifier: notifier, log: log,
	}
}

// SetSettler attaches the escrow settler, used when a dispute resolution moves
// money. Set after construction to avoid a cycle with the payment usecase.
func (u *AdminUsecase) SetSettler(settler EscrowSettler) { u.settler = settler }

// ─── dashboard & reports ────────────────────────────────────────────

// Dashboard returns the live marketplace snapshot.
func (u *AdminUsecase) Dashboard(ctx context.Context, from, to *time.Time) (*domain.DashboardStats, error) {
	start, end := reportWindow(from, to)
	return u.analytics.Dashboard(ctx, start, end)
}

// Series returns a daily chart for one metric.
func (u *AdminUsecase) Series(ctx context.Context, metric string, from, to *time.Time) ([]domain.TimeSeriesPoint, error) {
	switch metric {
	case "users", "loads", "trips", "gmv":
	default:
		return nil, apperror.ErrValidation.WithMessage(
			"metric must be one of: users, loads, trips, gmv")
	}
	start, end := reportWindow(from, to)
	return u.analytics.Series(ctx, metric, start, end)
}

// TopCorridors ranks routes by volume, which is what tells operations where to
// recruit drivers.
func (u *AdminUsecase) TopCorridors(ctx context.Context, from, to *time.Time, limit int) ([]domain.CorridorStat, error) {
	start, end := reportWindow(from, to)
	return u.analytics.TopCorridors(ctx, start, end, limit)
}

// CompanyReport summarises a shipper's activity. A company may read its own
// report; only an admin may read anyone else's.
func (u *AdminUsecase) CompanyReport(ctx context.Context, companyID uuid.UUID, from, to *time.Time) (*domain.CompanyReport, error) {
	start, end := reportWindow(from, to)
	return u.analytics.CompanyReport(ctx, companyID, start, end)
}

// DriverReport summarises a driver's earnings.
func (u *AdminUsecase) DriverReport(ctx context.Context, driverID uuid.UUID, from, to *time.Time) (*domain.DriverReport, error) {
	start, end := reportWindow(from, to)
	return u.analytics.DriverReport(ctx, driverID, start, end)
}

// ─── user administration ────────────────────────────────────────────

// ListUsers serves the admin user directory.
func (u *AdminUsecase) ListUsers(ctx context.Context, filter domain.UserFilter, offset, limit int, order string) ([]dto.AdminUserResponse, int64, error) {
	users, total, err := u.users.List(ctx, filter, offset, limit, order)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.AdminUserResponse, 0, len(users))
	for i := range users {
		out = append(out, *u.adminUserResponse(ctx, &users[i]))
	}
	return out, total, nil
}

// GetUser returns one account with its roles.
func (u *AdminUsecase) GetUser(ctx context.Context, userID uuid.UUID) (*dto.AdminUserResponse, error) {
	user, err := u.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return u.adminUserResponse(ctx, user), nil
}

// BlockUser suspends an account and kills every live session.
//
// Revoking the sessions is the part that actually stops them: without it a
// blocked user keeps working until their access token expires.
func (u *AdminUsecase) BlockUser(ctx context.Context, adminID, userID uuid.UUID, reason string) error {
	user, err := u.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.ID == adminID {
		return apperror.ErrForbidden.WithMessage("You cannot block your own account")
	}

	now := time.Now().UTC()
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.users.UpdateColumns(txCtx, userID, map[string]any{
			"status":         domain.UserStatusBlocked,
			"blocked_reason": reason,
			"blocked_at":     now,
			"blocked_by":     adminID,
		}); err != nil {
			return err
		}
		return u.sessions.RevokeAllOfUser(txCtx, userID, "account blocked")
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "user.blocked",
		ResourceType: "user",
		ResourceID:   &userID,
		NewValues:    map[string]any{"reason": reason},
	})
	u.notify(ctx, userID, "Account suspended", reason, nil)
	return nil
}

// UnblockUser restores access.
func (u *AdminUsecase) UnblockUser(ctx context.Context, adminID, userID uuid.UUID) error {
	if err := u.users.UpdateColumns(ctx, userID, map[string]any{
		"status":         domain.UserStatusActive,
		"blocked_reason": nil,
		"blocked_at":     nil,
		"blocked_by":     nil,
	}); err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "user.unblocked",
		ResourceType: "user",
		ResourceID:   &userID,
	})
	u.notify(ctx, userID, "Account restored",
		"Your account has been reinstated. Welcome back.", nil)
	return nil
}

// AssignRole grants a platform role.
//
// Only a super admin may reach this, because granting `admin` is the one action
// that lets somebody grant themselves everything else.
func (u *AdminUsecase) AssignRole(ctx context.Context, adminID, userID uuid.UUID, roleCode string, companyID *uuid.UUID) error {
	role, err := u.roles.FindByCode(ctx, roleCode)
	if err != nil {
		return err
	}
	if err := u.roles.AssignRole(ctx, userID, role.ID, companyID, &adminID); err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "user.role_assigned",
		ResourceType: "user",
		ResourceID:   &userID,
		NewValues:    map[string]any{"role": roleCode, "company_id": companyID},
	})
	return nil
}

// RevokeRole removes a platform role.
func (u *AdminUsecase) RevokeRole(ctx context.Context, adminID, userID uuid.UUID, roleCode string, companyID *uuid.UUID) error {
	role, err := u.roles.FindByCode(ctx, roleCode)
	if err != nil {
		return err
	}
	if userID == adminID && (roleCode == domain.RoleAdmin || roleCode == domain.RoleSuperAdmin) {
		return apperror.ErrForbidden.WithMessage("You cannot remove your own administrator role")
	}
	if err := u.roles.RevokeRole(ctx, userID, role.ID, companyID); err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "user.role_revoked",
		ResourceType: "user",
		ResourceID:   &userID,
		NewValues:    map[string]any{"role": roleCode},
	})
	return nil
}

// ─── complaints ─────────────────────────────────────────────────────

// FileComplaint records a report from a user.
func (u *AdminUsecase) FileComplaint(ctx context.Context, userID uuid.UUID, req dto.FileComplaintRequest) (*dto.ComplaintResponse, error) {
	for _, fileID := range req.EvidenceFileIDs {
		if err := u.files.AssertOwnership(ctx, userID, fileID); err != nil {
			return nil, err
		}
	}

	complaint := &domain.Complaint{
		ID:              uuid.New(),
		ReporterUserID:  userID,
		TargetType:      domain.ComplaintTargetType(req.TargetType),
		TargetID:        req.TargetID,
		TripID:          req.TripID,
		Category:        req.Category,
		Description:     req.Description,
		EvidenceFileIDs: domain.UUIDArray(req.EvidenceFileIDs),
		Status:          domain.ComplaintOpen,
		Priority:        "medium",
	}
	// Fraud and damage claims involve money or safety, so they jump the queue.
	if req.Category == "fraud" || req.Category == "damage" {
		complaint.Priority = "high"
	}

	if err := u.complaints.Create(ctx, complaint); err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "complaint.filed",
		ResourceType: "complaint",
		ResourceID:   &complaint.ID,
		NewValues:    map[string]any{"category": req.Category, "target": req.TargetType},
	})
	return u.complaintResponse(ctx, complaint), nil
}

// ListComplaints serves the moderation queue.
func (u *AdminUsecase) ListComplaints(ctx context.Context, q dto.ComplaintSearchQuery, offset, limit int) ([]dto.ComplaintResponse, int64, error) {
	filter := domain.ComplaintFilter{
		Category:   q.Category,
		Priority:   q.Priority,
		TargetType: domain.ComplaintTargetType(q.TargetType),
	}
	for _, s := range q.Statuses {
		filter.Statuses = append(filter.Statuses, domain.ComplaintStatus(s))
	}
	if q.OpenOnly {
		filter.Statuses = []domain.ComplaintStatus{domain.ComplaintOpen, domain.ComplaintInReview}
	}
	if q.AssignedTo != "" {
		if id, err := uuid.Parse(q.AssignedTo); err == nil {
			filter.AssignedTo = &id
		}
	}

	complaints, total, err := u.complaints.List(ctx, filter, offset, limit, "")
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.ComplaintResponse, 0, len(complaints))
	for i := range complaints {
		out = append(out, *u.complaintResponse(ctx, &complaints[i]))
	}
	return out, total, nil
}

// GetComplaint returns one complaint. The reporter may read their own; anyone
// else needs the moderation permission, which the route enforces.
func (u *AdminUsecase) GetComplaint(ctx context.Context, complaintID uuid.UUID) (*dto.ComplaintResponse, error) {
	complaint, err := u.complaints.FindByID(ctx, complaintID)
	if err != nil {
		return nil, err
	}
	return u.complaintResponse(ctx, complaint), nil
}

// AssignComplaint puts a complaint on a moderator's desk.
func (u *AdminUsecase) AssignComplaint(ctx context.Context, adminID, complaintID, assigneeID uuid.UUID) error {
	if err := u.complaints.UpdateColumns(ctx, complaintID, map[string]any{
		"assigned_to": assigneeID,
		"status":      domain.ComplaintInReview,
	}); err != nil {
		return err
	}
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "complaint.assigned",
		ResourceType: "complaint",
		ResourceID:   &complaintID,
		NewValues:    map[string]any{"assigned_to": assigneeID},
	})
	return nil
}

// ResolveComplaint closes a complaint and applies whatever it decided.
//
// A dispute over a trip can move money, so the resolution optionally settles the
// escrow in the same action rather than leaving it to a second manual step.
func (u *AdminUsecase) ResolveComplaint(ctx context.Context, adminID, complaintID uuid.UUID, req dto.ResolveComplaintRequest) error {
	complaint, err := u.complaints.FindByID(ctx, complaintID)
	if err != nil {
		return err
	}
	if complaint.Status == domain.ComplaintResolved || complaint.Status == domain.ComplaintRejected {
		return apperror.ErrConflict.WithMessage("This complaint is already closed")
	}

	status := domain.ComplaintResolved
	if !req.Upheld {
		status = domain.ComplaintRejected
	}

	if err := u.complaints.UpdateColumns(ctx, complaintID, map[string]any{
		"status":      status,
		"resolution":  req.Resolution,
		"resolved_by": adminID,
		"resolved_at": time.Now().UTC(),
	}); err != nil {
		return err
	}

	// Settling the escrow happens after the complaint is closed: the decision is
	// recorded either way, and a settlement failure is retryable.
	if complaint.TripID != nil && u.settler != nil {
		switch req.Settlement {
		case "refund":
			if err := u.settler.RefundEscrow(ctx, *complaint.TripID, &adminID, req.Resolution); err != nil {
				u.log.Error("refunding escrow on a dispute resolution failed",
					logger.String("complaint_id", complaintID.String()), logger.Err(err))
			}
		case "release":
			if err := u.settler.ReleaseEscrow(ctx, *complaint.TripID, &adminID); err != nil {
				u.log.Error("releasing escrow on a dispute resolution failed",
					logger.String("complaint_id", complaintID.String()), logger.Err(err))
			}
		}
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "complaint.resolved",
		ResourceType: "complaint",
		ResourceID:   &complaintID,
		NewValues: map[string]any{
			"upheld":     req.Upheld,
			"resolution": req.Resolution,
			"settlement": req.Settlement,
		},
	})
	u.notify(ctx, complaint.ReporterUserID, "Your report was reviewed", req.Resolution,
		map[string]any{"complaint_id": complaintID.String()})
	return nil
}

// ─── audit ──────────────────────────────────────────────────────────

// ListAuditLogs searches the audit trail.
func (u *AdminUsecase) ListAuditLogs(ctx context.Context, q dto.AuditSearchQuery, offset, limit int) ([]dto.AuditLogResponse, int64, error) {
	filter := domain.AuditFilter{
		Action:       q.Action,
		ResourceType: q.ResourceType,
		Range:        domain.TimeRange{From: q.From, To: q.To},
	}
	if q.ActorUserID != "" {
		if id, err := uuid.Parse(q.ActorUserID); err == nil {
			filter.ActorUserID = &id
		}
	}
	if q.ResourceID != "" {
		if id, err := uuid.Parse(q.ResourceID); err == nil {
			filter.ResourceID = &id
		}
	}

	logs, total, err := u.auditRead.List(ctx, filter, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.AuditLogResponse, 0, len(logs))
	for i := range logs {
		out = append(out, auditLogResponse(&logs[i]))
	}
	return out, total, nil
}

// ResourceHistory returns the audit trail of one object, which is where an
// investigation actually starts.
func (u *AdminUsecase) ResourceHistory(ctx context.Context, resourceType string, resourceID uuid.UUID, limit int) ([]dto.AuditLogResponse, error) {
	logs, err := u.auditRead.ListForResource(ctx, resourceType, resourceID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.AuditLogResponse, 0, len(logs))
	for i := range logs {
		out = append(out, auditLogResponse(&logs[i]))
	}
	return out, nil
}

// ─── settings & flags ───────────────────────────────────────────────

// ListSettings returns the operator-tunable values.
func (u *AdminUsecase) ListSettings(ctx context.Context, group string, publicOnly bool) ([]dto.SettingResponse, error) {
	settings, err := u.settings.List(ctx, group, publicOnly)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SettingResponse, 0, len(settings))
	for i := range settings {
		out = append(out, dto.SettingResponse{
			Key:         settings[i].Key,
			Value:       settings[i].Value.String(),
			ValueType:   settings[i].ValueType,
			Group:       settings[i].GroupName,
			Description: settings[i].Description,
			IsPublic:    settings[i].IsPublic,
			UpdatedAt:   settings[i].UpdatedAt,
		})
	}
	return out, nil
}

// UpdateSetting changes one tunable value.
func (u *AdminUsecase) UpdateSetting(ctx context.Context, adminID uuid.UUID, key string, value any) error {
	previous, err := u.settings.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := u.settings.Set(ctx, key, value, &adminID); err != nil {
		return err
	}

	// Settings change how the platform behaves for everyone, so the previous
	// value is recorded alongside the new one.
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "settings.updated",
		ResourceType: "system_setting",
		ResourceID:   &previous.ID,
		OldValues:    map[string]any{"key": key, "value": previous.Value.String()},
		NewValues:    map[string]any{"key": key, "value": value},
	})
	return nil
}

// ListFeatureFlags returns the deploy-free capability switches.
func (u *AdminUsecase) ListFeatureFlags(ctx context.Context) ([]dto.FeatureFlagResponse, error) {
	flags, err := u.settings.ListFlags(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.FeatureFlagResponse, 0, len(flags))
	for i := range flags {
		out = append(out, dto.FeatureFlagResponse{
			Key:            flags[i].Key,
			Description:    flags[i].Description,
			IsEnabled:      flags[i].IsEnabled,
			RolloutPercent: flags[i].RolloutPercent,
		})
	}
	return out, nil
}

// SetFeatureFlag toggles a capability.
func (u *AdminUsecase) SetFeatureFlag(ctx context.Context, adminID uuid.UUID, key string, enabled bool, rolloutPercent int) error {
	if err := u.settings.SetFlag(ctx, key, enabled, int16(rolloutPercent)); err != nil {
		return err
	}
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "feature_flag.updated",
		ResourceType: "feature_flag",
		NewValues:    map[string]any{"key": key, "enabled": enabled, "rollout": rolloutPercent},
	})
	return nil
}

// ─── internals ──────────────────────────────────────────────────────

func (u *AdminUsecase) adminUserResponse(ctx context.Context, user *domain.User) *dto.AdminUserResponse {
	res := &dto.AdminUserResponse{
		ID:              user.ID,
		Phone:           user.Phone,
		Email:           user.Email,
		FullName:        user.FullName(),
		AvatarURL:       u.files.ResolveURLByID(ctx, user.AvatarFileID),
		Locale:          string(user.Locale),
		Status:          string(user.Status),
		BlockedReason:   user.BlockedReason,
		PhoneVerifiedAt: user.PhoneVerifiedAt,
		LastLoginAt:     user.LastLoginAt,
		CreatedAt:       user.CreatedAt,
		Roles:           []string{},
	}
	if roles, err := u.roles.RolesOfUser(ctx, user.ID); err == nil {
		for _, r := range roles {
			if r.Role != nil {
				res.Roles = append(res.Roles, r.Role.Code)
			}
		}
	}
	return res
}

func (u *AdminUsecase) complaintResponse(ctx context.Context, c *domain.Complaint) *dto.ComplaintResponse {
	res := &dto.ComplaintResponse{
		ID:          c.ID,
		TargetType:  string(c.TargetType),
		TargetID:    c.TargetID,
		TripID:      c.TripID,
		Category:    c.Category,
		Description: c.Description,
		Status:      string(c.Status),
		Priority:    c.Priority,
		AssignedTo:  c.AssignedTo,
		Resolution:  c.Resolution,
		ResolvedAt:  c.ResolvedAt,
		CreatedAt:   c.CreatedAt,
		Evidence:    []dto.FileResponse{},
	}
	if c.Reporter != nil {
		res.ReporterName = c.Reporter.FullName()
		res.ReporterID = c.Reporter.ID
	}
	for _, fileID := range c.EvidenceFileIDs {
		item := dto.FileResponse{ID: fileID}
		if url := u.files.ResolveURLByID(ctx, &fileID); url != nil {
			item.URL = *url
		}
		res.Evidence = append(res.Evidence, item)
	}
	return res
}

func (u *AdminUsecase) notify(ctx context.Context, userID uuid.UUID, title, body string, data map[string]any) {
	if u.notifier == nil {
		return
	}
	if err := u.notifier.Notify(ctx, domain.Notification{
		UserID: userID, Type: "system", Title: title, Body: body,
		Data: data, Priority: "high",
	}); err != nil {
		u.log.Warn("queueing an admin notification failed", logger.Err(err))
	}
}

func auditLogResponse(l *domain.AuditLog) dto.AuditLogResponse {
	res := dto.AuditLogResponse{
		ID:           l.ID,
		Action:       l.Action,
		ResourceType: l.ResourceType,
		ResourceID:   l.ResourceID,
		Description:  l.Description,
		CreatedAt:    l.CreatedAt,
	}
	if l.ActorUserID != nil {
		res.ActorID = l.ActorUserID
	}
	if l.Actor != nil {
		res.ActorName = l.Actor.FullName()
	}
	if l.ActorRole != nil {
		res.ActorRole = *l.ActorRole
	}
	if len(l.OldValues) > 0 {
		res.OldValues = l.OldValues.String()
	}
	if len(l.NewValues) > 0 {
		res.NewValues = l.NewValues.String()
	}
	if l.IPAddress != nil {
		res.IPAddress = *l.IPAddress
	}
	return res
}

// reportWindow defaults an unspecified period to the last 30 days.
func reportWindow(from, to *time.Time) (time.Time, time.Time) {
	end := time.Now().UTC()
	if to != nil {
		end = to.UTC()
	}
	start := end.Add(-defaultReportWindow)
	if from != nil {
		start = from.UTC()
	}
	return start, end
}
