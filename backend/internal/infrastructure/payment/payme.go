package payment

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// Payme JSON-RPC method names.
const (
	paymeCheckPerform = "CheckPerformTransaction"
	paymeCreate       = "CreateTransaction"
	paymePerform      = "PerformTransaction"
	paymeCancel       = "CancelTransaction"
	paymeCheck        = "CheckTransaction"
)

// Payme error codes. These are part of the merchant protocol and must be
// returned exactly.
const (
	paymeErrTransportOrAuth  = -32504
	paymeErrOrderNotFound    = -31050
	paymeErrWrongAmount      = -31001
	paymeErrCannotPerform    = -31008
	paymeErrTransactionState = -31003
)

type paymeGateway struct {
	cfg config.PaymeConfig
	log logger.Logger
}

// NewPayme builds the Payme adapter.
func NewPayme(cfg config.PaymeConfig, log logger.Logger) domain.PaymentGateway {
	return &paymeGateway{cfg: cfg, log: log}
}

func (g *paymeGateway) Provider() domain.PaymentProvider { return domain.ProviderPayme }

// CreateCheckout builds the Payme checkout redirect.
//
// Payme encodes the whole parameter set as base64 in the path rather than as a
// query string, and there is no server call to make first.
func (g *paymeGateway) CreateCheckout(_ context.Context, req domain.CheckoutRequest) (*domain.CheckoutSession, error) {
	if g.cfg.MerchantID == "" {
		return nil, fmt.Errorf("payme: merchant id is required")
	}

	// Payme takes the amount in tiyin, which is already our storage unit.
	parts := []string{
		"m=" + g.cfg.MerchantID,
		"ac.order_id=" + req.OrderID.String(),
		fmt.Sprintf("a=%d", req.Amount),
	}
	if req.ReturnURL != "" {
		parts = append(parts, "c="+req.ReturnURL)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Join(parts, ";")))

	return &domain.CheckoutSession{
		CheckoutURL: "https://checkout.paycom.uz/" + url.PathEscape(encoded),
		Payload:     map[string]any{"merchant_id": g.cfg.MerchantID, "amount": req.Amount},
	}, nil
}

// paymeRequest is the JSON-RPC envelope Payme sends.
type paymeRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		ID      string         `json:"id"`
		Time    int64          `json:"time"`
		Amount  int64          `json:"amount"`
		Reason  *int           `json:"reason"`
		Account map[string]any `json:"account"`
	} `json:"params"`
}

// ParseWebhook authenticates the caller and normalises the JSON-RPC call.
//
// Payme authenticates with HTTP Basic using the literal login "Paycom" and the
// merchant key as the password — there is no per-request signature, so the
// header is the only thing standing between the endpoint and the world.
func (g *paymeGateway) ParseWebhook(_ context.Context, headers map[string]string, body []byte) (*domain.WebhookResult, error) {
	var req paymeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("payme: parse request: %w", err)
	}

	raw := map[string]any{}
	_ = json.Unmarshal(body, &raw)

	result := &domain.WebhookResult{
		EventID:         req.Params.ID + ":" + req.Method,
		EventType:       req.Method,
		ProviderOrderID: req.Params.ID,
		Amount:          req.Params.Amount,
		SignatureValid:  g.authorised(headers),
		Raw:             raw,
	}
	// Payme carries our order id inside the account object, under the field name
	// configured on the merchant cabinet.
	if orderID, ok := req.Params.Account["order_id"].(string); ok {
		if parsed, err := uuid.Parse(orderID); err == nil {
			result.OrderID = &parsed
		}
	}

	switch req.Method {
	case paymePerform:
		result.Paid = true
	case paymeCancel:
		result.Cancelled = true
		if req.Params.Reason != nil {
			result.FailureReason = fmt.Sprintf("payme cancel reason %d", *req.Params.Reason)
		}
	}
	return result, nil
}

// authorised checks the Basic credentials in constant time.
func (g *paymeGateway) authorised(headers map[string]string) bool {
	header := headerValue(headers, "Authorization")
	after, found := strings.CutPrefix(header, "Basic ")
	if !found {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(after))
	if err != nil {
		return false
	}
	expected := "Paycom:" + g.cfg.SecretKey
	return subtle.ConstantTimeCompare(decoded, []byte(expected)) == 1
}

// WebhookResponse renders the JSON-RPC reply.
//
// Payme always expects HTTP 200 with the outcome in the body; a non-200 is
// treated as a transport failure and retried indefinitely.
func (g *paymeGateway) WebhookResponse(result *domain.WebhookResult, processErr error) any {
	if !result.SignatureValid {
		return paymeError(paymeErrTransportOrAuth, "Insufficient privileges to perform this method")
	}
	if processErr != nil {
		code, message := paymeErrorFor(processErr)
		return paymeError(code, message)
	}

	// The success shape differs per method; anything the merchant protocol does
	// not define a payload for gets a bare allow.
	switch result.EventType {
	case paymeCheckPerform:
		return map[string]any{"result": map[string]any{"allow": true}}
	case paymeCreate:
		return map[string]any{"result": map[string]any{
			"transaction": result.ProviderOrderID,
			"state":       1,
		}}
	case paymePerform:
		return map[string]any{"result": map[string]any{
			"transaction": result.ProviderOrderID,
			"state":       2,
		}}
	case paymeCancel:
		return map[string]any{"result": map[string]any{
			"transaction": result.ProviderOrderID,
			"state":       -1,
		}}
	case paymeCheck:
		return map[string]any{"result": map[string]any{
			"transaction": result.ProviderOrderID,
			"state":       2,
		}}
	default:
		return map[string]any{"result": map[string]any{"allow": true}}
	}
}

func paymeError(code int, message string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code": code,
			"message": map[string]string{
				"uz": message,
				"ru": message,
				"en": message,
			},
		},
	}
}

// paymeErrorFor maps our failure onto the merchant protocol's codes.
func paymeErrorFor(err error) (int, string) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return paymeErrOrderNotFound, "Order not found"
	case strings.Contains(msg, "amount"):
		return paymeErrWrongAmount, "Incorrect amount"
	case strings.Contains(msg, "already"), strings.Contains(msg, "settled"):
		return paymeErrTransactionState, "Transaction is in an invalid state"
	default:
		return paymeErrCannotPerform, "Unable to perform the operation"
	}
}

// headerValue reads a header case-insensitively, since header maps arrive
// canonicalised differently depending on the transport.
func headerValue(headers map[string]string, name string) string {
	if v, ok := headers[name]; ok {
		return v
	}
	lower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return ""
}
