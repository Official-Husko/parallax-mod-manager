// Package deepl talks to DeepL's own official API - the one service of the
// three that needs an API key. Confirmed live against
// https://developers.deepl.com/api-reference/translate/request-translation:
// free-tier base URL https://api-free.deepl.com, pro-tier api.deepl.com,
// path /v2/translate, header "Authorization: DeepL-Auth-Key <key>", JSON
// body {"text":["..."],"source_lang":"EN","target_lang":"<code>"} (text is
// an array; this package always sends exactly one string per call, per
// this whole feature's own one-key-at-a-time rule), response
// {"translations":[{"text":"...","detected_source_language":"..."}]}.
// Real, documented error codes for a free account: 429 (retryable), 456
// (quota exceeded, never retry), 400/403/404/413/414/500/504/529 (never
// retry - the request itself is invalid or unexpected).
package deepl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
)

// httpClient is a shared package-level client with a bounded timeout,
// mirroring internal/steamapi's own httpClient var - the same reasoning
// applies here: this is the one place a slow or unresponsive third party
// could otherwise hang a call indefinitely.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// freeBaseURL and proBaseURL are the two confirmed, distinct hostnames -
// vars, not consts, so tests can point them at a local httptest.Server.
var (
	freeBaseURL = "https://api-free.deepl.com"
	proBaseURL  = "https://api.deepl.com"
)

// maxResponseBytes bounds a response: a single translated string never
// comes close to this; it only guards against something unexpected.
const maxResponseBytes = 1 << 20

// Client is one configured DeepL client - one API key, one tier.
type Client struct {
	apiKey  string
	baseURL string
}

// New returns a Client for apiKey. pro selects the pro-tier hostname;
// false selects the free-tier one. Tier is an explicit caller choice, not
// auto-detected from any key-suffix convention - that convention was not
// among the facts confirmed by fetching DeepL's own docs this session, so
// it is not assumed.
func New(apiKey string, pro bool) *Client {
	base := freeBaseURL
	if pro {
		base = proBaseURL
	}
	return &Client{apiKey: apiKey, baseURL: base}
}

func (c *Client) Name() string { return "DeepL API" }

type translateRequest struct {
	Text       []string `json:"text"`
	SourceLang string   `json:"source_lang"`
	TargetLang string   `json:"target_lang"`
}

type translateResponse struct {
	Translations []struct {
		Text                   string `json:"text"`
		DetectedSourceLanguage string `json:"detected_source_language"`
	} `json:"translations"`
}

// maxRetryAttempts, initialRetryDelay and maxRetryDelay govern Translate's
// own 429 backoff, below - package-level vars (not consts) so tests can
// shrink the delay to milliseconds instead of a real multi-second wait.
var (
	maxRetryAttempts  = 5
	initialRetryDelay = 2 * time.Second
	maxRetryDelay     = 60 * time.Second
)

// Translate sends exactly one string. On a 429 it retries with backoff
// starting at the real Retry-After header (or a 2s default), doubling up
// to a cap, for a bounded number of attempts - if still rate limited after
// that, it returns *translate.RateLimitedError, since a persistent 429
// means the account itself is throttled, not just this one call, and the
// caller (internal/app/translate.go) treats that as fatal for the whole
// run rather than skipping ahead. A 456 or any other non-2xx status is
// never retried at all, per DeepL's own documented contract.
func (c *Client) Translate(ctx context.Context, text string, target translate.Language) (string, error) {
	delay := initialRetryDelay
	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		result, err := c.attempt(ctx, text, target)
		if err == nil {
			return result, nil
		}
		var rl *translate.RateLimitedError
		if !isRateLimited(err, &rl) {
			return "", err
		}
		lastErr = err
		wait := delay
		if rl.RetryAfter > 0 {
			wait = rl.RetryAfter
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		delay *= 2
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
	}
	return "", lastErr
}

func isRateLimited(err error, rl **translate.RateLimitedError) bool {
	e, ok := err.(*translate.RateLimitedError)
	if !ok {
		return false
	}
	*rl = e
	return true
}

// attempt makes exactly one real HTTP request - no retry logic here, that
// lives in Translate above so it can be unit-tested against a fake
// Translator without ever needing a real, slow multi-attempt sequence.
func (c *Client) attempt(ctx context.Context, text string, target translate.Language) (string, error) {
	body, err := json.Marshal(translateRequest{Text: []string{text}, SourceLang: "EN", TargetLang: target.Code})
	if err != nil {
		return "", fmt.Errorf("deepl: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/translate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("deepl: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "DeepL-Auth-Key "+c.apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deepl: request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var parsed translateResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&parsed); err != nil {
			return "", fmt.Errorf("deepl: decoding response: %w", err)
		}
		if len(parsed.Translations) == 0 {
			return "", translate.InvalidRequestError{Detail: "response had no translations"}
		}
		return parsed.Translations[0].Text, nil
	case http.StatusTooManyRequests:
		return "", &translate.RateLimitedError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	case 456:
		return "", translate.QuotaExceededError{}
	default:
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", translate.InvalidRequestError{Detail: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))}
	}
}

// parseRetryAfter reads a Retry-After header as a number of seconds -
// mirrors internal/steamapi/keyed.go's own parseRetryAfter, simplified
// since DeepL's own docs only ever show a seconds-count form, never an
// HTTP date.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
