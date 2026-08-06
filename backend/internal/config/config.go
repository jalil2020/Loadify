// Package config loads and validates every setting the process needs.
// If Load returns an error the process must not start — a half-configured
// service is worse than one that refuses to boot.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the complete application configuration.
type Config struct {
	App      AppConfig
	DB       DBConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Security SecurityConfig
	S3       S3Config
	SMS      SMSConfig
	FCM      FCMConfig
	Maps     MapsConfig
	Payment  PaymentConfig
}

type AppConfig struct {
	Name          string
	Env           string
	Port          int
	BaseURL       string
	Timezone      string
	DefaultLocale string
	LogLevel      string
	ShutdownGrace time.Duration
	BodyLimitMB   int
}

func (a AppConfig) IsProduction() bool  { return a.Env == "production" }
func (a AppConfig) IsDevelopment() bool { return a.Env == "development" }

type DBConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	LogSlowQueries  time.Duration
}

// DSN builds the PostgreSQL connection string.
func (d DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
}

func (r RedisConfig) Addr() string { return fmt.Sprintf("%s:%d", r.Host, r.Port) }

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Issuer        string
}

type SecurityConfig struct {
	BcryptCost           int
	RateLimitMax         int
	RateLimitWindow      time.Duration
	CORSAllowedOrigins   []string
	OTPTTL               time.Duration
	OTPLength            int
	OTPMaxAttempts       int
	OTPResendCooldown    time.Duration
	LoginMaxFailed       int
	LoginLockoutDuration time.Duration
	PasswordResetTTL     time.Duration
	TrustedProxies       []string
}

type S3Config struct {
	Endpoint  string
	PublicURL string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
	MaxSizeMB int
}

type SMSConfig struct {
	Provider      string // log | eskiz | playmobile
	EskizEmail    string
	EskizPassword string
	EskizBaseURL  string
	Sender        string
}

type FCMConfig struct {
	ProjectID       string
	CredentialsFile string
}

func (f FCMConfig) Enabled() bool { return f.ProjectID != "" && f.CredentialsFile != "" }

type MapsConfig struct {
	GoogleAPIKey string
}

// PaymentConfig groups the checkout providers. Each provider is configured
// independently so one can be enabled without the others.
type PaymentConfig struct {
	CommissionPercent float64
	Click             ClickConfig
	Payme             PaymeConfig
	Uzum              UzumConfig
}

// ClickConfig configures the Click merchant integration.
type ClickConfig struct {
	MerchantID   string
	ServiceID    string
	SecretKey    string
	MerchantUser string
}

// Enabled reports whether Click can accept payments.
func (c ClickConfig) Enabled() bool {
	return c.MerchantID != "" && c.ServiceID != "" && c.SecretKey != ""
}

// PaymeConfig configures the Payme merchant integration.
type PaymeConfig struct {
	MerchantID string
	SecretKey  string
}

// Enabled reports whether Payme can accept payments.
func (c PaymeConfig) Enabled() bool { return c.MerchantID != "" && c.SecretKey != "" }

// UzumConfig configures the Uzum Bank merchant integration.
type UzumConfig struct {
	MerchantID      string
	MerchantService string
	SecretKey       string
}

// Enabled reports whether Uzum can accept payments.
func (c UzumConfig) Enabled() bool { return c.MerchantID != "" && c.SecretKey != "" }

// Load reads .env (when present) and the process environment, then validates.
func Load(envFiles ...string) (*Config, error) {
	for _, f := range envFiles {
		// A missing .env is normal in containers; real config comes from the environment.
		_ = godotenv.Load(f)
	}

	cfg := &Config{
		App: AppConfig{
			Name:          getString("APP_NAME", "Loadify"),
			Env:           getString("APP_ENV", "development"),
			Port:          getInt("APP_PORT", 8080),
			BaseURL:       getString("APP_BASE_URL", "http://localhost:8080"),
			Timezone:      getString("APP_TIMEZONE", "Asia/Tashkent"),
			DefaultLocale: getString("APP_DEFAULT_LOCALE", "uz"),
			LogLevel:      getString("LOG_LEVEL", "info"),
			ShutdownGrace: getDuration("APP_SHUTDOWN_GRACE", 20*time.Second),
			BodyLimitMB:   getInt("APP_BODY_LIMIT_MB", 25),
		},
		DB: DBConfig{
			Host:            getString("DB_HOST", "localhost"),
			Port:            getInt("DB_PORT", 5432),
			User:            getString("DB_USER", "loadify"),
			Password:        getString("DB_PASSWORD", ""),
			Name:            getString("DB_NAME", "loadify"),
			SSLMode:         getString("DB_SSLMODE", "disable"),
			MaxOpenConns:    getInt("DB_MAX_OPEN_CONNS", 50),
			MaxIdleConns:    getInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getDuration("DB_CONN_MAX_LIFETIME", time.Hour),
			LogSlowQueries:  getDuration("DB_SLOW_QUERY_THRESHOLD", 300*time.Millisecond),
		},
		Redis: RedisConfig{
			Host:     getString("REDIS_HOST", "localhost"),
			Port:     getInt("REDIS_PORT", 6379),
			Password: getString("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
			PoolSize: getInt("REDIS_POOL_SIZE", 20),
		},
		JWT: JWTConfig{
			AccessSecret:  getString("JWT_ACCESS_SECRET", ""),
			RefreshSecret: getString("JWT_REFRESH_SECRET", ""),
			AccessTTL:     getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getDuration("JWT_REFRESH_TTL", 720*time.Hour),
			Issuer:        getString("JWT_ISSUER", "loadify"),
		},
		Security: SecurityConfig{
			BcryptCost:           getInt("BCRYPT_COST", 12),
			RateLimitMax:         getInt("RATE_LIMIT_MAX", 120),
			RateLimitWindow:      getDuration("RATE_LIMIT_WINDOW", time.Minute),
			CORSAllowedOrigins:   getStringSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
			OTPTTL:               getDuration("OTP_TTL", 5*time.Minute),
			OTPLength:            getInt("OTP_LENGTH", 6),
			OTPMaxAttempts:       getInt("OTP_MAX_ATTEMPTS", 5),
			OTPResendCooldown:    getDuration("OTP_RESEND_COOLDOWN", 60*time.Second),
			LoginMaxFailed:       getInt("LOGIN_MAX_FAILED_ATTEMPTS", 5),
			LoginLockoutDuration: getDuration("LOGIN_LOCKOUT_DURATION", 15*time.Minute),
			PasswordResetTTL:     getDuration("PASSWORD_RESET_TTL", 30*time.Minute),
			TrustedProxies:       getStringSlice("TRUSTED_PROXIES", nil),
		},
		S3: S3Config{
			Endpoint:  getString("S3_ENDPOINT", "http://localhost:9000"),
			PublicURL: getString("S3_PUBLIC_URL", "http://localhost:9000"),
			AccessKey: getString("S3_ACCESS_KEY", ""),
			SecretKey: getString("S3_SECRET_KEY", ""),
			Bucket:    getString("S3_BUCKET", "loadify"),
			Region:    getString("S3_REGION", "us-east-1"),
			UseSSL:    getBool("S3_USE_SSL", false),
			MaxSizeMB: getInt("S3_MAX_SIZE_MB", 20),
		},
		SMS: SMSConfig{
			Provider:      getString("SMS_PROVIDER", "log"),
			EskizEmail:    getString("SMS_ESKIZ_EMAIL", ""),
			EskizPassword: getString("SMS_ESKIZ_PASSWORD", ""),
			EskizBaseURL:  getString("SMS_ESKIZ_BASE_URL", "https://notify.eskiz.uz/api"),
			Sender:        getString("SMS_SENDER", "4546"),
		},
		FCM: FCMConfig{
			ProjectID:       getString("FCM_PROJECT_ID", ""),
			CredentialsFile: getString("FCM_CREDENTIALS_FILE", ""),
		},
		Maps: MapsConfig{
			GoogleAPIKey: getString("GOOGLE_MAPS_API_KEY", ""),
		},
		Payment: PaymentConfig{
			CommissionPercent: getFloat("PLATFORM_COMMISSION_PERCENT", 5),
			Click: ClickConfig{
				MerchantID:   getString("CLICK_MERCHANT_ID", ""),
				ServiceID:    getString("CLICK_SERVICE_ID", ""),
				SecretKey:    getString("CLICK_SECRET_KEY", ""),
				MerchantUser: getString("CLICK_MERCHANT_USER_ID", ""),
			},
			Payme: PaymeConfig{
				MerchantID: getString("PAYME_MERCHANT_ID", ""),
				SecretKey:  getString("PAYME_SECRET_KEY", ""),
			},
			Uzum: UzumConfig{
				MerchantID:      getString("UZUM_MERCHANT_ID", ""),
				MerchantService: getString("UZUM_MERCHANT_SERVICE_ID", ""),
				SecretKey:       getString("UZUM_SECRET_KEY", ""),
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate rejects configurations that would fail at runtime or silently weaken security.
func (c *Config) Validate() error {
	var problems []string

	switch c.App.Env {
	case "development", "staging", "production":
	default:
		problems = append(problems, "APP_ENV must be development, staging or production")
	}
	if c.App.Port < 1 || c.App.Port > 65535 {
		problems = append(problems, "APP_PORT must be between 1 and 65535")
	}
	if c.DB.Name == "" || c.DB.User == "" {
		problems = append(problems, "DB_NAME and DB_USER are required")
	}
	if len(c.JWT.AccessSecret) < 32 {
		problems = append(problems, "JWT_ACCESS_SECRET must be at least 32 characters")
	}
	if len(c.JWT.RefreshSecret) < 32 {
		problems = append(problems, "JWT_REFRESH_SECRET must be at least 32 characters")
	}
	if c.JWT.AccessSecret == c.JWT.RefreshSecret {
		problems = append(problems, "JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must differ")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.AccessTTL > time.Hour {
		problems = append(problems, "JWT_ACCESS_TTL must be greater than zero and at most 1h")
	}
	if c.Security.BcryptCost < 10 {
		problems = append(problems, "BCRYPT_COST must be at least 10")
	}
	if c.Security.OTPLength < 4 || c.Security.OTPLength > 8 {
		problems = append(problems, "OTP_LENGTH must be between 4 and 8")
	}
	if c.Payment.CommissionPercent < 0 || c.Payment.CommissionPercent > 100 {
		problems = append(problems, "PLATFORM_COMMISSION_PERCENT must be between 0 and 100")
	}

	if c.App.IsProduction() {
		if c.DB.Password == "" {
			problems = append(problems, "DB_PASSWORD is required in production")
		}
		if c.DB.SSLMode == "disable" {
			problems = append(problems, "DB_SSLMODE must not be disable in production")
		}
		if c.SMS.Provider == "log" {
			problems = append(problems, "SMS_PROVIDER must be a real provider in production")
		}
		for _, o := range c.Security.CORSAllowedOrigins {
			if o == "*" {
				problems = append(problems, "CORS_ALLOWED_ORIGINS must not be * in production")
			}
		}
	}

	if len(problems) > 0 {
		return errors.New("invalid configuration:\n  - " + strings.Join(problems, "\n  - "))
	}
	return nil
}
