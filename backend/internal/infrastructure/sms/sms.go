// Package sms delivers transactional SMS (OTP codes, critical alerts).
package sms

import (
	"context"
	"fmt"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/pkg/logger"
)

// Sender is the SMS port used by usecases.
type Sender interface {
	Send(ctx context.Context, phone, message string) error
	Name() string
}

// New selects the provider named in the configuration.
func New(cfg config.SMSConfig, log logger.Logger) (Sender, error) {
	switch cfg.Provider {
	case "eskiz":
		return newEskiz(cfg, log), nil
	case "log", "":
		return &logSender{log: log}, nil
	default:
		return nil, fmt.Errorf("sms: unknown provider %q", cfg.Provider)
	}
}

// logSender writes the message to the application log instead of sending it.
// It exists so local development and tests never need SMS credentials — and it
// refuses to be used in production (see config.Validate).
type logSender struct{ log logger.Logger }

func (l *logSender) Name() string { return "log" }

func (l *logSender) Send(_ context.Context, phone, message string) error {
	l.log.Info("sms (not actually sent)",
		logger.String("provider", "log"),
		logger.String("to", phone),
		logger.String("message", message))
	return nil
}
