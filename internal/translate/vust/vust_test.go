package vust

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestTranslateReturnsTheRealTranslatedTextAndQuota(t *testing.T) {
	var gotPath, gotCookie, gotReferer string
	var gotBody translateRequest
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCookie = r.Header.Get("cookie")
		gotReferer = r.Header.Get("Referer")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(translateResponse{
			Output: "Hola, hermana",
			Quota:  Quota{Remaining: 9, MaxBucket: 10, TotalSaved: 0.1, IsPro: false},
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
	if gotPath != "/api/tools/translate" {
		t.Errorf("path = %q, want /api/tools/translate", gotPath)
	}
	if gotReferer != baseURL+"/translate" {
		t.Errorf("Referer = %q, want %q", gotReferer, baseURL+"/translate")
	}
	if gotCookie == "" || !strings.Contains(gotCookie, "vust_client_id=") {
		t.Errorf("cookie header = %q, want it to contain vust_client_id=", gotCookie)
	}
	if gotBody.Text != "Hello Sister" || gotBody.SourceLang != "EN" || gotBody.TargetLang != "ES" {
		t.Errorf("request body = %+v, want {Hello Sister ES EN}", gotBody)
	}

	quota := c.LastQuota()
	if quota.Remaining != 9 || quota.MaxBucket != 10 {
		t.Errorf("LastQuota() = %+v, want Remaining=9 MaxBucket=10", quota)
	}
}

// TestTranslateNeverReusesTheSameCookieAcrossRequests is the automated
// version of the plan's own required check: with a genuinely fresh
// vust_client_id every request, the server-visible cookie must differ each
// time - the real Vust service can then never see the same client identity
// twice, which is exactly why its own quota.remaining should stay flat
// (see the package doc comment) rather than depleting.
func TestTranslateNeverReusesTheSameCookieAcrossRequests(t *testing.T) {
	var cookies []string
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		cookies = append(cookies, r.Header.Get("cookie"))
		_ = json.NewEncoder(w).Encode(translateResponse{Output: "ok", Quota: Quota{Remaining: 9, MaxBucket: 10}})
	})

	c := New()
	for i := 0; i < 3; i++ {
		if _, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"}); err != nil {
			t.Fatalf("Translate() call %d error = %v", i, err)
		}
	}
	if len(cookies) != 3 {
		t.Fatalf("server saw %d requests, want 3", len(cookies))
	}
	seen := map[string]bool{}
	for i, c := range cookies {
		if seen[c] {
			t.Errorf("cookie on request %d (%q) repeats an earlier request's cookie - the hard \"never reuse\" requirement was violated", i, c)
		}
		seen[c] = true
	}
}

// TestLastQuotaStaysFlatAcrossRepeatedCallsAgainstAFakeServer mirrors the
// plan's own "live proof" check, against a fake server that always answers
// with the same quota (exactly what a real, correctly-fresh-cookied
// sequence of calls should observe against the real Vust service too - see
// the package doc comment). A real, live version of this same assertion
// (hitting the real service) is the manual verification step before
// trusting this client in production, per the plan.
func TestLastQuotaStaysFlatAcrossRepeatedCallsAgainstAFakeServer(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(translateResponse{Output: "ok", Quota: Quota{Remaining: 9, MaxBucket: 10}})
	})

	c := New()
	for i := 0; i < 3; i++ {
		if _, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"}); err != nil {
			t.Fatalf("Translate() call %d error = %v", i, err)
		}
		if got := c.LastQuota().Remaining; got != 9 {
			t.Errorf("call %d: LastQuota().Remaining = %d, want 9 every time", i, got)
		}
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

func TestTranslateFailsCleanlyOnEmptyOutput(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(translateResponse{Output: ""})
	})
	c := New()
	_, err := c.Translate(context.Background(), "hi", translate.Language{Code: "DE"})
	if err == nil {
		t.Fatal("want an error for an empty output field")
	}
}
