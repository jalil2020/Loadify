// Package config loads and validates every setting the process needs.
// If Load returns an error the process must not start — a half-configured
// service is worse than one that refuses to boot.
package config

import (
	"encoding/base64"
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
	Telegram TelegramConfig
	AI       AIConfig
	Metrics  MetricsConfig
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
	// RequirePhoneVerification makes registration wait for an SMS code before
	// the account can sign in. Off, a person registers with a phone number and
	// a password and is signed in at once; the number is recorded as
	// unconfirmed.
	RequirePhoneVerification bool
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

// TelegramConfig configures the MTProto client that posts on behalf of a
// logistician's own Telegram account, plus the platform-wide ceilings that keep
// that account out of trouble.
//
// Telegram publishes no numeric limits for user accounts, so these values are
// our own conservative policy rather than a documented maximum. A tariff or a
// group rule may lower them; nothing may raise them.
type TelegramConfig struct {
	APIID      int
	APIHash    string
	SessionKEK string // base64, 32 bytes: wraps every stored session
	KEKVersion int
	// PreviousKEK lets sessions sealed before a rotation still be opened.
	PreviousKEK        string
	PreviousKEKVersion int

	DeviceModel string
	AppVersion  string

	AccountGap      time.Duration // minimum spacing between two posts of one account
	AccountHourly   int
	AccountDaily    int
	GroupCooldown   time.Duration // floor; a group's own slow mode may be longer
	GroupDaily      int
	DuplicateWindow time.Duration // same load, same group
	GlobalPerSecond int           // across every account: protects the shared api_id
	MaxAttempts     int
	LeaseTTL        time.Duration
	// TrustRandomID says whether Telegram has been shown to refuse a message
	// whose random_id it has already seen. Until the spike confirms it, a job
	// whose worker stopped mid-send is failed as "outcome unknown" instead of
	// being sent again: a missed post is recoverable, a duplicate is spam.
	TrustRandomID bool
}

// Enabled reports whether posting can run at all.
func (t TelegramConfig) Enabled() bool {
	return t.APIID != 0 && t.APIHash != "" && t.SessionKEK != ""
}

// KEKs returns the key-encryption keys by version, and the version new secrets
// are sealed with.
func (t TelegramConfig) KEKs() (map[int][]byte, int, error) {
	keys := make(map[int][]byte, 2)
	primary, err := decodeKEK(t.SessionKEK)
	if err != nil {
		return nil, 0, fmt.Errorf("TELEGRAM_SESSION_KEK: %w", err)
	}
	keys[t.KEKVersion] = primary
	if t.PreviousKEK != "" {
		previous, err := decodeKEK(t.PreviousKEK)
		if err != nil {
			return nil, 0, fmt.Errorf("TELEGRAM_SESSION_KEK_PREVIOUS: %w", err)
		}
		keys[t.PreviousKEKVersion] = previous
	}
	return keys, t.KEKVersion, nil
}

func decodeKEK(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("must be base64: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("must decode to 32 bytes, got %d", len(raw))
	}
	return raw, nil
}

// AI provider values.
const (
	AIProviderNone   = "none"
	AIProviderLLM    = "llm"
	AIProviderHermes = "hermes"
)

// MetricsConfig says where each process serves Prometheus metrics. The API and
// the posting worker have their own addresses so both can run on one host.
// An empty address means off.
//
// The defaults listen on loopback only. Inside Docker the compose file opens
// them to the internal network, never to the internet: nginx does not proxy
// them.
type MetricsConfig struct {
	Addr       string
	PosterAddr string
}

// AIConfig configures the optional model-backed helpers. The platform works
// without them: parsing falls back to the rules, ranking to the score.
type AIConfig struct {
	Provider  string // none | llm | hermes
	BaseURL   string
	APIKey    string
	Model     string
	Timeout   time.Duration
	CacheTTL  time.Duration
	HermesURL string
	HermesKey string
	// Prices in dollars per million tokens, used to account for each call.
	PriceInPerMTok  float64
	PriceOutPerMTok float64
	// DailyBudgetUSD stops model calls across the platform once a day's
	// spending reaches it. Zero means no brake.
	DailyBudgetUSD float64
}

// Enabled reports whether a model may be called at all.
func (a AIConfig) Enabled() bool { return a.Provider != "" && a.Provider != AIProviderNone }

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

			RequirePhoneVerification: getBool("AUTH_REQUIRE_PHONE_VERIFICATION", false),
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

	cfg.Telegram = TelegramConfig{
		APIID:              getInt("TELEGRAM_API_ID", 0),
		APIHash:            getString("TELEGRAM_API_HASH", ""),
		SessionKEK:         getString("TELEGRAM_SESSION_KEK", ""),
		KEKVersion:         getInt("TELEGRAM_SESSION_KEK_VERSION", 1),
		PreviousKEK:        getString("TELEGRAM_SESSION_KEK_PREVIOUS", ""),
		PreviousKEKVersion: getInt("TELEGRAM_SESSION_KEK_PREVIOUS_VERSION", 0),
		DeviceModel:        getString("TELEGRAM_DEVICE_MODEL", "Loadify"),
		AppVersion:         getString("TELEGRAM_APP_VERSION", "1.0"),
		AccountGap:         getDuration("TELEGRAM_ACCOUNT_GAP", 90*time.Second),
		AccountHourly:      getInt("TELEGRAM_ACCOUNT_HOURLY", 20),
		AccountDaily:       getInt("TELEGRAM_ACCOUNT_DAILY", 120),
		GroupCooldown:      getDuration("TELEGRAM_GROUP_COOLDOWN", time.Hour),
		GroupDaily:         getInt("TELEGRAM_GROUP_DAILY", 3),
		DuplicateWindow:    getDuration("TELEGRAM_DUPLICATE_WINDOW", 24*time.Hour),
		GlobalPerSecond:    getInt("TELEGRAM_GLOBAL_PER_SECOND", 10),
		MaxAttempts:        getInt("TELEGRAM_MAX_ATTEMPTS", 5),
		LeaseTTL:           getDuration("TELEGRAM_LEASE_TTL", 2*time.Minute),
		TrustRandomID:      getBool("TELEGRAM_TRUST_RANDOM_ID", false),
	}
	cfg.AI = AIConfig{
		Provider:  getString("AI_PROVIDER", AIProviderNone),
		BaseURL:   getString("AI_BASE_URL", ""),
		APIKey:    getString("AI_API_KEY", ""),
		Model:     getString("AI_MODEL", ""),
		Timeout:   getDuration("AI_TIMEOUT", 20*time.Second),
		CacheTTL:  getDuration("AI_CACHE_TTL", 24*time.Hour),
		HermesURL: getString("HERMES_BASE_URL", ""),
		HermesKey: getString("HERMES_API_KEY", ""),

		PriceInPerMTok:  getFloat("AI_PRICE_INPUT_PER_MTOK", 0),
		PriceOutPerMTok: getFloat("AI_PRICE_OUTPUT_PER_MTOK", 0),
		DailyBudgetUSD:  getFloat("AI_DAILY_BUDGET_USD", 0),
	}
	cfg.Metrics = MetricsConfig{
		Addr:       metricsAddr("METRICS_ADDR", "127.0.0.1:9090"),
		PosterAddr: metricsAddr("POSTER_METRICS_ADDR", "127.0.0.1:9091"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// metricsAddr reads a metrics address; "off" turns it off, since an empty
// value falls back to the default like every other setting.
func metricsAddr(key, fallback string) string {
	if v := getString(key, fallback); !strings.EqualFold(v, "off") {
		return v
	}
	return ""
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

	// Posting is optional, but a half-configured client would fail at the first
	// send rather than at boot, so partial configuration is refused outright.
	set := 0
	for _, v := range []string{fmt.Sprint(c.Telegram.APIID), c.Telegram.APIHash, c.Telegram.SessionKEK} {
		if v != "" && v != "0" {
			set++
		}
	}
	switch set {
	case 0: // posting disabled
	case 3:
		if _, _, err := c.Telegram.KEKs(); err != nil {
			problems = append(problems, err.Error())
		}
		if c.Telegram.KEKVersion < 1 {
			problems = append(problems, "TELEGRAM_SESSION_KEK_VERSION must be at least 1")
		}
		if c.Telegram.PreviousKEK != "" && c.Telegram.PreviousKEKVersion >= c.Telegram.KEKVersion {
			problems = append(problems, "TELEGRAM_SESSION_KEK_PREVIOUS_VERSION must be lower than the current version")
		}
		if c.Telegram.AccountGap <= 0 || c.Telegram.AccountHourly < 1 || c.Telegram.AccountDaily < 1 {
			problems = append(problems, "Telegram account limits must all be positive")
		}
		if c.Telegram.GroupCooldown <= 0 || c.Telegram.GroupDaily < 1 {
			problems = append(problems, "Telegram group limits must all be positive")
		}
		if c.Telegram.GlobalPerSecond < 1 {
			problems = append(problems, "TELEGRAM_GLOBAL_PER_SECOND must be at least 1")
		}
		if c.Telegram.MaxAttempts < 1 {
			problems = append(problems, "TELEGRAM_MAX_ATTEMPTS must be at least 1")
		}
	default:
		problems = append(problems, "TELEGRAM_API_ID, TELEGRAM_API_HASH and TELEGRAM_SESSION_KEK must be set together")
	}

	switch c.AI.Provider {
	case AIProviderNone, AIProviderLLM, AIProviderHermes:
	default:
		problems = append(problems, "AI_PROVIDER must be none, llm or hermes")
	}
	if c.AI.Provider == AIProviderLLM && c.AI.APIKey == "" {
		problems = append(problems, "AI_API_KEY is required when AI_PROVIDER is llm")
	}
	if c.AI.PriceInPerMTok < 0 || c.AI.PriceOutPerMTok < 0 || c.AI.DailyBudgetUSD < 0 {
		problems = append(problems, "AI prices and AI_DAILY_BUDGET_USD cannot be negative")
	}
	if c.AI.Provider == AIProviderHermes && (c.AI.HermesURL == "" || c.AI.HermesKey == "") {
		problems = append(problems, "HERMES_BASE_URL and HERMES_API_KEY are required when AI_PROVIDER is hermes")
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
