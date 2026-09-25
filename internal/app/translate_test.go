package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
)

const translateTestModName = "My Translatable Mod"

// fakeTranslator is a translate.Translator that never makes a real
// network call - it records every call and returns a canned, deterministic
// "translation" (or the scripted error) instead.
type fakeTranslator struct {
	name   string
	err    error
	calls  []string // "<key-text>/<target-code>" per call, in order
	prefix string
}

func (f *fakeTranslator) Translate(_ context.Context, text string, target translate.Language) (string, error) {
	f.calls = append(f.calls, text+"/"+target.Code)
	if f.err != nil {
		return "", f.err
	}
	p := f.prefix
	if p == "" {
		p = "TR"
	}
	return p + ":" + target.Code + ":" + text, nil
}

func (f *fakeTranslator) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

// newTranslateTestApp builds an App wired for Stellaris only, with a real,
// scannable local mod (translateTestModName) carrying real English
// localisation content, discovered via preferences.ExtraModFolders - the
// same light-weight fixture approach newWorkshopTestApp already
// established (see that function's own doc comment for why this doesn't
// need the full launcher-mod-dir machinery), plus XDG_DATA_HOME isolation
// (confirmed necessary this session: without it, a scan resolves this
// machine's own real ~/.local/share/Paradox Interactive/Stellaris mod
// list, not a test fixture).
func newTranslateTestApp(t *testing.T, englishEntries map[string]string) (*App, string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	extra := t.TempDir()
	modPath := filepath.Join(extra, translateTestModName)
	if err := os.MkdirAll(filepath.Join(modPath, "localisation", "english"), 0o755); err != nil {
		t.Fatal(err)
	}
	descriptor := "name=\"" + translateTestModName + "\"\nversion=\"1.0\"\nsupported_version=\"v4.*\"\n"
	if err := os.WriteFile(filepath.Join(modPath, "descriptor.mod"), []byte(descriptor), 0o644); err != nil {
		t.Fatal(err)
	}

	var ymlBody string
	ymlBody += "l_english:\n"
	for key, value := range englishEntries {
		ymlBody += " " + key + ":0 \"" + value + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(modPath, "localisation", "english", "a.yml"), []byte(ymlBody), 0o644); err != nil {
		t.Fatal(err)
	}

	prefs := preferences.Defaults()
	prefs.ExtraModFolders = map[string][]string{game.Stellaris.ID: {extra}}
	a := &App{
		ctx:          context.Background(),
		registry:     game.NewRegistry([]game.GameConfig{game.Stellaris}),
		preferences:  prefs,
		configAppDir: t.TempDir(),
		eventSink:    func(string, ...any) {},
	}

	summary, err := library.LoadGame(a.ctx, game.Stellaris, library.Options{ExtraFolders: a.extraModFolders(game.Stellaris.ID)})
	if err != nil {
		t.Fatalf("scanning the fixture mod: %v", err)
	}
	if len(summary.Mods) != 1 {
		t.Fatalf("fixture produced %d mods, want exactly 1: %+v", len(summary.Mods), summary.Mods)
	}
	return a, summary.Mods[0].ID
}

func TestTranslateEligibilityReportsAuthorModeOfferedForALocalMod(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	elig, err := a.TranslateEligibility(game.Stellaris.ID, modID)
	if err != nil {
		t.Fatalf("TranslateEligibility() error = %v", err)
	}
	if !elig.AuthorModeOffered {
		t.Errorf("elig = %+v, want AuthorModeOffered for a local mod", elig)
	}
	if !elig.HasEnglishContent {
		t.Errorf("elig = %+v, want HasEnglishContent", elig)
	}
	if elig.DeepLKeyReady {
		t.Errorf("elig = %+v, want DeepLKeyReady false with no key saved", elig)
	}
}

func TestTranslateEligibilityReportsNoEnglishContentForAModWithNone(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{})
	elig, err := a.TranslateEligibility(game.Stellaris.ID, modID)
	if err != nil {
		t.Fatalf("TranslateEligibility() error = %v", err)
	}
	if elig.HasEnglishContent {
		t.Error("want HasEnglishContent false for a mod with an empty English catalog")
	}
}

func TestTranslateModAuthorModeWritesTheTranslatedFileIntoTheSourceMod(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-1", TranslateRequest{
		Service: "translanova", TargetCode: "DE", Mode: "author",
	})
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	if result.Translated != 1 {
		t.Errorf("Translated = %d, want 1", result.Translated)
	}

	contentPath, err := library.ModFolderPath(a.ctx, game.Stellaris, library.Options{ExtraFolders: a.extraModFolders(game.Stellaris.ID)}, modID)
	if err != nil {
		t.Fatalf("ModFolderPath() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(contentPath, "localisation", "german", authorModeFilePrefix+"german.yml"))
	if err != nil {
		t.Fatalf("translated file was not written: %v", err)
	}
	if !containsSubstring(string(data), "TR:DE:Hello") {
		t.Errorf("translated file = %s, want it to contain the fake translation", data)
	}
}

func TestTranslateModPlayerModeGeneratesACompanionModWithoutTouchingTheSource(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-2", TranslateRequest{
		Service: "translanova", TargetCode: "DE", Mode: "player",
	})
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	if result.CompanionModID == "" {
		t.Fatal("CompanionModID was not set for player mode")
	}
	if result.CompanionModID != library.TranslationCompanionModID(modID) {
		t.Errorf("CompanionModID = %q, want %q", result.CompanionModID, library.TranslationCompanionModID(modID))
	}

	contentPath, err := library.ModFolderPath(a.ctx, game.Stellaris, library.Options{ExtraFolders: a.extraModFolders(game.Stellaris.ID)}, modID)
	if err != nil {
		t.Fatalf("ModFolderPath() error = %v", err)
	}
	// Only the fixture's own pre-existing localisation/english should
	// exist - player mode must never add a new language folder inside
	// the source mod itself.
	entries, err := os.ReadDir(filepath.Join(contentPath, "localisation"))
	if err != nil {
		t.Fatalf("reading the source mod's own localisation folder: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "english" {
			t.Errorf("found %q under the source mod's own localisation folder - player mode must only ever read from it", e.Name())
		}
	}
}

func TestTranslateModSkipsAnAlreadyCachedKeyOnASecondRun(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	if _, err := a.TranslateMod(game.Stellaris.ID, modID, "req-3", TranslateRequest{Service: "translanova", TargetCode: "DE", Mode: "author"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("first run made %d calls, want 1", len(fake.calls))
	}

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-4", TranslateRequest{Service: "translanova", TargetCode: "DE", Mode: "author"})
	if err != nil {
		t.Fatalf("second TranslateMod() error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Errorf("second run made %d total calls, want still 1 (the cached key must not be re-translated)", len(fake.calls))
	}
	if result.Translated != 0 {
		t.Errorf("second run Translated = %d, want 0", result.Translated)
	}
}

func TestTranslateModRejectsDeepLWithNoSavedKey(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	_, err := a.TranslateMod(game.Stellaris.ID, modID, "req-5", TranslateRequest{Service: "deepl", TargetCode: "DE", Mode: "author"})
	if err == nil {
		t.Fatal("want an error when no DeepL key is saved")
	}
}

func TestTranslateModRejectsAnUnknownService(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	_, err := a.TranslateMod(game.Stellaris.ID, modID, "req-6", TranslateRequest{Service: "bing-translate", TargetCode: "DE", Mode: "author"})
	if err == nil {
		t.Fatal("want an error for an unknown service")
	}
}

func TestTranslateModRejectsAnUnknownLanguageCode(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return &fakeTranslator{}, nil }
	_, err := a.TranslateMod(game.Stellaris.ID, modID, "req-7", TranslateRequest{Service: "translanova", TargetCode: "XX", Mode: "author"})
	if err == nil {
		t.Fatal("want an error for an unknown target language code")
	}
}

func TestTranslateModFailsWithNoEnglishContentToTranslate(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{})
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return &fakeTranslator{}, nil }
	_, err := a.TranslateMod(game.Stellaris.ID, modID, "req-8", TranslateRequest{Service: "translanova", TargetCode: "DE", Mode: "author"})
	if err == nil {
		t.Fatal("want an error when the mod has no English content")
	}
}

func TestTranslateModStopsCleanlyWhenTheTranslatorFails(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{err: translate.QuotaExceededError{}}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	_, err := a.TranslateMod(game.Stellaris.ID, modID, "req-9", TranslateRequest{Service: "deepl", TargetCode: "DE", Mode: "author"})
	if err == nil {
		t.Fatal("want an error propagated from the translator")
	}
}

func TestTranslateModEmitsProgressEvents(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	var stages []string
	a.eventSink = func(name string, data ...any) {
		if name != "translate-progress" {
			return
		}
		if len(data) < 2 {
			return
		}
		if p, ok := data[1].(TranslateProgress); ok {
			stages = append(stages, p.Stage)
		}
	}

	if _, err := a.TranslateMod(game.Stellaris.ID, modID, "req-10", TranslateRequest{Service: "translanova", TargetCode: "DE", Mode: "author"}); err != nil {
		t.Fatal(err)
	}
	if len(stages) == 0 {
		t.Fatal("no progress events were emitted")
	}
	if stages[0] != "opening" {
		t.Errorf("first stage = %q, want \"opening\"", stages[0])
	}
	if stages[len(stages)-1] != "done" {
		t.Errorf("last stage = %q, want \"done\"", stages[len(stages)-1])
	}
}

func TestTranslateEligibilityReportsSourceStats(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello", "FAREWELL": "Bye"})
	elig, err := a.TranslateEligibility(game.Stellaris.ID, modID)
	if err != nil {
		t.Fatalf("TranslateEligibility() error = %v", err)
	}
	if elig.EnglishKeyCount != 2 {
		t.Errorf("EnglishKeyCount = %d, want 2", elig.EnglishKeyCount)
	}
	if elig.EnglishFileCount != 1 {
		t.Errorf("EnglishFileCount = %d, want 1", elig.EnglishFileCount)
	}
	wantTargets := len(translate.AllLanguages) - 2 // every real language except EN and the synthetic ALL entry
	if elig.TargetCount != wantTargets {
		t.Errorf("TargetCount = %d, want %d", elig.TargetCount, wantTargets)
	}
	if elig.SourcePath != "localisation/english/ or localization/english/" {
		t.Errorf("SourcePath = %q, want %q", elig.SourcePath, "localisation/english/ or localization/english/")
	}
}

func TestTranslateModProgressCarriesLanguageIndexAndCount(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	var languageStages, doneStages []TranslateProgress
	a.eventSink = func(name string, data ...any) {
		if name != "translate-progress" || len(data) < 2 {
			return
		}
		p, ok := data[1].(TranslateProgress)
		if !ok {
			return
		}
		switch p.Stage {
		case "language":
			languageStages = append(languageStages, p)
		case "language_done":
			doneStages = append(doneStages, p)
		}
	}

	// BG (Bulgarian) is one of the dropdown's own unconfirmed-folder entries.
	if _, err := a.TranslateMod(game.Stellaris.ID, modID, "req-lang", TranslateRequest{Service: "translanova", TargetCode: "BG", Mode: "author"}); err != nil {
		t.Fatal(err)
	}
	if len(languageStages) != 1 || len(doneStages) != 1 {
		t.Fatalf("got %d language stages and %d language_done stages, want 1 each", len(languageStages), len(doneStages))
	}
	if languageStages[0].LanguageIndex != 1 || languageStages[0].LanguageTotal != 1 {
		t.Errorf("language stage LanguageIndex/Total = %d/%d, want 1/1", languageStages[0].LanguageIndex, languageStages[0].LanguageTotal)
	}
	if languageStages[0].Message == "" {
		t.Error("language stage Message is empty, want a note about BG's unconfirmed folder name")
	}
	if doneStages[0].Count != 1 {
		t.Errorf("language_done Count = %d, want 1 (one key translated and written)", doneStages[0].Count)
	}
	if doneStages[0].LanguageIndex != 1 || doneStages[0].LanguageTotal != 1 {
		t.Errorf("language_done LanguageIndex/Total = %d/%d, want 1/1", doneStages[0].LanguageIndex, doneStages[0].LanguageTotal)
	}
	wantFile := "localisation/bulgarian/zzz_parallax_auto_translated_l_bulgarian.yml"
	if languageStages[0].OutputFile != wantFile {
		t.Errorf("language stage OutputFile = %q, want %q", languageStages[0].OutputFile, wantFile)
	}
	if doneStages[0].OutputFile != wantFile {
		t.Errorf("language_done OutputFile = %q, want %q", doneStages[0].OutputFile, wantFile)
	}
}

func TestTranslateModAllLanguagesTranslatesIntoEveryRealLanguage(t *testing.T) {
	a, modID := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	fake := &fakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-11", TranslateRequest{
		Service: "translanova", TargetCode: translate.AllLanguagesCode, Mode: "author",
	})
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	wantCalls := len(translate.AllLanguages) - 2 // every real language except EN and the synthetic ALL entry
	if result.Translated != wantCalls {
		t.Errorf("Translated = %d, want %d (one call per real target language)", result.Translated, wantCalls)
	}
}

// concurrentFakeTranslator is fakeTranslator's own thread-safe twin -
// fakeTranslator's plain slice append is fine everywhere else in this file
// (Workers unset there, so TranslateMod's own worker pool never exceeds one
// in flight), but the concurrency slider tests below deliberately drive
// several goroutines through the same Translator at once, tracking the real
// high-water mark of simultaneously in-flight calls.
type concurrentFakeTranslator struct {
	mu           sync.Mutex
	calls        int
	inFlight     int
	maxInFlight  int
	perCallDelay time.Duration
}

func (f *concurrentFakeTranslator) Translate(ctx context.Context, text string, target translate.Language) (string, error) {
	f.mu.Lock()
	f.calls++
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	f.mu.Unlock()

	if f.perCallDelay > 0 {
		select {
		case <-time.After(f.perCallDelay):
		case <-ctx.Done():
			f.mu.Lock()
			f.inFlight--
			f.mu.Unlock()
			return "", ctx.Err()
		}
	}

	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
	return "TR:" + target.Code + ":" + text, nil
}

func (f *concurrentFakeTranslator) Name() string { return "fake-concurrent" }

// manyEnglishEntries builds n distinct localisation keys - enough real work
// for a worker pool to actually have something to spread across goroutines.
func manyEnglishEntries(n int) map[string]string {
	out := make(map[string]string, n)
	for i := 0; i < n; i++ {
		out[fmt.Sprintf("KEY_%02d", i)] = fmt.Sprintf("English text %d", i)
	}
	return out
}

// TestTranslateModWithoutWorkersSetStaysFullySequential is the concurrency
// slider's own default-off guarantee: every other test in this file omits
// Workers entirely (the zero value), and none of them may observe more than
// one call in flight at a time - a silent behavior change for existing
// callers would be exactly the kind of regression -race is meant to catch.
func TestTranslateModWithoutWorkersSetStaysFullySequential(t *testing.T) {
	a, modID := newTranslateTestApp(t, manyEnglishEntries(8))
	fake := &concurrentFakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-workers-0", TranslateRequest{
		Service: "translanova", TargetCode: "DE", Mode: "author",
	})
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	if result.Translated != 8 {
		t.Errorf("Translated = %d, want 8", result.Translated)
	}
	if fake.maxInFlight != 1 {
		t.Errorf("maxInFlight = %d, want 1 (Workers unset must never run more than one call at a time)", fake.maxInFlight)
	}
}

// TestTranslateModRunsUpToWorkersKeysConcurrently is the slider's own
// positive case: with Workers set above 1 and a translator slow enough that
// overlap is only possible under real concurrency, several calls must
// actually be in flight together - not just requested with more workers and
// still run one at a time.
func TestTranslateModRunsUpToWorkersKeysConcurrently(t *testing.T) {
	a, modID := newTranslateTestApp(t, manyEnglishEntries(12))
	fake := &concurrentFakeTranslator{perCallDelay: 40 * time.Millisecond}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	start := time.Now()
	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-workers-4", TranslateRequest{
		Service: "translanova", TargetCode: "DE", Mode: "author", Workers: 4,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	if result.Translated != 12 {
		t.Errorf("Translated = %d, want 12", result.Translated)
	}
	if fake.calls != 12 {
		t.Errorf("calls = %d, want exactly 12 (no dropped or duplicated key)", fake.calls)
	}
	if fake.maxInFlight < 2 {
		t.Errorf("maxInFlight = %d, want at least 2 - Workers: 4 should let calls overlap", fake.maxInFlight)
	}
	if fake.maxInFlight > 4 {
		t.Errorf("maxInFlight = %d, want at most 4 (Workers: 4)", fake.maxInFlight)
	}
	// Fully sequential would take 12*40ms=480ms; four-wide concurrency should
	// finish in roughly a third of that. A generous ceiling keeps this from
	// flaking under a loaded CI machine while still catching "not actually
	// concurrent" regressions.
	if elapsed > 350*time.Millisecond {
		t.Errorf("took %s, want well under the ~480ms a fully sequential run would take", elapsed)
	}
}

// TestTranslateModClampsWorkersAboveTheDocumentedMaximum is the slider's own
// upper-bound guarantee - a caller asking for far more than 16 (a stale
// frontend, a hand-crafted request) must never actually run more than 16
// calls at once.
func TestTranslateModClampsWorkersAboveTheDocumentedMaximum(t *testing.T) {
	a, modID := newTranslateTestApp(t, manyEnglishEntries(24))
	fake := &concurrentFakeTranslator{perCallDelay: 10 * time.Millisecond}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-workers-999", TranslateRequest{
		Service: "translanova", TargetCode: "DE", Mode: "author", Workers: 999,
	})
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	if result.Translated != 24 {
		t.Errorf("Translated = %d, want 24", result.Translated)
	}
	if fake.maxInFlight > maxTranslateWorkers {
		t.Errorf("maxInFlight = %d, want at most the documented maximum of %d", fake.maxInFlight, maxTranslateWorkers)
	}
}

// TestTranslateModClampsNegativeWorkersToOne is the same clamp's own lower
// bound - a negative value must never reach errgroup.Group.SetLimit, where
// 0 would deadlock every worker forever and a negative number means
// "unlimited" instead of "invalid".
func TestTranslateModClampsNegativeWorkersToOne(t *testing.T) {
	a, modID := newTranslateTestApp(t, manyEnglishEntries(4))
	fake := &concurrentFakeTranslator{}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	result, err := a.TranslateMod(game.Stellaris.ID, modID, "req-workers-neg", TranslateRequest{
		Service: "translanova", TargetCode: "DE", Mode: "author", Workers: -5,
	})
	if err != nil {
		t.Fatalf("TranslateMod() error = %v", err)
	}
	if result.Translated != 4 {
		t.Errorf("Translated = %d, want 4", result.Translated)
	}
	if fake.maxInFlight != 1 {
		t.Errorf("maxInFlight = %d, want 1 (a negative Workers must clamp to 1, not be treated as unlimited)", fake.maxInFlight)
	}
}

// TestTranslateModStopsPromptlyWhenCancelledMidRunWithSeveralWorkersInFlight
// is the worker pool's own cancellation guarantee: CancelTranslate has to
// still stop a run promptly when several goroutines are genuinely in flight
// together, not just when there was ever only one call outstanding at a
// time. perCallDelay is deliberately far longer than this test's own
// timeout, so a broken cancellation path (for example the per-language
// errgroup not actually deriving from the cancellable context) would make
// this test time out rather than quietly pass late.
func TestTranslateModStopsPromptlyWhenCancelledMidRunWithSeveralWorkersInFlight(t *testing.T) {
	a, modID := newTranslateTestApp(t, manyEnglishEntries(20))
	fake := &concurrentFakeTranslator{perCallDelay: 3 * time.Second}
	a.translatorFor = func(string, string, string) (translate.Translator, error) { return fake, nil }

	const requestID = "req-cancel-concurrent"
	errCh := make(chan error, 1)
	go func() {
		_, err := a.TranslateMod(game.Stellaris.ID, modID, requestID, TranslateRequest{
			Service: "translanova", TargetCode: "DE", Mode: "author", Workers: 4,
		})
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond) // let the run actually start and register its cancel func
	a.CancelTranslate(requestID)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("want an error from a cancelled run")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("TranslateMod did not stop promptly after CancelTranslate - a worker may have kept running past cancellation")
	}
}

func TestCancelTranslateIsANoOpForAnUnknownRequestID(t *testing.T) {
	a, _ := newTranslateTestApp(t, map[string]string{"GREETING": "Hello"})
	a.CancelTranslate("no-such-request") // must not panic
}

func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
