package usecase

import (
	"context"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// toResponse converts a driver entity to its API representation.
//
// includePrivate gates the fields that identify the person: phone number,
// licence number and licence expiry. Public search results must never carry
// them, or the marketplace becomes a contact-scraping target.
func (u *DriverUsecase) toResponse(ctx context.Context, d *domain.Driver, includePrivate bool, distanceKm *float64) *dto.DriverResponse {
	res := &dto.DriverResponse{
		ID:                 d.ID,
		UserID:             d.UserID,
		CompanyID:          d.CompanyID,
		LicenseCategories:  d.LicenseCategories,
		ExperienceYears:    int(d.ExperienceYears),
		Languages:          d.Languages,
		Bio:                d.Bio,
		Status:             string(d.Status),
		VerificationStatus: string(d.VerificationStatus),
		Rating:             d.Rating,
		RatingCount:        d.RatingCount,
		CompletedTrips:     d.CompletedTrips,
		CancelledTrips:     d.CancelledTrips,
		AcceptanceRate:     d.AcceptanceRate,
		LocationUpdatedAt:  d.LocationUpdatedAt,
		DistanceKm:         distanceKm,
		CreatedAt:          d.CreatedAt,
	}
	if res.LicenseCategories == nil {
		res.LicenseCategories = []string{}
	}
	if res.Languages == nil {
		res.Languages = []string{}
	}

	if d.User != nil {
		res.FirstName = d.User.FirstName
		res.LastName = d.User.LastName
		res.FullName = d.User.FullName()
		res.AvatarURL = u.files.ResolveURLByID(ctx, d.User.AvatarFileID)
		if includePrivate {
			res.Phone = d.User.Phone
		}
	}
	if d.HomeCity != nil {
		res.HomeCity = d.HomeCity.NameEN
	}
	if d.CurrentLocation != nil && d.CurrentLocation.Valid() {
		res.CurrentLocation = &dto.PointResponse{
			Lat: d.CurrentLocation.Lat,
			Lng: d.CurrentLocation.Lng,
		}
	}
	if includePrivate {
		res.LicenseNumber = d.LicenseNumber
		res.LicenseExpiresAt = d.LicenseExpiresAt
		res.RejectionReason = d.RejectionReason
		for i := range d.Docs {
			res.Documents = append(res.Documents, *u.documentResponse(ctx, &d.Docs[i]))
		}
	}
	for i := range d.Vehicles {
		res.Vehicles = append(res.Vehicles, *u.vehicleResponse(ctx, &d.Vehicles[i]))
	}
	return res
}

// documentResponse converts a driver document, resolving a download URL.
func (u *DriverUsecase) documentResponse(ctx context.Context, d *domain.DriverDocument) *dto.DocumentResponse {
	res := &dto.DocumentResponse{
		ID:              d.ID,
		Type:            string(d.Type),
		Side:            d.Side,
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

// vehicleResponse converts a vehicle with its photos and trailer.
func (u *DriverUsecase) vehicleResponse(ctx context.Context, v *domain.Vehicle) *dto.VehicleResponse {
	res := &dto.VehicleResponse{
		ID:                      v.ID,
		DriverID:                v.DriverID,
		Type:                    string(v.Type),
		Make:                    v.Make,
		Model:                   v.Model,
		Year:                    v.Year,
		PlateNumber:             v.PlateNumber,
		Color:                   v.Color,
		CapacityKg:              v.CapacityKg,
		VolumeM3:                v.VolumeM3,
		LengthCm:                v.LengthCm,
		WidthCm:                 v.WidthCm,
		HeightCm:                v.HeightCm,
		FuelType:                v.FuelType,
		Axles:                   v.Axles,
		HasADR:                  v.HasADR,
		HasRefrigeration:        v.HasRefrigeration,
		TempMinC:                v.TempMinC,
		TempMaxC:                v.TempMaxC,
		HasTailLift:             v.HasTailLift,
		HasGPS:                  v.HasGPS,
		InsuranceExpiresAt:      v.InsuranceExpiresAt,
		TechInspectionExpiresAt: v.TechInspectionExpiresAt,
		Status:                  string(v.Status),
		RejectionReason:         v.RejectionReason,
		IsPrimary:               v.IsPrimary,
		Photos:                  []dto.FileResponse{},
		CreatedAt:               v.CreatedAt,
	}
	for i := range v.Photos {
		photo := v.Photos[i]
		item := dto.FileResponse{ID: photo.FileID}
		if photo.File != nil {
			item.URL = u.files.ResolveURL(ctx, photo.File)
			item.OriginalName = photo.File.OriginalName
			item.MimeType = photo.File.MimeType
			item.SizeBytes = photo.File.SizeBytes
			item.CreatedAt = photo.File.CreatedAt
		} else if url := u.files.ResolveURLByID(ctx, &photo.FileID); url != nil {
			item.URL = *url
		}
		res.Photos = append(res.Photos, item)
	}
	if v.Trailer != nil {
		res.Trailer = &dto.TrailerResponse{
			ID:          v.Trailer.ID,
			Type:        v.Trailer.Type,
			PlateNumber: v.Trailer.PlateNumber,
			CapacityKg:  v.Trailer.CapacityKg,
			VolumeM3:    v.Trailer.VolumeM3,
			Axles:       v.Trailer.Axles,
		}
	}
	return res
}

// availabilityResponse converts a published availability window.
func (u *DriverUsecase) availabilityResponse(ctx context.Context, a *domain.DriverAvailability) *dto.AvailabilityResponse {
	res := &dto.AvailabilityResponse{
		ID:                 a.ID,
		VehicleID:          a.VehicleID,
		AvailableFrom:      a.AvailableFrom,
		AvailableTo:        a.AvailableTo,
		PreferredRegionIDs: a.PreferredRegionIDs,
		MaxDistanceKm:      a.MaxDistanceKm,
		MinPriceAmount:     a.MinPriceAmount,
		Currency:           a.Currency,
		Notes:              a.Notes,
		IsActive:           a.IsActive,
		CreatedAt:          a.CreatedAt,
	}
	if res.PreferredRegionIDs == nil {
		res.PreferredRegionIDs = domain.UUIDArray{}
	}
	if a.FromAddress != nil {
		res.FromAddress = addressResponse(a.FromAddress)
	}
	_ = ctx
	return res
}

// addressResponse converts a stored address.
func addressResponse(a *domain.Address) *dto.AddressResponse {
	if a == nil {
		return nil
	}
	res := &dto.AddressResponse{
		ID:           a.ID,
		RegionID:     a.RegionID,
		CityID:       a.CityID,
		Line1:        a.Line1,
		Line2:        a.Line2,
		Formatted:    a.Formatted,
		Location:     dto.PointResponse{Lat: a.Location.Lat, Lng: a.Location.Lng},
		ContactName:  a.ContactName,
		ContactPhone: a.ContactPhone,
	}
	if a.Region != nil {
		res.RegionName = a.Region.NameEN
	}
	if a.City != nil {
		res.CityName = a.City.NameEN
	}
	return res
}
