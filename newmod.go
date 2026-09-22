package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/backup"
	"github.com/Official-Husko/parallax-mod-manager/internal/fsutil"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
)

// The Editor's New tab: creating a brand-new local mod from scratch (CreateMod), or duplicating
// an existing one, including a Steam Workshop mod, into a brand-new independent one
// (DuplicateMod) - never touching the mod being duplicated. Neither ever feeds the Edit tab's
// own "Undo last save" history: that is about edits to a mod that already exists, and a partial
// undo of a freshly-created or freshly-copied mod folder would cause more trouble than it solves,
// especially for a big one. Removing an unwanted new or duplicated mod is Open folder + delete,
// the same as for any other mod.

// duplicateBigThreshold: past this many bytes, PreviewDuplicateMod's Big signals the frontend to
// show a warning ("this might take a while, make sure there is room") before the copy starts.
const duplicateBigThreshold = 1 << 30 // 1 GiB

// NewModLocation is one place a new (or duplicated) mod's folder could be created.
type NewModLocation struct {
	Path    string
	Label   string
	Default bool
}

// NewModLocations lists where a new mod for gameID could be created: the game's own mod folder
// first (always present, Default true), then any extra folder already configured for it in
// Settings - see preferences.Preferences.ExtraModFolders. Refuses for a game whose mods are not
// in the classic descriptor format, which is the only format CreateMod/DuplicateMod write.
func (a *App) NewModLocations(gameID string) ([]NewModLocation, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	if cfg.DescriptorType != mod.DescriptorClassic {
		return nil, errors.New("This game's mods are described by a format the editor does not handle yet.")
	}
	userDir, err := cfg.UserDataDir()
	if err != nil {
		return nil, err
	}
	locations := []NewModLocation{{Path: filepath.Join(userDir, "mod"), Label: "This game's mod folder", Default: true}}
	for _, dir := range a.extraModFolders(gameID) {
		locations = append(locations, NewModLocation{Path: dir, Label: dir})
	}
	return locations, nil
}

// resolveNewLocation re-validates location against gameID's current NewModLocations - never
// trusts a path the frontend sent back without checking it is still one of the real choices.
// isModDir is true when location is the game's own mod folder, the only case that also needs a
// stub alongside the mod's own descriptor.
func (a *App) resolveNewLocation(gameID, location string) (dir string, isModDir bool, err error) {
	locations, err := a.NewModLocations(gameID)
	if err != nil {
		return "", false, err
	}
	for _, l := range locations {
		if l.Path == location {
			return l.Path, l.Default, nil
		}
	}
	return "", false, fmt.Errorf("app: %q is not one of %s's mod locations", location, a.gameLabel(gameID))
}

// resolveModLocation works out the safe folder name, the mod's own new content folder, and (only
// when location is the game's own mod folder) the stub it would need - shared by CreateMod,
// DuplicateMod and their previews, since both are "pick a name and a location, then check nothing
// is already there" with the same rules. problems is non-empty (with err nil) for a refusal a
// preview should show inline, such as a name already taken; err is a real failure (an unknown
// game, an unreadable location).
func (a *App) resolveModLocation(gameID, location, name string) (contentDir, stubPath string, problems []string, err error) {
	dir, isModDir, err := a.resolveNewLocation(gameID, location)
	if err != nil {
		return "", "", nil, err
	}
	folderName, ferr := modedit.FolderName(strings.TrimSpace(name))
	if ferr != nil {
		return "", "", []string{ferr.Error()}, nil
	}
	contentDir = filepath.Join(dir, folderName)
	if isModDir {
		stubPath = filepath.Join(dir, folderName+".mod")
	}
	if _, statErr := os.Stat(contentDir); statErr == nil {
		problems = append(problems, fmt.Sprintf("A mod folder named %q already exists there.", folderName))
	} else if stubPath != "" {
		if _, statErr := os.Stat(stubPath); statErr == nil {
			problems = append(problems, fmt.Sprintf("A mod folder named %q already exists there.", folderName))
		}
	}
	return contentDir, stubPath, problems, nil
}

// NewModRequest is what the New tab's Create form asks for.
type NewModRequest struct {
	Fields   modedit.Fields // Dependencies/ReplacePaths are left empty by the New form
	Location string         // one of NewModLocations(gameID)'s Path values
}

// PreviewNewMod works out what CreateMod would write, without writing anything - reuses
// EditPreview/EditPreviewFile exactly as PreviewModEdit already returns them, so the frontend's
// existing diff renderer needs no new type.
func (a *App) PreviewNewMod(gameID string, req NewModRequest) (EditPreview, error) {
	contentDir, stubPath, problems, err := a.resolveModLocation(gameID, req.Location, req.Fields.Name)
	if err != nil {
		return EditPreview{}, err
	}
	if len(problems) > 0 {
		return EditPreview{Problems: problems}, nil
	}
	edits, err := modedit.NewFiles(req.Fields, contentDir, stubPath)
	if err != nil {
		return EditPreview{Problems: []string{err.Error()}}, nil
	}
	out := EditPreview{Warnings: req.Fields.Normalized().Warnings()}
	for _, e := range edits {
		out.Files = append(out.Files, EditPreviewFile{Path: e.Path, Kind: e.Kind, Create: e.Create, Changed: e.Changed() || e.Create, Before: e.Before, After: e.After})
	}
	out.Nothing = len(out.Files) == 0
	return out, nil
}

// CreateMod creates a brand-new mod: its own descriptor.mod and, only when Location is the
// game's own mod folder, the stub that registers it there - written content descriptor first,
// stub last, so a failure never leaves a half-visible mod (the game only sees a mod once its
// stub exists in its own mod folder). A failed write rolls back whatever it already wrote.
func (a *App) CreateMod(gameID string, req NewModRequest) (SaveResult, error) {
	modEditMu.Lock()
	defer modEditMu.Unlock()
	log := applog.For("Editor")

	contentDir, stubPath, problems, err := a.resolveModLocation(gameID, req.Location, req.Fields.Name)
	if err != nil {
		return SaveResult{}, err
	}
	if len(problems) > 0 {
		return SaveResult{}, errors.New(problems[0])
	}
	edits, err := modedit.NewFiles(req.Fields, contentDir, stubPath)
	if err != nil {
		return SaveResult{}, err
	}

	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()

	var paths []string
	for _, e := range edits {
		if _, writeErr := atomicfile.Write(filepath.Dir(e.Path), filepath.Base(e.Path), []byte(e.After)); writeErr != nil {
			_ = os.RemoveAll(contentDir)
			if stubPath != "" {
				_ = os.Remove(stubPath)
			}
			log.Errorf("creating '%s' in '%s' failed: %v", req.Fields.Name, a.gameLabel(gameID), writeErr)
			return SaveResult{}, writeErr
		}
		paths = append(paths, e.Path)
	}

	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	log.Infof("created '%s' in '%s': %s", req.Fields.Name, a.gameLabel(gameID), strings.Join(names, ", "))
	a.emit("mods-changed", gameID)
	return SaveResult{Files: paths, SavedAt: time.Now().Unix()}, nil
}

// resolveDuplicateSource resolves modID for duplicating: works for any source - local, Workshop,
// Paradox Launcher - since it only reads the mod's own content and descriptor, never checking
// whether it may be edited in place (that is what makes duplicating a safe way to "edit" a
// Workshop mod without ever touching Steam's own copy). Refuses only what genuinely cannot be
// duplicated: a non-classic game, the app's own generated patch, or a mod with no real folder.
func (a *App) resolveDuplicateSource(gameID, modID string) (editTarget, error) {
	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return editTarget{}, err
	}
	if t.cfg.DescriptorType != mod.DescriptorClassic {
		return editTarget{}, errors.New("This game's mods are described by a format the editor does not handle yet.")
	}
	if t.m.ID == library.PatchModID {
		return editTarget{}, errors.New("This mod is generated by Parallax Mod Manager, so there is nothing of its own to duplicate.")
	}
	if t.m.ContentPath == "" {
		return editTarget{}, errors.New("The mod's folder was not found on disk.")
	}
	info, statErr := os.Stat(t.m.ContentPath)
	if statErr != nil || !info.IsDir() {
		return editTarget{}, errors.New("This mod is a packed archive; only mods in a folder can be duplicated.")
	}
	return t, nil
}

// duplicateFileEdits works out the descriptor.mod a duplicate would have (in place, from the
// source's own current text, if it has one - fresh otherwise, both through the ordinary Plan) and,
// when stubPath is not "", its stub - always fresh (see modedit.NewStub), never copied from the
// source, since the source's own stub (if it has one) lives outside the folder being copied (in
// the game's mod folder, not inside the mod's own content folder). Shared by PreviewDuplicateMod
// and DuplicateMod so a preview never promises something the real action would not do.
func duplicateFileEdits(src editTarget, contentDir, stubPath string, want modedit.Fields, picture string) ([]modedit.FileEdit, error) {
	descFile := modedit.File{Path: filepath.Join(contentDir, "descriptor.mod"), Kind: modedit.KindDescriptor}
	for _, f := range src.files {
		if f.Kind == modedit.KindDescriptor {
			descFile.Exists, descFile.Text = true, f.Text
		}
	}
	edits, err := modedit.Plan([]modedit.File{descFile}, want, picture)
	if err != nil {
		return nil, err
	}
	if stubPath == "" {
		return edits, nil
	}
	stub, err := modedit.NewStub(want, contentDir, stubPath)
	if err != nil {
		return nil, err
	}
	return append(edits, stub), nil
}

// duplicateWant is the Fields a duplicate keeps from its source - everything except the name,
// which the person typing it in chooses fresh.
func duplicateWant(src editTarget, name string) modedit.Fields {
	return modedit.Fields{
		Name:             strings.TrimSpace(name),
		Version:          src.m.Descriptor.Version,
		SupportedVersion: src.m.Descriptor.SupportedVersion,
		Tags:             src.m.Descriptor.Tags,
		Dependencies:     src.m.Descriptor.Dependencies,
		ReplacePaths:     src.m.Descriptor.ReplacePath,
	}
}

// DuplicateRequest is what the New tab's Duplicate form asks for - just a new name and a
// location; every other field is kept from the mod being duplicated.
type DuplicateRequest struct {
	Name     string
	Location string
}

// DuplicatePreview is a normal EditPreview (the descriptor.mod, renamed or created fresh, and the
// stub when the target is the game's own mod folder) plus what the bulk folder copy itself would
// mean: its size, and, when it is over duplicateBigThreshold, a warning the frontend turns into
// a "this might take a while" screen before it starts.
type DuplicatePreview struct {
	EditPreview
	SourceFiles  int
	SourceBytes  int64
	Big          bool
	FreeAtTarget int64 // -1 when it could not be determined
}

// PreviewDuplicateMod works out what DuplicateMod would do, without copying or writing anything.
func (a *App) PreviewDuplicateMod(gameID, modID string, req DuplicateRequest) (DuplicatePreview, error) {
	src, err := a.resolveDuplicateSource(gameID, modID)
	if err != nil {
		return DuplicatePreview{}, err
	}

	files, bytes, _ := fsutil.DirStat(src.m.ContentPath)
	out := DuplicatePreview{SourceFiles: files, SourceBytes: bytes, Big: bytes > duplicateBigThreshold, FreeAtTarget: -1}

	contentDir, stubPath, problems, err := a.resolveModLocation(gameID, req.Location, req.Name)
	if err != nil {
		return DuplicatePreview{}, err
	}
	if free, ok := backup.DiskFree(filepath.Dir(contentDir)); ok {
		out.FreeAtTarget = int64(free)
	}
	if len(problems) > 0 {
		out.EditPreview.Problems = problems
		return out, nil
	}

	edits, err := duplicateFileEdits(src, contentDir, stubPath, duplicateWant(src, req.Name), src.m.Descriptor.Picture)
	if err != nil {
		out.EditPreview.Problems = []string{err.Error()}
		return out, nil
	}
	for _, e := range edits {
		out.EditPreview.Files = append(out.EditPreview.Files, EditPreviewFile{Path: e.Path, Kind: e.Kind, Create: e.Create, Changed: e.Changed() || e.Create, Before: e.Before, After: e.After})
	}
	out.EditPreview.Nothing = len(out.EditPreview.Files) == 0
	return out, nil
}

// DuplicateProgress is one moment of a running DuplicateMod call.
type DuplicateProgress struct {
	RequestID string
	File      string
	Done      int64
	Total     int64
}

// DuplicateMod copies modID's folder into a brand-new, independent mod, then writes its
// (renamed) descriptor.mod and, when Location is the game's own mod folder, its stub - the same
// content-first, stub-last order as CreateMod, for the same reason. requestID (any string the
// caller makes up per attempt) tags every "duplicate-progress" event emitted while the copy runs,
// and is what CancelDuplicate stops. Never touches the mod being duplicated.
func (a *App) DuplicateMod(gameID, modID, requestID string, req DuplicateRequest) (SaveResult, error) {
	modEditMu.Lock()
	defer modEditMu.Unlock()
	log := applog.For("Editor")

	src, err := a.resolveDuplicateSource(gameID, modID)
	if err != nil {
		return SaveResult{}, err
	}
	contentDir, stubPath, problems, err := a.resolveModLocation(gameID, req.Location, req.Name)
	if err != nil {
		return SaveResult{}, err
	}
	if len(problems) > 0 {
		return SaveResult{}, errors.New(problems[0])
	}

	ctx, cancel := context.WithCancel(a.baseContext())
	a.duplicateMu.Lock()
	if a.duplicateCancel == nil {
		a.duplicateCancel = map[string]context.CancelFunc{}
	}
	a.duplicateCancel[requestID] = cancel
	a.duplicateMu.Unlock()
	defer func() {
		a.duplicateMu.Lock()
		delete(a.duplicateCancel, requestID)
		a.duplicateMu.Unlock()
		cancel()
	}()

	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()

	_, totalBytes, _ := fsutil.DirStat(src.m.ContentPath)
	_, copyErr := fsutil.CopyTree(ctx, src.m.ContentPath, contentDir, totalBytes, func(file string, done, tot int64) {
		a.emit("duplicate-progress", DuplicateProgress{RequestID: requestID, File: file, Done: done, Total: tot})
	})
	if copyErr != nil {
		_ = os.RemoveAll(contentDir)
		if errors.Is(copyErr, context.Canceled) {
			log.Infof("duplicating '%s' in '%s' was cancelled", src.m.Descriptor.Name, a.gameLabel(gameID))
			return SaveResult{}, errors.New("Duplicating was cancelled.")
		}
		log.Errorf("duplicating '%s' in '%s' failed: %v", src.m.Descriptor.Name, a.gameLabel(gameID), copyErr)
		return SaveResult{}, copyErr
	}

	edits, err := duplicateFileEdits(src, contentDir, stubPath, duplicateWant(src, req.Name), src.m.Descriptor.Picture)
	if err != nil {
		_ = os.RemoveAll(contentDir)
		return SaveResult{}, err
	}

	var paths []string
	for _, e := range edits {
		if !e.Changed() && !e.Create {
			continue
		}
		if _, writeErr := atomicfile.Write(filepath.Dir(e.Path), filepath.Base(e.Path), []byte(e.After)); writeErr != nil {
			_ = os.RemoveAll(contentDir)
			if stubPath != "" {
				_ = os.Remove(stubPath)
			}
			log.Errorf("duplicating '%s' in '%s' failed: %v", src.m.Descriptor.Name, a.gameLabel(gameID), writeErr)
			return SaveResult{}, writeErr
		}
		paths = append(paths, e.Path)
	}

	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	log.Infof("duplicated '%s' as '%s' in '%s': %s", src.m.Descriptor.Name, req.Name, a.gameLabel(gameID), strings.Join(names, ", "))
	a.emit("mods-changed", gameID)
	return SaveResult{Files: paths, SavedAt: time.Now().Unix()}, nil
}

// CancelDuplicate stops the DuplicateMod call tagged with requestID, if it is still running - a
// no-op otherwise (the copy may already be finished).
func (a *App) CancelDuplicate(requestID string) {
	a.duplicateMu.Lock()
	cancel := a.duplicateCancel[requestID]
	a.duplicateMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
