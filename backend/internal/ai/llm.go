package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
	"github.com/loadify/backend/pkg/secret"
)

// maxResponseBytes bounds what we will read back. A model that starts
// generating an essay is a cost problem, not something to page through.
const maxResponseBytes = 256 << 10

// Extractor talks to an OpenAI-compatible chat completions endpoint.
//
// One adapter covers both configured providers: a hosted model and a
// self-hosted Hermes agent, which exposes the same shape. What it sends is only
// the advert and what the rules could not read — no vocabulary, no identifiers,
// nothing about the company — because the model's job is to point at words in
// the text, and everything else it could be given is something it might echo
// back as a fact.
type Extractor struct {
	client   *http.Client
	baseURL  string
	apiKey   secret.Value
	model    string
	provider string
	log      logger.Logger
}

// NewExtractor builds the adapter for the configured provider.
func NewExtractor(cfg config.AIConfig, log logger.Logger) (*Extractor, error) {
	baseURL, key := cfg.BaseURL, cfg.APIKey
	if cfg.Provider == config.AIProviderHermes {
		baseURL, key = cfg.HermesURL, cfg.HermesKey
	}
	if baseURL == "" {
		return nil, fmt.Errorf("ai: no endpoint configured for provider %q", cfg.Provider)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Extractor{
		client:   &http.Client{Timeout: timeout},
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		apiKey:   secret.Value(key),
		model:    cfg.Model,
		provider: cfg.Provider,
		log:      log,
	}, nil
}

// systemPrompt is deliberately about pointing rather than answering.
//
// Every instruction here is also enforced after the fact — evidence has to
// appear in the advert, names have to resolve, values are recomputed from the
// words — because a prompt is a request and a check is a guarantee.
const systemPrompt = `You read freight adverts written by logisticians in Uzbek, Russian or English.

You are given an advert and the fields a rules-based parser could not fill. For each of those fields, if the advert really states it, point at the exact words that state it.

Rules:
- "evidence" must be an exact substring of the advert, copied character for character. Never translate, normalise or reformat it.
- "value" is only used for names: a city, a region, a body type, a cargo type. Give the name in its dictionary form. For numbers, money and phone numbers leave "value" empty - the words in "evidence" are what matter.
- Never compute, convert or infer. If the advert does not state a field, omit that field entirely.
- Never include a field that was not asked for.
- Answer with JSON only, in this shape:
{"fields": {"<field>": {"value": "<name or empty>", "evidence": "<exact words from the advert>"}}}`

// Extract asks the model about the fields the rules left empty.
func (e *Extractor) Extract(ctx context.Context, req domain.AdExtractRequest) (domain.AdExtractResult, error) {
	payload, err := json.Marshal(chatRequest{
		Model: e.model,
		// Zero temperature because this is a reading task: the same advert
		// should produce the same answer twice.
		Temperature:    0,
		ResponseFormat: &responseFormat{Type: "json_object"},
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt(req)},
		},
	})
	if err != nil {
		return domain.AdExtractResult{}, fmt.Errorf("ai: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return domain.AdExtractResult{}, fmt.Errorf("ai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if !e.apiKey.IsZero() {
		httpReq.Header.Set("Authorization", "Bearer "+e.apiKey.Reveal())
	}

	res, err := e.client.Do(httpReq)
	if err != nil {
		return domain.AdExtractResult{}, fmt.Errorf("ai: call %s: %w", e.provider, err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return domain.AdExtractResult{}, fmt.Errorf("ai: read response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		// The body can carry the provider's own message, which is useful, but
		// it is not shown to anyone outside the log.
		return domain.AdExtractResult{}, fmt.Errorf("ai: %s returned %d: %s",
			e.provider, res.StatusCode, firstLine(string(body)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return domain.AdExtractResult{}, fmt.Errorf("ai: decode response: %w", err)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return domain.AdExtractResult{}, fmt.Errorf("ai: %s: %s", e.provider, decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return domain.AdExtractResult{}, errors.New("ai: the model returned no answer")
	}

	fields, err := parseExtraction(decoded.Choices[0].Message.Content)
	if err != nil {
		return domain.AdExtractResult{}, err
	}

	model := decoded.Model
	if model == "" {
		model = e.model
	}
	return domain.AdExtractResult{
		Fields:    fields,
		Provider:  e.provider,
		Model:     model,
		TokensIn:  decoded.Usage.PromptTokens,
		TokensOut: decoded.Usage.CompletionTokens,
	}, nil
}

// userPrompt carries the advert and the gaps, and nothing else.
func userPrompt(req domain.AdExtractRequest) string {
	var b strings.Builder
	b.WriteString("Advert:\n")
	b.WriteString(req.Text)
	b.WriteString("\n\nFields still missing: ")
	b.WriteString(strings.Join(req.Missing, ", "))
	if len(req.Unresolved) > 0 {
		b.WriteString("\nWords the parser could not place: ")
		b.WriteString(strings.Join(req.Unresolved, ", "))
	}
	return b.String()
}

// parseExtraction reads the model's JSON, tolerating the code fence some models
// wrap it in.
func parseExtraction(content string) (map[string]domain.AdExtractedValue, error) {
	trimmed := strings.TrimSpace(content)
	if fenced, found := strings.CutPrefix(trimmed, "```"); found {
		if line, rest, ok := strings.Cut(fenced, "\n"); ok && !strings.Contains(line, "{") {
			trimmed = rest
		} else {
			trimmed = fenced
		}
		trimmed = strings.TrimSuffix(strings.TrimSpace(trimmed), "```")
	}

	var decoded extraction
	if err := json.Unmarshal([]byte(strings.TrimSpace(trimmed)), &decoded); err != nil {
		return nil, fmt.Errorf("ai: the model did not answer with JSON: %w", err)
	}

	fields := make(map[string]domain.AdExtractedValue, len(decoded.Fields))
	for name, field := range decoded.Fields {
		fields[name] = domain.AdExtractedValue{Value: field.Value, Evidence: field.Evidence}
	}
	return fields, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 200 {
		return line[:200]
	}
	return line
}

// ─── the provider's wire shapes ─────────────────────────────────────

type chatRequest struct {
	Model          string          `json:"model,omitempty"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Messages       []chatMessage   `json:"messages"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type extraction struct {
	Fields map[string]struct {
		Value    string `json:"value"`
		Evidence string `json:"evidence"`
	} `json:"fields"`
}
