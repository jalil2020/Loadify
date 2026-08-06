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
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/database"
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
}

// Repositories groups the persistence adapters.
type Repositories struct {
	Tx       domain.TxManager
	Audit    domain.AuditLogger
	Users    domain.UserRepository
	Roles    domain.RoleRepository
	Sessions domain.SessionRepository
	OTPs     domain.OTPRepository
	Resets   domain.PasswordResetRepository
	Identity domain.IdentityRepository
}

// Usecases groups the business-rule services.
type Usecases struct {
	Auth *usecase.AuthUsecase
}

// Handlers groups the HTTP handlers.
type Handlers struct {
	Health *handler.HealthHandler
	Auth   *handler.AuthHandler
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
		Tx:       repository.NewTxManager(db.DB),
		Audit:    repository.NewAuditLogger(db.DB, log),
		Users:    repository.NewUserRepository(db.DB),
		Roles:    repository.NewRoleRepository(db.DB),
		Sessions: repository.NewSessionRepository(db.DB),
		OTPs:     repository.NewOTPRepository(db.DB),
		Resets:   repository.NewPasswordResetRepository(db.DB),
		Identity: repository.NewIdentityRepository(db.DB),
	}

	authUsecase := usecase.NewAuthUsecase(
		repos.Users, repos.Roles, repos.Sessions, repos.OTPs, repos.Resets,
		repos.Identity, repos.Tx, repos.Audit,
		tokens, hasher, smsSender, redisCache, cfg, log,
	)

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
		Usecases:  Usecases{Auth: authUsecase},
		Handlers: Handlers{
			Health: handler.NewHealthHandler(db, redisCache, Version, cfg.App.Env),
			Auth:   handler.NewAuthHandler(authUsecase, v),
		},
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
