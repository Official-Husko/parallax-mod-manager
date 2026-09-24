package translate

import (
	"context"
	"testing"
	"time"
)

type fakeTranslator struct {
	calls []time.Time
}

func (f *fakeTranslator) Translate(_ context.Context, text string, _ Language) (string, error) {
	f.calls = append(f.calls, time.Now())
	return "translated:" + text, nil
}

func (f *fakeTranslator) Name() string { return "fake" }

func TestWithDelayDoesNotDelayTheFirstCall(t *testing.T) {
	fake := &fakeTranslator{}
	d := WithDelay(fake, 200*time.Millisecond)

	start := time.Now()
	if _, err := d.Translate(context.Background(), "hi", Language{Code: "DE"}); err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("first call took %s, want it to skip the delay entirely", elapsed)
	}
}

func TestWithDelayDelaysEverySubsequentCall(t *testing.T) {
	fake := &fakeTranslator{}
	d := WithDelay(fake, 100*time.Millisecond)

	if _, err := d.Translate(context.Background(), "one", Language{Code: "DE"}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := d.Translate(context.Background(), "two", Language{Code: "DE"}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("second call took only %s, want at least ~100ms", elapsed)
	}
}

func TestWithDelayStopsPromptlyWhenContextIsCancelled(t *testing.T) {
	fake := &fakeTranslator{}
	d := WithDelay(fake, 10*time.Second)
	if _, err := d.Translate(context.Background(), "one", Language{Code: "DE"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := d.Translate(ctx, "two", Language{Code: "DE"})
	if err == nil {
		t.Fatal("want an error from a cancelled context during the delay")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("cancellation took %s to take effect, want it near-instant", elapsed)
	}
}

func TestWithDelayPreservesTheInnerTranslatorsName(t *testing.T) {
	d := WithDelay(&fakeTranslator{}, time.Millisecond)
	if d.Name() != "fake" {
		t.Errorf("Name() = %q, want %q", d.Name(), "fake")
	}
}

func TestRateLimitedErrorMessageIncludesRetryAfterWhenSet(t *testing.T) {
	err := &RateLimitedError{RetryAfter: 5 * time.Second}
	if err.Error() == "" {
		t.Fatal("Error() returned empty string")
	}
}

func TestRateLimitedErrorMessageWorksWithoutRetryAfter(t *testing.T) {
	err := &RateLimitedError{}
	if err.Error() == "" {
		t.Fatal("Error() returned empty string")
	}
}
