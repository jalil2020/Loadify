// Package payment adapts the Uzbek payment providers to one domain interface.
//
// Each provider has its own signature scheme, callback shape and acknowledgement
// format. The adapters normalise all three so the usecase handles a single flow,
// and every adapter treats a bad signature as SignatureValid=false rather than
// an error — a rejected callback must still be recorded.
package payment

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// Click callback actions.
const (
	clickActionPrepare  = 0
	clickActionComplete = 1
)

// Click error codes. Zero means success; the negative values are the provider's
// own vocabulary and must be returned verbatim.
const (
	clickErrOK              = 0
	clickErrSignCheckFailed = -1
	clickErrIncorrectAmount = -2
	clickErrAlreadyPaid     = -4
	clickErrOrderNotFound   = -5
	clickErrTransCancelled  = -9
)

type clickGateway struct {
	cfg config.ClickConfig
	log logger.Logger
}

// NewClick builds the Click adapter.
func NewClick(cfg config.ClickConfig, log logger.Logger) domain.PaymentGateway {
	return &clickGateway{cfg: cfg, log: log}
}

func (g *clickGateway) Provider() domain.PaymentProvider { return domain.ProviderClick }

// CreateCheckout builds the hosted-checkout redirect.
//
// Click has no server-side session to create: the merchant redirects the payer
// to a signed URL and waits for the callbacks.
func (g *clickGateway) CreateCheckout(_ context.Context, req domain.CheckoutRequest) (*domain.CheckoutSession, error) {
	if g.cfg.ServiceID == "" || g.cfg.MerchantID == "" {
		return nil, fmt.Errorf("click: service id and merchant id are required")
	}

	// Click prices in soʻm, not tiyin, so the minor-unit amount is scaled down.
	amount := formatMajorUnits(req.Amount)

	params := url.Values{}
	params.Set("service_id", g.cfg.ServiceID)
	params.Set("merchant_id", g.cfg.MerchantID)
	params.Set("amount", amount)
	params.Set("transaction_param", req.OrderID.String())
	if req.ReturnURL != "" {
		params.Set("return_url", req.ReturnURL)
	}

	return &domain.CheckoutSession{
		CheckoutURL: "https://my.click.uz/services/pay?" + params.Encode(),
		Payload: map[string]any{
			"service_id": g.cfg.ServiceID,
			"amount":     amount,
		},
	}, nil
}

// ParseWebhook verifies the Click signature and normalises the callback.
//
// The signature is md5 over a fixed field order; merchant_prepare_id is present
// only on the Complete call, which is why it is appended conditionally.
func (g *clickGateway) ParseWebhook(_ context.Context, _ map[string]string, body []byte) (*domain.WebhookResult, error) {
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("click: parse callback body: %w", err)
	}

	var (
		clickTransID  = form.Get("click_trans_id")
		serviceID     = form.Get("service_id")
		merchantTrans = form.Get("merchant_trans_id")
		merchantPrep  = form.Get("merchant_prepare_id")
		amount        = form.Get("amount")
		signTime      = form.Get("sign_time")
		signString    = form.Get("sign_string")
	)
	action, _ := strconv.Atoi(form.Get("action"))
	providerError, _ := strconv.Atoi(form.Get("error"))

	expected := clickSignature(clickSignatureInput{
		ClickTransID:      clickTransID,
		ServiceID:         serviceID,
		SecretKey:         g.cfg.SecretKey,
		MerchantTransID:   merchantTrans,
		MerchantPrepareID: merchantPrep,
		Amount:            amount,
		Action:            action,
		SignTime:          signTime,
	})

	result := &domain.WebhookResult{
		EventID:         clickTransID + ":" + strconv.Itoa(action),
		EventType:       clickActionName(action),
		ProviderOrderID: clickTransID,
		SignatureValid: subtle.ConstantTimeCompare(
			[]byte(strings.ToLower(signString)), []byte(expected)) == 1,
		Raw: formToMap(form),
	}

	if orderID, err := uuid.Parse(merchantTrans); err == nil {
		result.OrderID = &orderID
	}
	if parsed, err := strconv.ParseFloat(amount, 64); err == nil {
		result.Amount = int64(parsed * 100)
	}

	// A negative error code from Click means the payer's transaction failed or
	// was reversed on their side.
	if providerError < 0 {
		result.Cancelled = true
		result.FailureReason = fmt.Sprintf("click error %d", providerError)
	} else if action == clickActionComplete {
		result.Paid = true
	}
	return result, nil
}

// WebhookResponse renders the acknowledgement Click expects.
//
// Click reads `error`, not the HTTP status, so a processing failure must be
// reported in the body with the provider's own code.
func (g *clickGateway) WebhookResponse(result *domain.WebhookResult, processErr error) any {
	response := map[string]any{
		"click_trans_id":    result.ProviderOrderID,
		"merchant_trans_id": "",
		"error":             clickErrOK,
		"error_note":        "Success",
	}
	if result.OrderID != nil {
		response["merchant_trans_id"] = result.OrderID.String()
		// Click echoes this back on the Complete call to tie the two together.
		response["merchant_prepare_id"] = result.OrderID.String()
		response["merchant_confirm_id"] = result.OrderID.String()
	}

	switch {
	case !result.SignatureValid:
		response["error"] = clickErrSignCheckFailed
		response["error_note"] = "Signature check failed"
	case processErr != nil:
		response["error"] = clickProcessingCode(processErr)
		response["error_note"] = processErr.Error()
	}
	return response
}

// clickSignatureInput is the exact field set Click signs, in order.
type clickSignatureInput struct {
	ClickTransID      string
	ServiceID         string
	SecretKey         string
	MerchantTransID   string
	MerchantPrepareID string
	Amount            string
	Action            int
	SignTime          string
}

func clickSignature(in clickSignatureInput) string {
	var b strings.Builder
	b.WriteString(in.ClickTransID)
	b.WriteString(in.ServiceID)
	b.WriteString(in.SecretKey)
	b.WriteString(in.MerchantTransID)
	// Present on Complete only; including an empty string on Prepare would
	// change the digest and break every Prepare callback.
	if in.MerchantPrepareID != "" {
		b.WriteString(in.MerchantPrepareID)
	}
	b.WriteString(in.Amount)
	b.WriteString(strconv.Itoa(in.Action))
	b.WriteString(in.SignTime)

	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func clickActionName(action int) string {
	switch action {
	case clickActionPrepare:
		return "prepare"
	case clickActionComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// clickProcessingCode maps our failure to Click's vocabulary, so the provider
// retries or gives up appropriately.
func clickProcessingCode(err error) int {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return clickErrOrderNotFound
	case strings.Contains(msg, "amount"):
		return clickErrIncorrectAmount
	case strings.Contains(msg, "already"):
		return clickErrAlreadyPaid
	case strings.Contains(msg, "cancel"):
		return clickErrTransCancelled
	default:
		return clickErrOrderNotFound
	}
}

// formatMajorUnits converts minor units (tiyin) to the decimal string the
// providers price in.
func formatMajorUnits(minor int64) string {
	return strconv.FormatFloat(float64(minor)/100, 'f', 2, 64)
}

func formToMap(form url.Values) map[string]any {
	out := make(map[string]any, len(form))
	for key := range form {
		out[key] = form.Get(key)
	}
	return out
}
