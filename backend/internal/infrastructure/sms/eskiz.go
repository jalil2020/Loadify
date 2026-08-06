package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/pkg/logger"
)

// eskizSender talks to notify.eskiz.uz, the SMS gateway used in Uzbekistan.
// The auth token is valid for 30 days; we cache it and refresh on 401.
type eskizSender struct {
	cfg    config.SMSConfig
	log    logger.Logger
	client *http.Client

	mu        sync.RWMutex
	token     string
	tokenTime time.Time
}

func newEskiz(cfg config.SMSConfig, log logger.Logger) Sender {
	return &eskizSender{
		cfg:    cfg,
		log:    log,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *eskizSender) Name() string { return "eskiz" }

func (e *eskizSender) Send(ctx context.Context, phone, message string) error {
	token, err := e.ensureToken(ctx, false)
	if err != nil {
		return err
	}

	status, body, err := e.postMessage(ctx, token, phone, message)
	if err != nil {
		return err
	}

	// A stale token is the one failure worth retrying automatically.
	if status == http.StatusUnauthorized {
		token, err = e.ensureToken(ctx, true)
		if err != nil {
			return err
		}
		status, body, err = e.postMessage(ctx, token, phone, message)
		if err != nil {
			return err
		}
	}

	if status < 200 || status >= 300 {
		return fmt.Errorf("eskiz: send failed with status %d: %s", status, truncate(body, 300))
	}
	return nil
}

func (e *eskizSender) postMessage(ctx context.Context, token, phone, message string) (int, string, error) {
	form := url.Values{}
	// Eskiz expects the national number without the leading plus.
	form.Set("mobile_phone", strings.TrimPrefix(phone, "+"))
	form.Set("message", message)
	form.Set("from", e.cfg.Sender)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.cfg.EskizBaseURL+"/message/sms/send", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, "", fmt.Errorf("eskiz: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := e.client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("eskiz: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("eskiz: read response: %w", err)
	}
	return resp.StatusCode, string(body), nil
}

func (e *eskizSender) ensureToken(ctx context.Context, force bool) (string, error) {
	e.mu.RLock()
	token, issued := e.token, e.tokenTime
	e.mu.RUnlock()

	if !force && token != "" && time.Since(issued) < 20*24*time.Hour {
		return token, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	// Another goroutine may have refreshed while we waited for the write lock.
	if !force && e.token != "" && time.Since(e.tokenTime) < 20*24*time.Hour {
		return e.token, nil
	}

	payload := map[string]string{
		"email":    e.cfg.EskizEmail,
		"password": e.cfg.EskizPassword,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("eskiz: encode login: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.cfg.EskizBaseURL+"/auth/login", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("eskiz: build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("eskiz: login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", fmt.Errorf("eskiz: read login response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("eskiz: login failed with status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("eskiz: decode login response: %w", err)
	}
	if parsed.Data.Token == "" {
		return "", fmt.Errorf("eskiz: login response contained no token")
	}

	e.token, e.tokenTime = parsed.Data.Token, time.Now()
	e.log.Info("eskiz token refreshed")
	return e.token, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
