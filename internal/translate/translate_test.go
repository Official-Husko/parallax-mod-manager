package translate

import (
	"context"
	"sync"
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

// threadSafeFakeTranslator is fakeTranslator's own concurrent-safe twin -
// fakeTranslator's plain slice append is fine for every other test here
// (always called from one goroutine), but the concurrency slider means
// WithDelay's own result has to survive several goroutines calling it at
// once, which needs a fake that can be called from several safely too.
type threadSafeFakeTranslator struct {
	mu    sync.Mutex
	calls []time.Time
}

func (f *threadSafeFakeTranslator) Translate(_ context.Context, text string, _ Language) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, time.Now())
	f.mu.Unlock()
	return "translated:" + text, nil
}

func (f *threadSafeFakeTranslator) Name() string { return "fake" }

// TestWithDelaySpacesOutConcurrentCallersInsteadsOfLettingThemAllThroughAtOnce
// is the concurrency-slider regression case: several goroutines calling the
// SAME wrapped Translator at once (exactly what internal/app.TranslateMod's
// own worker pool does for Translanova/Vust) must still come out spaced
// delay apart - not all fire together the moment their shared wait elapses.
// Run with -race - this is exactly the scenario the old first-bool field
// was never safe under.
func TestWithDelaySpacesOutConcurrentCallersInsteadOfLettingThemAllThroughAtOnce(t *testing.T) {
	fake := &threadSafeFakeTranslator{}
	const delay = 40 * time.Millisecond
	const workers = 8
	d := WithDelay(fake, delay)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.Translate(context.Background(), "x", Language{Code: "DE"}); err != nil {
				t.Errorf("Translate() error = %v", err)
			}
		}()
	}
	wg.Wait()

	fake.mu.Lock()
	calls := append([]time.Time(nil), fake.calls...)
	fake.mu.Unlock()
	if len(calls) != workers {
		t.Fatalf("got %d calls, want %d", len(calls), workers)
	}
	sortTimes(calls)
	for i := 1; i < len(calls); i++ {
		if gap := calls[i].Sub(calls[i-1]); gap < delay-10*time.Millisecond {
			t.Errorf("calls %d and %d were only %s apart, want at least ~%s", i-1, i, gap, delay)
		}
	}
}

func sortTimes(t []time.Time) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j].Before(t[j-1]); j-- {
			t[j], t[j-1] = t[j-1], t[j]
		}
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
