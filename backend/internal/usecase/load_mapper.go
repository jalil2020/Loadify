package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// toResponse converts a load entity to its API representation.
//
// The shipper's phone number is included here and stripped by the caller when
// the viewer has not earned contact access; keeping the mapper unconditional
// means one place decides shape and one place decides visibility.
func (u *LoadUsecase) toResponse(ctx context.Context, l *domain.Load, pickupDistanceKm *float64) *dto.LoadResponse {
	res := &dto.LoadResponse{
		ID:                    l.ID,
		ReferenceNo:           l.ReferenceNo,
		CompanyID:             l.CompanyID,
		Title:                 l.Title,
		Description:           l.Description,
		CargoTypeID:           l.CargoTypeID,
		WeightKg:              l.WeightKg,
		VolumeM3:              l.VolumeM3,
		LengthCm:              l.LengthCm,
		WidthCm:               l.WidthCm,
		HeightCm:              l.HeightCm,
		Packaging:             l.Packaging,
		Quantity:              l.Quantity,
		PickupFrom:            l.PickupFrom,
		PickupTo:              l.PickupTo,
		DeliveryFrom:          l.DeliveryFrom,
		DeliveryTo:            l.DeliveryTo,
		DistanceKm:            l.DistanceKm,
		PickupDistanceKm:      pickupDistanceKm,
		RequiredCapacityKg:    l.RequiredCapacityKg,
		RequiresADR:           l.RequiresADR,
		RequiresRefrigeration: l.RequiresRefrigeration,
		TempMinC:              l.TempMinC,
		TempMaxC:              l.TempMaxC,
		RequiresTailLift:      l.RequiresTailLift,
		LoadingType:           l.LoadingType,
		SpecialRequirements:   l.SpecialRequirements,
		PriceAmount:           l.PriceAmount,
		PriceCurrency:         l.PriceCurrency,
		PriceType:             string(l.PriceType),
		PricePerKm:            l.PricePerKm(),
		PaymentTerms:          string(l.PaymentTerms),
		PaymentMethod:         l.PaymentMethod,
		PrepaymentPercent:     l.PrepaymentPercent,
		DeferredDays:          l.DeferredDays,
		Status:                string(l.Status),
		PublishedAt:           l.PublishedAt,
		ExpiresAt:             l.ExpiresAt,
		CancellationReason:    l.CancellationReason,
		ViewsCount:            l.ViewsCount,
		ReservationsCount:     l.ReservationsCount,
		IsFeatured:            l.IsFeatured,
		IsUrgent:              l.IsUrgent,
		CreatedAt:             l.CreatedAt,
		UpdatedAt:             l.UpdatedAt,
	}

	if l.RequiredVehicleType != nil {
		vt := string(*l.RequiredVehicleType)
		res.RequiredVehicleType = &vt
	}
	if l.CargoType != nil {
		name := l.CargoType.NameEN
		res.CargoType = &name
	}
	res.PickupAddress = addressResponse(l.PickupAddress)
	res.DeliveryAddress = addressResponse(l.DeliveryAddress)

	if l.Company != nil {
		res.Company = &dto.LoadCompanyResponse{
			ID:             l.Company.ID,
			BrandName:      l.Company.BrandName,
			Rating:         l.Company.Rating,
			RatingCount:    l.Company.RatingCount,
			CompletedTrips: l.Company.CompletedTrips,
			IsVerified:     l.Company.IsVerified(),
			Phone:          &l.Company.Phone,
		}
		if l.Company.Logo != nil {
			url := u.files.ResolveURL(ctx, l.Company.Logo)
			res.Company.LogoURL = &url
		} else {
			res.Company.LogoURL = u.files.ResolveURLByID(ctx, l.Company.LogoFileID)
		}
	}

	for i := range l.Photos {
		res.Photos = append(res.Photos, fileResponseFrom(ctx, u.files, l.Photos[i].FileID, l.Photos[i].File))
	}
	for i := range l.Documents {
		res.Documents = append(res.Documents, fileResponseFrom(ctx, u.files, l.Documents[i].FileID, l.Documents[i].File))
	}
	return res
}

// loadSummary is the compact projection embedded in bookings and trips.
func loadSummary(l *domain.Load) *dto.LoadSummary {
	if l == nil {
		return nil
	}
	summary := &dto.LoadSummary{
		ID:            l.ID,
		ReferenceNo:   l.ReferenceNo,
		Title:         l.Title,
		Status:        string(l.Status),
		WeightKg:      l.WeightKg,
		PickupFrom:    l.PickupFrom,
		PriceAmount:   l.PriceAmount,
		PriceCurrency: l.PriceCurrency,
		DistanceKm:    l.DistanceKm,
	}
	if l.PickupAddress != nil {
		summary.PickupFormatted = l.PickupAddress.Formatted
	}
	if l.DeliveryAddress != nil {
		summary.DeliveryFormatted = l.DeliveryAddress.Formatted
	}
	return summary
}

// fileResponseFrom builds a file payload, using the preloaded row when present
// and falling back to a lookup by id.
func fileResponseFrom(ctx context.Context, files *FileUsecase, fileID uuid.UUID, file *domain.File) dto.FileResponse {
	item := dto.FileResponse{ID: fileID}
	if file != nil {
		item.URL = files.ResolveURL(ctx, file)
		item.OriginalName = file.OriginalName
		item.MimeType = file.MimeType
		item.SizeBytes = file.SizeBytes
		item.CreatedAt = file.CreatedAt
		return item
	}
	if url := files.ResolveURLByID(ctx, &fileID); url != nil {
		item.URL = *url
	}
	return item
}

// vehicleSummary is the compact truck projection used on bookings and trips.
func vehicleSummary(v *domain.Vehicle) *dto.VehicleSummary {
	if v == nil {
		return nil
	}
	return &dto.VehicleSummary{
		ID:          v.ID,
		Type:        string(v.Type),
		Make:        v.Make,
		Model:       v.Model,
		PlateNumber: v.PlateNumber,
		CapacityKg:  v.CapacityKg,
		VolumeM3:    v.VolumeM3,
	}
}
