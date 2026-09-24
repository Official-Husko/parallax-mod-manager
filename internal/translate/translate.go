// Package translate is the shared vocabulary every translation-service
// client (internal/translate/deepl, internal/translate/translanova,
// internal/translate/vust) speaks: one small interface, the hardcoded
// target-language table, and the typed errors a caller needs to tell a
// "wait and retry" failure from a "stop, this will never succeed" one.
//
// Deliberately no companions/ isolation here, unlike internal/workshop -
// these are plain HTTPS/JSON calls, the same shape internal/steamapi and
// internal/loverslab already make directly from this process; nothing
// about native ABI interop applies.
package translate

import (
	"context"
	"fmt"
	"time"
)

// Translator translates one piece of text from English to target - always
// exactly one string per call, never a batch of several keys in one
// request, by this project's own deliberate choice (keeps each key's
// success/failure independently attributable, and matches how a person
// would use any of these three services by hand).
type Translator interface {
	Translate(ctx context.Context, text string, target Language) (string, error)
	// Name identifies the service for logs and the upload log's own lines -
	// "DeepL API", "Translanova", "Vust".
	Name() string
}

// RateLimitedError mirrors internal/steamapi/keyed.go's own RateLimitedError
// exactly (same field, same reasoning) - a typed error a caller can
// errors.As against to decide "wait and retry" rather than parsing a string.
type RateLimitedError struct {
	// RetryAfter is what the service asked for, or 0 when it did not say.
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("translate: rate limited (retry after %s)", e.RetryAfter.Round(time.Second))
	}
	return "translate: rate limited"
}

// QuotaExceededError means the account/key's own quota is used up - DeepL's
// real, documented HTTP 456. Retrying will not help until the quota resets;
// a run hitting this stops immediately.
type QuotaExceededError struct{}

func (QuotaExceededError) Error() string { return "translate: quota exceeded" }

// InvalidRequestError means the service rejected the request itself
// (DeepL's 400, or any other unexpected/error status from any of the three
// services) - never worth retrying the same request.
type InvalidRequestError struct {
	Detail string
}

func (e InvalidRequestError) Error() string {
	if e.Detail != "" {
		return "translate: invalid request: " + e.Detail
	}
	return "translate: invalid request"
}

// delayed wraps a Translator with a fixed pause before every call after the
// first - see WithDelay. Not safe for concurrent use, matching this whole
// feature's own "one key at a time, never in parallel" design - there is
// never a reason to call one Translator from more than one goroutine.
type delayed struct {
	inner Translator
	delay time.Duration
	first bool
}

// WithDelay adds a fixed politeness delay before every call after the
// first one - for the two unofficial services, which document no rate
// limit at all, so nothing better is confirmed to pace against (see
// docs/workshop-upload.md's own "confirm, don't guess" standard, applied
// here: this is a conservative default, not a discovered real limit).
// DeepL's own official client needs no such wrapping - its real, documented
// 429/Retry-After handling lives in internal/translate/deepl instead.
func WithDelay(t Translator, delay time.Duration) Translator {
	return &delayed{inner: t, delay: delay}
}

func (d *delayed) Translate(ctx context.Context, text string, target Language) (string, error) {
	if d.first {
		select {
		case <-time.After(d.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	d.first = true
	return d.inner.Translate(ctx, text, target)
}

func (d *delayed) Name() string { return d.inner.Name() }
