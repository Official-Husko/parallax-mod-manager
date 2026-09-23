package app

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/library"
)

// TestWorkshopAndLauncherModsAreOverridableButRefuseWithoutForce covers the "Continue anyway"
// override: a Steam Workshop or Paradox Launcher mod reports Overridable and stays refused until
// Force is set, at which point the same edit that was refused a moment ago succeeds.
func TestWorkshopAndLauncherModsAreOverridableButRefuseWithoutForce(t *testing.T) {
	env := newEditorEnv(t)
	for _, id := range []string{"ugc_999", "pdx_00001"} {
		info := env.info(t, id)
		if info.Editable {
			t.Fatalf("%s: Editable = true, want false", id)
		}
		if !info.Overridable {
			t.Fatalf("%s: Overridable = false, want true", id)
		}
		if info.Reason == "" {
			t.Fatalf("%s: Reason is empty", id)
		}

		edit := ModEdit{Fields: info.Fields}
		edit.Fields.Name = info.Fields.Name + ", changed"

		if _, err := env.a.SaveModEdit(env.cfg.ID, id, edit); err == nil {
			t.Fatalf("%s: SaveModEdit without Force succeeded, want a refusal", id)
		}
		if prev, err := env.a.PreviewModEdit(env.cfg.ID, id, edit); err != nil || len(prev.Problems) == 0 {
			t.Fatalf("%s: PreviewModEdit without Force = %+v, err %v, want a Problems entry", id, prev, err)
		}

		edit.Force = true
		if prev, err := env.a.PreviewModEdit(env.cfg.ID, id, edit); err != nil || len(prev.Problems) != 0 {
			t.Fatalf("%s: PreviewModEdit with Force = %+v, err %v, want no problems", id, prev, err)
		}
		res, err := env.a.SaveModEdit(env.cfg.ID, id, edit)
		if err != nil {
			t.Fatalf("%s: SaveModEdit with Force: %v", id, err)
		}
		if len(res.Files) == 0 {
			t.Fatalf("%s: SaveModEdit with Force wrote nothing", id)
		}
	}
}

// TestThePatchAndAPackedArchiveAreNeverOverridable makes sure Force cannot unlock what genuinely
// cannot be edited here - the app's own generated patch, and a packed archive.
func TestThePatchAndAPackedArchiveAreNeverOverridable(t *testing.T) {
	env := newEditorEnv(t)
	for _, id := range []string{library.PatchModID, "packed"} {
		info := env.info(t, id)
		if info.Overridable {
			t.Fatalf("%s: Overridable = true, want false", id)
		}
		edit := ModEdit{Fields: info.Fields, Force: true}
		if _, err := env.a.SaveModEdit(env.cfg.ID, id, edit); err == nil {
			t.Fatalf("%s: SaveModEdit with Force succeeded, want a refusal", id)
		}
	}
}

// TestForceDoesNotPersistBetweenCalls checks ModEditInfo keeps reporting the real, unforced state
// regardless of an earlier forced save - forcing is per-request, never remembered.
func TestForceDoesNotPersistBetweenCalls(t *testing.T) {
	env := newEditorEnv(t)
	info := env.info(t, "ugc_999")
	edit := ModEdit{Fields: info.Fields, Force: true}
	if _, err := env.a.SaveModEdit(env.cfg.ID, "ugc_999", edit); err != nil {
		t.Fatalf("SaveModEdit with Force: %v", err)
	}

	again := env.info(t, "ugc_999")
	if again.Editable {
		t.Fatal("Editable = true after an earlier forced save, want it to still report false")
	}
	if !again.Overridable {
		t.Fatal("Overridable = false after an earlier forced save, want it to still report true")
	}
}
