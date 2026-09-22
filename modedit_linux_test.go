package main

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// editorEnv is a game with a few kinds of mod, all in temp folders - never the person's own.
type editorEnv struct {
	a       *App
	cfg     game.GameConfig
	modDir  string
	lib     string
	extra   string
	events  []string
	eventMu sync.Mutex
}

func writeText(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(b)
}

func newEditorEnv(t *testing.T) *editorEnv {
	t.Helper()
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	env := &editorEnv{cfg: game.Stellaris, lib: t.TempDir(), extra: t.TempDir()}
	env.modDir = filepath.Join(data, "Paradox Interactive", "Stellaris", "mod")

	// A local mod with a descriptor.mod and a stub that names its folder.
	local := filepath.Join(env.lib, "Local A")
	writeText(t, filepath.Join(local, "descriptor.mod"), "name=\"Local A\"\nversion=\"1.0\"\nsupported_version=\"v4.*\"\nremote_file_id=\"555\"\ntags={\n\t\"Gameplay\"\n}\n")
	writeText(t, filepath.Join(local, "common", "a.txt"), "x = 1")
	writeText(t, filepath.Join(env.modDir, "local_a.mod"), "name=\"Local A\"\nversion=\"1.0\"\nsupported_version=\"v4.*\"\ntags={\n\t\"Gameplay\"\n}\npath=\""+local+"\"\nremote_file_id=\"555\"\n")

	// A local mod that has only a stub (the real Lustful Void looks like this: tabs, no descriptor.mod).
	stubOnly := filepath.Join(env.lib, "Stub Only")
	writeText(t, filepath.Join(stubOnly, "common", "b.txt"), "y = 1")
	writeText(t, filepath.Join(env.modDir, "Stub Only.mod"), "name=\"Stub Only\"\ntags={\n\t\"No more tags!\"\n}\nsupported_version=\"v4.0.*\"\npath=\""+stubOnly+"\"\n")

	// A mod found in an extra folder: a descriptor.mod and no stub.
	writeText(t, filepath.Join(env.extra, "Extra Mod", "descriptor.mod"), "name=\"Extra Mod\"\nsupported_version=\"v4.*\"\n")
	writeText(t, filepath.Join(env.extra, "Extra Mod", "common", "c.txt"), "z = 1")

	// A subscribed Workshop mod, a Paradox Launcher mod, the app's own patch and a packed archive.
	workshop := filepath.Join(env.lib, "workshop", "999")
	writeText(t, filepath.Join(workshop, "common", "w.txt"), "w = 1")
	writeText(t, filepath.Join(env.modDir, "ugc_999.mod"), "name=\"Steam Mod\"\npath=\""+workshop+"\"\nremote_file_id=\"999\"\n")
	pdx := filepath.Join(env.lib, "pdx", "1")
	writeText(t, filepath.Join(pdx, "common", "p.txt"), "p = 1")
	writeText(t, filepath.Join(env.modDir, "pdx_00001.mod"), "name=\"Launcher Mod\"\npath=\""+pdx+"\"\n")
	patch := filepath.Join(env.lib, "patch")
	writeText(t, filepath.Join(patch, "common", "q.txt"), "q = 1")
	writeText(t, filepath.Join(env.modDir, library.PatchModID+".mod"), "name=\"Parallax Patch\"\npath=\""+patch+"\"\n")
	archive := filepath.Join(env.lib, "packed.zip")
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("common/z.txt")
	w.Write([]byte("z"))
	zw.Close()
	if err := os.WriteFile(archive, zbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(env.modDir, "packed.mod"), "name=\"Packed\"\narchive=\""+archive+"\"\n")

	prefs := preferences.Defaults()
	prefs.ExtraModFolders = map[string][]string{env.cfg.ID: {env.extra}}
	env.a = &App{
		ctx:          context.Background(),
		registry:     game.NewRegistry([]game.GameConfig{env.cfg}),
		preferences:  prefs,
		configAppDir: t.TempDir(),
		eventSink: func(name string, args ...any) {
			env.eventMu.Lock()
			defer env.eventMu.Unlock()
			env.events = append(env.events, name)
		},
	}
	return env
}

func (e *editorEnv) info(t *testing.T, id string) EditInfo {
	t.Helper()
	info, err := e.a.ModEditInfo(e.cfg.ID, id)
	if err != nil {
		t.Fatalf("ModEditInfo(%s): %v", id, err)
	}
	return info
}

func (e *editorEnv) sawEvent(name string) bool {
	e.eventMu.Lock()
	defer e.eventMu.Unlock()
	for _, n := range e.events {
		if n == name {
			return true
		}
	}
	return false
}

func TestEditingALocalModChangesItsDescriptorAndItsStubAndKeepsTheRest(t *testing.T) {
	env := newEditorEnv(t)
	info := env.info(t, "local_a")
	if !info.Editable || len(info.Files) != 2 || info.Fields.Name != "Local A" || info.CanCreateDescriptor {
		t.Fatalf("info = %+v", info)
	}

	edit := ModEdit{Fields: info.Fields}
	edit.Fields.Name = "Local A, renamed"
	edit.Fields.Tags = []string{"Gameplay", "Balance"}
	edit.Fields.Dependencies = []string{"Other Mod"}
	edit.Fields.ReplacePaths = []string{"common/buildings"}

	// Previewing writes nothing.
	descPath := filepath.Join(env.lib, "Local A", "descriptor.mod")
	stubPath := filepath.Join(env.modDir, "local_a.mod")
	beforeDesc, beforeStub := readText(t, descPath), readText(t, stubPath)
	prev, err := env.a.PreviewModEdit(env.cfg.ID, "local_a", edit)
	if err != nil {
		t.Fatal(err)
	}
	if len(prev.Files) != 2 || prev.Nothing || len(prev.Problems) != 0 || !prev.Files[0].Changed || !prev.Files[1].Changed {
		t.Fatalf("preview = %+v", prev)
	}
	if readText(t, descPath) != beforeDesc || readText(t, stubPath) != beforeStub {
		t.Fatal("previewing changed a file")
	}

	res, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit)
	if err != nil {
		t.Fatalf("SaveModEdit: %v", err)
	}
	if len(res.Files) != 2 {
		t.Errorf("wrote %v", res.Files)
	}
	desc, stub := readText(t, descPath), readText(t, stubPath)
	for _, text := range []string{desc, stub} {
		for _, want := range []string{`name="Local A, renamed"`, `"Balance"`, `"Other Mod"`, `replace_path="common/buildings"`, `remote_file_id="555"`, `version="1.0"`} {
			if !strings.Contains(text, want) {
				t.Errorf("%q missing from:\n%s", want, text)
			}
		}
	}
	if !strings.Contains(stub, `path="`+filepath.Join(env.lib, "Local A")+`"`) {
		t.Errorf("the stub's path= changed:\n%s", stub)
	}
	if !env.sawEvent("mods-changed") {
		t.Error("the lists were not told to refresh")
	}
	if !env.a.watchMute.Muted() {
		t.Error("the app's own writes were not kept from the folder watcher")
	}
	// The scan now sees the new values.
	if got := env.info(t, "local_a").Fields; got.Name != "Local A, renamed" || len(got.Tags) != 2 || got.ReplacePaths[0] != "common/buildings" {
		t.Errorf("after saving the mod reads as %+v", got)
	}
	if info := env.info(t, "local_a"); info.HistoryCount != 1 || info.LastSavedAt == 0 {
		t.Errorf("history = %d / %d", info.HistoryCount, info.LastSavedAt)
	}

	// An edit that changes nothing says so and writes nothing.
	same := ModEdit{Fields: env.info(t, "local_a").Fields}
	if prev, _ := env.a.PreviewModEdit(env.cfg.ID, "local_a", same); !prev.Nothing {
		t.Errorf("an unchanged edit is not 'nothing': %+v", prev)
	}
	if r, err := env.a.SaveModEdit(env.cfg.ID, "local_a", same); err != nil || len(r.Files) != 0 {
		t.Errorf("an unchanged save = %+v, %v", r, err)
	}
	if info := env.info(t, "local_a"); info.HistoryCount != 1 {
		t.Errorf("a save that changed nothing added history: %d", info.HistoryCount)
	}
}

func TestUndoPutsBothFilesBackAndRefusesWhatChangedSince(t *testing.T) {
	env := newEditorEnv(t)
	descPath := filepath.Join(env.lib, "Local A", "descriptor.mod")
	stubPath := filepath.Join(env.modDir, "local_a.mod")
	origDesc, origStub := readText(t, descPath), readText(t, stubPath)

	edit := ModEdit{Fields: env.info(t, "local_a").Fields}
	edit.Fields.Name = "Renamed"
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err != nil {
		t.Fatal(err)
	}
	if _, err := env.a.UndoModEdit(env.cfg.ID, "local_a"); err != nil {
		t.Fatalf("UndoModEdit: %v", err)
	}
	if readText(t, descPath) != origDesc || readText(t, stubPath) != origStub {
		t.Error("undo did not restore the files byte for byte")
	}
	if _, err := env.a.UndoModEdit(env.cfg.ID, "local_a"); err == nil {
		t.Error("undoing with nothing saved should say so")
	}

	// Save again, then change a file behind the app's back.
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err != nil {
		t.Fatal(err)
	}
	writeText(t, stubPath, readText(t, stubPath)+"# edited by hand\n")
	_, err := env.a.UndoModEdit(env.cfg.ID, "local_a")
	if err == nil || !strings.Contains(err.Error(), "changed after that save") {
		t.Fatalf("undo = %v, want a refusal that says a file changed", err)
	}
	if !strings.Contains(readText(t, stubPath), "# edited by hand") || !strings.Contains(readText(t, descPath), `name="Renamed"`) {
		t.Error("a refused undo changed files")
	}
}

func TestAStubOnlyModEditsTheStubAndCanGetADescriptor(t *testing.T) {
	env := newEditorEnv(t)
	info := env.info(t, "Stub Only")
	if !info.Editable || len(info.Files) != 1 || info.Files[0].Kind != modedit.KindStub || !info.CanCreateDescriptor {
		t.Fatalf("info = %+v", info)
	}
	edit := ModEdit{Fields: info.Fields}
	edit.Fields.Version = "0.1"
	if _, err := env.a.SaveModEdit(env.cfg.ID, "Stub Only", edit); err != nil {
		t.Fatal(err)
	}
	descPath := filepath.Join(env.lib, "Stub Only", "descriptor.mod")
	if _, err := os.Stat(descPath); !os.IsNotExist(err) {
		t.Fatal("a descriptor.mod was created without being asked for")
	}
	stub := readText(t, filepath.Join(env.modDir, "Stub Only.mod"))
	if !strings.Contains(stub, `version="0.1"`) || !strings.Contains(stub, "\t\"No more tags!\"") {
		t.Errorf("stub:\n%s", stub)
	}

	edit.CreateDescriptor = true
	edit.Fields.Version = "0.2"
	if _, err := env.a.SaveModEdit(env.cfg.ID, "Stub Only", edit); err != nil {
		t.Fatal(err)
	}
	created := readText(t, descPath)
	if !strings.Contains(created, `name="Stub Only"`) || !strings.Contains(created, `version="0.2"`) || strings.Contains(created, "path=") {
		t.Errorf("descriptor.mod:\n%s", created)
	}
	if info := env.info(t, "Stub Only"); len(info.Files) != 2 || info.CanCreateDescriptor {
		t.Errorf("after creating: %+v", info)
	}
	// Undoing the save that created it removes it again.
	if _, err := env.a.UndoModEdit(env.cfg.ID, "Stub Only"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(descPath); !os.IsNotExist(err) {
		t.Error("undo left the descriptor.mod it had created")
	}
}

func TestAModFoundInAnExtraFolderIsEditedThroughItsDescriptor(t *testing.T) {
	env := newEditorEnv(t)
	var id string
	res, _ := scanIDs(env)
	for _, m := range res {
		if m.Descriptor.Name == "Extra Mod" {
			id = m.ID
		}
	}
	if id == "" {
		t.Fatal("the extra-folder mod was not found")
	}
	info := env.info(t, id)
	if !info.Editable || len(info.Files) != 1 || info.Files[0].Kind != modedit.KindDescriptor {
		t.Fatalf("info = %+v", info)
	}
	edit := ModEdit{Fields: info.Fields}
	edit.Fields.SupportedVersion = "v4.4.*"
	if _, err := env.a.SaveModEdit(env.cfg.ID, id, edit); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, filepath.Join(env.extra, "Extra Mod", "descriptor.mod")); !strings.Contains(got, `supported_version="v4.4.*"`) {
		t.Errorf("descriptor.mod:\n%s", got)
	}
	// No stub was invented: that is the launch step's job.
	if entries, _ := os.ReadDir(env.modDir); len(entries) != 6 {
		t.Errorf("mod folder has %d entries, want the 6 that were there", len(entries))
	}
}

func scanIDs(env *editorEnv) ([]mod.Mod, error) {
	res, err := scan.Scan(context.Background(), scan.Options{Game: env.cfg, ExtraFolders: []string{env.extra}})
	return res.Mods, err
}

func TestModsThatAreNotTheirsToEditAreRefusedWithAReason(t *testing.T) {
	env := newEditorEnv(t)
	for id, reason := range map[string]string{
		"ugc_999":          "Steam Workshop",
		"pdx_00001":        "Paradox Launcher",
		library.PatchModID: "generated by Parallax",
		"packed":           "packed archive",
	} {
		info := env.info(t, id)
		if info.Editable || !strings.Contains(info.Reason, reason) {
			t.Errorf("%s: editable=%v reason=%q, want a refusal mentioning %q", id, info.Editable, info.Reason, reason)
		}
		before := readText(t, filepath.Join(env.modDir, id+".mod"))
		edit := ModEdit{Fields: modedit.Fields{Name: "Hacked"}}
		if _, err := env.a.SaveModEdit(env.cfg.ID, id, edit); err == nil {
			t.Errorf("%s: a save should be refused", id)
		}
		if prev, _ := env.a.PreviewModEdit(env.cfg.ID, id, edit); len(prev.Problems) == 0 {
			t.Errorf("%s: the preview should carry the reason as a problem", id)
		}
		if readText(t, filepath.Join(env.modDir, id+".mod")) != before {
			t.Errorf("%s: a refused save changed the file", id)
		}
	}
	if _, err := env.a.ModEditInfo(env.cfg.ID, "no-such-mod"); err == nil {
		t.Error("an unknown mod should be an error")
	}
	if _, err := env.a.ModEditInfo("no-such-game", "local_a"); err == nil {
		t.Error("an unknown game should be an error")
	}
}

func TestAnInvalidEditWritesNothing(t *testing.T) {
	env := newEditorEnv(t)
	stubPath := filepath.Join(env.modDir, "local_a.mod")
	before := readText(t, stubPath)
	edit := ModEdit{Fields: env.info(t, "local_a").Fields}
	edit.Fields.Name = "   "
	prev, err := env.a.PreviewModEdit(env.cfg.ID, "local_a", edit)
	if err != nil || len(prev.Problems) == 0 {
		t.Errorf("preview = %+v, %v; want a problem about the name", prev, err)
	}
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err == nil {
		t.Error("an empty name should be refused")
	}
	if readText(t, stubPath) != before {
		t.Error("a refused save changed a file")
	}
	if info := env.info(t, "local_a"); info.HistoryCount != 0 {
		t.Errorf("a refused save left history: %d", info.HistoryCount)
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{30, 140, 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestANewThumbnailIsResizedSavedAndPointedAtAndUndoRestoresTheOldOne(t *testing.T) {
	env := newEditorEnv(t)
	content := filepath.Join(env.lib, "Local A")
	thumbPath := filepath.Join(content, "thumbnail.png")
	writePNG(t, thumbPath, 64, 64) // the current thumbnail
	oldThumb, _ := os.ReadFile(thumbPath)

	src := filepath.Join(t.TempDir(), "mine.png")
	writePNG(t, src, 1600, 800)
	prev, err := env.a.PreviewThumbnail(src)
	if err != nil || prev.Width != 512 || prev.Height != 256 || !prev.Resized || !strings.HasPrefix(prev.DataURI, "data:image/png;base64,") {
		t.Fatalf("PreviewThumbnail = %+v, %v", prev, err)
	}

	edit := ModEdit{Fields: env.info(t, "local_a").Fields, ThumbnailFrom: src}
	full, err := env.a.PreviewModEdit(env.cfg.ID, "local_a", edit)
	if err != nil || full.Thumbnail == nil || full.Nothing || !strings.Contains(full.Files[0].After, `picture="thumbnail.png"`) {
		t.Fatalf("preview = %+v, %v", full, err)
	}
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(thumbPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 512 || img.Bounds().Dy() != 256 {
		t.Fatalf("the saved thumbnail is not the resized picture: %v %v", err, img)
	}
	for _, p := range []string{filepath.Join(content, "descriptor.mod"), filepath.Join(env.modDir, "local_a.mod")} {
		if !strings.Contains(readText(t, p), `picture="thumbnail.png"`) {
			t.Errorf("%s does not point at the thumbnail", p)
		}
	}
	// The app's own thumbnail lookup finds it.
	uri, err := library.ModThumbnail(context.Background(), env.cfg, library.Options{ExtraFolders: []string{env.extra}}, "local_a")
	if err != nil || !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Errorf("ModThumbnail = %q, %v", uri[:min(len(uri), 30)], err)
	}

	if _, err := env.a.UndoModEdit(env.cfg.ID, "local_a"); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(thumbPath); !bytes.Equal(now, oldThumb) {
		t.Error("undo did not bring the old thumbnail back")
	}
	if strings.Contains(readText(t, filepath.Join(env.modDir, "local_a.mod")), "picture=") {
		t.Error("undo left the picture= line")
	}
}

func TestABadThumbnailIsAProblemNotACrash(t *testing.T) {
	env := newEditorEnv(t)
	notPicture := filepath.Join(t.TempDir(), "x.png")
	writeText(t, notPicture, "not a picture")
	edit := ModEdit{Fields: env.info(t, "local_a").Fields, ThumbnailFrom: notPicture}
	prev, err := env.a.PreviewModEdit(env.cfg.ID, "local_a", edit)
	if err != nil || len(prev.Problems) == 0 {
		t.Errorf("preview = %+v, %v", prev, err)
	}
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err == nil {
		t.Error("saving with a bad picture should fail")
	}
	if _, err := env.a.PreviewThumbnail(notPicture); err == nil {
		t.Error("PreviewThumbnail should refuse a file that is not a picture")
	}
}

func TestSavingNeedsSomewhereToKeepTheOldVersion(t *testing.T) {
	env := newEditorEnv(t)
	env.a.configAppDir = ""
	stubPath := filepath.Join(env.modDir, "local_a.mod")
	before := readText(t, stubPath)
	edit := ModEdit{Fields: env.info(t, "local_a").Fields}
	edit.Fields.Name = "Renamed"
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err == nil {
		t.Error("a save with no place for the safety net must fail")
	}
	if readText(t, stubPath) != before {
		t.Error("the file was written without a safety net")
	}
}

func TestTheLogSaysWhatWasSavedButNeverWhatItSaid(t *testing.T) {
	applog.Default().Clear()
	env := newEditorEnv(t)
	edit := ModEdit{Fields: env.info(t, "local_a").Fields}
	edit.Fields.Name = "A Very Secret Working Title"
	if _, err := env.a.SaveModEdit(env.cfg.ID, "local_a", edit); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range applog.Default().Entries() {
		if e.Component == "Editor" && strings.Contains(e.Message, "saved") && strings.Contains(e.Message, "descriptor.mod") {
			found = true
		}
		if strings.Contains(e.Message, "Secret Working Title") {
			t.Errorf("edited text reached the log: %s", e.Message)
		}
	}
	if !found {
		t.Error("the save left no line in the log")
	}
	applog.Default().Clear()
}
