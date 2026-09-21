package steamapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is a Logger that keeps every line, so a test can search them.
type recorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *recorder) Infof(f string, a ...any) { r.add("INFO " + fmt.Sprintf(f, a...)) }
func (r *recorder) Warnf(f string, a ...any) { r.add("WARN " + fmt.Sprintf(f, a...)) }
func (r *recorder) add(s string) {
	r.mu.Lock()
	r.lines = append(r.lines, s)
	r.mu.Unlock()
}
func (r *recorder) text() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

// fakeSteam is both endpoints, scripted per test.
type fakeSteam struct {
	// freeResult gives the free API's Result per id; an id not listed is answered 1.
	// freeErr makes the whole free call fail.
	freeResult map[string]int
	freeErr    error
	keyedErr   error // returned by every keyed call (after the results below)
	// keyedResult is the keyed API's Result per id (default 1).
	keyedResult map[string]int
	// keyedPartial, when true, makes the keyed call return only the first id together with keyedErr.
	keyedPartial bool

	freeCalls  [][]string
	keyedCalls [][]string
	keyedKeys  []string
}

func (f *fakeSteam) free(ctx context.Context, ids []string) (map[string]PublishedFileDetails, error) {
	f.freeCalls = append(f.freeCalls, append([]string(nil), ids...))
	if f.freeErr != nil {
		return map[string]PublishedFileDetails{}, f.freeErr
	}
	res := map[string]PublishedFileDetails{}
	for _, id := range ids {
		r, ok := f.freeResult[id]
		if !ok {
			r = 1
		}
		if r == -1 { // Steam does not know the id at all: no entry
			continue
		}
		res[id] = PublishedFileDetails{ID: id, Result: r, Title: "free " + id, Source: SourceFree}
	}
	return res, nil
}

func (f *fakeSteam) keyed(ctx context.Context, key string, ids []string) (map[string]PublishedFileDetails, error) {
	f.keyedCalls = append(f.keyedCalls, append([]string(nil), ids...))
	f.keyedKeys = append(f.keyedKeys, key)
	res := map[string]PublishedFileDetails{}
	if f.keyedErr != nil && !f.keyedPartial {
		return res, f.keyedErr
	}
	for i, id := range ids {
		if f.keyedPartial && i > 0 {
			break
		}
		r, ok := f.keyedResult[id]
		if !ok {
			r = 1
		}
		res[id] = PublishedFileDetails{ID: id, Result: r, Title: "key " + id, Source: SourceKey}
	}
	return res, f.keyedErr
}

func newTestService(f *fakeSteam, mode Mode) (*Service, *recorder, *time.Time) {
	rec := &recorder{}
	s := NewService(rec)
	s.free, s.keyed = f.free, f.keyed
	clock := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	s.Configure(mode, sentinelKey)
	return s, rec, &clock
}

func TestServiceFreeModeNeverUsesTheKey(t *testing.T) {
	f := &fakeSteam{freeResult: map[string]int{"2": 9}}
	s, _, _ := newTestService(f, ModeFree)
	res, err := s.Fetch(context.Background(), []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.keyedCalls) != 0 {
		t.Errorf("free mode made %d keyed calls", len(f.keyedCalls))
	}
	if res["2"].Result != 9 {
		t.Errorf("result for the 9 item = %+v", res["2"])
	}
	if s.Status().HasKey {
		t.Error("free mode kept the key in memory")
	}
}

func TestServiceCompleteModeUsesTheKeyForEverything(t *testing.T) {
	f := &fakeSteam{}
	s, _, _ := newTestService(f, ModeComplete)
	res, err := s.Fetch(context.Background(), []string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.freeCalls) != 0 {
		t.Errorf("complete mode with a healthy key made %d free calls", len(f.freeCalls))
	}
	if len(res) != 3 || res["1"].Source != SourceKey {
		t.Errorf("res = %+v, want 3 keyed answers", res)
	}
	if f.keyedKeys[0] != sentinelKey {
		t.Error("the keyed call did not get the key")
	}
}

func TestServiceCompleteFallsBackWhenTheKeyRunsOut(t *testing.T) {
	f := &fakeSteam{keyedErr: &RateLimitedError{RetryAfter: 10 * time.Minute}}
	s, rec, clock := newTestService(f, ModeComplete)

	res, err := s.Fetch(context.Background(), []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if res["1"].Source != SourceFree || len(res) != 2 {
		t.Errorf("res = %+v, want both answered by the free API", res)
	}
	st := s.Status()
	if st.ExhaustedUntil.IsZero() || st.KeyInUse {
		t.Errorf("status = %+v, want an exhausted key that is not in use", st)
	}
	if want := clock.Add(10 * time.Minute); !st.ExhaustedUntil.Equal(want) {
		t.Errorf("ExhaustedUntil = %v, want %v (Retry-After)", st.ExhaustedUntil, want)
	}

	// While exhausted the key is not even tried.
	callsBefore := len(f.keyedCalls)
	if _, err := s.Fetch(context.Background(), []string{"3"}); err != nil {
		t.Fatal(err)
	}
	if len(f.keyedCalls) != callsBefore {
		t.Error("the key was tried again while exhausted")
	}

	// After the cooldown it is tried again.
	*clock = clock.Add(11 * time.Minute)
	f.keyedErr = nil
	res, _ = s.Fetch(context.Background(), []string{"4"})
	if res["4"].Source != SourceKey {
		t.Errorf("after the cooldown res = %+v, want the key back in use", res)
	}
	if !s.Status().ExhaustedUntil.IsZero() {
		t.Error("the exhausted state did not clear")
	}
	if strings.Contains(rec.text(), sentinelKey) {
		t.Error("a log line contains the key")
	}
}

func TestServiceExhaustionDefaultsAndBounds(t *testing.T) {
	cases := []struct {
		retry time.Duration
		want  time.Duration
	}{
		{0, time.Hour},
		{time.Second, 5 * time.Minute},
		{72 * time.Hour, 24 * time.Hour},
	}
	for _, tc := range cases {
		f := &fakeSteam{keyedErr: &RateLimitedError{RetryAfter: tc.retry}}
		s, _, clock := newTestService(f, ModeComplete)
		_, _ = s.Fetch(context.Background(), []string{"1"})
		if got := s.Status().ExhaustedUntil.Sub(*clock); got != tc.want {
			t.Errorf("Retry-After %v: cooldown = %v, want %v", tc.retry, got, tc.want)
		}
	}
}

func TestServiceRejectedKeyFallsBackAndStaysOff(t *testing.T) {
	f := &fakeSteam{keyedErr: ErrKeyRejected}
	s, rec, _ := newTestService(f, ModeComplete)

	res, err := s.Fetch(context.Background(), []string{"1"})
	if err != nil || res["1"].Source != SourceFree {
		t.Fatalf("res = %+v, err = %v, want the free answer", res, err)
	}
	if !s.Status().Rejected {
		t.Error("the key was not marked rejected")
	}
	before := len(f.keyedCalls)
	_, _ = s.Fetch(context.Background(), []string{"2"})
	if len(f.keyedCalls) != before {
		t.Error("a rejected key was tried again")
	}

	// Saving a key anew starts over.
	s.Configure(ModeComplete, "ANOTHERKEY")
	if s.Status().Rejected {
		t.Error("Configure did not clear the rejected state")
	}
	if strings.Contains(rec.text(), sentinelKey) {
		t.Error("a log line contains the key")
	}
}

func TestServiceCompleteFillsGapsFromTheFreeAPI(t *testing.T) {
	// The key returned the first item, then the quota ran out: the rest come from
	// the free API and the first keeps its keyed answer.
	f := &fakeSteam{keyedErr: &RateLimitedError{}, keyedPartial: true}
	s, _, _ := newTestService(f, ModeComplete)
	res, err := s.Fetch(context.Background(), []string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if res["1"].Source != SourceKey || res["2"].Source != SourceFree || res["3"].Source != SourceFree {
		t.Errorf("sources = %s/%s/%s, want key/free/free", res["1"].Source, res["2"].Source, res["3"].Source)
	}
	if fmt.Sprint(f.freeCalls) != "[[2 3]]" {
		t.Errorf("free calls = %v, want only the two missing ids", f.freeCalls)
	}
}

func TestServiceCompleteOtherKeyedFailureUsesFreeForThisCall(t *testing.T) {
	f := &fakeSteam{keyedErr: errors.New("steamapi: keyed details request failed: HTTP 503")}
	s, _, _ := newTestService(f, ModeComplete)
	res, err := s.Fetch(context.Background(), []string{"1"})
	if err != nil || res["1"].Source != SourceFree {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	st := s.Status()
	if st.Rejected || !st.ExhaustedUntil.IsZero() || !st.KeyInUse {
		t.Errorf("a transient failure changed the key's state: %+v", st)
	}
	if st.LastError == "" {
		t.Error("the failure was not recorded in LastError")
	}
}

func TestServiceBackupAsksTheKeyOnlyForWhatFreeCouldNotAnswer(t *testing.T) {
	f := &fakeSteam{freeResult: map[string]int{"2780180614": 9, "5": -1}}
	s, rec, _ := newTestService(f, ModeBackup)
	res, err := s.Fetch(context.Background(), []string{"1", "2780180614", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.keyedCalls) != "[[2780180614 5]]" {
		t.Errorf("keyed calls = %v, want only the unlisted id and the one free did not know", f.keyedCalls)
	}
	if res["1"].Source != SourceFree {
		t.Errorf("item 1 came from %q, want the free API", res["1"].Source)
	}
	if res["2780180614"].Source != SourceKey || res["2780180614"].Result != 1 {
		t.Errorf("the unlisted item = %+v, want the keyed answer", res["2780180614"])
	}
	if got := s.Status().Stats.Rescued; got != 2 {
		t.Errorf("Rescued = %d, want 2", got)
	}
	if !strings.Contains(rec.text(), "2 Workshop items the free API could not") {
		t.Errorf("the rescue was not logged: %q", rec.text())
	}
}

func TestServiceBackupWithNothingToRescueNeverCallsTheKey(t *testing.T) {
	f := &fakeSteam{}
	s, _, _ := newTestService(f, ModeBackup)
	if _, err := s.Fetch(context.Background(), []string{"1", "2"}); err != nil {
		t.Fatal(err)
	}
	if len(f.keyedCalls) != 0 {
		t.Errorf("backup mode called the key %d times with nothing to rescue", len(f.keyedCalls))
	}
}

func TestServiceBackupKeyedAnswerIsMorePreciseForADeletedItem(t *testing.T) {
	f := &fakeSteam{freeResult: map[string]int{"7": 9}, keyedResult: map[string]int{"7": 86}}
	s, _, _ := newTestService(f, ModeBackup)
	res, _ := s.Fetch(context.Background(), []string{"7"})
	if res["7"].Result != 86 {
		t.Errorf("Result = %d, want 86 (ItemDeleted from the key, not the free API's 9)", res["7"].Result)
	}
	if s.Status().Stats.Rescued != 0 {
		t.Error("a deleted item counted as rescued")
	}
}

func TestServiceBackupWhenFreeFailsEntirelyUsesTheKeyForAll(t *testing.T) {
	f := &fakeSteam{freeErr: errors.New("steamapi: down")}
	s, _, _ := newTestService(f, ModeBackup)
	res, err := s.Fetch(context.Background(), []string{"1", "2"})
	if err != nil || len(res) != 2 || res["1"].Source != SourceKey {
		t.Fatalf("res = %+v, err = %v, want both from the key", res, err)
	}
}

func TestServiceBackupKeyProblemsKeepFreeAnswers(t *testing.T) {
	for _, keyErr := range []error{ErrKeyRejected, &RateLimitedError{}} {
		f := &fakeSteam{freeResult: map[string]int{"2": 9}, keyedErr: keyErr}
		s, _, _ := newTestService(f, ModeBackup)
		res, err := s.Fetch(context.Background(), []string{"1", "2"})
		if err != nil {
			t.Fatalf("%v: %v", keyErr, err)
		}
		if res["1"].Result != 1 || res["2"].Result != 9 {
			t.Errorf("%v: res = %+v, want the free answers untouched", keyErr, res)
		}
	}
}

func TestServiceReturnsAnErrorOnlyWhenThereIsNothing(t *testing.T) {
	f := &fakeSteam{freeErr: errors.New("free down"), keyedErr: errors.New("keyed down")}
	s, _, _ := newTestService(f, ModeComplete)
	if _, err := s.Fetch(context.Background(), []string{"1"}); err == nil {
		t.Error("want an error when both APIs failed")
	}
	f2 := &fakeSteam{freeErr: errors.New("free down")}
	s2, _, _ := newTestService(f2, ModeFree)
	if _, err := s2.Fetch(context.Background(), []string{"1"}); err == nil {
		t.Error("want an error when the free API failed in free mode")
	}
}

func TestServiceNoIDsIsANoOp(t *testing.T) {
	f := &fakeSteam{}
	s, _, _ := newTestService(f, ModeComplete)
	res, err := s.Fetch(context.Background(), nil)
	if err != nil || len(res) != 0 || len(f.keyedCalls)+len(f.freeCalls) != 0 {
		t.Errorf("res = %v, err = %v, calls = %d", res, err, len(f.keyedCalls)+len(f.freeCalls))
	}
}

func TestParseModeDefaultsToFree(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeFree, "nonsense": ModeFree, "free": ModeFree, "complete": ModeComplete, "backup": ModeBackup} {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigureFreeDropsTheKey(t *testing.T) {
	s := NewService(nil)
	s.Configure(ModeComplete, sentinelKey)
	if s.Key() != sentinelKey {
		t.Fatal("key not kept")
	}
	s.Configure(ModeFree, sentinelKey)
	if s.Key() != "" || s.Status().HasKey {
		t.Error("free mode kept the key in memory")
	}
}

func TestStatusNeverContainsTheKey(t *testing.T) {
	f := &fakeSteam{keyedErr: errors.New("boom")}
	s, _, _ := newTestService(f, ModeComplete)
	_, _ = s.Fetch(context.Background(), []string{"1"})
	if strings.Contains(fmt.Sprintf("%+v", s.Status()), sentinelKey) {
		t.Error("Status leaks the key")
	}
}
