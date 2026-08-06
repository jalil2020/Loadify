// Package route registers every HTTP endpoint. Route files are the map of the
// API: one place to see what exists and what protects it.
package route

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/delivery/http/handler"
	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/logger"
)

// Handlers bundles every handler the router needs.
type Handlers struct {
	Health    *handler.HealthHandler
	Auth      *handler.AuthHandler
	Driver    *handler.DriverHandler
	Company   *handler.CompanyHandler
	File      *handler.FileHandler
	Reference *handler.ReferenceHandler
	Load      *handler.LoadHandler
	Booking   *handler.BookingHandler
	Trip      *handler.TripHandler
}

// Deps is everything route registration needs beyond the handlers.
type Deps struct {
	Cfg   *config.Config
	Auth  *middleware.AuthMiddleware
	Cache cache.Cache
	Log   logger.Logger
}

// Register mounts all routes on the app.
func Register(app *fiber.App, h Handlers, d Deps) {
	app.Get("/health", h.Health.Live)
	app.Get("/health/ready", h.Health.Ready)

	v1 := app.Group("/api/v1")

	registerAuthRoutes(v1, h, d)
	registerReferenceRoutes(v1, h, d)
	registerFileRoutes(v1, h, d)
	registerDriverRoutes(v1, h, d)
	registerCompanyRoutes(v1, h, d)
	registerLoadRoutes(v1, h, d)
	registerBookingRoutes(v1, h, d)
	registerTripRoutes(v1, h, d)
	registerAdminRoutes(v1, h, d)
}

// registerAuthRoutes mounts /api/v1/auth.
//
// The unauthenticated endpoints carry their own, much tighter rate limits:
// credential and OTP endpoints are the ones worth brute-forcing.
func registerAuthRoutes(v1 fiber.Router, h Handlers, d Deps) {
	auth := v1.Group("/auth")

	strict := middleware.RateLimit(d.Cache, d.Log, "auth_strict", 10, time.Minute)
	otp := middleware.RateLimit(d.Cache, d.Log, "auth_otp", 5, 10*time.Minute)

	auth.Post("/register", otp, h.Auth.Register)
	auth.Post("/verify-otp", strict, h.Auth.VerifyOTP)
	auth.Post("/resend-otp", otp, h.Auth.ResendOTP)
	auth.Post("/login", strict, h.Auth.Login)
	auth.Post("/refresh", strict, h.Auth.Refresh)
	auth.Post("/forgot-password", otp, h.Auth.ForgotPassword)
	auth.Post("/reset-password", strict, h.Auth.ResetPassword)

	protected := auth.Group("", d.Auth.Authenticate())
	protected.Get("/me", h.Auth.Me)
	protected.Post("/logout", h.Auth.Logout)
	protected.Post("/logout-all", h.Auth.LogoutAll)
	protected.Post("/change-password", strict, h.Auth.ChangePassword)
	protected.Get("/sessions", h.Auth.Sessions)
	protected.Delete("/sessions/:id", h.Auth.RevokeSession)
}

// registerReferenceRoutes mounts the public vocabularies. These are cached and
// unauthenticated because a client needs them to render the sign-up form.
func registerReferenceRoutes(v1 fiber.Router, h Handlers, _ Deps) {
	ref := v1.Group("/reference")
	ref.Get("/countries", h.Reference.Countries)
	ref.Get("/regions", h.Reference.Regions)
	ref.Get("/cities", h.Reference.Cities)
	ref.Get("/vehicle-types", h.Reference.VehicleTypes)
	ref.Get("/cargo-types", h.Load.CargoTypes)
}

// registerFileRoutes mounts uploads. Uploads are expensive, so they get their
// own limit well below the global one.
func registerFileRoutes(v1 fiber.Router, h Handlers, d Deps) {
	files := v1.Group("/files", d.Auth.Authenticate())
	uploadLimit := middleware.RateLimit(d.Cache, d.Log, "upload", 60, time.Hour)

	files.Post("/", uploadLimit, h.File.Upload)
	files.Post("/batch", uploadLimit, h.File.UploadMany)
	files.Get("/:id", h.File.Get)
	files.Delete("/:id", h.File.Delete)
}

// registerDriverRoutes mounts /api/v1/drivers.
func registerDriverRoutes(v1 fiber.Router, h Handlers, d Deps) {
	drivers := v1.Group("/drivers")

	// Browsing drivers is a company-side capability; the optional auth lets an
	// anonymous visitor see the marketplace without contact details.
	drivers.Get("/", d.Auth.Optional(), h.Driver.Search)

	me := drivers.Group("/me", d.Auth.Authenticate())
	me.Get("/", h.Driver.GetMyProfile)
	me.Put("/", d.Auth.RequirePermission("drivers.update"), h.Driver.SaveProfile)
	me.Patch("/status", d.Auth.RequirePermission("drivers.update"), h.Driver.SetStatus)
	me.Post("/location", d.Auth.RequireDriver(), h.Driver.UpdateLocation)

	me.Get("/documents", h.Driver.ListDocuments)
	me.Post("/documents", d.Auth.RequirePermission("drivers.update"), h.Driver.UploadDocument)

	me.Get("/vehicles", h.Driver.ListVehicles)
	me.Post("/vehicles", d.Auth.RequirePermission("vehicles.create"), h.Driver.AddVehicle)
	me.Put("/vehicles/:id", d.Auth.RequirePermission("vehicles.update"), h.Driver.UpdateVehicle)
	me.Post("/vehicles/:id/primary", d.Auth.RequirePermission("vehicles.update"), h.Driver.SetPrimaryVehicle)
	me.Delete("/vehicles/:id", d.Auth.RequirePermission("vehicles.delete"), h.Driver.DeleteVehicle)

	me.Get("/availability", h.Driver.ListAvailability)
	me.Post("/availability", d.Auth.RequireDriver(), h.Driver.PublishAvailability)
	me.Delete("/availability/:id", d.Auth.RequireDriver(), h.Driver.WithdrawAvailability)

	me.Get("/preferences", h.Driver.GetPreferences)
	me.Put("/preferences", d.Auth.RequireDriver(), h.Driver.SavePreferences)

	// Registered last so /me is not swallowed by the :id pattern.
	drivers.Get("/:id", d.Auth.Optional(), h.Driver.GetProfile)
	drivers.Get("/:id/ratings", d.Auth.Optional(), h.Trip.DriverRatings)
}

// registerCompanyRoutes mounts /api/v1/companies.
func registerCompanyRoutes(v1 fiber.Router, h Handlers, d Deps) {
	companies := v1.Group("/companies")

	companies.Get("/", d.Auth.Optional(), h.Company.List)
	companies.Post("/", d.Auth.Authenticate(), h.Company.Create)

	me := companies.Group("/me", d.Auth.Authenticate(), d.Auth.RequireCompany())
	me.Get("/", h.Company.GetMine)
	me.Put("/", d.Auth.RequirePermission("companies.update"), h.Company.Update)
	me.Get("/documents", h.Company.ListDocuments)
	me.Post("/documents", d.Auth.RequirePermission("companies.update"), h.Company.UploadDocument)
	me.Get("/members", h.Company.ListMembers)
	me.Post("/members", d.Auth.RequirePermission("companies.members"), h.Company.InviteMember)
	me.Patch("/members/:id", d.Auth.RequirePermission("companies.members"), h.Company.UpdateMember)
	me.Delete("/members/:id", d.Auth.RequirePermission("companies.members"), h.Company.RemoveMember)

	companies.Get("/:id", d.Auth.Optional(), h.Company.Get)
	companies.Get("/:id/ratings", d.Auth.Optional(), h.Trip.CompanyRatings)
}

// registerLoadRoutes mounts /api/v1/loads — the marketplace itself.
//
// Fixed segments (/my, /saved) are declared before the /:id pattern so Fiber
// does not match them as a load id.
func registerLoadRoutes(v1 fiber.Router, h Handlers, d Deps) {
	loads := v1.Group("/loads")

	// A guest may browse the board; contact details are withheld by the usecase.
	loads.Get("/", d.Auth.Optional(), h.Load.Search)

	authed := loads.Group("", d.Auth.Authenticate())
	authed.Get("/my", d.Auth.RequireCompany(), h.Load.ListMine)
	authed.Get("/saved", h.Load.ListSaved)
	authed.Post("/", d.Auth.RequirePermission("loads.create"), h.Load.Create)

	authed.Put("/:id", d.Auth.RequirePermission("loads.update"), h.Load.Update)
	authed.Delete("/:id", d.Auth.RequirePermission("loads.delete"), h.Load.Delete)
	authed.Post("/:id/publish", d.Auth.RequirePermission("loads.publish"), h.Load.Publish)
	authed.Post("/:id/unpublish", d.Auth.RequirePermission("loads.publish"), h.Load.Unpublish)
	authed.Post("/:id/cancel", d.Auth.RequirePermission("loads.update"), h.Load.Cancel)
	authed.Post("/:id/save", h.Load.Save)
	authed.Delete("/:id/save", h.Load.Unsave)
	authed.Post("/:id/reserve", d.Auth.RequirePermission("bookings.create"), h.Booking.Reserve)

	loads.Get("/:id", d.Auth.Optional(), h.Load.Get)

	searches := v1.Group("/saved-searches", d.Auth.Authenticate())
	searches.Get("/", h.Load.ListSavedSearches)
	searches.Post("/", h.Load.CreateSavedSearch)
	searches.Delete("/:id", h.Load.DeleteSavedSearch)
}

// registerBookingRoutes mounts /api/v1/bookings.
func registerBookingRoutes(v1 fiber.Router, h Handlers, d Deps) {
	bookings := v1.Group("/bookings", d.Auth.Authenticate())

	bookings.Get("/", d.Auth.RequirePermission("bookings.read"), h.Booking.ListMine)
	bookings.Post("/offer", d.Auth.RequirePermission("bookings.create"), h.Booking.Offer)
	bookings.Get("/:id", d.Auth.RequirePermission("bookings.read"), h.Booking.Get)
	bookings.Post("/:id/respond", d.Auth.RequirePermission("bookings.respond"), h.Booking.Respond)
	bookings.Post("/:id/withdraw", d.Auth.RequirePermission("bookings.cancel"), h.Booking.Withdraw)
	bookings.Post("/:id/cancel", d.Auth.RequirePermission("bookings.cancel"), h.Booking.Cancel)
}

// registerTripRoutes mounts /api/v1/trips.
func registerTripRoutes(v1 fiber.Router, h Handlers, d Deps) {
	trips := v1.Group("/trips", d.Auth.Authenticate())

	trips.Get("/", d.Auth.RequirePermission("trips.read"), h.Trip.ListMine)
	trips.Get("/:id", d.Auth.RequirePermission("trips.read"), h.Trip.Get)
	trips.Get("/:id/track", d.Auth.RequirePermission("trips.track"), h.Trip.Track)
	trips.Post("/:id/advance", d.Auth.RequireDriver(), h.Trip.Advance)
	trips.Post("/:id/confirm", d.Auth.RequireCompany(), h.Trip.Confirm)
	trips.Post("/:id/cancel", d.Auth.RequirePermission("trips.cancel"), h.Trip.Cancel)
	trips.Post("/:id/dispute", d.Auth.RequirePermission("complaints.create"), h.Trip.Dispute)
	trips.Post("/:id/documents", d.Auth.RequirePermission("trips.update"), h.Trip.AddDocument)
	trips.Post("/:id/rate", h.Trip.Rate)
}

// registerAdminRoutes mounts the moderation queues. Every route here requires
// an explicit permission — the group guard alone is never the only check.
func registerAdminRoutes(v1 fiber.Router, h Handlers, d Deps) {
	admin := v1.Group("/admin",
		d.Auth.Authenticate(),
		d.Auth.RequireRole(domain.RoleAdmin, domain.RoleSuperAdmin))

	admin.Get("/driver-documents/pending",
		d.Auth.RequirePermission("drivers.verify"), h.Driver.PendingDocuments)
	admin.Post("/driver-documents/:id/review",
		d.Auth.RequirePermission("drivers.verify"), h.Driver.ReviewDocument)

	admin.Get("/vehicles/pending",
		d.Auth.RequirePermission("vehicles.verify"), h.Driver.PendingVehicles)
	admin.Post("/vehicles/:id/review",
		d.Auth.RequirePermission("vehicles.verify"), h.Driver.ReviewVehicle)

	admin.Get("/company-documents/pending",
		d.Auth.RequirePermission("companies.verify"), h.Company.PendingDocuments)
	admin.Post("/company-documents/:id/review",
		d.Auth.RequirePermission("companies.verify"), h.Company.ReviewDocument)
	admin.Post("/companies/:id/review",
		d.Auth.RequirePermission("companies.verify"), h.Company.Review)
}
