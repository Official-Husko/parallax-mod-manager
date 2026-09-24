package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/fsutil"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// The Editor (see internal/modedit): changing the descriptor files and the thumbnail of a mod the
// person made. Only mods whose files are theirs are editable - a subscribed Workshop mod is
// rewritten by Steam when it updates, a Paradox Launcher mod belongs to the launcher, and the patch
// this app generates is the app's own.

// modEditMu serialises saves and undos: each reads files, plans against them and writes.
var modEditMu sync.Mutex

// EditFile is one descriptor file of a mod.
type EditFile struct {
	Path   string
	Kind   string
	Exists bool
}

// EditInfo is what the Editor needs to know about one mod.
type EditInfo struct {
	ModID string
	Name  string
	// Editable is false for a mod the person cannot change here; Reason says why.
	Editable bool
	Reason   string
	// Overridable is true when Reason is a stated risk (a Steam Workshop or Paradox Launcher
	// mod) the person can knowingly accept instead - see ModEdit.Force.
	Overridable bool
	// ContentPath is the mod's folder.
	ContentPath string
	Fields      modedit.Fields
	// Picture is the descriptor's picture= value.
	Picture string
	// Files are the descriptor files that exist (a save changes each of them).
	Files []EditFile
	// CanCreateDescriptor is true when the mod has a folder but no descriptor.mod in it.
	CanCreateDescriptor bool
	// HistoryCount and LastSavedAt (unix seconds, 0 for none) describe the saves that can be undone.
	HistoryCount int
	LastSavedAt  int64
	// MaxHistoryCount is modedit.KeptSaves, surfaced so the frontend's own "Undo keeps the last
	// N saves" note never has to duplicate that number by hand.
	MaxHistoryCount int
	// FolderName is the mod's own content folder's name, read-only here - renaming it means
	// duplicating under a new name, not an in-place field edit.
	FolderName string
	// VersionBump is set when this mod's descriptor fields or file count have changed since its
	// last save through this app, and its current version parses as a plain number - see
	// modedit.SuggestBump. nil otherwise (nothing to suggest, or the version isn't a plain
	// number to bump from).
	VersionBump *modedit.VersionBumpSuggestion
}

// ModEdit is a change the person wants to make.
type ModEdit struct {
	Fields modedit.Fields
	// ThumbnailFrom is a picture file to make the mod's thumbnail; "" keeps the current one.
	ThumbnailFrom string
	// CreateDescriptor also writes a descriptor.mod into a mod's folder that has none.
	CreateDescriptor bool
	// Force saves anyway when EditInfo.Overridable was true - a Steam Workshop or Paradox
	// Launcher mod the person has chosen to edit knowing the stated risk. Ignored otherwise.
	Force bool
}

// EditPreviewFile is what a save would do to one file.
type EditPreviewFile struct {
	Path    string
	Kind    string
	Create  bool
	Changed bool
	Before  string
	After   string
}

// ThumbnailPreview is the picture a save would write as the mod's thumbnail.
type ThumbnailPreview struct {
	// DataURI is the picture as it would be saved, for showing.
	DataURI                   string
	Width, Height             int
	SourceWidth, SourceHeight int
	Bytes, SourceBytes        int64
	Resized                   bool
	Warnings                  []string
}

// EditPreview is what a save would do, worked out without writing anything.
type EditPreview struct {
	Files []EditPreviewFile
	// Thumbnail is set when a new picture was chosen.
	Thumbnail *ThumbnailPreview
	// Problems stop the save (an empty name, a picture that cannot be used); Warnings do not.
	Problems []string
	Warnings []string
	// Nothing is true when a save would change no file.
	Nothing bool
}

// SaveResult says what a save wrote.
type SaveResult struct {
	Files   []string
	SavedAt int64
}

// EditHistory says how many saves of a mod can be undone.
type EditHistory struct {
	Count       int
	LastSavedAt int64
}

// editTarget is a mod resolved for editing.
type editTarget struct {
	cfg     game.GameConfig
	m       mod.Mod
	modDir  string
	files   []modedit.File // the descriptor files that exist
	descNew string         // where a descriptor.mod would be created; "" when the mod has no folder
	hasDesc bool
	// editable and reason: see EditInfo.
	editable bool
	reason   string
	// overridable is true when reason is a risk the person can knowingly accept (a Steam
	// Workshop or Paradox Launcher mod) rather than something that can never be edited here (the
	// generated patch, a packed archive, a missing folder, a non-classic game) - see
	// EditInfo.Overridable and ModEdit.Force.
	overridable bool
}

// effectiveEditable is whether a save may proceed: editable outright, or overridable and the
// person has said Force (ModEdit.Force) to go ahead anyway, knowing the risk in reason.
func (t editTarget) effectiveEditable(force bool) bool {
	return t.editable || (t.overridable && force)
}

func (t editTarget) roots() []string {
	roots := []string{t.modDir}
	if t.m.ContentPath != "" {
		roots = append(roots, t.m.ContentPath)
	}
	return roots
}

func (a *App) editTarget(ctx context.Context, gameID, modID string) (editTarget, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return editTarget{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	userDir, err := cfg.UserDataDir()
	if err != nil {
		return editTarget{}, err
	}
	res, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		return editTarget{}, err
	}
	var m mod.Mod
	found := false
	for _, c := range res.Mods {
		if c.ID == modID {
			m, found = c, true
			break
		}
	}
	if !found {
		return editTarget{}, fmt.Errorf("app: mod %q was not found", modID)
	}
	return resolveEditTarget(cfg, m, filepath.Join(userDir, "mod")), nil
}

// resolveEditTarget works out which of a mod's files can be edited and whether it may be.
func resolveEditTarget(cfg game.GameConfig, m mod.Mod, modDir string) editTarget {
	t := editTarget{cfg: cfg, m: m, modDir: modDir}

	contentIsDir, contentIsFile := false, false
	if m.ContentPath != "" {
		if info, err := os.Stat(m.ContentPath); err == nil {
			contentIsDir, contentIsFile = info.IsDir(), !info.IsDir()
		}
	}
	if contentIsDir {
		t.descNew = filepath.Join(m.ContentPath, "descriptor.mod")
	}

	add := func(path, kind string) {
		for _, f := range t.files {
			if f.Path == path {
				return
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		t.files = append(t.files, modedit.File{Path: path, Kind: kind, Exists: true, Text: string(data)})
		if kind == modedit.KindDescriptor {
			t.hasDesc = true
		}
	}
	if t.descNew != "" {
		add(t.descNew, modedit.KindDescriptor)
	}
	stub := filepath.Join(modDir, m.ID+".mod")
	if filepath.Clean(filepath.Dir(m.DescriptorPath)) == filepath.Clean(modDir) {
		stub = m.DescriptorPath
	}
	add(stub, modedit.KindStub)

	switch {
	case cfg.DescriptorType != mod.DescriptorClassic:
		t.reason = "This game's mods are described by a format the editor does not handle yet."
	case m.ID == library.PatchModID:
		t.reason = "This mod is generated by Parallax Mod Manager, which rewrites it whenever you generate the patch."
	case m.Source == mod.SourceWorkshop:
		t.reason = "This is a Steam Workshop mod. Steam replaces its files whenever the mod updates, so an edit would not last."
		t.overridable = true
	case m.Source == mod.SourceParadoxLauncher:
		t.reason = "This mod is managed by the Paradox Launcher."
		t.overridable = true
	case contentIsFile:
		t.reason = "This mod is a packed archive; only mods in a folder can be edited."
	case !contentIsDir:
		t.reason = "The mod's folder was not found on disk."
	case len(t.files) == 0:
		t.reason = "The mod has no descriptor file the editor can find."
	default:
		t.editable = true
	}
	return t
}

func (a *App) modEditStore(t editTarget) (modedit.Store, error) {
	if a.configAppDir == "" {
		return modedit.Store{}, errors.New("the settings folder could not be found, so earlier versions cannot be kept")
	}
	safe := strings.NewReplacer("/", "_", `\`, "_", "..", "_").Replace(t.m.ID)
	return modedit.Store{
		Dir:   filepath.Join(a.configAppDir, "mod_edit_history", t.cfg.ID, safe),
		Roots: t.roots(),
	}, nil
}

// ModEditInfo describes one mod for the Editor: whether it can be edited and, if not, why; its
// current values; and how many earlier saves can be undone.
func (a *App) ModEditInfo(gameID, modID string) (EditInfo, error) {
	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return EditInfo{}, err
	}
	info := EditInfo{
		ModID: t.m.ID, Name: t.m.Descriptor.Name, Editable: t.editable, Reason: t.reason, Overridable: t.overridable, ContentPath: t.m.ContentPath,
		Fields: modedit.Fields{
			Name:             t.m.Descriptor.Name,
			Version:          t.m.Descriptor.Version,
			SupportedVersion: t.m.Descriptor.SupportedVersion,
			Tags:             nonNil(t.m.Descriptor.Tags),
			Dependencies:     nonNil(t.m.Descriptor.Dependencies),
			ReplacePaths:     nonNil(t.m.Descriptor.ReplacePath),
		},
		Picture:             t.m.Descriptor.Picture,
		CanCreateDescriptor: t.editable && !t.hasDesc && t.descNew != "",
		MaxHistoryCount:     modedit.KeptSaves,
	}
	if t.m.ContentPath != "" {
		info.FolderName = filepath.Base(t.m.ContentPath)
	}
	for _, f := range t.files {
		info.Files = append(info.Files, EditFile{Path: f.Path, Kind: f.Kind, Exists: f.Exists})
	}
	if store, err := a.modEditStore(t); err == nil {
		n, at := store.History()
		info.HistoryCount = n
		if !at.IsZero() {
			info.LastSavedAt = at.Unix()
		}
		if snap, ok := store.LatestVersionSnapshot(); ok && t.m.ContentPath != "" {
			if files, _, statErr := fsutil.DirStat(t.m.ContentPath); statErr == nil {
				if suggestion, ok := modedit.SuggestBump(snap, info.Fields, files, info.Fields.Version); ok {
					info.VersionBump = &suggestion
				}
			}
		}
	}
	return info, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// editPlan works out an edit against the mod's files: the plan, any thumbnail, and what to write.
func (t editTarget) editPlan(edit ModEdit) (edits []modedit.FileEdit, thumb *modedit.Thumbnail, problems, warnings []string) {
	files := append([]modedit.File(nil), t.files...)
	if edit.CreateDescriptor && !t.hasDesc && t.descNew != "" {
		files = append(files, modedit.File{Path: t.descNew, Kind: modedit.KindDescriptor, Exists: false})
	}

	picture := ""
	if strings.TrimSpace(edit.ThumbnailFrom) != "" {
		th, err := modedit.PrepareThumbnail(edit.ThumbnailFrom)
		if err != nil {
			problems = append(problems, err.Error())
		} else {
			thumb = &th
			picture = modedit.ThumbnailFile
			warnings = append(warnings, th.Warnings...)
		}
	}

	planned, err := modedit.Plan(files, edit.Fields, picture)
	if err != nil {
		problems = append(problems, err.Error())
		return nil, thumb, problems, warnings
	}
	warnings = append(warnings, edit.Fields.Normalized().Warnings()...)
	return planned, thumb, problems, warnings
}

// PreviewModEdit works out what saving the edit would do - each file's text before and after, the
// resized thumbnail, and any problems - without writing anything.
func (a *App) PreviewModEdit(gameID, modID string, edit ModEdit) (EditPreview, error) {
	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return EditPreview{}, err
	}
	if !t.effectiveEditable(edit.Force) {
		return EditPreview{Problems: []string{t.reason}}, nil
	}
	edits, thumb, problems, warnings := t.editPlan(edit)
	out := EditPreview{Problems: problems, Warnings: warnings}
	changed := thumb != nil
	for _, e := range edits {
		out.Files = append(out.Files, EditPreviewFile{Path: e.Path, Kind: e.Kind, Create: e.Create, Changed: e.Changed() || e.Create, Before: e.Before, After: e.After})
		if e.Changed() || e.Create {
			changed = true
		}
	}
	if thumb != nil {
		out.Thumbnail = thumbnailPreview(*thumb)
	}
	out.Nothing = !changed
	return out, nil
}

func thumbnailPreview(th modedit.Thumbnail) *ThumbnailPreview {
	return &ThumbnailPreview{
		DataURI: "data:image/png;base64," + base64.StdEncoding.EncodeToString(th.PNG),
		Width:   th.Width, Height: th.Height, SourceWidth: th.SourceWidth, SourceHeight: th.SourceHeight,
		Bytes: int64(len(th.PNG)), SourceBytes: th.SourceBytes, Resized: th.Resized, Warnings: th.Warnings,
	}
}

// PreviewThumbnail reads a picture the person chose and shows what would be saved: resized to fit,
// as a PNG.
func (a *App) PreviewThumbnail(path string) (ThumbnailPreview, error) {
	th, err := modedit.PrepareThumbnail(path)
	if err != nil {
		return ThumbnailPreview{}, err
	}
	return *thumbnailPreview(th), nil
}

// PickThumbnailFile opens a file picker for a picture; "" when the person cancels.
func (a *App) PickThumbnailFile() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Choose a thumbnail picture",
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "Pictures (*.png, *.jpg, *.jpeg, *.gif)",
			Pattern:     "*.png;*.jpg;*.jpeg;*.gif",
		}},
	})
}

// SaveModEdit writes the edit: every descriptor file the mod has, and the thumbnail when one was
// chosen. The files it replaces are kept first (see UndoModEdit). Nothing is written if any part of
// the edit cannot be, and the mod-folder watcher is told to ignore the app's own writes.
func (a *App) SaveModEdit(gameID, modID string, edit ModEdit) (SaveResult, error) {
	modEditMu.Lock()
	defer modEditMu.Unlock()
	log := applog.For("Editor")

	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return SaveResult{}, err
	}
	if !t.effectiveEditable(edit.Force) {
		return SaveResult{}, errors.New(t.reason)
	}
	edits, thumb, problems, _ := t.editPlan(edit)
	if len(problems) > 0 {
		return SaveResult{}, errors.New(problems[0])
	}

	var writes []modedit.Write
	for _, e := range edits {
		if e.Changed() || e.Create {
			writes = append(writes, modedit.Write{Path: e.Path, Data: []byte(e.After)})
		}
	}
	if thumb != nil {
		writes = append(writes, modedit.Write{Path: filepath.Join(t.m.ContentPath, modedit.ThumbnailFile), Data: thumb.PNG})
	}
	if len(writes) == 0 {
		return SaveResult{}, nil
	}

	store, err := a.modEditStore(t)
	if err != nil {
		return SaveResult{}, err
	}
	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()
	at, err := store.Apply(writes)
	if err != nil {
		log.Errorf("saving '%s' failed: %v", editLabel(t), err)
		return SaveResult{}, err
	}
	if t.m.ContentPath != "" {
		if files, _, statErr := fsutil.DirStat(t.m.ContentPath); statErr == nil {
			snap := modedit.VersionSnapshot{Fields: edit.Fields.Normalized(), ContentFileCount: files, SavedAt: at}
			if snapErr := modedit.WriteVersionSnapshot(store.SaveDir(at), snap); snapErr != nil {
				// Never fails the save itself - the version-bump feature simply has nothing to
				// compare against next time, the same as for a mod saved before this feature
				// existed at all.
				log.Warnf("keeping a version-bump snapshot for '%s' failed: %v", editLabel(t), snapErr)
			}
		}
	}

	names := make([]string, 0, len(writes))
	paths := make([]string, 0, len(writes))
	for _, w := range writes {
		names = append(names, filepath.Base(w.Path))
		paths = append(paths, w.Path)
	}
	log.Infof("saved '%s' in '%s': %s", editLabel(t), a.gameLabel(gameID), strings.Join(names, ", "))
	a.emit("mods-changed", gameID)
	return SaveResult{Files: paths, SavedAt: at.Unix()}, nil
}

// UndoModEdit puts back the files the last save replaced - unless one of them was changed since, in
// which case nothing is touched.
func (a *App) UndoModEdit(gameID, modID string) (SaveResult, error) {
	modEditMu.Lock()
	defer modEditMu.Unlock()
	log := applog.For("Editor")

	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return SaveResult{}, err
	}
	store, err := a.modEditStore(t)
	if err != nil {
		return SaveResult{}, err
	}
	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()
	restored, err := store.Undo()
	if err != nil {
		if errors.Is(err, modedit.ErrChangedSince) {
			log.Warnf("undoing the last save of '%s' was refused: %v", editLabel(t), err)
			return SaveResult{}, errors.New("A file was changed after that save (by another program, or by hand), so it was left alone.")
		}
		return SaveResult{}, err
	}
	names := make([]string, 0, len(restored))
	for _, p := range restored {
		names = append(names, filepath.Base(p))
	}
	log.Infof("undid the last save of '%s' in '%s': %s", editLabel(t), a.gameLabel(gameID), strings.Join(names, ", "))
	a.emit("mods-changed", gameID)
	return SaveResult{Files: restored, SavedAt: time.Now().Unix()}, nil
}

// ModEditHistory says how many saves of the mod can be undone.
func (a *App) ModEditHistory(gameID, modID string) (EditHistory, error) {
	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return EditHistory{}, err
	}
	store, err := a.modEditStore(t)
	if err != nil {
		return EditHistory{}, nil
	}
	n, at := store.History()
	out := EditHistory{Count: n}
	if !at.IsZero() {
		out.LastSavedAt = at.Unix()
	}
	return out, nil
}

func editLabel(t editTarget) string {
	if t.m.Descriptor.Name != "" {
		return t.m.Descriptor.Name
	}
	return t.m.ID
}
