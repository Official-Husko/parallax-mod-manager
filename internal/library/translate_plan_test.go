package library

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/translatecache"
)

func localeTestGameConfig() game.GameConfig {
	return game.GameConfig{
		ID:             "test-game",
		DisplayName:    "Test Game",
		DescriptorType: mod.DescriptorClassic,
		ScanFolders:    []string{"localisation"},
	}
}

func TestEnglishCatalogReadsAndMergesEveryEnglishFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/english/a.yml", "l_english:\n GREETING:0 \"Hello\"\n")
	writeFile(t, dir, "localisation/english/b.yml", "l_english:\n FAREWELL:0 \"Goodbye\"\n")

	got, err := EnglishCatalog(localeTestGameConfig(), dir)
	if err != nil {
		t.Fatalf("EnglishCatalog() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	byKey := map[string]SourceEntry{}
	for _, e := range got {
		byKey[e.Key] = e
	}
	if byKey["GREETING"].Text != "Hello" || byKey["FAREWELL"].Text != "Goodbye" {
		t.Errorf("entries = %+v", byKey)
	}
	if byKey["GREETING"].Hash == "" {
		t.Error("Hash was left empty")
	}
}

func TestEnglishCatalogLaterFileWinsOnASameKeyClash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/english/a_first.yml", "l_english:\n GREETING:0 \"First\"\n")
	writeFile(t, dir, "localisation/english/b_second.yml", "l_english:\n GREETING:0 \"Second\"\n")

	got, err := EnglishCatalog(localeTestGameConfig(), dir)
	if err != nil {
		t.Fatalf("EnglishCatalog() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "Second" {
		t.Errorf("got %+v, want exactly one entry with the later file's text \"Second\"", got)
	}
}

func TestEnglishCatalogIgnoresOtherLanguageFolders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/english/a.yml", "l_english:\n GREETING:0 \"Hello\"\n")
	writeFile(t, dir, "localisation/german/a.yml", "l_german:\n GREETING:0 \"Hallo\"\n")

	got, err := EnglishCatalog(localeTestGameConfig(), dir)
	if err != nil {
		t.Fatalf("EnglishCatalog() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "Hello" {
		t.Errorf("got %+v, want only the english/ entry", got)
	}
}

// TestEnglishCatalogFindsFilesUnderTheAmericanSpellingToo is the real regression test for the
// repo-wide scan-folder fix: CK3/Imperator/Victoria 3's own real launcher-settings.json convention
// spells this folder "localization", not "localisation" - a game config listing that spelling
// (not Stellaris' own British one) must still find its mods' English text.
func TestEnglishCatalogFindsFilesUnderTheAmericanSpellingToo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localization/english/a.yml", "l_english:\n GREETING:0 \"Hello\"\n")

	cfg := game.GameConfig{ID: "ck3-like", DescriptorType: mod.DescriptorClassic, ScanFolders: []string{"localization"}}
	got, err := EnglishCatalog(cfg, dir)
	if err != nil {
		t.Fatalf("EnglishCatalog() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "Hello" {
		t.Errorf("got %+v, want the American-spelled folder's own entry", got)
	}
}

// TestEnglishCatalogMergesBothSpellingsWhenAGameListsBoth mirrors data/games.jsonc's own new
// per-game ScanFolders entries (both spellings listed defensively) - a mod using either (or, in
// this fixture, both at once) must still be read correctly, with the later file winning a clash
// exactly like two files under one spelling already do.
func TestEnglishCatalogMergesBothSpellingsWhenAGameListsBoth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/english/a.yml", "l_english:\n GREETING:0 \"Hello\"\n FAREWELL:0 \"Old\"\n")
	writeFile(t, dir, "localization/english/z.yml", "l_english:\n FAREWELL:0 \"New\"\n")

	cfg := game.GameConfig{ID: "both-spellings", DescriptorType: mod.DescriptorClassic, ScanFolders: []string{"localisation", "localization"}}
	got, err := EnglishCatalog(cfg, dir)
	if err != nil {
		t.Fatalf("EnglishCatalog() error = %v", err)
	}
	byKey := map[string]SourceEntry{}
	for _, e := range got {
		byKey[e.Key] = e
	}
	if len(got) != 2 || byKey["GREETING"].Text != "Hello" || byKey["FAREWELL"].Text != "New" {
		t.Errorf("got %+v, want GREETING=Hello (localisation/) and FAREWELL=New (the later file, localization/z.yml, wins)", got)
	}
}

func TestEnglishCatalogOnAModWithNoLocalisationIsEmptyNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "common/buildings/a.txt", "thing = { cost = 1 }")

	got, err := EnglishCatalog(localeTestGameConfig(), dir)
	if err != nil {
		t.Fatalf("EnglishCatalog() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want none - this mod is not eligible for translation", got)
	}
}

func TestEnglishFileCountCountsRealFilesOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/english/a.yml", "l_english:\n GREETING:0 \"Hello\"\n")
	writeFile(t, dir, "localisation/english/b.yml", "l_english:\n FAREWELL:0 \"Goodbye\"\n")
	writeFile(t, dir, "localisation/german/a.yml", "l_german:\n GREETING:0 \"Hallo\"\n")

	got, err := EnglishFileCount(localeTestGameConfig(), dir)
	if err != nil {
		t.Fatalf("EnglishFileCount() error = %v", err)
	}
	if got != 2 {
		t.Errorf("EnglishFileCount() = %d, want 2", got)
	}
}

func TestEnglishFileCountOnAModWithNoneIsZero(t *testing.T) {
	dir := t.TempDir()
	got, err := EnglishFileCount(localeTestGameConfig(), dir)
	if err != nil {
		t.Fatalf("EnglishFileCount() error = %v", err)
	}
	if got != 0 {
		t.Errorf("EnglishFileCount() = %d, want 0", got)
	}
}

func TestTargetCatalogReadsOneLanguageFolder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/german/a.yml", "l_german:\n GREETING:0 \"Hallo\"\n")

	got, err := TargetCatalog(localeTestGameConfig(), dir, "german")
	if err != nil {
		t.Fatalf("TargetCatalog() error = %v", err)
	}
	if got["GREETING"] != "Hallo" {
		t.Errorf("got %+v, want GREETING=Hallo", got)
	}
}

func TestPlanClassifiesAMissingKeyAsANeed(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello", Hash: "h1"}}
	needs, seeds, rewrite, covered := Plan(english, map[string]string{}, translatecache.Record{}, "DE")
	if len(needs) != 1 || needs[0].Reason != "missing" {
		t.Errorf("needs = %+v, want one \"missing\" need", needs)
	}
	if len(seeds) != 0 || len(rewrite) != 0 || covered != 0 {
		t.Errorf("seeds=%v rewrite=%v covered=%d, want all empty/zero", seeds, rewrite, covered)
	}
}

func TestPlanSeedsAPreExistingValueWithNoCacheRecordInsteadOfTranslatingIt(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello", Hash: "h1"}}
	existing := map[string]string{"A": "a hand-made translation"}
	needs, seeds, _, _ := Plan(english, existing, translatecache.Record{}, "DE")
	if len(needs) != 0 {
		t.Errorf("needs = %+v, want none - a pre-existing value must never be auto-translated", needs)
	}
	if len(seeds) != 1 || seeds[0].Key != "A" {
		t.Errorf("seeds = %+v, want exactly the A entry", seeds)
	}
}

func TestPlanTreatsAMatchingCacheEntryAsCovered(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello", Hash: "h1"}}
	cache := translatecache.Record{Entries: map[string]map[string]translatecache.Entry{
		"DE": {"A": {SourceHash: "h1", TranslatedText: "Hallo", Service: "deepl"}},
	}}
	existing := map[string]string{"A": "Hallo"} // already on disk too
	needs, seeds, rewrite, covered := Plan(english, existing, cache, "DE")
	if len(needs) != 0 || len(seeds) != 0 || len(rewrite) != 0 {
		t.Errorf("needs=%v seeds=%v rewrite=%v, want all empty", needs, seeds, rewrite)
	}
	if covered != 1 {
		t.Errorf("covered = %d, want 1", covered)
	}
}

func TestPlanRewritesFromCacheWhenTheOutputFileIsMissingTheKeyButTheHashStillMatches(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello", Hash: "h1"}}
	cache := translatecache.Record{Entries: map[string]map[string]translatecache.Entry{
		"DE": {"A": {SourceHash: "h1", TranslatedText: "Hallo", Service: "deepl"}},
	}}
	// No existing target value this time - someone deleted just this line.
	needs, seeds, rewrite, covered := Plan(english, map[string]string{}, cache, "DE")
	if len(needs) != 0 || len(seeds) != 0 {
		t.Errorf("needs=%v seeds=%v, want both empty - this should be a local rewrite, not a fresh API call", needs, seeds)
	}
	if len(rewrite) != 1 || rewrite[0] != "A" {
		t.Errorf("rewrite = %v, want exactly [A]", rewrite)
	}
	if covered != 1 {
		t.Errorf("covered = %d, want 1", covered)
	}
}

func TestPlanRetranslatesWhenTheCachedSourceHashIsStale(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello there", Hash: "h2"}} // English text changed since translation
	cache := translatecache.Record{Entries: map[string]map[string]translatecache.Entry{
		"DE": {"A": {SourceHash: "h1", TranslatedText: "Hallo", Service: "deepl"}},
	}}
	needs, _, _, _ := Plan(english, map[string]string{"A": "Hallo"}, cache, "DE")
	if len(needs) != 1 || needs[0].Reason != "source-changed" {
		t.Errorf("needs = %+v, want one \"source-changed\" need", needs)
	}
}

func TestPlanNeverRetranslatesAnExemptExistingEntryEvenWhenTheSourceHashIsStale(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello there", Hash: "h2"}}
	cache := translatecache.Record{Entries: map[string]map[string]translatecache.Entry{
		"DE": {"A": {SourceHash: "h1", TranslatedText: "Hallo (human made)", Service: translatecache.ServiceExisting}},
	}}
	needs, seeds, rewrite, covered := Plan(english, map[string]string{"A": "Hallo (human made)"}, cache, "DE")
	if len(needs) != 0 {
		t.Errorf("needs = %+v, want none - an \"existing\" entry is permanently exempt", needs)
	}
	if len(seeds) != 0 || len(rewrite) != 0 {
		t.Errorf("seeds=%v rewrite=%v, want both empty", seeds, rewrite)
	}
	if covered != 1 {
		t.Errorf("covered = %d, want 1", covered)
	}
}

func TestPlanIsScopedToOneLanguageAtATime(t *testing.T) {
	english := []SourceEntry{{Key: "A", Text: "hello", Hash: "h1"}}
	cache := translatecache.Record{Entries: map[string]map[string]translatecache.Entry{
		"DE": {"A": {SourceHash: "h1", TranslatedText: "Hallo"}},
	}}
	// Asking about FR, which has no cache entries at all, must not see DE's coverage.
	needs, _, _, covered := Plan(english, map[string]string{}, cache, "FR")
	if len(needs) != 1 {
		t.Errorf("needs = %+v, want one need for FR (DE's own cache must not leak into it)", needs)
	}
	if covered != 0 {
		t.Errorf("covered = %d, want 0", covered)
	}
}
