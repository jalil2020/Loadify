package usecase

import (
	"context"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
)

// toResponse converts a trip to its API representation.
//
// Trips are only ever returned to their two parties, so contact details are
// included: the whole point of an active trip is that the driver and the
// dispatcher can reach each other.
func (u *TripUsecase) toResponse(ctx context.Context, t *domain.Trip) *dto.TripResponse {
	res := &dto.TripResponse{
		ID:                 t.ID,
		TripNumber:         t.TripNumber,
		BookingID:          t.BookingID,
		LoadID:             t.LoadID,
		Load:               loadSummary(t.Load),
		DriverID:           t.DriverID,
		CompanyID:          t.CompanyID,
		Vehicle:            vehicleSummary(t.Vehicle),
		Status:             string(t.Status),
		AgreedPrice:        t.AgreedPriceAmount,
		CommissionAmount:   t.CommissionAmount,
		DriverPayout:       t.DriverPayout(),
		Currency:           t.Currency,
		DistancePlannedKm:  t.DistancePlannedKm,
		DistanceActualKm:   t.DistanceActualKm,
		ETAAt:              t.ETAAt,
		StartedAt:          t.StartedAt,
		ArrivedPickupAt:    t.ArrivedPickupAt,
		LoadedAt:           t.LoadedAt,
		ArrivedDeliveryAt:  t.ArrivedDeliveryAt,
		DeliveredAt:        t.DeliveredAt,
		CompletedAt:        t.CompletedAt,
		CancellationReason: t.CancellationReason,
		CreatedAt:          t.CreatedAt,
	}

	if t.Driver != nil {
		res.Driver = driverParty(ctx, u.files, t.Driver)
		if t.Driver.CurrentLocation != nil && t.Driver.CurrentLocation.Valid() && t.Status.IsActive() {
			// The live marker only makes sense while the truck is actually moving
			// on this trip; a finished trip shows its recorded route instead.
			res.CurrentLocation = &dto.PointResponse{
				Lat: t.Driver.CurrentLocation.Lat,
				Lng: t.Driver.CurrentLocation.Lng,
			}
		}
	}
	if t.Company != nil {
		res.Company = companyParty(ctx, u.files, t.Company)
	}

	for i := range t.Events {
		res.Events = append(res.Events, tripEventResponse(&t.Events[i]))
	}
	for i := range t.Documents {
		doc := t.Documents[i]
		item := dto.TripDocumentResponse{
			ID:        doc.ID,
			FileID:    doc.FileID,
			Type:      doc.Type,
			Name:      doc.Name,
			CreatedAt: doc.CreatedAt,
		}
		if doc.File != nil {
			item.URL = u.files.ResolveURL(ctx, doc.File)
		} else if url := u.files.ResolveURLByID(ctx, &doc.FileID); url != nil {
			item.URL = *url
		}
		res.Documents = append(res.Documents, item)
	}
	return res
}

// tripEventResponse converts one timeline entry.
func tripEventResponse(e *domain.TripEvent) dto.TripEventResponse {
	item := dto.TripEventResponse{
		ID:         e.ID,
		Type:       string(e.Type),
		FromStatus: e.FromStatus,
		ToStatus:   e.ToStatus,
		Payload:    map[string]any(e.Payload),
		CreatedAt:  e.CreatedAt,
	}
	if e.Location != nil && e.Location.Valid() {
		item.Location = &dto.PointResponse{Lat: e.Location.Lat, Lng: e.Location.Lng}
	}
	return item
}
