package translate

// Language is one selectable target language: the DeepL code every one of
// the three services accepts, and this project's own best-effort guess at
// the real, on-disk Paradox localisation folder name it corresponds to.
type Language struct {
	// Code is the DeepL-style two-letter target language code (e.g. "DE"),
	// or the synthetic AllLanguagesCode.
	Code string
	// Name is the English display name, for the dropdown.
	Name string
	// ParadoxFolder is the real on-disk folder name under a mod's own
	// localisation/ directory this language corresponds to (e.g. "german")
	// - never asserted with confidence beyond what Confirmed says.
	ParadoxFolder string
	// Confirmed is true only for the handful of languages actually
	// confirmed (against a real Stellaris install, cross-checked with
	// docs/patch-mods.md and this project's own test fixtures) to be real,
	// working Paradox folder names. Everything else uses a lower-cased-
	// English-name best-effort guess instead - offered anyway per the
	// user's own explicit choice (writing an unconfirmed language's folder
	// is harmless: the game just never offers it in its own in-game
	// language picker), but never claimed as verified.
	Confirmed bool
}

// AllLanguagesCode is a synthetic entry, never sent to any translation
// service - only ExpandTargets ever looks at it, to mean "every real
// language below, not just one".
const AllLanguagesCode = "ALL"

// AllLanguages is the hardcoded dropdown, in the exact order given: the
// synthetic "All languages" entry first and default-selected, then the 27
// target-language codes/names verbatim (28 entries in total).
//
// Only 9 entries are Confirmed: EN, FR, DE, ES, RU, PL, PT (-> braz_por) and
// ZH (-> simp_chinese), JA - the Stellaris-confirmed folder set found this
// session, minus Korean, which has no code at all in this required
// dropdown and so is simply absent here.
var AllLanguages = []Language{
	{Code: AllLanguagesCode, Name: "All languages", ParadoxFolder: "", Confirmed: false},
	{Code: "BG", Name: "Bulgarian", ParadoxFolder: "bulgarian", Confirmed: false},
	{Code: "ZH", Name: "Chinese (Simplified)", ParadoxFolder: "simp_chinese", Confirmed: true},
	{Code: "CS", Name: "Czech", ParadoxFolder: "czech", Confirmed: false},
	{Code: "DA", Name: "Danish", ParadoxFolder: "danish", Confirmed: false},
	{Code: "NL", Name: "Dutch", ParadoxFolder: "dutch", Confirmed: false},
	{Code: "EN", Name: "English", ParadoxFolder: "english", Confirmed: true},
	{Code: "ET", Name: "Estonian", ParadoxFolder: "estonian", Confirmed: false},
	{Code: "FI", Name: "Finnish", ParadoxFolder: "finnish", Confirmed: false},
	{Code: "FR", Name: "French", ParadoxFolder: "french", Confirmed: true},
	{Code: "DE", Name: "German", ParadoxFolder: "german", Confirmed: true},
	{Code: "EL", Name: "Greek", ParadoxFolder: "greek", Confirmed: false},
	{Code: "HU", Name: "Hungarian", ParadoxFolder: "hungarian", Confirmed: false},
	{Code: "ID", Name: "Indonesian", ParadoxFolder: "indonesian", Confirmed: false},
	{Code: "IT", Name: "Italian", ParadoxFolder: "italian", Confirmed: false},
	{Code: "JA", Name: "Japanese", ParadoxFolder: "japanese", Confirmed: true},
	{Code: "LV", Name: "Latvian", ParadoxFolder: "latvian", Confirmed: false},
	{Code: "LT", Name: "Lithuanian", ParadoxFolder: "lithuanian", Confirmed: false},
	{Code: "PL", Name: "Polish", ParadoxFolder: "polish", Confirmed: true},
	{Code: "PT", Name: "Portuguese", ParadoxFolder: "braz_por", Confirmed: true},
	{Code: "RO", Name: "Romanian", ParadoxFolder: "romanian", Confirmed: false},
	{Code: "RU", Name: "Russian", ParadoxFolder: "russian", Confirmed: true},
	{Code: "SK", Name: "Slovak", ParadoxFolder: "slovak", Confirmed: false},
	{Code: "SL", Name: "Slovenian", ParadoxFolder: "slovenian", Confirmed: false},
	{Code: "ES", Name: "Spanish", ParadoxFolder: "spanish", Confirmed: true},
	{Code: "SV", Name: "Swedish", ParadoxFolder: "swedish", Confirmed: false},
	{Code: "TR", Name: "Turkish", ParadoxFolder: "turkish", Confirmed: false},
	{Code: "UK", Name: "Ukrainian", ParadoxFolder: "ukrainian", Confirmed: false},
}

// Lookup finds one language by its DeepL code (never AllLanguagesCode -
// use ExpandTargets for that).
func Lookup(code string) (Language, bool) {
	for _, l := range AllLanguages {
		if l.Code == code {
			return l, true
		}
	}
	return Language{}, false
}

// ExpandTargets turns a dropdown selection into the real language(s) to
// translate into: every real language except English for the synthetic
// "All languages" choice (translating English to English is a no-op), or
// just the one named language otherwise. An unrecognized code expands to
// nothing, never a guess.
func ExpandTargets(code string) []Language {
	if code != AllLanguagesCode {
		if l, ok := Lookup(code); ok {
			return []Language{l}
		}
		return nil
	}
	targets := make([]Language, 0, len(AllLanguages)-2)
	for _, l := range AllLanguages {
		if l.Code == AllLanguagesCode || l.Code == "EN" {
			continue
		}
		targets = append(targets, l)
	}
	return targets
}
