package translate

import "testing"

func TestAllLanguagesHasExactlyTwentyEightEntries(t *testing.T) {
	// The 27 given dropdown languages (verified by literally counting the
	// user's own supplied <option> list) plus the synthetic "All languages" entry.
	if len(AllLanguages) != 28 {
		t.Fatalf("len(AllLanguages) = %d, want 28", len(AllLanguages))
	}
}

func TestAllLanguagesConfirmedSetMatchesTheStellarisConfirmedFolders(t *testing.T) {
	wantConfirmed := map[string]string{
		"EN": "english", "FR": "french", "DE": "german", "ES": "spanish",
		"RU": "russian", "PL": "polish", "PT": "braz_por", "ZH": "simp_chinese", "JA": "japanese",
	}
	gotConfirmed := map[string]string{}
	for _, l := range AllLanguages {
		if l.Confirmed {
			gotConfirmed[l.Code] = l.ParadoxFolder
		}
	}
	if len(gotConfirmed) != len(wantConfirmed) {
		t.Fatalf("got %d confirmed languages, want %d: %v", len(gotConfirmed), len(wantConfirmed), gotConfirmed)
	}
	for code, folder := range wantConfirmed {
		if gotConfirmed[code] != folder {
			t.Errorf("Confirmed[%s].ParadoxFolder = %q, want %q", code, gotConfirmed[code], folder)
		}
	}
}

func TestAllLanguagesHasNoKoreanEntry(t *testing.T) {
	// Korean has no code at all in the required 26-entry dropdown, so it
	// must not silently appear here either - a real, deliberate absence,
	// not an oversight.
	for _, l := range AllLanguages {
		if l.ParadoxFolder == "korean" {
			t.Errorf("found a korean entry (%s) - the required dropdown has no Korean code at all", l.Code)
		}
	}
}

func TestLookupFindsARealCode(t *testing.T) {
	l, ok := Lookup("DE")
	if !ok || l.Name != "German" {
		t.Errorf("Lookup(\"DE\") = %+v, %v, want German, true", l, ok)
	}
}

func TestLookupReportsFalseForAnUnknownCode(t *testing.T) {
	if _, ok := Lookup("XX"); ok {
		t.Error("Lookup(\"XX\") reported found, want not found")
	}
}

func TestLookupNeverFindsTheSyntheticAllCode(t *testing.T) {
	// AllLanguagesCode is a real entry in AllLanguages (so the dropdown can
	// render it), but Lookup is documented as "never AllLanguagesCode - use
	// ExpandTargets for that" - only ExpandTargets should special-case it.
	// This just pins the actual current behavior so a future change to it
	// is a deliberate, reviewed choice.
	l, ok := Lookup(AllLanguagesCode)
	if !ok || l.Code != AllLanguagesCode {
		t.Errorf("Lookup(AllLanguagesCode) = %+v, %v", l, ok)
	}
}

func TestExpandTargetsForAllExcludesEnglishAndTheSyntheticEntryItself(t *testing.T) {
	targets := ExpandTargets(AllLanguagesCode)
	if len(targets) != len(AllLanguages)-2 {
		t.Fatalf("ExpandTargets(ALL) returned %d languages, want %d", len(targets), len(AllLanguages)-2)
	}
	for _, l := range targets {
		if l.Code == "EN" {
			t.Error("ExpandTargets(ALL) included English - translating English to English is a no-op")
		}
		if l.Code == AllLanguagesCode {
			t.Error("ExpandTargets(ALL) included the synthetic ALL entry itself")
		}
	}
}

func TestExpandTargetsForOneRealCodeReturnsJustThatOne(t *testing.T) {
	targets := ExpandTargets("DE")
	if len(targets) != 1 || targets[0].Code != "DE" {
		t.Errorf("ExpandTargets(\"DE\") = %+v, want exactly [DE]", targets)
	}
}

func TestExpandTargetsForAnUnknownCodeReturnsNothing(t *testing.T) {
	if targets := ExpandTargets("XX"); targets != nil {
		t.Errorf("ExpandTargets(\"XX\") = %+v, want nil", targets)
	}
}
