// Package vust talks to Vust (https://www.vust.ai), an unofficial, free,
// DeepL-powered wrapper - no API key, but a hard, explicit requirement:
// generate a fresh random client-id cookie for every single request and
// never let any cookie persist or be reused across requests. Request and
// response shapes are both confirmed real (the user's own working
// request, and a real response they captured):
//
//	POST https://www.vust.ai/api/tools/translate
//	{"text":"Hello Sister","target_lang":"ES","source_lang":"EN"}
//	cookie: vust_client_id=<uuid>
//	->
//	{
//	  "output": "...", "detected_source_lang": "EN",
//	  "meta": {"provider": "DeepL", "took_ms": 400},
//	  "quota": {"remaining": 9, "maxBucket": 10, "totalSaved": 0.1, "isPro": false}
//	}
//
// quota.remaining doubles as a live correctness check for the cookie
// requirement: a genuinely fresh client-id makes every request look like a
// brand-new client's first call, so remaining should read the same value
// (9, per the confirmed example) every time, never decreasing - see
// LastQuota and the package's own test for the automated version of this
// check.
package vust

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
)

// baseURL is a var so tests can point it at a local httptest.Server.
var baseURL = "https://www.vust.ai"

const maxResponseBytes = 1 << 20

const userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:155.0) Gecko/20100101 Firefox/155.0"

// Client carries no cookie/session state of its own between calls -
// deliberately, since none may ever persist. Its only state is the last
// observed quota, purely for diagnostics/verification (LastQuota), never
// used to make a real translate decision.
type Client struct {
	mu        sync.Mutex
	lastQuota Quota
}

func New() *Client { return &Client{} }

func (c *Client) Name() string { return "Vust" }

// Quota mirrors the confirmed "quota" object in Vust's own response.
type Quota struct {
	Remaining  int     `json:"remaining"`
	MaxBucket  int     `json:"maxBucket"`
	TotalSaved float64 `json:"totalSaved"`
	IsPro      bool    `json:"isPro"`
}

// LastQuota returns the quota reported by the most recent successful
// Translate call - for diagnostics and for the live "did cookie hygiene
// actually hold" check described in the package doc comment. Zero value
// before any call has succeeded.
func (c *Client) LastQuota() Quota {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastQuota
}

type translateRequest struct {
	Text       string `json:"text"`
	TargetLang string `json:"target_lang"`
	SourceLang string `json:"source_lang"`
}

type translateResponse struct {
	Output string `json:"output"`
	Quota  Quota  `json:"quota"`
}

// Translate makes exactly one request, with a brand-new http.Client (no
// Jar field set - nil, so it is structurally impossible for a cookie to
// persist on it) and a brand-new random vust_client_id cookie, constructed
// fresh right here and never stored or reused - the hard requirement this
// package exists to satisfy by construction, not by discipline.
func (c *Client) Translate(ctx context.Context, text string, target translate.Language) (string, error) {
	body, err := json.Marshal(translateRequest{Text: text, TargetLang: target.Code, SourceLang: "EN"})
	if err != nil {
		return "", fmt.Errorf("vust: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/tools/translate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("vust: building request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("cookie", "vust_client_id="+uuid.NewString())
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", baseURL+"/translate")

	fresh := &http.Client{Timeout: 15 * time.Second} // no Jar - a cookie cannot persist on this, by construction
	resp, err := fresh.Do(req)
	if err != nil {
		return "", fmt.Errorf("vust: request failed: %w", err)
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
		return "", fmt.Errorf("vust: decoding response: %w", err)
	}
	if parsed.Output == "" {
		return "", translate.InvalidRequestError{Detail: "response had no output"}
	}
	c.mu.Lock()
	c.lastQuota = parsed.Quota
	c.mu.Unlock()
	return parsed.Output, nil
}
