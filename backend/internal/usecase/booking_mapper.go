package usecase

import (
	"context"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// toResponse converts a booking to its API representation.
//
// A booking is only ever returned to its two parties, so both phone numbers are
// included: putting the driver and the shipper in touch is the point of the
// booking existing.
func (u *BookingUsecase) toResponse(ctx context.Context, b *domain.Booking) *dto.BookingResponse {
	res := &dto.BookingResponse{
		ID:                 b.ID,
		LoadID:             b.LoadID,
		Load:               loadSummary(b.Load),
		DriverID:           b.DriverID,
		CompanyID:          b.CompanyID,
		VehicleID:          b.VehicleID,
		Vehicle:            vehicleSummary(b.Vehicle),
		InitiatedBy:        string(b.InitiatedBy),
		Status:             string(b.Status),
		OfferedPrice:       b.OfferedPriceAmount,
		AgreedPrice:        b.AgreedPriceAmount,
		Currency:           b.Currency,
		Message:            b.Message,
		ResponseMessage:    b.ResponseMessage,
		ExpiresAt:          b.ExpiresAt,
		RespondedAt:        b.RespondedAt,
		CancellationReason: b.CancellationReason,
		CreatedAt:          b.CreatedAt,
	}

	if b.Driver != nil {
		res.Driver = driverParty(ctx, u.files, b.Driver)
	}
	if b.Company != nil {
		res.Company = companyParty(ctx, u.files, b.Company)
	}
	if b.Trip != nil {
		res.TripID = &b.Trip.ID
	}
	return res
}

// driverParty projects a driver as a booking counterparty.
func driverParty(ctx context.Context, files *FileUsecase, d *domain.Driver) *dto.BookingParty {
	party := &dto.BookingParty{
		Rating: d.Rating,
		Trips:  d.CompletedTrips,
	}
	if d.User != nil {
		party.UserID = d.User.ID
		party.FullName = d.User.FullName()
		party.AvatarURL = files.ResolveURLByID(ctx, d.User.AvatarFileID)
		phone := d.User.Phone
		party.Phone = &phone
	}
	return party
}

// companyParty projects a company as a booking counterparty.
func companyParty(ctx context.Context, files *FileUsecase, c *domain.Company) *dto.LoadCompanyResponse {
	phone := c.Phone
	party := &dto.LoadCompanyResponse{
		ID:             c.ID,
		BrandName:      c.BrandName,
		Rating:         c.Rating,
		RatingCount:    c.RatingCount,
		CompletedTrips: c.CompletedTrips,
		IsVerified:     c.IsVerified(),
		Phone:          &phone,
	}
	if c.Logo != nil {
		url := files.ResolveURL(ctx, c.Logo)
		party.LogoURL = &url
	} else {
		party.LogoURL = files.ResolveURLByID(ctx, c.LogoFileID)
	}
	return party
}
