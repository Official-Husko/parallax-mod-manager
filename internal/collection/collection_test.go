package collection

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

	c := Collection{
		Name: "Graphics",
		Mods: []ModRef{
			{GameID: "stellaris", ModID: "mod_a"},
			{GameID: "hoi4", ModID: "mod_b"},
		},
	}
	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, "Graphics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, c) {
		t.Errorf("Load = %+v, want %+v", got, c)
	}
}

func TestLoadNormalizesMissingModsToRealEmptySlice(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	raw := `{"version":1,"name":"Old Collection"}`
	if err := os.WriteFile(filepath.Join(store.Dir, "Old Collection.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := store.Load(context.Background(), "Old Collection")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mods == nil {
		t.Error("Mods is nil, want a real empty slice (marshals as JSON null, not [])")
	}
}

func TestLoadMissingReturnsErrNotFound(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	_, err := store.Load(context.Background(), "never_saved")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := FileStore{Dir: dir}
	_, err := store.Load(context.Background(), "broken")
	if err == nil {
		t.Fatal("expected an error for a corrupt file, want a real error not a silently-empty Collection")
	}
	if err == ErrNotFound {
		t.Error("a corrupt file must not report as ErrNotFound")
	}
}

func TestLoadWrongVersionErrors(t *testing.T) {
	dir := t.TempDir()
	content := `{"version":99999,"name":"old","mods":[]}`
	if err := os.WriteFile(filepath.Join(dir, "old.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := FileStore{Dir: dir}
	_, err := store.Load(context.Background(), "old")
	if err == nil {
		t.Fatal("expected an error for a version mismatch, want a real error not a silently-empty Collection")
	}
}

func TestListReturnsSavedCollectionsSortedByName(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	for _, name := range []string{"Zeta", "Alpha", "Middle"} {
		if err := store.Save(ctx, Collection{Name: name, Mods: []ModRef{{GameID: "g", ModID: "m"}}}); err != nil {
			t.Fatalf("Save(%q): %v", name, err)
		}
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d collections, want 3: %+v", len(got), got)
	}
	wantOrder := []string{"Alpha", "Middle", "Zeta"}
	for i, want := range wantOrder {
		if got[i].Name != want {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, want)
		}
		if len(got[i].Mods) != 1 {
			t.Errorf("got[%d].Mods = %+v, want 1 entry (List should return full data, not just names)", i, got[i].Mods)
		}
	}
}

func TestListSkipsCorruptFileWithoutErroring(t *testing.T) {
	dir := t.TempDir()
	store := FileStore{Dir: dir}
	ctx := context.Background()

	if err := store.Save(ctx, Collection{Name: "Good One"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned an error instead of skipping the corrupt file: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Good One" {
		t.Errorf("List = %+v, want exactly [Good One]", got)
	}
}

func TestListMissingDirIsEmptyNotError(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Error("got is nil, want a non-nil empty slice so it serializes as [] not null")
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestDeleteRemovesCollection(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	if err := store.Save(ctx, Collection{Name: "Temp"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete(ctx, "Temp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Load(ctx, "Temp"); err != ErrNotFound {
		t.Errorf("Load after delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteMissingReturnsErrNotFound(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	err := store.Delete(context.Background(), "never_existed")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestNameWithPathSeparatorSanitized(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	c := Collection{Name: "Sci-Fi / Space", Mods: []ModRef{{GameID: "stellaris", ModID: "x"}}}
	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load(ctx, "Sci-Fi / Space")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "Sci-Fi / Space" {
		t.Errorf("Name = %q, want original name preserved despite filename sanitization", got.Name)
	}
}

func TestOperationsRequireDir(t *testing.T) {
	store := FileStore{}
	ctx := context.Background()

	if _, err := store.List(ctx); err != ErrDirRequired {
		t.Errorf("List err = %v, want ErrDirRequired", err)
	}
	if _, err := store.Load(ctx, "x"); err != ErrDirRequired {
		t.Errorf("Load err = %v, want ErrDirRequired", err)
	}
	if err := store.Save(ctx, Collection{Name: "x"}); err != ErrDirRequired {
		t.Errorf("Save err = %v, want ErrDirRequired", err)
	}
	if err := store.Delete(ctx, "x"); err != ErrDirRequired {
		t.Errorf("Delete err = %v, want ErrDirRequired", err)
	}
}
