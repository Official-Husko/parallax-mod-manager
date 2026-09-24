package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate/deepl"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate/translanova"
	"github.com/Official-Husko/parallax-mod-manager/internal/translate/vust"
	"github.com/Official-Husko/parallax-mod-manager/internal/translatecache"
)

// unofficialServiceDelay is the fixed politeness pause between calls to
// Translanova/Vust - neither documents a real rate limit, so this is a
// conservative default (see internal/translate.WithDelay's own doc
// comment), not a discovered real one.
const unofficialServiceDelay = 1500 * time.Millisecond

// authorModeFilePrefix names the fixed .yml file author mode writes into
// the source mod's own localisation/<lang>/ folder - one per language,
// always overwritten in place, never a per-run unique name.
const authorModeFilePrefix = "zzz_parallax_auto_translated_l_"

// TranslateLanguage is one selectable target language, for the Translate
// tab's own dropdown.
type TranslateLanguage struct {
	Code      string
	Name      string
	Confirmed bool
}

// TranslateLanguages returns the whole hardcoded dropdown (the synthetic
// "All languages" entry first, then every real target language), sourced
// from Go rather than duplicated in TypeScript, so the two can never
// drift on something translation-correctness-sensitive.
func (a *App) TranslateLanguages() []TranslateLanguage {
	out := make([]TranslateLanguage, 0, len(translate.AllLanguages))
	for _, l := range translate.AllLanguages {
		out = append(out, TranslateLanguage{Code: l.Code, Name: l.Name, Confirmed: l.Confirmed})
	}
	return out
}

// TranslateEligibility is what the Translate tab needs to know before
// offering to translate one mod.
type TranslateEligibility struct {
	// AuthorModeOffered is true when this mod can be translated in place outright.
	AuthorModeOffered bool
	// AuthorModeOverridable is true when author mode needs "Continue anyway"
	// first (a Steam Workshop or Paradox Launcher mod) - mirrors EditInfo.Overridable.
	AuthorModeOverridable bool
	AuthorModeReason      string
	// DeepLKeyReady is true when a saved, readable DeepL key exists - the
	// DeepL option in the service dropdown is disabled otherwise.
	DeepLKeyReady bool
	// HasEnglishContent is false when the mod has no English localisation
	// at all - nothing to translate from, regardless of destination mode.
	HasEnglishContent bool
	// EnglishKeyCount/EnglishFileCount describe the mod's own English source (0/0 when there is
	// none). TargetCount is how many real target languages exist to translate into (the "All
	// languages" expansion's own length, translate.AllLanguagesCode's own real-language count).
	// SourcePath is where they were found, for display only.
	EnglishKeyCount  int
	EnglishFileCount int
	TargetCount      int
	SourcePath       string
}

// TranslateEligibility reports whether/how modID can be auto-translated -
// see TranslateEligibility's own fields.
func (a *App) TranslateEligibility(gameID, modID string) (TranslateEligibility, error) {
	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return TranslateEligibility{}, err
	}
	out := TranslateEligibility{
		AuthorModeOffered:     t.editable,
		AuthorModeOverridable: t.overridable,
		AuthorModeReason:      t.reason,
		DeepLKeyReady:         a.DeepLStatus().HasKey,
		TargetCount:           len(translate.ExpandTargets(translate.AllLanguagesCode)),
		SourcePath:            "localisation/english/ or localization/english/",
	}
	if t.m.ContentPath != "" {
		english, err := library.EnglishCatalog(t.cfg, t.m.ContentPath)
		if err != nil {
			return out, err
		}
		out.HasEnglishContent = len(english) > 0
		out.EnglishKeyCount = len(english)
		if fileCount, err := library.EnglishFileCount(t.cfg, t.m.ContentPath); err == nil {
			out.EnglishFileCount = fileCount
		}
	}
	return out, nil
}

// TranslateRequest is what the Translate tab asks TranslateMod to do.
type TranslateRequest struct {
	// Service is "deepl", "translanova" or "vust".
	Service string
	// TargetCode is a real DeepL code, or translate.AllLanguagesCode for every real language.
	TargetCode string
	// Mode is "author" (write straight into the source mod) or "player"
	// (a separate companion mod).
	Mode string
	// Force mirrors ModEdit.Force exactly: author mode on an overridable
	// (Workshop/Launcher) mod needs this set, after "Continue anyway".
	Force bool
}

// TranslateProgress is one moment of a running TranslateMod call, streamed
// as a "translate-progress" event - the same shape
// PublishModToWorkshop/"workshop-publish-progress" already established
// this session.
type TranslateProgress struct {
	RequestID string
	// Stage is one of: "opening", "opened", "language", "translating", "language_done",
	// "writing", "done", "error". "opened" fires once, right after this mod's own English
	// source is actually read (Count is how many localisation/english/*.yml files that was);
	// "language_done" fires once per language, right after its own output file is written
	// (Count is that file's own real key count); "writing" only ever fires once, for player
	// mode's own companion-mod manifest step, which happens after every language's file is
	// already on disk.
	Stage    string
	Language string
	Key      string
	Message  string
	Done     int
	Total    int
	// LanguageIndex/LanguageTotal say which target language this is out of how many in this
	// run - "German - 4 of 27" for an "All languages" run, "German - 1 of 1" for a single one.
	LanguageIndex int
	LanguageTotal int
	// Count is the real number of keys just written for Language, on a "language_done" stage.
	Count int
	// OutputFile is Language's own output file, relative to the mod's (or companion's) own
	// content folder - set from the "language" stage onward, once known.
	OutputFile string
}

// TranslateResult is a finished TranslateMod call's own outcome.
type TranslateResult struct {
	// Translated counts real API calls that succeeded.
	Translated int
	// AlreadyCovered counts keys that needed no API call at all (already
	// cached and unchanged, or a pre-existing value seeded as exempt).
	AlreadyCovered int
	// CompanionModID is set only for player mode.
	CompanionModID string
}

func resolveTranslator(service, apiKey, tier string) (translate.Translator, error) {
	switch service {
	case "deepl":
		if apiKey == "" {
			return nil, errors.New("no DeepL API key is saved - add one in Settings > Tools first")
		}
		return deepl.New(apiKey, tier == "pro"), nil
	case "translanova":
		return translate.WithDelay(translanova.New(), unofficialServiceDelay), nil
	case "vust":
		return translate.WithDelay(vust.New(), unofficialServiceDelay), nil
	default:
		return nil, fmt.Errorf("unknown translation service %q", service)
	}
}

// langPlan is one target language's own diff result, computed up front
// for every target before any real API call - so the progress bar's own
// Total is known from the start, matching "103/894 translated".
type langPlan struct {
	lang    translate.Language
	needs   []library.Need
	rewrite []string
}

// TranslateMod translates gameID's modID from English into req.TargetCode
// (or every real language, for translate.AllLanguagesCode) using
// req.Service, writing either straight into the mod itself (author mode)
// or into a separate, per-mod companion mod (player mode) - see
// internal/library's translate_plan.go/translate_companion.go for the
// real diff/write mechanics this drives. Blocking; streams
// "translate-progress" events (gameID, TranslateProgress) while running.
func (a *App) TranslateMod(gameID, modID, requestID string, req TranslateRequest) (TranslateResult, error) {
	log := applog.For("Translate")

	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return TranslateResult{}, err
	}
	if req.Mode == "author" {
		if !t.effectiveEditable(req.Force) {
			return TranslateResult{}, errors.New(t.reason)
		}
	} else if req.Mode != "player" {
		return TranslateResult{}, fmt.Errorf("unknown destination mode %q", req.Mode)
	}

	targets := translate.ExpandTargets(req.TargetCode)
	if len(targets) == 0 {
		return TranslateResult{}, fmt.Errorf("unknown target language %q", req.TargetCode)
	}

	var apiKey, tier string
	if req.Service == "deepl" {
		if apiKey, tier, err = a.deeplCredentials(); err != nil {
			return TranslateResult{}, err
		}
	}
	translatorFor := a.translatorFor
	if translatorFor == nil {
		translatorFor = resolveTranslator
	}
	translator, err := translatorFor(req.Service, apiKey, tier)
	if err != nil {
		return TranslateResult{}, err
	}

	ctx, cancel := context.WithCancel(a.baseContext())
	a.translateMu.Lock()
	if a.translateCancel == nil {
		a.translateCancel = map[string]context.CancelFunc{}
	}
	a.translateCancel[requestID] = cancel
	a.translateMu.Unlock()
	defer func() {
		a.translateMu.Lock()
		delete(a.translateCancel, requestID)
		a.translateMu.Unlock()
		cancel()
	}()

	emit := func(p TranslateProgress) {
		p.RequestID = requestID
		a.emit("translate-progress", gameID, p)
	}
	emit(TranslateProgress{Stage: "opening"})

	english, err := library.EnglishCatalog(t.cfg, t.m.ContentPath)
	if err != nil {
		emit(TranslateProgress{Stage: "error", Message: err.Error()})
		return TranslateResult{}, err
	}
	if len(english) == 0 {
		err := errors.New("this mod has no English localisation to translate")
		emit(TranslateProgress{Stage: "error", Message: err.Error()})
		return TranslateResult{}, err
	}
	if fileCount, ferr := library.EnglishFileCount(t.cfg, t.m.ContentPath); ferr == nil {
		emit(TranslateProgress{Stage: "opened", Count: fileCount})
	}

	cache, _ := translatecache.Load(a.configAppDir, gameID, modID)
	if cache.Entries == nil {
		cache.Entries = map[string]map[string]translatecache.Entry{}
	}

	var companion library.TranslationCompanionInfo
	outputRoot := t.m.ContentPath
	if req.Mode == "player" {
		companion = library.ResolveTranslationCompanion(t.modDir, modID)
		outputRoot = companion.ContentDir
	}

	plans := make([]langPlan, 0, len(targets))
	total := 0
	for _, lang := range targets {
		existing, err := library.TargetCatalog(t.cfg, outputRoot, lang.ParadoxFolder)
		if err != nil {
			emit(TranslateProgress{Stage: "error", Message: err.Error()})
			return TranslateResult{}, err
		}
		needs, seeds, rewrite, _ := library.Plan(english, existing, cache, lang.Code)
		if len(seeds) > 0 {
			if cache.Entries[lang.Code] == nil {
				cache.Entries[lang.Code] = map[string]translatecache.Entry{}
			}
			for _, s := range seeds {
				cache.Entries[lang.Code][s.Key] = translatecache.Entry{
					SourceHash: s.Hash, TranslatedText: existing[s.Key],
					Service: translatecache.ServiceExisting, TranslatedAt: time.Now().Unix(),
				}
			}
		}
		plans = append(plans, langPlan{lang: lang, needs: needs, rewrite: rewrite})
		total += len(needs)
	}
	// Persist the seeded "existing" entries even if nothing else needs
	// translating this run - a wasted API call is never spent finding
	// this out a second time.
	if err := translatecache.Save(a.configAppDir, gameID, modID, cache); err != nil {
		log.Warnf("saving the translation cache for %s failed: %v", modID, err)
	}

	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()

	result := TranslateResult{}
	done := 0
	languageTotal := len(plans)
	for i, p := range plans {
		if ctx.Err() != nil {
			return result, errors.New("translation was cancelled")
		}
		langMsg := ""
		if !p.lang.Confirmed {
			langMsg = fmt.Sprintf("folder name unconfirmed for this game, used %q", p.lang.ParadoxFolder)
		}
		filename := authorModeFilePrefix + p.lang.ParadoxFolder + ".yml"
		if req.Mode == "player" {
			filename = companion.ModID + "_l_" + p.lang.ParadoxFolder + ".yml"
		}
		outputFile := filepath.ToSlash(filepath.Join("localisation", p.lang.ParadoxFolder, filename))

		emit(TranslateProgress{Stage: "language", Language: p.lang.Code, Message: langMsg, OutputFile: outputFile, Done: done, Total: total, LanguageIndex: i + 1, LanguageTotal: languageTotal})

		for _, need := range p.needs {
			if ctx.Err() != nil {
				return result, errors.New("translation was cancelled")
			}
			emit(TranslateProgress{Stage: "translating", Language: p.lang.Code, Key: need.Key, OutputFile: outputFile, Done: done, Total: total, LanguageIndex: i + 1, LanguageTotal: languageTotal})

			translated, err := translator.Translate(ctx, need.EnglishText, p.lang)
			if err != nil {
				emit(TranslateProgress{Stage: "error", Language: p.lang.Code, Key: need.Key, Message: err.Error(), Done: done, Total: total, LanguageIndex: i + 1, LanguageTotal: languageTotal})
				return result, fmt.Errorf("translating %q into %s: %w", need.Key, p.lang.Name, err)
			}

			if cache.Entries[p.lang.Code] == nil {
				cache.Entries[p.lang.Code] = map[string]translatecache.Entry{}
			}
			cache.Entries[p.lang.Code][need.Key] = translatecache.Entry{
				SourceHash: need.EnglishHash, TranslatedText: translated,
				Service: req.Service, TranslatedAt: time.Now().Unix(),
			}
			if err := translatecache.Save(a.configAppDir, gameID, modID, cache); err != nil {
				log.Warnf("saving the translation cache for %s failed: %v", modID, err)
			}
			done++
			result.Translated++
			emit(TranslateProgress{Stage: "translating", Language: p.lang.Code, Key: need.Key, OutputFile: outputFile, Done: done, Total: total, LanguageIndex: i + 1, LanguageTotal: languageTotal})
		}
		result.AlreadyCovered += len(p.rewrite)

		// This language's own file is written right away, incrementally - a cancellation
		// partway through a run leaves every language finished so far actually on disk, and
		// "language_done" can report this language's own real, final key count.
		projected := map[string]string{}
		for _, e := range english {
			if entry, ok := cache.Entries[p.lang.Code][e.Key]; ok {
				projected[e.Key] = entry.TranslatedText
			}
		}
		if len(projected) > 0 {
			rendered := locale.RenderFile(p.lang.ParadoxFolder, projected)
			dir := filepath.Join(outputRoot, "localisation", p.lang.ParadoxFolder)
			if _, err := atomicfile.Write(dir, filename, rendered); err != nil {
				emit(TranslateProgress{Stage: "error", Language: p.lang.Code, Message: err.Error(), LanguageIndex: i + 1, LanguageTotal: languageTotal})
				return result, fmt.Errorf("writing %s: %w", filename, err)
			}
		}
		emit(TranslateProgress{Stage: "language_done", Language: p.lang.Code, Count: len(projected), OutputFile: outputFile, Done: done, Total: total, LanguageIndex: i + 1, LanguageTotal: languageTotal})
	}

	if req.Mode == "player" {
		emit(TranslateProgress{Stage: "writing", Done: done, Total: total})
		gameVersion, _ := a.GameVersion(gameID)
		if err := library.EnsureTranslationCompanion(companion, modID, t.m.Descriptor.Name, gameVersion); err != nil {
			emit(TranslateProgress{Stage: "error", Message: err.Error()})
			return result, err
		}
		result.CompanionModID = companion.ModID
	}

	log.Infof("translated '%s' in '%s': %d translated, %d already covered (%s, %s)",
		editLabel(t), a.gameLabel(gameID), result.Translated, result.AlreadyCovered, req.Service, req.Mode)
	a.emit("mods-changed", gameID)
	emit(TranslateProgress{Stage: "done", Done: done, Total: total})
	return result, nil
}

// CancelTranslate stops the TranslateMod call tagged with requestID, if it
// is still running - a no-op otherwise (it may already be finished).
func (a *App) CancelTranslate(requestID string) {
	a.translateMu.Lock()
	cancel := a.translateCancel[requestID]
	a.translateMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
