package library

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchmanifest"
)

// patchFixture is two mods fighting over one key, with a patch already
// generated for them.
type patchFixture struct {
	modDir string
	cache  string
	opts   Options
}

func newPatchFixture(t *testing.T) patchFixture {
	t.Helper()
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)
	f := patchFixture{modDir: modDir, cache: t.TempDir()}
	f.opts = Options{CacheDir: f.cache, ModDir: modDir, Order: conflict.LoadOrder{"mod_a", "mod_b"}}
	if _, err := GeneratePatch(context.Background(), testGameConfig(), f.opts); err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	return f
}

// load runs LoadGame the way the app does once the patch exists: with the
// patch mod appended to the load order (the Workspace does exactly that
// after generating one).
func (f patchFixture) load(t *testing.T, order conflict.LoadOrder, overrides map[string]string) Summary {
	t.Helper()
	opts := f.opts
	opts.Order = append(append(conflict.LoadOrder{}, order...), patchModID)
	opts.Overrides = overrides
	s, err := LoadGame(context.Background(), testGameConfig(), opts)
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	return s
}

var abOrder = conflict.LoadOrder{"mod_a", "mod_b"}

func TestNoPatchMeansNoPatchState(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)
	s, err := LoadGame(context.Background(), testGameConfig(), Options{CacheDir: t.TempDir(), ModDir: modDir, Order: abOrder})
	if err != nil {
		t.Fatal(err)
	}
	if s.Patch.Exists || s.Patch.NeedsAttention() {
		t.Errorf("no patch was generated, got %+v", s.Patch)
	}
	if s.Patch.ChangedMods == nil {
		t.Error("ChangedMods must be a real empty slice, not nil (marshals as null)")
	}
	if len(s.Conflicts) != 1 || s.Conflicts[0].PatchState != "" {
		t.Errorf("conflict should carry no patch state, got %+v", s.Conflicts)
	}
}

func TestPatchIsCurrentRightAfterGenerating(t *testing.T) {
	f := newPatchFixture(t)
	s := f.load(t, abOrder, nil)

	if !s.Patch.Exists || s.Patch.Generation != 1 {
		t.Fatalf("patch summary = %+v, want an existing generation-1 patch", s.Patch)
	}
	if s.Patch.Patched != 1 || s.Patch.Changed != 0 || s.Patch.New != 0 || s.Patch.Obsolete != 0 {
		t.Errorf("fresh patch should be fully current, got %+v", s.Patch)
	}
	if s.Patch.NeedsAttention() {
		t.Error("a fresh patch must not ask for attention")
	}
	if got := s.Conflicts[0].PatchState; got != PatchStatePatched {
		t.Errorf("PatchState = %q, want %q", got, PatchStatePatched)
	}
}

func TestGeneratedPatchIsNotACandidateForItsOwnConflicts(t *testing.T) {
	f := newPatchFixture(t)
	s := f.load(t, abOrder, nil)

	if len(s.Conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(s.Conflicts))
	}
	c := s.Conflicts[0]
	if len(c.Candidates) != 2 {
		t.Errorf("candidates = %+v, want only the two real mods", c.Candidates)
	}
	for _, cand := range c.Candidates {
		if cand.ModID == patchModID {
			t.Error("the generated patch must not compete in the conflicts it resolves")
		}
	}
	if c.Winner != "mod_b" {
		t.Errorf("Winner = %q, want the real load-order winner mod_b, not the patch", c.Winner)
	}
	// It's still a listed, enabled mod.
	found := false
	for _, m := range s.Mods {
		if m.ID == patchModID && m.Enabled {
			found = true
		}
	}
	if !found {
		t.Error("the patch mod should still be listed (and enabled) as a mod")
	}
}

func TestPatchGoesStaleWhenASourceModUpdates(t *testing.T) {
	f := newPatchFixture(t)
	// Longer text, so the cache can't mistake it for the old file by size.
	writeFile(t, f.modDir, filepath.Join("mod_b", "common", "x.txt"), `shared_thing = { cost = 20 extra = yes }`)

	s := f.load(t, abOrder, nil)
	c := s.Conflicts[0]
	if c.PatchState != PatchStateChanged {
		t.Fatalf("PatchState = %q, want %q", c.PatchState, PatchStateChanged)
	}
	if !strings.Contains(c.PatchNote, "Mod B") || !strings.Contains(c.PatchNote, "Changed since the patch") {
		t.Errorf("PatchNote = %q, want it to say Mod B changed", c.PatchNote)
	}
	if s.Patch.Changed != 1 || s.Patch.Patched != 0 {
		t.Errorf("summary = %+v, want 1 changed", s.Patch)
	}
	if len(s.Patch.ChangedMods) != 1 || s.Patch.ChangedMods[0] != "Mod B" {
		t.Errorf("ChangedMods = %v, want [Mod B]", s.Patch.ChangedMods)
	}
	if !s.Patch.NeedsAttention() {
		t.Error("a stale patch must ask for attention")
	}
}

func TestUpdateToAnUnrelatedKeyDoesNotStaleThePatch(t *testing.T) {
	f := newPatchFixture(t)
	// mod_b gains a definition nothing else touches; the contested key is
	// byte-for-byte what it was.
	writeFile(t, f.modDir, filepath.Join("mod_b", "common", "x.txt"), `shared_thing = { cost = 2 } unrelated_thing = { cost = 9 }`)

	s := f.load(t, abOrder, nil)
	if got := s.Conflicts[0].PatchState; got != PatchStatePatched {
		t.Errorf("PatchState = %q, want %q: an update that never touched the key must not flag it", got, PatchStatePatched)
	}
	if s.Patch.NeedsAttention() {
		t.Errorf("summary = %+v, nothing the patch covers changed", s.Patch)
	}
}

func TestPatchGoesStaleWhenAnotherModStartsDefiningTheKey(t *testing.T) {
	f := newPatchFixture(t)
	writeMod(t, f.modDir, "mod_c", "Mod C", `shared_thing = { cost = 3 }`)

	s := f.load(t, conflict.LoadOrder{"mod_a", "mod_b", "mod_c"}, nil)
	c := s.Conflicts[0]
	if c.PatchState != PatchStateChanged {
		t.Fatalf("PatchState = %q, want %q", c.PatchState, PatchStateChanged)
	}
	if !strings.Contains(c.PatchNote, "Now also defined by: Mod C") {
		t.Errorf("PatchNote = %q", c.PatchNote)
	}
	// mod_c also becomes the automatic winner, so the pinned winner differs too.
	if !strings.Contains(c.PatchNote, "The winner is now Mod C, but the patch pins Mod B") {
		t.Errorf("PatchNote = %q, want the winner change spelled out", c.PatchNote)
	}
}

func TestPatchGoesStaleWhenASourceModIsNoLongerLoaded(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)
	writeMod(t, modDir, "mod_c", "Mod C", `shared_thing = { cost = 3 }`)
	f := patchFixture{modDir: modDir, cache: t.TempDir()}
	f.opts = Options{CacheDir: f.cache, ModDir: modDir, Order: conflict.LoadOrder{"mod_a", "mod_b", "mod_c"}}
	if _, err := GeneratePatch(context.Background(), testGameConfig(), f.opts); err != nil {
		t.Fatal(err)
	}

	// mod_b (which the patch drew on) is switched off; a and c still conflict.
	s := f.load(t, conflict.LoadOrder{"mod_a", "mod_c"}, nil)
	c := s.Conflicts[0]
	if c.PatchState != PatchStateChanged || !strings.Contains(c.PatchNote, "No longer loaded: Mod B") {
		t.Errorf("PatchState=%q PatchNote=%q, want changed with Mod B no longer loaded", c.PatchState, c.PatchNote)
	}
}

func TestPatchedKeyThatStopsConflictingIsObsoleteNotChanged(t *testing.T) {
	f := newPatchFixture(t)
	// Only mod_a is loaded now: the key isn't contested at all any more.
	s := f.load(t, conflict.LoadOrder{"mod_a"}, nil)
	if len(s.Conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", s.Conflicts)
	}
	if s.Patch.Obsolete != 1 || s.Patch.Changed != 0 {
		t.Errorf("summary = %+v, want the one patched key reported obsolete", s.Patch)
	}
	if !s.Patch.NeedsAttention() {
		t.Error("an obsolete-only patch is still out of date")
	}
}

func TestPatchGoesStaleWhenTheChosenWinnerChanges(t *testing.T) {
	f := newPatchFixture(t)
	// The user picks mod_a to win after the patch pinned mod_b.
	s := f.load(t, abOrder, map[string]string{"common:shared_thing": "mod_a"})
	c := s.Conflicts[0]
	if c.PatchState != PatchStateChanged {
		t.Fatalf("PatchState = %q, want %q", c.PatchState, PatchStateChanged)
	}
	if !strings.Contains(c.PatchNote, "The winner is now Mod A, but the patch pins Mod B") {
		t.Errorf("PatchNote = %q", c.PatchNote)
	}
	if len(s.Patch.ChangedMods) != 0 {
		t.Errorf("ChangedMods = %v: no mod's content changed, only the choice", s.Patch.ChangedMods)
	}
}

func TestConflictThatAppearedAfterThePatchIsNew(t *testing.T) {
	f := newPatchFixture(t)
	writeFile(t, f.modDir, filepath.Join("mod_a", "common", "x.txt"), `shared_thing = { cost = 1 } fresh_thing = { cost = 1 }`)
	writeFile(t, f.modDir, filepath.Join("mod_b", "common", "x.txt"), `shared_thing = { cost = 2 } fresh_thing = { cost = 5 }`)

	s := f.load(t, abOrder, nil)
	states := map[string]string{}
	for _, c := range s.Conflicts {
		states[c.ID] = c.PatchState
	}
	if states["fresh_thing"] != PatchStateNew {
		t.Errorf("fresh_thing state = %q, want %q", states["fresh_thing"], PatchStateNew)
	}
	if states["shared_thing"] != PatchStatePatched {
		t.Errorf("shared_thing state = %q, want it untouched and %q", states["shared_thing"], PatchStatePatched)
	}
	if s.Patch.New != 1 || s.Patch.Patched != 1 {
		t.Errorf("summary = %+v", s.Patch)
	}
}

func TestPatchFromAnOlderHashVersionIsFlaggedNotCompared(t *testing.T) {
	f := newPatchFixture(t)
	path := filepath.Join(f.modDir, patchModID, patchmanifest.FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"hashVersion": 1`), []byte(`"hashVersion": 0`), 1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := f.load(t, abOrder, nil)
	c := s.Conflicts[0]
	if c.PatchState != PatchStateChanged || !strings.Contains(c.PatchNote, "older version of the manager") {
		t.Errorf("PatchState=%q PatchNote=%q, want an explicit older-version note", c.PatchState, c.PatchNote)
	}
	if len(s.Patch.ChangedMods) != 0 {
		t.Errorf("ChangedMods = %v: hashes weren't compared, so no mod may be blamed", s.Patch.ChangedMods)
	}
}

func TestRegeneratingClearsTheStaleness(t *testing.T) {
	f := newPatchFixture(t)
	writeFile(t, f.modDir, filepath.Join("mod_b", "common", "x.txt"), `shared_thing = { cost = 20 extra = yes }`)
	if s := f.load(t, abOrder, nil); !s.Patch.NeedsAttention() {
		t.Fatal("setup: the patch should be stale before regenerating")
	}

	result, err := GeneratePatch(context.Background(), testGameConfig(), f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation != 2 {
		t.Errorf("Generation = %d, want 2", result.Generation)
	}
	s := f.load(t, abOrder, nil)
	if s.Patch.NeedsAttention() || s.Patch.Generation != 2 || s.Conflicts[0].PatchState != PatchStatePatched {
		t.Errorf("after regenerating, summary = %+v state = %q, want current", s.Patch, s.Conflicts[0].PatchState)
	}
}

func TestGeneratePatchManifestRecordsWinnerAndEveryCandidatesHash(t *testing.T) {
	f := newPatchFixture(t)
	m, ok := patchmanifest.Load(filepath.Join(f.modDir, patchModID))
	if !ok {
		t.Fatal("expected a readable manifest inside the patch mod")
	}
	if m.Generation != 1 || len(m.Keys) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
	k := m.Keys[0]
	if k.Type != "common" || k.ID != "shared_thing" || k.Winner != "mod_b" || k.Manual || k.Skipped {
		t.Errorf("key record = %+v", k)
	}
	if len(k.Sources) != 2 || k.Sources["mod_a"] == "" || k.Sources["mod_b"] == "" || k.Sources["mod_a"] == k.Sources["mod_b"] {
		t.Errorf("sources = %v, want two distinct hashes", k.Sources)
	}
	if m.Mods["mod_b"].Name != "Mod B" || m.Mods["mod_b"].Version != "1.0" {
		t.Errorf("mods = %+v", m.Mods)
	}
}

func TestGeneratePatchWritesProperDescriptorsAndThumbnail(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)
	thumb := []byte("\x89PNG-not-really-but-bytes-are-bytes")

	opts := Options{CacheDir: t.TempDir(), ModDir: modDir, Order: abOrder, GameVersion: "v4.4.6", PatchThumbnail: thumb}
	if _, err := GeneratePatch(context.Background(), testGameConfig(), opts); err != nil {
		t.Fatal(err)
	}
	contentDir := filepath.Join(modDir, patchModID)

	got, err := os.ReadFile(filepath.Join(contentDir, patchThumbnailName))
	if err != nil {
		t.Fatalf("thumbnail not written: %v", err)
	}
	if !bytes.Equal(got, thumb) {
		t.Error("thumbnail bytes were altered")
	}

	inner, err := os.ReadFile(filepath.Join(contentDir, "descriptor.mod"))
	if err != nil {
		t.Fatalf("descriptor.mod not written inside the mod folder: %v", err)
	}
	innerDesc, err := mod.ParseDescriptor(inner, mod.DescriptorClassic)
	if err != nil {
		t.Fatalf("descriptor.mod doesn't parse: %v", err)
	}
	if innerDesc.Path != "" {
		t.Errorf("the in-folder descriptor must not carry a path, got %q", innerDesc.Path)
	}

	stub, err := os.ReadFile(filepath.Join(modDir, patchModID+".mod"))
	if err != nil {
		t.Fatal(err)
	}
	stubDesc, err := mod.ParseDescriptor(stub, mod.DescriptorClassic)
	if err != nil {
		t.Fatalf("the stub doesn't parse: %v", err)
	}
	if stubDesc.Path != contentDir {
		t.Errorf("stub path = %q, want %q", stubDesc.Path, contentDir)
	}

	for name, d := range map[string]mod.Descriptor{"descriptor.mod": innerDesc, "stub": stubDesc} {
		if d.Name != "Parallax Mod Manager - Generated Patch" {
			t.Errorf("%s name = %q", name, d.Name)
		}
		if d.Version != "1.1" {
			t.Errorf("%s version = %q, want 1.1 (first generation)", name, d.Version)
		}
		if d.SupportedVersion != "v4.4.*" {
			t.Errorf("%s supported_version = %q, want v4.4.*", name, d.SupportedVersion)
		}
		if d.Picture != patchThumbnailName {
			t.Errorf("%s picture = %q, want %q", name, d.Picture, patchThumbnailName)
		}
		if len(d.Tags) == 0 {
			t.Errorf("%s has no tags", name)
		}
	}

	// The app's own thumbnail lookup must find it through the descriptor.
	uri, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, patchModID)
	if err != nil || !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Errorf("ModThumbnail = %q, %v, want the patch's own thumbnail", uri, err)
	}
}

func TestGeneratePatchWithoutAThumbnailDeclaresNoPicture(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)
	if _, err := GeneratePatch(context.Background(), testGameConfig(), Options{CacheDir: t.TempDir(), ModDir: modDir, Order: abOrder}); err != nil {
		t.Fatal(err)
	}
	stub, _ := os.ReadFile(filepath.Join(modDir, patchModID+".mod"))
	d, err := mod.ParseDescriptor(stub, mod.DescriptorClassic)
	if err != nil {
		t.Fatal(err)
	}
	if d.Picture != "" {
		t.Errorf("picture = %q, want none when no thumbnail was supplied", d.Picture)
	}
	if d.SupportedVersion != "*" {
		t.Errorf("supported_version = %q, want %q when the game version is unknown", d.SupportedVersion, "*")
	}
	if _, err := os.Stat(filepath.Join(modDir, patchModID, patchThumbnailName)); err == nil {
		t.Error("no thumbnail file should be written without one supplied")
	}
}

func TestSupportedVersionPattern(t *testing.T) {
	for in, want := range map[string]string{
		"v4.4.6":   "v4.4.*",
		"4.4.6":    "4.4.*",
		"V3.9.2":   "v3.9.*",
		"v4.4":     "v4.4.*",
		" v4.4.6 ": "v4.4.*",
		"":         "*",
		"v4":       "*",
		"garbage":  "*",
		"v.4":      "*",
	} {
		if got := supportedVersionPattern(in); got != want {
			t.Errorf("supportedVersionPattern(%q) = %q, want %q", in, got, want)
		}
	}
}
