package payment

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

const paymeKey = "payme_secret_key"

func testPayme() *paymeGateway {
	return &paymeGateway{
		cfg: config.PaymeConfig{MerchantID: "merchant-1", SecretKey: paymeKey},
		log: logger.New("test", "error"),
	}
}

func basicAuth(login, password string) map[string]string {
	credentials := base64.StdEncoding.EncodeToString([]byte(login + ":" + password))
	return map[string]string{"Authorization": "Basic " + credentials}
}

func paymeBody(method string, orderID uuid.UUID, amount int64) []byte {
	return []byte(fmt.Sprintf(`{
		"id": 1,
		"method": %q,
		"params": {
			"id": "txn-abc",
			"amount": %d,
			"account": {"order_id": %q}
		}
	}`, method, amount, orderID.String()))
}

func TestPaymeAcceptsCorrectCredentials(t *testing.T) {
	orderID := uuid.New()
	result, err := testPayme().ParseWebhook(context.Background(),
		basicAuth("Paycom", paymeKey), paymeBody(paymePerform, orderID, 4_900_000))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if !result.SignatureValid {
		t.Error("correct Basic credentials were rejected")
	}
	if result.OrderID == nil || *result.OrderID != orderID {
		t.Errorf("OrderID = %v, want %v", result.OrderID, orderID)
	}
	if !result.Paid {
		t.Error("PerformTransaction should mark the order paid")
	}
	// Payme prices in tiyin, which is already our storage unit.
	if result.Amount != 4_900_000 {
		t.Errorf("Amount = %d, want 4900000", result.Amount)
	}
}

// The Basic header is the only authentication Payme provides, so every way of
// getting it wrong must fail closed.
func TestPaymeRejectsBadCredentials(t *testing.T) {
	orderID := uuid.New()
	body := paymeBody(paymePerform, orderID, 4_900_000)

	cases := map[string]map[string]string{
		"no header":       {},
		"wrong password":  basicAuth("Paycom", "not-the-key"),
		"wrong login":     basicAuth("Merchant", paymeKey),
		"not basic":       {"Authorization": "Bearer " + paymeKey},
		"malformed value": {"Authorization": "Basic !!!not-base64!!!"},
	}

	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := testPayme().ParseWebhook(context.Background(), headers, body)
			if err != nil {
				t.Fatalf("ParseWebhook() error = %v", err)
			}
			if result.SignatureValid {
				t.Errorf("%s was accepted", name)
			}
		})
	}
}

// Header maps arrive canonicalised differently depending on the transport, so
// the lookup must not be case-sensitive.
func TestPaymeAuthHeaderIsCaseInsensitive(t *testing.T) {
	credentials := base64.StdEncoding.EncodeToString([]byte("Paycom:" + paymeKey))
	headers := map[string]string{"authorization": "Basic " + credentials}

	result, err := testPayme().ParseWebhook(context.Background(), headers,
		paymeBody(paymeCheckPerform, uuid.New(), 1000))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if !result.SignatureValid {
		t.Error("a lowercase authorization header was rejected")
	}
}

func TestPaymeMapsMethodsToOutcomes(t *testing.T) {
	orderID := uuid.New()
	headers := basicAuth("Paycom", paymeKey)

	cases := []struct {
		method    string
		paid      bool
		cancelled bool
	}{
		{paymeCheckPerform, false, false},
		{paymeCreate, false, false},
		{paymePerform, true, false},
		{paymeCancel, false, true},
		{paymeCheck, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			result, err := testPayme().ParseWebhook(context.Background(), headers,
				paymeBody(tc.method, orderID, 1000))
			if err != nil {
				t.Fatalf("ParseWebhook() error = %v", err)
			}
			if result.Paid != tc.paid {
				t.Errorf("Paid = %v, want %v", result.Paid, tc.paid)
			}
			if result.Cancelled != tc.cancelled {
				t.Errorf("Cancelled = %v, want %v", result.Cancelled, tc.cancelled)
			}
		})
	}
}

// Payme retries forever on a non-200, so failures must be reported inside a
// JSON-RPC error object rather than by status code.
func TestPaymeResponseUsesProtocolErrorCodes(t *testing.T) {
	gateway := testPayme()

	unauthorised := gateway.WebhookResponse(&domain.WebhookResult{SignatureValid: false}, nil)
	if code := paymeErrorCode(t, unauthorised); code != paymeErrTransportOrAuth {
		t.Errorf("unauthorised code = %d, want %d", code, paymeErrTransportOrAuth)
	}

	notFound := gateway.WebhookResponse(
		&domain.WebhookResult{SignatureValid: true, EventType: paymePerform},
		errors.New("order not found"))
	if code := paymeErrorCode(t, notFound); code != paymeErrOrderNotFound {
		t.Errorf("not-found code = %d, want %d", code, paymeErrOrderNotFound)
	}

	wrongAmount := gateway.WebhookResponse(
		&domain.WebhookResult{SignatureValid: true, EventType: paymePerform},
		errors.New("amount mismatch"))
	if code := paymeErrorCode(t, wrongAmount); code != paymeErrWrongAmount {
		t.Errorf("amount code = %d, want %d", code, paymeErrWrongAmount)
	}
}

func TestPaymeSuccessResponseCarriesTheExpectedState(t *testing.T) {
	gateway := testPayme()

	performed := gateway.WebhookResponse(&domain.WebhookResult{
		SignatureValid: true, EventType: paymePerform, ProviderOrderID: "txn-abc",
	}, nil)
	result := performed.(map[string]any)["result"].(map[string]any)
	if result["state"] != 2 {
		t.Errorf("perform state = %v, want 2", result["state"])
	}

	cancelled := gateway.WebhookResponse(&domain.WebhookResult{
		SignatureValid: true, EventType: paymeCancel, ProviderOrderID: "txn-abc",
	}, nil)
	result = cancelled.(map[string]any)["result"].(map[string]any)
	if result["state"] != -1 {
		t.Errorf("cancel state = %v, want -1", result["state"])
	}

	allowed := gateway.WebhookResponse(&domain.WebhookResult{
		SignatureValid: true, EventType: paymeCheckPerform,
	}, nil)
	result = allowed.(map[string]any)["result"].(map[string]any)
	if result["allow"] != true {
		t.Errorf("check-perform allow = %v, want true", result["allow"])
	}
}

func TestPaymeCheckoutURLIsDecodable(t *testing.T) {
	orderID := uuid.New()
	session, err := testPayme().CreateCheckout(context.Background(), domain.CheckoutRequest{
		OrderID: orderID, Amount: 4_900_000, Currency: "UZS",
	})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if session.CheckoutURL == "" {
		t.Fatal("no checkout URL was produced")
	}
	// Payme encodes the parameters as base64 in the path; a client that cannot
	// decode it gets a broken payment page.
	if !containsBase64Params(session.CheckoutURL, orderID.String()) {
		t.Errorf("the checkout URL does not encode the order id: %s", session.CheckoutURL)
	}
}

func paymeErrorCode(t *testing.T, response any) int {
	t.Helper()
	body, ok := response.(map[string]any)
	if !ok {
		t.Fatalf("response is %T, want a map", response)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no error object: %v", body)
	}
	code, ok := errObj["code"].(int)
	if !ok {
		t.Fatalf("error code is %T, want int", errObj["code"])
	}
	return code
}

// containsBase64Params decodes the last path segment and looks for the marker,
// which is how a client recovers the parameters Payme was handed.
func containsBase64Params(checkoutURL, marker string) bool {
	idx := strings.LastIndex(checkoutURL, "/")
	if idx < 0 {
		return false
	}
	segment, err := url.PathUnescape(checkoutURL[idx+1:])
	if err != nil {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(segment)
	if err != nil {
		return false
	}
	return strings.Contains(string(decoded), marker)
}
