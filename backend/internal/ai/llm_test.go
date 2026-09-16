package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loadify/backend/internal/ai"
	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

const advert = "Кашри → Москва 20 тонн"

// recordingServer answers with the given body and keeps what it was sent.
type recordingServer struct {
	*httptest.Server
	body   []byte
	header http.Header
}

func newServer(t *testing.T, status int, response string) *recordingServer {
	t.Helper()
	rec := &recordingServer{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.body, _ = io.ReadAll(r.Body)
		rec.header = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(rec.Close)
	return rec
}

func extractorFor(t *testing.T, url string) *ai.Extractor {
	t.Helper()
	extractor, err := ai.NewExtractor(config.AIConfig{
		Provider: config.AIProviderLLM,
		BaseURL:  url,
		APIKey:   "sk-test-key",
		Model:    "claude-haiku-4-5",
		Timeout:  5 * time.Second,
	}, logger.New("test", "error"))
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}
	return extractor
}

const goodAnswer = `{
  "model": "claude-haiku-4-5",
  "choices": [{"message": {"role": "assistant", "content": "{\"fields\": {\"origin\": {\"value\": \"Карши\", \"evidence\": \"Кашри\"}}}"}}],
  "usage": {"prompt_tokens": 210, "completion_tokens": 24}
}`

func TestExtractReadsTheAnswer(t *testing.T) {
	server := newServer(t, http.StatusOK, goodAnswer)

	result, err := extractorFor(t, server.URL).Extract(context.Background(), domain.AdExtractRequest{
		Text:       advert,
		Missing:    []string{domain.FieldOrigin, domain.FieldContactPhone},
		Unresolved: []string{"Кашри"},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	origin, found := result.Fields[domain.FieldOrigin]
	if !found {
		t.Fatalf("fields = %v, want an origin", result.Fields)
	}
	if origin.Value != "Карши" || origin.Evidence != "Кашри" {
		t.Errorf("origin = %+v", origin)
	}
	if result.TokensIn != 210 || result.TokensOut != 24 {
		t.Errorf("usage = %d/%d, want what the provider reported", result.TokensIn, result.TokensOut)
	}
	if result.Model != "claude-haiku-4-5" || result.Provider != config.AIProviderLLM {
		t.Errorf("result names %s/%s", result.Provider, result.Model)
	}
}

// What we send matters as much as what we get back: the advert and the gaps,
// and nothing about the company, its groups or its vocabulary.
func TestTheRequestCarriesOnlyTheAdvertAndTheGaps(t *testing.T) {
	server := newServer(t, http.StatusOK, goodAnswer)

	_, err := extractorFor(t, server.URL).Extract(context.Background(), domain.AdExtractRequest{
		Text:       advert,
		Missing:    []string{domain.FieldOrigin},
		Unresolved: []string{"Кашри"},
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var sent struct {
		Model          string  `json:"model"`
		Temperature    float64 `json:"temperature"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(server.body, &sent); err != nil {
		t.Fatalf("the request body is not JSON: %v", err)
	}

	if sent.Model != "claude-haiku-4-5" {
		t.Errorf("model = %q", sent.Model)
	}
	if sent.Temperature != 0 {
		t.Errorf("temperature = %v, want 0: reading the same advert twice should read the same", sent.Temperature)
	}
	if sent.ResponseFormat.Type != "json_object" {
		t.Errorf("response_format = %q, want json_object", sent.ResponseFormat.Type)
	}
	if len(sent.Messages) != 2 || sent.Messages[0].Role != "system" {
		t.Fatalf("messages = %+v", sent.Messages)
	}
	if !strings.Contains(sent.Messages[1].Content, advert) {
		t.Error("the advert is missing from the prompt")
	}
	if !strings.Contains(sent.Messages[1].Content, domain.FieldOrigin) {
		t.Error("the prompt does not say which field is missing")
	}

	// The key travels in the header, never in the body.
	if strings.Contains(string(server.body), "sk-test-key") {
		t.Error("the API key is in the request body")
	}
	if got := server.header.Get("Authorization"); got != "Bearer sk-test-key" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestExtractFailsLoudlyOnAProviderError(t *testing.T) {
	server := newServer(t, http.StatusTooManyRequests, `{"error": {"message": "rate limit reached"}}`)

	_, err := extractorFor(t, server.URL).Extract(context.Background(), domain.AdExtractRequest{Text: advert})
	if err == nil {
		t.Fatal("a 429 must be an error, so the parse falls back to the rules")
	}
	if strings.Contains(err.Error(), "sk-test-key") {
		t.Errorf("the error leaks the API key: %v", err)
	}
}

func TestExtractRejectsAnAnswerThatIsNotJSON(t *testing.T) {
	server := newServer(t, http.StatusOK,
		`{"choices": [{"message": {"content": "I think the origin is Karshi."}}]}`)

	if _, err := extractorFor(t, server.URL).Extract(context.Background(), domain.AdExtractRequest{Text: advert}); err == nil {
		t.Error("prose is not an answer this adapter can use")
	}
}

// Several providers wrap JSON in a code fence however firmly they are asked not
// to, and that is not worth failing a parse over.
func TestExtractUnwrapsAFencedAnswer(t *testing.T) {
	fenced := "{\"choices\": [{\"message\": {\"content\": \"```json\\n{\\\"fields\\\": {\\\"weight_kg\\\": {\\\"value\\\": \\\"\\\", \\\"evidence\\\": \\\"20 тонн\\\"}}}\\n```\"}}]}"
	server := newServer(t, http.StatusOK, fenced)

	result, err := extractorFor(t, server.URL).Extract(context.Background(), domain.AdExtractRequest{Text: advert})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got := result.Fields[domain.FieldWeight].Evidence; got != "20 тонн" {
		t.Errorf("evidence = %q, want the words from the advert", got)
	}
}

func TestNewExtractorNeedsAnEndpoint(t *testing.T) {
	if _, err := ai.NewExtractor(config.AIConfig{Provider: config.AIProviderLLM}, logger.New("test", "error")); err == nil {
		t.Error("a provider with no endpoint is a misconfiguration, not a silent no-op")
	}
}

// The Hermes deployment speaks the same shape; only where it lives differs.
func TestHermesUsesItsOwnEndpointAndKey(t *testing.T) {
	server := newServer(t, http.StatusOK, goodAnswer)

	extractor, err := ai.NewExtractor(config.AIConfig{
		Provider:  config.AIProviderHermes,
		HermesURL: server.URL,
		HermesKey: "hermes-key",
		Timeout:   5 * time.Second,
	}, logger.New("test", "error"))
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}

	result, err := extractor.Extract(context.Background(), domain.AdExtractRequest{Text: advert})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result.Provider != config.AIProviderHermes {
		t.Errorf("provider = %q", result.Provider)
	}
	if got := server.header.Get("Authorization"); got != "Bearer hermes-key" {
		t.Errorf("Authorization = %q, want the Hermes key", got)
	}
}
