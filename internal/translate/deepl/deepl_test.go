package deepl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	originalFree, originalPro := freeBaseURL, proBaseURL
	freeBaseURL, proBaseURL = srv.URL, srv.URL
	t.Cleanup(func() { freeBaseURL, proBaseURL = originalFree, originalPro })

	// Shrink the retry backoff to milliseconds - these tests care about the
	// retry/give-up *logic*, never about really waiting seconds for it.
	originalAttempts, originalInitial, originalMax := maxRetryAttempts, initialRetryDelay, maxRetryDelay
	maxRetryAttempts, initialRetryDelay, maxRetryDelay = 3, time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() {
		maxRetryAttempts, initialRetryDelay, maxRetryDelay = originalAttempts, originalInitial, originalMax
	})

	return srv
}

func TestTranslateReturnsTheRealTranslatedText(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody translateRequest
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(translateResponse{
			Translations: []struct {
				Text                   string `json:"text"`
				DetectedSourceLanguage string `json:"detected_source_language"`
			}{{Text: "Hola, hermana", DetectedSourceLanguage: "EN"}},
		})
	})

	c := New("test-key", false)
	got, err := c.Translate(context.Background(), "Hello Sister", translate.Language{Code: "ES"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got != "Hola, hermana" {
		t.Errorf("Translate() = %q, want %q", got, "Hola, hermana")
	}
	if gotAuth != "DeepL-Auth-Key test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "DeepL-Auth-Key test-key")
	}
	if gotPath != "/v2/translate" {
		t.Errorf("path = %q, want /v2/translate", gotPath)
	}
	if len(gotBody.Text) != 1 || gotBody.Text[0] != "Hello Sister" {
		t.Errorf("request text = %v, want exactly one element [\"Hello Sister\"]", gotBody.Text)
	}
	if gotBody.SourceLang != "EN" || gotBody.TargetLang != "ES" {
		t.Errorf("source/target = %q/%q, want EN/ES", gotBody.SourceLang, gotBody.TargetLang)
	}
}

func TestTranslateRetriesOn429ThenSucceeds(t *testing.T) {
	calls := 0
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0") // 0 falls back to the small fixed initial delay in tests below
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(translateResponse{
			Translations: []struct {
				Text                   string `json:"text"`
				DetectedSourceLanguage string `json:"detected_source_language"`
			}{{Text: "ok"}},
		})
	})

	c := New("test-key", false)
	got, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got != "ok" {
		t.Errorf("Translate() = %q, want %q", got, "ok")
	}
	if calls != 2 {
		t.Errorf("server saw %d calls, want exactly 2 (one 429, one success)", calls)
	}
}

func TestTranslateStopsAfterRepeated429sWithRateLimitedError(t *testing.T) {
	calls := 0
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	c := New("test-key", false)
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if _, ok := err.(*translate.RateLimitedError); !ok {
		t.Fatalf("err = %v (%T), want *translate.RateLimitedError", err, err)
	}
	if calls == 0 {
		t.Error("server was never called")
	}
}

func TestTranslateReturnsQuotaExceededOn456WithoutRetrying(t *testing.T) {
	calls := 0
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(456)
	})

	c := New("test-key", false)
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if _, ok := err.(translate.QuotaExceededError); !ok {
		t.Fatalf("err = %v (%T), want translate.QuotaExceededError", err, err)
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want exactly 1 (456 must never be retried)", calls)
	}
}

func TestTranslateReturnsInvalidRequestOn400WithoutRetrying(t *testing.T) {
	calls := 0
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	})

	c := New("test-key", false)
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if _, ok := err.(translate.InvalidRequestError); !ok {
		t.Fatalf("err = %v (%T), want translate.InvalidRequestError", err, err)
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want exactly 1 (400 must never be retried)", calls)
	}
}

func TestNewSelectsTheProBaseURLWhenRequested(t *testing.T) {
	c := New("key", true)
	if c.baseURL != proBaseURL {
		t.Errorf("baseURL = %q, want the pro-tier URL %q", c.baseURL, proBaseURL)
	}
}

func TestNewSelectsTheFreeBaseURLByDefault(t *testing.T) {
	c := New("key", false)
	if c.baseURL != freeBaseURL {
		t.Errorf("baseURL = %q, want the free-tier URL %q", c.baseURL, freeBaseURL)
	}
}

func TestParseRetryAfterReadsASecondsCount(t *testing.T) {
	if got := parseRetryAfter("5"); got != 5*time.Second {
		t.Errorf("parseRetryAfter(\"5\") = %s, want 5s", got)
	}
}

func TestParseRetryAfterDefaultsToZeroWhenAbsent(t *testing.T) {
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("parseRetryAfter(\"\") = %s, want 0", got)
	}
}
