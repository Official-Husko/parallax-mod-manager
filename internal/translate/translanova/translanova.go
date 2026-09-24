// Package translanova talks to Translanova (https://translanova.com), an
// unofficial, free, DeepL-powered wrapper - no API key, no cookie. Request
// and response shapes are both confirmed real (the user's own working
// request, and a real response they captured):
//
//	POST https://translanova.com/api/translate
//	{"text":"Hello Sister","source_lang":"EN","target_lang":"ES"}
//	->
//	{
//	  "translations": [{"text": "...", "detected_source_language": "EN", "model_type_used": "quality_optimized"}],
//	  "request": {...}, "diagnostics": {"retryCount": 0, "model": "quality_optimized"}
//	}
//
// Nearly a passthrough of DeepL's own real response shape, plus their own
// request/diagnostics wrapper - request/diagnostics are purely
// informational (their own retry count against whatever they proxy),
// nothing this client needs to act on.
package translanova

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// baseURL is a var so tests can point it at a local httptest.Server.
var baseURL = "https://translanova.com"

const maxResponseBytes = 1 << 20

const userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:155.0) Gecko/20100101 Firefox/155.0"

// Client is stateless - Translanova needs no key, no cookie, nothing to
// configure. New exists anyway so callers construct every translator the
// same way.
type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) Name() string { return "Translanova" }

type translateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

type translateResponse struct {
	Translations []struct {
		Text string `json:"text"`
	} `json:"translations"`
}

// Translate makes exactly one request. Neither Translanova's own request
// nor response carries any documented rate-limit signal, so the caller
// (internal/app/translate.go) is expected to wrap this with
// translate.WithDelay for a conservative, fixed pace instead - nothing
// here paces on its own.
func (c *Client) Translate(ctx context.Context, text string, target translate.Language) (string, error) {
	body, err := json.Marshal(translateRequest{Text: text, SourceLang: "EN", TargetLang: target.Code})
	if err != nil {
		return "", fmt.Errorf("translanova: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/translate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("translanova: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("translanova: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", &translate.RateLimitedError{}
	}
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", translate.InvalidRequestError{Detail: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))}
	}

	var parsed translateResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("translanova: decoding response: %w", err)
	}
	if len(parsed.Translations) == 0 {
		return "", translate.InvalidRequestError{Detail: "response had no translations"}
	}
	return parsed.Translations[0].Text, nil
}
