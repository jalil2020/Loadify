// Package push delivers notifications through Firebase Cloud Messaging.
//
// The FCM HTTP v1 API is called directly rather than through the Firebase Admin
// SDK: the SDK pulls in a large dependency tree for what amounts to one
// authenticated POST per device token.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// fcmScope is the OAuth scope the messaging API requires.
const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// requestTimeout bounds one send. Notifications are best-effort; a slow provider
// must not hold a request open.
const requestTimeout = 10 * time.Second

// fcmSender is the HTTP v1 implementation.
type fcmSender struct {
	projectID string
	client    *http.Client
	log       logger.Logger
}

// logSender is the fallback used when FCM is not configured. It logs instead of
// sending, so development works without Firebase credentials.
type logSender struct{ log logger.Logger }

// New builds a push sender. When no credentials are configured it returns the
// logging implementation rather than an error, so a developer can run the whole
// stack without a Firebase project.
func New(ctx context.Context, cfg config.FCMConfig, log logger.Logger) (domain.PushSender, error) {
	if cfg.ProjectID == "" || cfg.CredentialsFile == "" {
		log.Warn("FCM not configured: push notifications will be logged, not delivered")
		return &logSender{log: log}, nil
	}

	// google.FindDefaultCredentials honours GOOGLE_APPLICATION_CREDENTIALS as
	// well as the explicit file, which is what container deployments use.
	creds, err := google.FindDefaultCredentials(ctx, fcmScope)
	if err != nil {
		return nil, fmt.Errorf("fcm: load credentials: %w", err)
	}

	return &fcmSender{
		projectID: cfg.ProjectID,
		client: &http.Client{
			Timeout: requestTimeout,
			// oauth2.Transport caches the access token and refreshes it when it
			// expires, so every send carries a valid bearer without us tracking it.
			Transport: &oauth2.Transport{Source: creds.TokenSource},
		},
		log: log,
	}, nil
}

// Send delivers one notification to every supplied token.
//
// The v1 API accepts a single token per request, so tokens are sent in sequence.
// A per-token failure is recorded rather than aborting the batch: one dead
// device must not stop the others from being notified.
func (s *fcmSender) Send(ctx context.Context, msg domain.PushMessage) (*domain.PushResult, error) {
	result := &domain.PushResult{}
	if len(msg.Tokens) == 0 {
		return result, nil
	}

	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectID)

	for _, token := range msg.Tokens {
		body, err := json.Marshal(buildPayload(token, msg))
		if err != nil {
			return nil, fmt.Errorf("fcm: encode payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("fcm: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		res, err := s.client.Do(req)
		if err != nil {
			result.FailureCount++
			s.log.Warn("fcm request failed", logger.Err(err))
			continue
		}

		switch {
		case res.StatusCode >= 200 && res.StatusCode < 300:
			result.SuccessCount++
		case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusBadRequest:
			// UNREGISTERED / INVALID_ARGUMENT mean the token will never work
			// again, so the caller should stop using it.
			result.FailureCount++
			result.InvalidTokens = append(result.InvalidTokens, token)
		default:
			result.FailureCount++
			s.log.Warn("fcm rejected a message",
				logger.Int("status", res.StatusCode),
				logger.String("token_suffix", tokenSuffix(token)))
		}
		_ = res.Body.Close()
	}
	return result, nil
}

// Send logs the notification instead of delivering it.
func (s *logSender) Send(_ context.Context, msg domain.PushMessage) (*domain.PushResult, error) {
	s.log.Info("push notification (not delivered: FCM disabled)",
		logger.String("title", msg.Title),
		logger.String("body", msg.Body),
		logger.Int("tokens", len(msg.Tokens)))
	return &domain.PushResult{SuccessCount: len(msg.Tokens)}, nil
}

// buildPayload produces the FCM v1 message body.
//
// Android and APNs both need their own priority block; without them a
// notification can be delayed for minutes while a device is dozing, which is
// useless for a load offer that expires in half an hour.
func buildPayload(token string, msg domain.PushMessage) map[string]any {
	highPriority := msg.Priority == "high"

	notification := map[string]any{"title": msg.Title, "body": msg.Body}
	if msg.ImageURL != "" {
		notification["image"] = msg.ImageURL
	}

	androidPriority := "normal"
	apnsPriority := "5"
	if highPriority {
		androidPriority = "high"
		apnsPriority = "10"
	}

	return map[string]any{
		"message": map[string]any{
			"token":        token,
			"notification": notification,
			"data":         msg.Data,
			"android": map[string]any{
				"priority": androidPriority,
				"notification": map[string]any{
					"sound":        "default",
					"channel_id":   "loadify_default",
					"click_action": "FLUTTER_NOTIFICATION_CLICK",
				},
			},
			"apns": map[string]any{
				"headers": map[string]any{"apns-priority": apnsPriority},
				"payload": map[string]any{
					"aps": map[string]any{"sound": "default", "content-available": 1},
				},
			},
		},
	}
}

// tokenSuffix returns the tail of a device token for log correlation without
// writing the whole credential to the log.
func tokenSuffix(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return "..." + token[len(token)-8:]
}
