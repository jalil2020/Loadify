// Package bootstrap is the composition root. It is the only package that knows
// how every adapter, usecase and handler fits together, which keeps the
// dependency graph explicit and testable.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/delivery/http/handler"
	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/delivery/ws"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/database"
	"github.com/loadify/backend/internal/infrastructure/push"
	"github.com/loadify/backend/internal/infrastructure/sms"
	"github.com/loadify/backend/internal/infrastructure/storage"
	"github.com/loadify/backend/internal/repository"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/hash"
	"github.com/loadify/backend/pkg/jwt"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/validator"
)

// Version is stamped at build time with -ldflags "-X ...Version=1.2.3".
var Version = "dev"

// Container holds every constructed dependency.
type Container struct {
	Cfg   *config.Config
	Log   logger.Logger
	DB    *database.DB
	Cache cache.Cache

	Storage   domain.Storage
	SMS       sms.Sender
	Tokens    jwt.Manager
	Hasher    hash.Hasher
	Validator validator.Validator

	Repos    Repositories
	Usecases Usecases
	Handlers Handlers
	Auth     *middleware.AuthMiddleware
	// Hub is exposed so the server can start its Redis subscription and close it
	// during graceful shutdown.
	Hub *ws.Hub
}

// Repositories groups the persistence adapters.
type Repositories struct {
	Tx            domain.TxManager
	Audit         domain.AuditLogger
	Users         domain.UserRepository
	Roles         domain.RoleRepository
	Sessions      domain.SessionRepository
	OTPs          domain.OTPRepository
	Resets        domain.PasswordResetRepository
	Identity      domain.IdentityRepository
	Files         domain.FileRepository
	Geo           domain.GeoRepository
	Drivers       domain.DriverRepository
	DriverDocs    domain.DriverDocumentRepository
	Vehicles      domain.VehicleRepository
	DriverPrefs   domain.DriverPreferenceRepository
	Locations     domain.LocationRepository
	Companies     domain.CompanyRepository
	CompanyDocs   domain.CompanyDocumentRepository
	Loads         domain.LoadRepository
	CargoTypes    domain.CargoTypeRepository
	Saved         domain.SavedRepository
	Bookings      domain.BookingRepository
	Trips         domain.TripRepository
	Ratings       domain.RatingRepository
	Settings      domain.SettingsRepository
	Chat          domain.ChatRepository
	Notifications domain.NotificationRepository
}

// Usecases groups the business-rule services.
type Usecases struct {
	Auth         *usecase.AuthUsecase
	File         *usecase.FileUsecase
	Driver       *usecase.DriverUsecase
	Company      *usecase.CompanyUsecase
	Reference    *usecase.ReferenceUsecase
	Load         *usecase.LoadUsecase
	Booking      *usecase.BookingUsecase
	Trip         *usecase.TripUsecase
	Chat         *usecase.ChatUsecase
	Notification *usecase.NotificationUsecase
}

// Handlers groups the HTTP handlers.
type Handlers struct {
	Health       *handler.HealthHandler
	Auth         *handler.AuthHandler
	Driver       *handler.DriverHandler
	Company      *handler.CompanyHandler
	File         *handler.FileHandler
	Reference    *handler.ReferenceHandler
	Load         *handler.LoadHandler
	Booking      *handler.BookingHandler
	Trip         *handler.TripHandler
	Chat         *handler.ChatHandler
	Notification *handler.NotificationHandler
	WS           *ws.Handler
}

// NewContainer builds every dependency, failing fast on any misconfiguration.
// Storage is optional: the API can serve without object storage, so a failure
// there degrades uploads rather than preventing start-up.
func NewContainer(cfg *config.Config, log logger.Logger) (*Container, error) {
	db, err := database.New(cfg.DB, cfg.App.Env, log)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	redisCache, err := cache.New(cfg.Redis, log)
	if err != nil {
		return nil, fmt.Errorf("cache: %w", err)
	}

	tokens, err := jwt.New(jwt.Config{
		AccessSecret:  cfg.JWT.AccessSecret,
		RefreshSecret: cfg.JWT.RefreshSecret,
		AccessTTL:     cfg.JWT.AccessTTL,
		RefreshTTL:    cfg.JWT.RefreshTTL,
		Issuer:        cfg.JWT.Issuer,
	})
	if err != nil {
		return nil, fmt.Errorf("jwt: %w", err)
	}

	smsSender, err := sms.New(cfg.SMS, log)
	if err != nil {
		return nil, fmt.Errorf("sms: %w", err)
	}

	objectStorage, err := storage.New(cfg.S3, log)
	if err != nil {
		log.Warn("object storage unavailable, uploads will fail until it recovers",
			logger.Err(err))
		objectStorage = nil
	}

	hasher := hash.NewBcrypt(cfg.Security.BcryptCost)
	v := validator.New()

	repos := Repositories{
		Tx:            repository.NewTxManager(db.DB),
		Audit:         repository.NewAuditLogger(db.DB, log),
		Users:         repository.NewUserRepository(db.DB),
		Roles:         repository.NewRoleRepository(db.DB),
		Sessions:      repository.NewSessionRepository(db.DB),
		OTPs:          repository.NewOTPRepository(db.DB),
		Resets:        repository.NewPasswordResetRepository(db.DB),
		Identity:      repository.NewIdentityRepository(db.DB),
		Files:         repository.NewFileRepository(db.DB),
		Geo:           repository.NewGeoRepository(db.DB),
		Drivers:       repository.NewDriverRepository(db.DB),
		DriverDocs:    repository.NewDriverDocumentRepository(db.DB),
		Vehicles:      repository.NewVehicleRepository(db.DB),
		DriverPrefs:   repository.NewDriverPreferenceRepository(db.DB),
		Locations:     repository.NewLocationRepository(db.DB),
		Companies:     repository.NewCompanyRepository(db.DB),
		CompanyDocs:   repository.NewCompanyDocumentRepository(db.DB),
		Loads:         repository.NewLoadRepository(db.DB),
		CargoTypes:    repository.NewCargoTypeRepository(db.DB),
		Saved:         repository.NewSavedRepository(db.DB),
		Bookings:      repository.NewBookingRepository(db.DB),
		Trips:         repository.NewTripRepository(db.DB),
		Ratings:       repository.NewRatingRepository(db.DB),
		Settings:      repository.NewSettingsRepository(db.DB, log),
		Chat:          repository.NewChatRepository(db.DB),
		Notifications: repository.NewNotificationRepository(db.DB),
	}

	// The realtime hub and its publisher must exist before any usecase that
	// emits events, so it is constructed here and started by the server.
	hub := ws.NewHub(redisCache, repos.Companies, log)
	eventPublisher := ws.NewPublisher(hub)

	pushSender, err := push.New(context.Background(), cfg.FCM, log)
	if err != nil {
		// Push is a delivery channel, not a prerequisite: in-app notifications
		// still work, so this degrades rather than blocking start-up.
		log.Warn("push notifications unavailable", logger.Err(err))
		pushSender = nil
	}

	authUsecase := usecase.NewAuthUsecase(
		repos.Users, repos.Roles, repos.Sessions, repos.OTPs, repos.Resets,
		repos.Identity, repos.Tx, repos.Audit,
		tokens, hasher, smsSender, redisCache, cfg, log,
	)
	fileUsecase := usecase.NewFileUsecase(repos.Files, objectStorage, cfg, log)
	notificationUsecase := usecase.NewNotificationUsecase(
		repos.Notifications, repos.Users, pushSender, eventPublisher, log,
	)
	chatUsecase := usecase.NewChatUsecase(
		repos.Chat, repos.Bookings, repos.Drivers, repos.Companies, repos.Users,
		fileUsecase, repos.Tx, eventPublisher, notificationUsecase, hub, log,
	)
	driverUsecase := usecase.NewDriverUsecase(
		repos.Drivers, repos.DriverDocs, repos.Vehicles, repos.DriverPrefs,
		repos.Users, repos.Geo, repos.Locations, fileUsecase, repos.Tx,
		repos.Audit, redisCache, log,
	)
	companyUsecase := usecase.NewCompanyUsecase(
		repos.Companies, repos.CompanyDocs, repos.Users, repos.Roles, repos.Geo,
		fileUsecase, repos.Tx, repos.Audit, redisCache, log,
	)
	referenceUsecase := usecase.NewReferenceUsecase(repos.Geo, redisCache, log)

	loadUsecase := usecase.NewLoadUsecase(
		repos.Loads, repos.CargoTypes, repos.Saved, repos.Bookings, repos.Companies,
		repos.Drivers, repos.Vehicles, repos.Geo, fileUsecase, repos.Tx, repos.Audit,
		eventPublisher, redisCache, log,
	)
	bookingUsecase := usecase.NewBookingUsecase(
		repos.Bookings, repos.Loads, repos.Trips, repos.Drivers, repos.Vehicles,
		repos.Companies, repos.Users, fileUsecase, repos.Tx, repos.Audit,
		eventPublisher, notificationUsecase, log,
	)
	tripUsecase := usecase.NewTripUsecase(
		repos.Trips, repos.Bookings, repos.Loads, repos.Drivers, repos.Companies,
		repos.Ratings, repos.Locations, fileUsecase, repos.Settings, repos.Tx,
		repos.Audit, eventPublisher, notificationUsecase, log,
	)

	// The socket router is built last: it depends on the chat, driver and trip
	// usecases, which in turn depend on the hub the router drives.
	wsRouter := ws.NewRouter(hub, chatUsecase, driverUsecase, tripUsecase, log)
	wsHandler := ws.NewHandler(hub, wsRouter, tokens, repos.Users, log)

	authMiddleware := middleware.NewAuthMiddleware(tokens, repos.Users, redisCache, log)

	return &Container{
		Cfg:       cfg,
		Log:       log,
		DB:        db,
		Cache:     redisCache,
		Storage:   objectStorage,
		SMS:       smsSender,
		Tokens:    tokens,
		Hasher:    hasher,
		Validator: v,
		Repos:     repos,
		Usecases: Usecases{
			Auth:         authUsecase,
			File:         fileUsecase,
			Driver:       driverUsecase,
			Company:      companyUsecase,
			Reference:    referenceUsecase,
			Load:         loadUsecase,
			Booking:      bookingUsecase,
			Trip:         tripUsecase,
			Chat:         chatUsecase,
			Notification: notificationUsecase,
		},
		Handlers: Handlers{
			Health:       handler.NewHealthHandler(db, redisCache, Version, cfg.App.Env),
			Auth:         handler.NewAuthHandler(authUsecase, v),
			Driver:       handler.NewDriverHandler(driverUsecase, v),
			Company:      handler.NewCompanyHandler(companyUsecase, v),
			File:         handler.NewFileHandler(fileUsecase, v),
			Reference:    handler.NewReferenceHandler(referenceUsecase),
			Load:         handler.NewLoadHandler(loadUsecase, v),
			Booking:      handler.NewBookingHandler(bookingUsecase, v),
			Trip:         handler.NewTripHandler(tripUsecase, v),
			Chat:         handler.NewChatHandler(chatUsecase, v),
			Notification: handler.NewNotificationHandler(notificationUsecase, chatUsecase, v),
			WS:           wsHandler,
		},
		Hub:  hub,
		Auth: authMiddleware,
	}, nil
}

// Close releases every resource the container owns.
func (c *Container) Close(_ context.Context) {
	if c.Cache != nil {
		if err := c.Cache.Close(); err != nil {
			c.Log.Warn("closing redis failed", logger.Err(err))
		}
	}
	if c.DB != nil {
		if err := c.DB.Close(); err != nil {
			c.Log.Warn("closing postgres failed", logger.Err(err))
		}
	}
}
