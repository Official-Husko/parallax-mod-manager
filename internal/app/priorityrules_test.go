package app

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/priorityrules"
)

func newPriorityRulesApp(t *testing.T) *App {
	t.Helper()
	return &App{
		priorityRules: priorityrules.Store{Dir: t.TempDir()},
		registry:      game.NewRegistry([]game.GameConfig{game.Stellaris}),
	}
}

func TestBuiltInPriorityRulesReflectsTheRealPackageVar(t *testing.T) {
	a := newPriorityRulesApp(t)
	got := a.BuiltInPriorityRules()
	if len(got) != len(conflict.DefaultPriorityRules) {
		t.Fatalf("got %+v, want exactly %d entries", got, len(conflict.DefaultPriorityRules))
	}
	for _, e := range got {
		if e.Type != "common/static_modifiers" || e.Rule != "FIOS" {
			t.Errorf("entry = %+v, want the confirmed common/static_modifiers: FIOS", e)
		}
	}
}

func TestBuiltInPriorityRulesIsSortedByType(t *testing.T) {
	a := newPriorityRulesApp(t)
	got := a.BuiltInPriorityRules()
	for i := 1; i < len(got); i++ {
		if got[i-1].Type > got[i].Type {
			t.Errorf("not sorted: %+v", got)
		}
	}
}

func TestSetPriorityRuleOverrideThenPriorityRuleOverridesRoundTrips(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.SetPriorityRuleOverride("stellaris", "common/scripted_variables", "fios"); err != nil {
		t.Fatalf("SetPriorityRuleOverride: %v", err)
	}
	got := a.PriorityRuleOverrides("stellaris")
	if got["common/scripted_variables"] != "FIOS" {
		t.Errorf("got %+v, want common/scripted_variables: FIOS (normalized to uppercase)", got)
	}
}

func TestSetPriorityRuleOverrideEmptyRuleClearsIt(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.SetPriorityRuleOverride("stellaris", "common/x", "FIOS"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := a.SetPriorityRuleOverride("stellaris", "common/x", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := a.PriorityRuleOverrides("stellaris"); len(got) != 0 {
		t.Errorf("got %+v, want empty after clearing", got)
	}
}

func TestSetPriorityRuleOverrideRejectsAnInvalidRule(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.SetPriorityRuleOverride("stellaris", "common/x", "MAYBE"); err == nil {
		t.Error("expected an error for a rule that isn't FIOS, LIOS, or empty")
	}
}

func TestSetPriorityRuleOverrideRejectsAnEmptyType(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.SetPriorityRuleOverride("stellaris", "  ", "FIOS"); err == nil {
		t.Error("expected an error for a blank Type")
	}
}

func TestSetPriorityRuleOverrideIsScopedPerGame(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.SetPriorityRuleOverride("stellaris", "common/x", "FIOS"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := a.PriorityRuleOverrides("hoi4"); len(got) != 0 {
		t.Errorf("hoi4 should be unaffected by a stellaris override, got %+v", got)
	}
}

func TestParsedPriorityRuleOverridesConvertsToConflictPriorityRules(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.SetPriorityRuleOverride("stellaris", "common/scripted_variables", "FIOS"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := a.SetPriorityRuleOverride("stellaris", "common/buildings", "LIOS"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got := a.parsedPriorityRuleOverrides("stellaris")
	if got["common/scripted_variables"] != conflict.FIOS {
		t.Errorf("got %+v, want common/scripted_variables to be conflict.FIOS", got)
	}
	if got["common/buildings"] != conflict.LIOS {
		t.Errorf("got %+v, want common/buildings to be conflict.LIOS", got)
	}
}

func TestParsedPriorityRuleOverridesWithNoneSavedIsNil(t *testing.T) {
	a := newPriorityRulesApp(t)
	if got := a.parsedPriorityRuleOverrides("stellaris"); got != nil {
		t.Errorf("got %+v, want nil (no overrides saved at all)", got)
	}
}

// A hand-edited overrides file with a rule string that isn't FIOS or LIOS
// (never something SetPriorityRuleOverride itself would write) must be
// skipped, not treated as an error that would block the whole scan.
func TestParsedPriorityRuleOverridesSkipsAnUnreadableRule(t *testing.T) {
	a := newPriorityRulesApp(t)
	if err := a.priorityRules.Save("stellaris", map[string]string{
		"common/good": "FIOS",
		"common/bad":  "not a real rule",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := a.parsedPriorityRuleOverrides("stellaris")
	if got["common/good"] != conflict.FIOS {
		t.Errorf("got %+v, want common/good kept", got)
	}
	if _, ok := got["common/bad"]; ok {
		t.Errorf("got %+v, want the unreadable entry skipped, not present at all", got)
	}
}
