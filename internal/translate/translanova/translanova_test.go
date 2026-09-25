package translanova

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	original := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = original })
	return srv
}

func TestTranslateReturnsTheRealTranslatedText(t *testing.T) {
	var gotPath string
	var gotBody translateRequest
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"translations": []map[string]string{{"text": "Hola, hermana", "detected_source_language": "EN", "model_type_used": "quality_optimized"}},
			"request":      map[string]string{"requestedSource": "EN", "requestedTarget": "ES"},
			"diagnostics":  map[string]any{"retryCount": 0, "model": "quality_optimized"},
		})
	})

	c := New()
	got, err := c.Translate(context.Background(), "Hello Sister", translate.Language{Code: "ES"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got != "Hola, hermana" {
		t.Errorf("Translate() = %q, want %q", got, "Hola, hermana")
	}
	if gotPath != "/api/translate" {
		t.Errorf("path = %q, want /api/translate", gotPath)
	}
	if gotBody.Text != "Hello Sister" || gotBody.SourceLang != "EN" || gotBody.TargetLang != "ES" {
		t.Errorf("request body = %+v, want {Hello Sister EN ES}", gotBody)
	}
}

func TestTranslateReturnsRateLimitedOn429(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c := New()
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if _, ok := err.(*translate.RateLimitedError); !ok {
		t.Fatalf("err = %v (%T), want *translate.RateLimitedError", err, err)
	}
}

func TestTranslateReturnsInvalidRequestOnUnexpectedStatus(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := New()
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if _, ok := err.(translate.InvalidRequestError); !ok {
		t.Fatalf("err = %v (%T), want translate.InvalidRequestError", err, err)
	}
}

// TestTranslateNeverSendsACookieEvenAfterTheServerTriesToSetOne is the
// package's own "refuse and delete" guarantee, driven end to end: a server
// that tries to plant a cookie on the very first response must never see it
// come back on a later call through the same client - concurrent, matching
// how internal/app.TranslateMod's own worker pool shares one Client across
// several goroutines when the concurrency slider is above 1.
func TestTranslateNeverSendsACookieEvenAfterTheServerTriesToSetOne(t *testing.T) {
	var mu sync.Mutex
	sawCookie := false
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Header.Get("Cookie") != "" {
			sawCookie = true
		}
		mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sneaky", Value: "1"})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"translations": []map[string]string{{"text": "ok"}},
		})
	})

	c := New()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"}); err != nil {
				t.Errorf("Translate() error = %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if sawCookie {
		t.Error("a later request sent a Cookie header - the server's Set-Cookie should never have been kept, let alone shared across concurrent calls")
	}
}

func TestTranslateFailsCleanlyOnEmptyTranslationsArray(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"translations": []any{}})
	})
	c := New()
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if err == nil {
		t.Fatal("want an error for an empty translations array")
	}
}
