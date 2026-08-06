package payment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

const testSecret = "click_secret_key"

func testClick() *clickGateway {
	return &clickGateway{
		cfg: config.ClickConfig{
			MerchantID: "12345",
			ServiceID:  "67890",
			SecretKey:  testSecret,
		},
		log: logger.New("test", "error"),
	}
}

// signedCallback builds a callback body carrying a correct signature, which is
// what Click actually sends.
func signedCallback(orderID string, action int, amount string, prepareID string) []byte {
	form := url.Values{}
	form.Set("click_trans_id", "999")
	form.Set("service_id", "67890")
	form.Set("merchant_trans_id", orderID)
	form.Set("amount", amount)
	form.Set("action", strconv.Itoa(action))
	form.Set("sign_time", "2026-01-01 12:00:00")
	form.Set("error", "0")
	if prepareID != "" {
		form.Set("merchant_prepare_id", prepareID)
	}

	var raw string
	raw += "999" + "67890" + testSecret + orderID
	if prepareID != "" {
		raw += prepareID
	}
	raw += amount + strconv.Itoa(action) + "2026-01-01 12:00:00"
	sum := md5.Sum([]byte(raw))
	form.Set("sign_string", hex.EncodeToString(sum[:]))

	return []byte(form.Encode())
}

func TestClickAcceptsAValidPrepareCallback(t *testing.T) {
	orderID := uuid.New()
	body := signedCallback(orderID.String(), clickActionPrepare, "50000.00", "")

	result, err := testClick().ParseWebhook(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if !result.SignatureValid {
		t.Error("a correctly signed callback was rejected")
	}
	if result.OrderID == nil || *result.OrderID != orderID {
		t.Errorf("OrderID = %v, want %v", result.OrderID, orderID)
	}
	// Click prices in soʻm; we store tiyin.
	if result.Amount != 5_000_000 {
		t.Errorf("Amount = %d, want 5000000 tiyin", result.Amount)
	}
	if result.Paid {
		t.Error("a prepare callback must not mark the order paid")
	}
}

func TestClickCompleteMarksPaid(t *testing.T) {
	orderID := uuid.New()
	// The Complete callback includes merchant_prepare_id, which is part of the
	// signed field set.
	body := signedCallback(orderID.String(), clickActionComplete, "50000.00", orderID.String())

	result, err := testClick().ParseWebhook(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if !result.SignatureValid {
		t.Fatal("a correctly signed complete callback was rejected")
	}
	if !result.Paid {
		t.Error("a complete callback should mark the order paid")
	}
}

// The signature is the only thing standing between this endpoint and anyone on
// the internet, so a tampered field must fail.
func TestClickRejectsTamperedFields(t *testing.T) {
	orderID := uuid.New()

	cases := map[string]func(url.Values){
		"amount raised":   func(f url.Values) { f.Set("amount", "500000.00") },
		"order swapped":   func(f url.Values) { f.Set("merchant_trans_id", uuid.NewString()) },
		"action changed":  func(f url.Values) { f.Set("action", "1") },
		"time changed":    func(f url.Values) { f.Set("sign_time", "2026-01-02 12:00:00") },
		"signature wiped": func(f url.Values) { f.Set("sign_string", "") },
	}

	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			body := signedCallback(orderID.String(), clickActionPrepare, "50000.00", "")
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parsing the fixture failed: %v", err)
			}
			tamper(form)

			result, err := testClick().ParseWebhook(context.Background(), nil, []byte(form.Encode()))
			if err != nil {
				t.Fatalf("ParseWebhook() error = %v", err)
			}
			if result.SignatureValid {
				t.Errorf("a callback with %s passed the signature check", name)
			}
		})
	}
}

// A signature valid for the Prepare call must not also validate the Complete
// call, or a replayed Prepare could be upgraded into a payment confirmation.
func TestClickPrepareSignatureDoesNotValidateComplete(t *testing.T) {
	orderID := uuid.New()
	body := signedCallback(orderID.String(), clickActionPrepare, "50000.00", "")
	form, _ := url.ParseQuery(string(body))

	form.Set("action", strconv.Itoa(clickActionComplete))
	form.Set("merchant_prepare_id", orderID.String())

	result, err := testClick().ParseWebhook(context.Background(), nil, []byte(form.Encode()))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if result.SignatureValid {
		t.Error("a prepare signature was accepted for a complete callback")
	}
}

func TestClickNegativeErrorCodeCancels(t *testing.T) {
	orderID := uuid.New()
	body := signedCallback(orderID.String(), clickActionComplete, "50000.00", orderID.String())
	form, _ := url.ParseQuery(string(body))
	form.Set("error", "-9")

	result, err := testClick().ParseWebhook(context.Background(), nil, []byte(form.Encode()))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if !result.Cancelled {
		t.Error("a negative provider error code should mark the callback cancelled")
	}
	if result.Paid {
		t.Error("a failed transaction must not be marked paid")
	}
}

// Click reads the outcome from the body, not the HTTP status, so a rejected
// signature must be reported with the provider's own error code.
func TestClickWebhookResponseReportsSignatureFailure(t *testing.T) {
	orderID := uuid.New()
	response := testClick().WebhookResponse(&domain.WebhookResult{
		ProviderOrderID: "999",
		OrderID:         &orderID,
		SignatureValid:  false,
	}, nil)

	body, ok := response.(map[string]any)
	if !ok {
		t.Fatalf("WebhookResponse returned %T, want a map", response)
	}
	if body["error"] != clickErrSignCheckFailed {
		t.Errorf("error = %v, want %d", body["error"], clickErrSignCheckFailed)
	}
}

func TestClickWebhookResponseMapsProcessingFailures(t *testing.T) {
	cases := map[string]int{
		"order not found":        clickErrOrderNotFound,
		"amount mismatch":        clickErrIncorrectAmount,
		"this was already paid":  clickErrAlreadyPaid,
		"the payer cancelled it": clickErrTransCancelled,
	}
	for message, want := range cases {
		response := testClick().WebhookResponse(
			&domain.WebhookResult{SignatureValid: true},
			errors.New(message),
		)
		body := response.(map[string]any)
		if body["error"] != want {
			t.Errorf("%q → error %v, want %d", message, body["error"], want)
		}
	}
}

func TestClickWebhookResponseSucceedsWhenProcessed(t *testing.T) {
	response := testClick().WebhookResponse(&domain.WebhookResult{
		ProviderOrderID: "999",
		SignatureValid:  true,
	}, nil)

	body := response.(map[string]any)
	if body["error"] != clickErrOK {
		t.Errorf("error = %v, want %d", body["error"], clickErrOK)
	}
}

func TestClickCheckoutURLCarriesTheOrder(t *testing.T) {
	orderID := uuid.New()
	session, err := testClick().CreateCheckout(context.Background(), domain.CheckoutRequest{
		OrderID:  orderID,
		Amount:   5_000_000,
		Currency: "UZS",
	})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	parsed, err := url.Parse(session.CheckoutURL)
	if err != nil {
		t.Fatalf("the checkout URL is not a URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("transaction_param") != orderID.String() {
		t.Errorf("transaction_param = %q, want the order id", query.Get("transaction_param"))
	}
	// The amount must be scaled from tiyin to soʻm, or every payment would be
	// a hundred times too large.
	if query.Get("amount") != "50000.00" {
		t.Errorf("amount = %q, want 50000.00", query.Get("amount"))
	}
}

func TestClickCheckoutRequiresConfiguration(t *testing.T) {
	gateway := &clickGateway{cfg: config.ClickConfig{}, log: logger.New("test", "error")}
	if _, err := gateway.CreateCheckout(context.Background(), domain.CheckoutRequest{
		OrderID: uuid.New(), Amount: 1000,
	}); err == nil {
		t.Error("an unconfigured gateway created a checkout")
	}
}
