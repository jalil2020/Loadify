package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// Uzum Bank callback operations.
const (
	uzumOpCheck   = "check"
	uzumOpCreate  = "create"
	uzumOpConfirm = "confirm"
	uzumOpReverse = "reverse"
	uzumOpStatus  = "status"
)

// Uzum response codes.
const (
	uzumCodeOK              = 0
	uzumCodeInvalidSign     = -10001
	uzumCodeOrderNotFound   = -31050
	uzumCodeInvalidAmount   = -31001
	uzumCodeAlreadySettled  = -31003
	uzumCodeInternalFailure = -32400
)

type uzumGateway struct {
	cfg config.UzumConfig
	log logger.Logger
}

// NewUzum builds the Uzum Bank adapter.
func NewUzum(cfg config.UzumConfig, log logger.Logger) domain.PaymentGateway {
	return &uzumGateway{cfg: cfg, log: log}
}

func (g *uzumGateway) Provider() domain.PaymentProvider { return domain.ProviderUzum }

// CreateCheckout builds the Uzum hosted-checkout redirect.
func (g *uzumGateway) CreateCheckout(_ context.Context, req domain.CheckoutRequest) (*domain.CheckoutSession, error) {
	if g.cfg.MerchantID == "" {
		return nil, fmt.Errorf("uzum: merchant id is required")
	}

	params := url.Values{}
	params.Set("merchant_id", g.cfg.MerchantID)
	if g.cfg.MerchantService != "" {
		params.Set("service_id", g.cfg.MerchantService)
	}
	params.Set("order_id", req.OrderID.String())
	// Uzum takes tiyin, matching our storage unit.
	params.Set("amount", strconv.FormatInt(req.Amount, 10))
	if req.ReturnURL != "" {
		params.Set("redirect_url", req.ReturnURL)
	}
	// The outbound signature proves the redirect came from us, so a payer cannot
	// hand-edit the amount in the URL.
	params.Set("sign", g.sign(req.OrderID.String()+strconv.FormatInt(req.Amount, 10)))

	return &domain.CheckoutSession{
		CheckoutURL: "https://checkout.uzumbank.uz/pay?" + params.Encode(),
		Payload:     map[string]any{"merchant_id": g.cfg.MerchantID, "amount": req.Amount},
	}, nil
}

// uzumRequest is the callback envelope.
type uzumRequest struct {
	RequestID     string `json:"requestId"`
	ServiceID     string `json:"serviceId"`
	Operation     string `json:"operation"`
	TransactionID string `json:"transactionId"`
	OrderID       string `json:"orderId"`
	Amount        int64  `json:"amount"`
	Timestamp     int64  `json:"timestamp"`
	Reason        string `json:"reason"`
	Sign          string `json:"sign"`
}

// ParseWebhook verifies the HMAC signature and normalises the callback.
func (g *uzumGateway) ParseWebhook(_ context.Context, headers map[string]string, body []byte) (*domain.WebhookResult, error) {
	var req uzumRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("uzum: parse request: %w", err)
	}

	raw := map[string]any{}
	_ = json.Unmarshal(body, &raw)

	// The signature may arrive in the body or as a header, depending on the
	// integration profile the merchant account is on.
	presented := req.Sign
	if presented == "" {
		presented = headerValue(headers, "X-Signature")
	}
	expected := g.sign(req.RequestID + req.Operation + req.OrderID +
		strconv.FormatInt(req.Amount, 10) + strconv.FormatInt(req.Timestamp, 10))

	result := &domain.WebhookResult{
		EventID:         req.RequestID,
		EventType:       req.Operation,
		ProviderOrderID: req.TransactionID,
		Amount:          req.Amount,
		SignatureValid: subtle.ConstantTimeCompare(
			[]byte(strings.ToLower(presented)), []byte(expected)) == 1,
		Raw: raw,
	}
	if orderID, err := uuid.Parse(req.OrderID); err == nil {
		result.OrderID = &orderID
	}

	switch req.Operation {
	case uzumOpConfirm:
		result.Paid = true
	case uzumOpReverse:
		result.Cancelled = true
		result.FailureReason = req.Reason
	}
	return result, nil
}

// WebhookResponse renders the acknowledgement Uzum expects.
func (g *uzumGateway) WebhookResponse(result *domain.WebhookResult, processErr error) any {
	response := map[string]any{
		"requestId":     result.EventID,
		"transactionId": result.ProviderOrderID,
		"code":          uzumCodeOK,
		"message":       "OK",
	}
	if result.OrderID != nil {
		response["orderId"] = result.OrderID.String()
	}

	switch {
	case !result.SignatureValid:
		response["code"] = uzumCodeInvalidSign
		response["message"] = "Invalid signature"
	case processErr != nil:
		code, message := uzumErrorFor(processErr)
		response["code"] = code
		response["message"] = message
	}
	return response
}

// sign computes the lowercase hex HMAC-SHA256 of a payload under the merchant key.
func (g *uzumGateway) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(g.cfg.SecretKey))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func uzumErrorFor(err error) (int, string) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return uzumCodeOrderNotFound, "Order not found"
	case strings.Contains(msg, "amount"):
		return uzumCodeInvalidAmount, "Invalid amount"
	case strings.Contains(msg, "already"), strings.Contains(msg, "settled"):
		return uzumCodeAlreadySettled, "Already settled"
	default:
		return uzumCodeInternalFailure, "Unable to process the request"
	}
}

// ─── registry ───────────────────────────────────────────────────────

// Registry holds the enabled gateways, keyed by provider.
type Registry struct {
	gateways map[domain.PaymentProvider]domain.PaymentGateway
}

// NewRegistry builds the set of gateways whose credentials are configured.
//
// A provider with no credentials is simply absent, so a deployment can go live
// with Click alone and add the others later without a code change.
func NewRegistry(cfg config.PaymentConfig, log logger.Logger) *Registry {
	gateways := make(map[domain.PaymentProvider]domain.PaymentGateway, 3)

	if cfg.Click.Enabled() {
		gateways[domain.ProviderClick] = NewClick(cfg.Click, log)
	}
	if cfg.Payme.Enabled() {
		gateways[domain.ProviderPayme] = NewPayme(cfg.Payme, log)
	}
	if cfg.Uzum.Enabled() {
		gateways[domain.ProviderUzum] = NewUzum(cfg.Uzum, log)
	}

	if len(gateways) == 0 {
		log.Warn("no payment provider configured: checkout will be unavailable")
	} else {
		enabled := make([]string, 0, len(gateways))
		for provider := range gateways {
			enabled = append(enabled, string(provider))
		}
		log.Info("payment providers enabled", logger.String("providers", strings.Join(enabled, ", ")))
	}
	return &Registry{gateways: gateways}
}

// Get returns the gateway for a provider, or nil when it is not configured.
func (r *Registry) Get(provider domain.PaymentProvider) domain.PaymentGateway {
	return r.gateways[provider]
}

// Enabled lists the configured providers, for the client's checkout picker.
func (r *Registry) Enabled() []string {
	out := make([]string, 0, len(r.gateways))
	for provider := range r.gateways {
		out = append(out, string(provider))
	}
	return out
}
