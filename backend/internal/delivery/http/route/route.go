// Package route registers every HTTP endpoint. Route files are the map of the
// API: one place to see what exists and what protects it.
package route

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/delivery/http/handler"
	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/delivery/ws"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/logger"
)

// Handlers bundles every handler the router needs.
type Handlers struct {
	Health       *handler.HealthHandler
	Auth         *handler.AuthHandler
	Driver       *handler.DriverHandler
	Company      *handler.CompanyHandler
	File         *handler.FileHandler
	Reference    *handler.ReferenceHandler
	Search       *handler.SearchHandler
	Load         *handler.LoadHandler
	LoadAd       *handler.LoadAdHandler
	Publish      *handler.PublishHandler
	Booking      *handler.BookingHandler
	Trip         *handler.TripHandler
	Chat         *handler.ChatHandler
	Notification *handler.NotificationHandler
	Payment      *handler.PaymentHandler
	Admin        *handler.AdminHandler
	// WS is the WebSocket entry point. It is nil when realtime is disabled, in
	// which case the /ws route is simply not mounted.
	WS *ws.Handler
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
	registerSearchRoutes(v1, h, d)
	registerFileRoutes(v1, h, d)
	registerDriverRoutes(v1, h, d)
	registerCompanyRoutes(v1, h, d)
	registerLoadRoutes(v1, h, d)
	registerLoadAdRoutes(v1, h, d)
	registerBookingRoutes(v1, h, d)
	registerTripRoutes(v1, h, d)
	registerChatRoutes(v1, h, d)
	registerNotificationRoutes(v1, h, d)
	registerPaymentRoutes(v1, h, d)
	registerSupportRoutes(v1, h, d)
	registerWebSocketRoutes(v1, h)
	registerAdminRoutes(v1, h, d)
}

// registerPaymentRoutes mounts wallet, subscription and checkout endpoints.
func registerPaymentRoutes(v1 fiber.Router, h Handlers, d Deps) {
	// Provider callbacks are unauthenticated by necessity — the provider holds
	// no token — and are protected by their per-provider signature check. The
	// rate limit is generous because a provider may legitimately retry.
	v1.Post("/payments/webhooks/:provider",
		middleware.RateLimit(d.Cache, d.Log, "webhook", 600, time.Minute),
		h.Payment.Webhook)

	// The plan catalogue is public so the pricing page needs no account.
	v1.Get("/plans", d.Auth.Optional(), h.Payment.ListPlans)
	v1.Get("/payments/methods", h.Payment.PaymentMethods)

	subscriptions := v1.Group("/subscriptions", d.Auth.Authenticate())
	subscriptions.Get("/me", h.Payment.GetSubscription)
	subscriptions.Get("/entitlements", h.Payment.Entitlements)
	subscriptions.Post("/", d.Auth.RequirePermission("payments.manage"), h.Payment.Subscribe)
	subscriptions.Post("/cancel", d.Auth.RequirePermission("payments.manage"), h.Payment.CancelSubscription)

	wallet := v1.Group("/wallet", d.Auth.Authenticate(), d.Auth.RequirePermission("payments.read"))
	wallet.Get("/", h.Payment.GetWallet)
	wallet.Get("/transactions", h.Payment.ListTransactions)
	wallet.Post("/topup", d.Auth.RequirePermission("payments.manage"), h.Payment.Topup)
	wallet.Post("/payout", d.Auth.RequirePermission("payments.manage"), h.Payment.RequestPayout)

	escrow := v1.Group("/escrow", d.Auth.Authenticate(), d.Auth.RequirePermission("payments.read"))
	escrow.Post("/fund", d.Auth.RequirePermission("payments.escrow"), h.Payment.FundEscrow)
	escrow.Get("/:trip_id", h.Payment.GetEscrow)

	payments := v1.Group("/payments", d.Auth.Authenticate(), d.Auth.RequirePermission("payments.read"))
	payments.Get("/orders", h.Payment.ListOrders)
	payments.Get("/invoices", h.Payment.ListInvoices)
	payments.Post("/refunds", h.Payment.RequestRefund)
}

// registerChatRoutes mounts /api/v1/chats.
func registerChatRoutes(v1 fiber.Router, h Handlers, d Deps) {
	chats := v1.Group("/chats", d.Auth.Authenticate(), d.Auth.RequirePermission("chat.read"))

	chats.Get("/", h.Chat.ListRooms)
	chats.Post("/booking/:booking_id", d.Auth.RequirePermission("chat.write"), h.Chat.EnsureBookingRoom)
	chats.Get("/:id", h.Chat.Get)
	chats.Get("/:id/messages", h.Chat.ListMessages)
	chats.Post("/:id/messages", d.Auth.RequirePermission("chat.write"), h.Chat.SendMessage)
	chats.Post("/:id/read", h.Chat.MarkRead)
	chats.Patch("/:id/mute", h.Chat.SetMuted)
}

// registerNotificationRoutes mounts /api/v1/notifications.
//
// These need no extra permission: every authenticated principal has an inbox.
func registerNotificationRoutes(v1 fiber.Router, h Handlers, d Deps) {
	notifications := v1.Group("/notifications", d.Auth.Authenticate())

	notifications.Get("/", h.Notification.List)
	notifications.Get("/unread-count", h.Notification.UnreadCount)
	notifications.Post("/read", h.Notification.MarkRead)
	notifications.Get("/preferences", h.Notification.ListPreferences)
	notifications.Put("/preferences", h.Notification.SavePreference)
	notifications.Post("/devices", h.Notification.RegisterDevice)
	notifications.Delete("/devices", h.Notification.UnregisterDevice)
}

// registerWebSocketRoutes mounts the realtime endpoint.
//
// The socket does its own token check in Upgrade rather than reusing the HTTP
// auth middleware: a browser cannot set an Authorization header on a WebSocket
// handshake, so the token may arrive as a query parameter.
func registerWebSocketRoutes(v1 fiber.Router, h Handlers) {
	if h.WS == nil {
		return
	}
	v1.Get("/ws", h.WS.Upgrade, h.WS.Serve())
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

// registerSearchRoutes mounts the natural-language parser.
//
// Unauthenticated, like the board it feeds: someone evaluating the app can
// type a corridor and see it understood before signing up. It resolves names
// against the public vocabularies and touches no account data.
func registerSearchRoutes(v1 fiber.Router, h Handlers, d Deps) {
	search := v1.Group("/search")
	search.Post("/parse", d.Auth.Optional(), h.Search.Parse)
}

// registerSupportRoutes mounts complaints and public settings.
//
// Filing a complaint is a right every authenticated user has; reviewing them is
// what needs a permission, and that lives on the admin group.
func registerSupportRoutes(v1 fiber.Router, h Handlers, d Deps) {
	v1.Get("/settings", h.Admin.PublicSettings)
	v1.Post("/complaints",
		d.Auth.Authenticate(), d.Auth.RequirePermission("complaints.create"),
		h.Admin.FileComplaint)
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
	me.Get("/report", h.Admin.DriverReport)

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
	me.Get("/report", h.Admin.CompanyReport)

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

	admin.Get("/refunds",
		d.Auth.RequirePermission("payments.refund"), h.Payment.ListRefunds)
	admin.Post("/refunds/:id/review",
		d.Auth.RequirePermission("payments.refund"), h.Payment.ReviewRefund)

	admin.Get("/dashboard", d.Auth.RequirePermission("admin.dashboard"), h.Admin.Dashboard)
	admin.Get("/analytics/series", d.Auth.RequirePermission("admin.reports"), h.Admin.Series)
	admin.Get("/analytics/corridors", d.Auth.RequirePermission("admin.reports"), h.Admin.TopCorridors)

	admin.Get("/users", d.Auth.RequirePermission("admin.users"), h.Admin.ListUsers)
	admin.Get("/users/:id", d.Auth.RequirePermission("admin.users"), h.Admin.GetUser)
	admin.Post("/users/:id/block", d.Auth.RequirePermission("admin.users"), h.Admin.BlockUser)
	admin.Post("/users/:id/unblock", d.Auth.RequirePermission("admin.users"), h.Admin.UnblockUser)

	// Granting a role is the one action that lets somebody grant themselves
	// everything else, so it is super-admin only.
	admin.Post("/users/:id/roles", d.Auth.RequirePermission("admin.roles"), h.Admin.AssignRole)
	admin.Delete("/users/:id/roles", d.Auth.RequirePermission("admin.roles"), h.Admin.RevokeRole)

	admin.Get("/complaints", d.Auth.RequirePermission("complaints.manage"), h.Admin.ListComplaints)
	admin.Get("/complaints/:id", d.Auth.RequirePermission("complaints.manage"), h.Admin.GetComplaint)
	admin.Post("/complaints/:id/assign", d.Auth.RequirePermission("complaints.manage"), h.Admin.AssignComplaint)
	admin.Post("/complaints/:id/resolve", d.Auth.RequirePermission("complaints.manage"), h.Admin.ResolveComplaint)

	admin.Get("/audit-logs", d.Auth.RequirePermission("admin.audit"), h.Admin.ListAuditLogs)
	admin.Get("/audit-logs/:resource_type/:id", d.Auth.RequirePermission("admin.audit"), h.Admin.ResourceHistory)

	admin.Get("/settings", d.Auth.RequirePermission("admin.settings"), h.Admin.ListSettings)
	admin.Put("/settings/:key", d.Auth.RequirePermission("admin.settings"), h.Admin.UpdateSetting)
	admin.Get("/feature-flags", d.Auth.RequirePermission("admin.settings"), h.Admin.ListFeatureFlags)
	admin.Put("/feature-flags/:key", d.Auth.RequirePermission("admin.settings"), h.Admin.SetFeatureFlag)
}

// registerLoadAdRoutes mounts /api/v1/load-ads — the adverts a forwarder
// distributes to Telegram groups.
//
// They are separate from /loads on purpose. A marketplace load is a shipment
// with an address, a price and a lifecycle; an advert is what someone writes in
// a group, where half of that is routinely left out. Sharing a path would mean
// sharing a shape, and the shape is the difference.
func registerLoadAdRoutes(v1 fiber.Router, h Handlers, d Deps) {
	ads := v1.Group("/load-ads", d.Auth.Authenticate(), d.Auth.RequireCompany())

	// Parsing is free of side effects but not free of cost, so it gets its own
	// limit: it runs while someone types.
	ads.Post("/parse",
		middleware.RateLimit(d.Cache, d.Log, "ad-parse", 60, time.Minute),
		d.Auth.RequirePermission("load_ads.create"), h.LoadAd.Parse)

	ads.Get("/", d.Auth.RequirePermission("load_ads.view"), h.LoadAd.List)
	ads.Post("/", d.Auth.RequirePermission("load_ads.create"), h.LoadAd.Create)
	ads.Get("/:id", d.Auth.RequirePermission("load_ads.view"), h.LoadAd.Get)
	ads.Put("/:id", d.Auth.RequirePermission("load_ads.create"), h.LoadAd.Update)
	ads.Delete("/:id", d.Auth.RequirePermission("load_ads.delete"), h.LoadAd.Archive)

	// Reviewing costs nothing and sends nothing; publishing is the only step
	// that reaches Telegram, and it carries the reviewed list with it.
	ads.Post("/:id/publish/preview", d.Auth.RequirePermission("load_ads.publish"), h.Publish.Preview)
	ads.Post("/:id/publish", d.Auth.RequirePermission("load_ads.publish"), h.Publish.Publish)
	ads.Get("/:id/posts", d.Auth.RequirePermission("posting.view"), h.Publish.Jobs)

	history := v1.Group("/posting-history", d.Auth.Authenticate(), d.Auth.RequireCompany())
	history.Get("/", d.Auth.RequirePermission("posting.view"), h.Publish.History)

	jobs := v1.Group("/posting-jobs", d.Auth.Authenticate(), d.Auth.RequireCompany())
	jobs.Post("/:id/cancel", d.Auth.RequirePermission("load_ads.publish"), h.Publish.Cancel)
}
