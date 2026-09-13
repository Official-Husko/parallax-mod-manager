package playset

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	p := Playset{
		Name:        "Vanilla+ Historical",
		GameKey:     "stellaris",
		ModIDs:      []string{"mod_a", "mod_b"},
		DisabledDLC: []string{"some_dlc"},
	}
	if err := store.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, "stellaris", "Vanilla+ Historical")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("Load = %+v, want %+v", got, p)
	}
}

// TestLoadNormalizesMissingSlicesToRealEmptySlices pins a real bug: a
// playset file saved before DisabledDLC existed (or one hand-edited to
// omit a field, or literally containing "disabledDlc": null) leaves that
// Go field at its nil-slice zero value, which crosses the Wails/JS
// boundary as null where the frontend's type says string[] - crashing the
// first .length/.map call against it, the same class of bug already found
// once for library.ModSummary.
func TestLoadNormalizesMissingSlicesToRealEmptySlices(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	dir := filepath.Join(store.Dir, "stellaris")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A pre-DisabledDLC-era file: no "disabledDlc" key at all, and
	// "modIds" explicitly null for good measure.
	raw := `{"version":1,"name":"Old Playset","gameKey":"stellaris","modIds":null}`
	if err := os.WriteFile(filepath.Join(dir, "Old Playset.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := store.Load(context.Background(), "stellaris", "Old Playset")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DisabledDLC == nil {
		t.Error("DisabledDLC is nil, want a real empty slice (marshals as JSON null, not [])")
	}
	if got.ModIDs == nil {
		t.Error("ModIDs is nil, want a real empty slice (marshals as JSON null, not [])")
	}
}

func TestLoadMissingReturnsErrNotFound(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	_, err := store.Load(context.Background(), "stellaris", "never_saved")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	gameDir := filepath.Join(dir, "stellaris")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "broken.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := FileStore{Dir: dir}
	_, err := store.Load(context.Background(), "stellaris", "broken")
	if err == nil {
		t.Fatal("expected an error for a corrupt file, want a real error not a silently-empty Playset")
	}
	if err == ErrNotFound {
		t.Error("a corrupt file must not report as ErrNotFound")
	}
}

func TestLoadWrongVersionErrors(t *testing.T) {
	dir := t.TempDir()
	gameDir := filepath.Join(dir, "stellaris")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{"version":99999,"name":"old","gameKey":"stellaris","modIds":["a"]}`
	if err := os.WriteFile(filepath.Join(gameDir, "old.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := FileStore{Dir: dir}
	_, err := store.Load(context.Background(), "stellaris", "old")
	if err == nil {
		t.Fatal("expected an error for a version mismatch, want a real error not a silently-empty Playset")
	}
}

func TestListReturnsSavedNames(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	for _, name := range []string{"Zeta", "Alpha", "Middle"} {
		if err := store.Save(ctx, Playset{Name: name, GameKey: "stellaris"}); err != nil {
			t.Fatalf("Save(%q): %v", name, err)
		}
	}

	names, err := store.List(ctx, "stellaris")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Alpha", "Middle", "Zeta"} // sorted
	if !reflect.DeepEqual(names, want) {
		t.Errorf("List = %v, want %v", names, want)
	}
}

func TestListSkipsCorruptFileWithoutErroring(t *testing.T) {
	dir := t.TempDir()
	store := FileStore{Dir: dir}
	ctx := context.Background()

	if err := store.Save(ctx, Playset{Name: "Good One", GameKey: "stellaris"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	gameDir := filepath.Join(dir, "stellaris")
	if err := os.WriteFile(filepath.Join(gameDir, "broken.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	names, err := store.List(ctx, "stellaris")
	if err != nil {
		t.Fatalf("List returned an error instead of skipping the corrupt file: %v", err)
	}
	want := []string{"Good One", "broken"} // corrupt file falls back to its filename stem
	if !reflect.DeepEqual(names, want) {
		t.Errorf("List = %v, want %v", names, want)
	}
}

func TestListMissingDirIsEmptyNotError(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	names, err := store.List(context.Background(), "stellaris")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want empty", names)
	}
	// A nil slice serializes to JSON null across the Wails boundary, not
	// [] - that broke a real frontend caller (it only guarded against
	// undefined). Pin the fix: this must be a real, non-nil empty slice.
	if names == nil {
		t.Error("names is nil, want a non-nil empty slice so it serializes as [] not null")
	}
}

func TestDeleteRemovesPlayset(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	if err := store.Save(ctx, Playset{Name: "Temp", GameKey: "stellaris"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete(ctx, "stellaris", "Temp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	names, err := store.List(ctx, "stellaris")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("names = %v, want empty after delete", names)
	}
	if _, err := store.Load(ctx, "stellaris", "Temp"); err != ErrNotFound {
		t.Errorf("Load after delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteMissingReturnsErrNotFound(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	err := store.Delete(context.Background(), "stellaris", "never_existed")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestNameWithPathSeparatorSanitized(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	p := Playset{Name: "A/B Test", GameKey: "stellaris", ModIDs: []string{"x"}}
	if err := store.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, "stellaris", "A/B Test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "A/B Test" {
		t.Errorf("Name = %q, want original name preserved despite filename sanitization", got.Name)
	}
}

func TestOperationsRequireDir(t *testing.T) {
	store := FileStore{}
	ctx := context.Background()

	if _, err := store.List(ctx, "stellaris"); err != ErrDirRequired {
		t.Errorf("List err = %v, want ErrDirRequired", err)
	}
	if _, err := store.Load(ctx, "stellaris", "x"); err != ErrDirRequired {
		t.Errorf("Load err = %v, want ErrDirRequired", err)
	}
	if err := store.Save(ctx, Playset{Name: "x", GameKey: "stellaris"}); err != ErrDirRequired {
		t.Errorf("Save err = %v, want ErrDirRequired", err)
	}
	if err := store.Delete(ctx, "stellaris", "x"); err != ErrDirRequired {
		t.Errorf("Delete err = %v, want ErrDirRequired", err)
	}
}
