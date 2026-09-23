package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabinstall"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabtracking"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
)

// Downloading and installing a mod from the Browse tab - the one part of LoversLab
// integration that writes into the game's own mod folder, unlike everything else in
// loverslab.go (pure browsing/reading). Mirrors newmod.go's CreateMod/DuplicateMod in
// every way that carries over: modEditMu guards the same shared resource (the game's
// mod folder), the watcher is muted the same way while writing, content is written
// before the stub that makes the game see it (so a failure never leaves a half-visible
// mod), and a long-running request is cancellable through the same requestID pattern
// DuplicateMod/CancelDuplicate already use - just its own map, since this is an
// unrelated feature that happens to need the same shape of thing.

// LoversLabDownloadDialog lists a file's downloadable versions/attachments, for the
// Browse tab's detail view to offer when there is more than one.
func (a *App) LoversLabDownloadDialog(filePageURL string) ([]loverslab.FileDownload, error) {
	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return nil, err
	}
	downloads, err := client.ListDownloads(a.baseContext(), filePageURL)
	if err != nil {
		applog.For("LoversLab").Warnf("listing downloads for %s failed: %v", filePageURL, err)
	}
	return downloads, err
}

// LoversLabInstallProgress is one moment of a running LoversLabInstall call.
type LoversLabInstallProgress struct {
	RequestID string
	// Stage is "downloading" or "extracting".
	Stage string
	Done  int64
	// Total is -1 when it could not be determined (no Content-Length on the
	// download response) - the frontend shows an indeterminate progress bar then,
	// rather than a wrong or divide-by-zero one.
	Total int64
	// FileName, FileIndex and FileCount describe which of a possibly multi-file
	// batch install (the person's own checked selection on the Files tab) this
	// progress event belongs to - FileIndex is 1-based, and FileCount is always at
	// least 1 (a single selected file is just a batch of one, never a special case).
	FileName  string
	FileIndex int
	FileCount int
}

// locateLoversLabInstall finds fileID's own stub descriptor for gameID, if one
// exists, and returns where its content actually lives on disk - the shared lookup
// resolveLoversLabInstallLocation (installing) and UninstallLoversLabMod (removing)
// both need: "is this file already installed, and if so, where." found is false
// when there's no stub, or the stub exists but can't be read/parsed (never a reason
// to fail the caller outright - LoversLabInstall treats that as a fresh install,
// UninstallLoversLabMod treats it as "nothing to remove").
func locateLoversLabInstall(modDir string, fileID int) (contentDir, stubPath string, found bool) {
	stubPath = filepath.Join(modDir, mod.LoversLabFilePrefix+strconv.Itoa(fileID)+".mod")
	data, err := os.ReadFile(stubPath)
	if err != nil {
		return "", stubPath, false
	}
	desc, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
	if err != nil || desc.Path == "" {
		return "", stubPath, false
	}
	dir := desc.Path
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(modDir, dir)
	}
	return dir, stubPath, true
}

// resolveLoversLabInstallLocation works out where a LoversLab file should be
// installed for gameID: if it (identified by fileID, not by name - a file can be
// retitled on LoversLab without this app losing track of it) is already installed,
// its own stub's declared path is reused, so a later download updates it in place
// rather than creating a second copy alongside it; otherwise a fresh folder name is
// picked from title, disambiguated with fileID in the rare case that collides with
// some unrelated mod's own folder name.
func (a *App) resolveLoversLabInstallLocation(gameID string, fileID int, title string) (contentDir, stubPath string, isUpdate bool, err error) {
	locations, err := a.NewModLocations(gameID)
	if err != nil {
		return "", "", false, err
	}
	if len(locations) == 0 {
		return "", "", false, errors.New("no install location was found for this game")
	}
	modDir := locations[0].Path // NewModLocations always lists the game's own mod folder first, Default true

	if dir, stub, found := locateLoversLabInstall(modDir, fileID); found {
		return dir, stub, true, nil
	}

	folderName, err := modedit.FolderName(strings.TrimSpace(title))
	if err != nil {
		return "", "", false, err
	}
	contentDir = filepath.Join(modDir, folderName)
	stubPath = filepath.Join(modDir, mod.LoversLabFilePrefix+strconv.Itoa(fileID)+".mod")
	if _, statErr := os.Stat(contentDir); statErr == nil {
		// An unrelated mod already has this exact folder name - not this file's own
		// previous install (that was already handled above), just a naming
		// collision. Disambiguate with the file id rather than refusing outright.
		contentDir = filepath.Join(modDir, folderName+"-"+strconv.Itoa(fileID))
	}
	return contentDir, stubPath, false, nil
}

// UninstallLoversLabMod removes a mod this app previously installed from LoversLab
// for gameID (identified by fileID, the same identity LoversLabInstall itself uses):
// its content folder, its stub descriptor, and its entry in
// internal/loverslabtracking (so the update check stops looking for it) - mirrors
// LoversLabInstall's own conventions exactly (the shared modEditMu lock, muting the
// folder watcher while writing, emitting "mods-changed" once done). A file that
// isn't actually installed returns a clear error rather than silently doing nothing.
func (a *App) UninstallLoversLabMod(gameID string, fileID int) error {
	modEditMu.Lock()
	defer modEditMu.Unlock()
	log := applog.For("LoversLab")

	locations, err := a.NewModLocations(gameID)
	if err != nil {
		return err
	}
	if len(locations) == 0 {
		return errors.New("no install location was found for this game")
	}
	modDir := locations[0].Path

	contentDir, stubPath, found := locateLoversLabInstall(modDir, fileID)
	if !found {
		return errors.New("this mod isn't currently installed from LoversLab")
	}

	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()

	if err := os.RemoveAll(contentDir); err != nil {
		return fmt.Errorf("could not remove %s: %w", contentDir, err)
	}
	if err := os.Remove(stubPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not remove %s: %w", stubPath, err)
	}

	modID := mod.LoversLabFilePrefix + strconv.Itoa(fileID)
	installs, err := a.loverslabInstalls.Load(gameID)
	if err != nil {
		log.Warnf("could not read the LoversLab install tracking file, so this mod's entry was left behind: %v", err)
	} else {
		installs, _ = loverslabtracking.With(installs, modID, loverslabtracking.Entry{})
		if err := a.loverslabInstalls.Save(gameID, installs); err != nil {
			log.Warnf("could not save LoversLab install tracking after uninstalling %s: %v", modID, err)
		}
	}

	log.Infof("uninstalled %s from LoversLab for '%s'", modID, a.gameLabel(gameID))
	a.emit("mods-changed", gameID)
	return nil
}

// LoversLabInstalledMod is one mod this app has tracked as installed from
// LoversLab for a game - see internal/loverslabtracking.Entry, which is what this
// is actually built from.
type LoversLabInstalledMod struct {
	FileID                int
	Title                 string
	FileURL               string
	InstalledAt           int64
	InstalledDateModified string
	// ContentMissing is true when this entry's own stub descriptor and content
	// folder can no longer be found on disk - the mod was removed by hand outside
	// this app, or the drive it lived on isn't connected. Shown so a person isn't
	// offered to "uninstall" something that's already gone.
	ContentMissing bool
}

// LoversLabInstalledMods lists every mod this app has tracked as installed from
// LoversLab for gameID, newest install first - the Browse tab's own "Installed"
// section, for reviewing and uninstalling them.
func (a *App) LoversLabInstalledMods(gameID string) ([]LoversLabInstalledMod, error) {
	installs, err := a.loverslabInstalls.Load(gameID)
	if err != nil {
		return nil, err
	}

	var modDir string
	if locations, locErr := a.NewModLocations(gameID); locErr == nil && len(locations) > 0 {
		modDir = locations[0].Path
	}

	out := make([]LoversLabInstalledMod, 0, len(installs))
	for _, e := range installs {
		missing := true
		if modDir != "" {
			if _, _, found := locateLoversLabInstall(modDir, e.FileID); found {
				missing = false
			}
		}
		out = append(out, LoversLabInstalledMod{
			FileID:                e.FileID,
			Title:                 e.Title,
			FileURL:               e.FileURL,
			InstalledAt:           e.InstalledAt,
			InstalledDateModified: e.InstalledDateModified,
			ContentMissing:        missing,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstalledAt > out[j].InstalledAt })
	return out, nil
}

// LoversLabInstall downloads every file in downloads (LoversLabDownloadDialog's own
// results - the person's own checked selection on the Files tab, one or several) and
// installs them together for gameID: extracted content first, then the stub that
// makes the game see it - see the package comment above for why, and
// resolveLoversLabInstallLocation for how an existing install of the same file is
// updated in place rather than duplicated. Every selected download is fetched and
// extracted into that one shared content folder in order - never wiped between them,
// only once up front when this replaces a previous install - so picking, say, a main
// archive together with an addon zip lands both inside the one mod folder, exactly as
// extracting them there by hand one after another would. dateModified is the file's
// own current "dateModified" (loverslab.FileDetail, already fetched by the detail
// view this button lives on - not re-fetched here, since the frontend already has
// it) - recorded for the update check to later compare against; an empty string just
// means this install won't be checked for updates until the next one, never a reason
// to fail the install itself. requestID tags the progress events this emits while it
// runs, and is what CancelLoversLabInstall stops.
func (a *App) LoversLabInstall(gameID, requestID string, file loverslab.FileSummary, dateModified string, downloads []loverslab.FileDownload) (SaveResult, error) {
	if len(downloads) == 0 {
		return SaveResult{}, errors.New("no files were selected to install")
	}

	modEditMu.Lock()
	defer modEditMu.Unlock()
	log := applog.For("LoversLab")

	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return SaveResult{}, err
	}

	contentDir, stubPath, isUpdate, err := a.resolveLoversLabInstallLocation(gameID, file.ID, file.Title)
	if err != nil {
		return SaveResult{}, err
	}

	ctx, cancel := context.WithCancel(a.baseContext())
	a.installMu.Lock()
	if a.installCancel == nil {
		a.installCancel = map[string]context.CancelFunc{}
	}
	a.installCancel[requestID] = cancel
	a.installMu.Unlock()
	defer func() {
		a.installMu.Lock()
		delete(a.installCancel, requestID)
		a.installMu.Unlock()
		cancel()
	}()

	endMute := a.watchMute.Begin(modWatchMuteGrace)
	defer endMute()

	// isUpdate: wipe the previous install's content before extracting any of the
	// newly selected files into it, so content the new selection no longer includes
	// doesn't linger - only now, after resolving where to install, never before any
	// of the selected downloads is even attempted.
	if isUpdate {
		if err := os.RemoveAll(contentDir); err != nil {
			return SaveResult{}, fmt.Errorf("could not clear the previous install: %w", err)
		}
	}

	var fileCount int
	var stubDescriptor []byte
	for i, dl := range downloads {
		tmp, err := os.CreateTemp("", "parallax-loverslab-*.download")
		if err != nil {
			_ = os.RemoveAll(contentDir)
			return SaveResult{}, fmt.Errorf("could not create a temporary file to download into: %w", err)
		}
		tmpPath := tmp.Name()

		_, downloadErr := client.DownloadFile(ctx, dl.URL, tmp, func(done, total int64) {
			a.emit("loverslab-install-progress", LoversLabInstallProgress{
				RequestID: requestID, Stage: "downloading", Done: done, Total: total,
				FileName: dl.Name, FileIndex: i + 1, FileCount: len(downloads),
			})
		})
		closeErr := tmp.Close()
		if downloadErr != nil {
			os.Remove(tmpPath)
			_ = os.RemoveAll(contentDir)
			if errors.Is(downloadErr, context.Canceled) {
				log.Infof("downloading '%s' for '%s' was cancelled", file.Title, a.gameLabel(gameID))
				return SaveResult{}, errors.New("The download was cancelled.")
			}
			log.Errorf("downloading '%s' for '%s' failed: %v", dl.Name, a.gameLabel(gameID), downloadErr)
			return SaveResult{}, downloadErr
		}
		if closeErr != nil {
			os.Remove(tmpPath)
			_ = os.RemoveAll(contentDir)
			return SaveResult{}, fmt.Errorf("could not finish writing the download: %w", closeErr)
		}
		if ctx.Err() != nil {
			os.Remove(tmpPath)
			_ = os.RemoveAll(contentDir)
			return SaveResult{}, errors.New("The download was cancelled.")
		}

		a.emit("loverslab-install-progress", LoversLabInstallProgress{
			RequestID: requestID, Stage: "extracting", Done: 0, Total: -1,
			FileName: dl.Name, FileIndex: i + 1, FileCount: len(downloads),
		})
		extracted, extractErr := loverslabinstall.ExtractZip(tmpPath, contentDir)
		os.Remove(tmpPath)
		if extractErr != nil {
			_ = os.RemoveAll(contentDir)
			log.Errorf("extracting '%s' for '%s' failed: %v", dl.Name, a.gameLabel(gameID), extractErr)
			return SaveResult{}, extractErr
		}
		fileCount += extracted.Files
		// Only the first selected download's own sibling stub (see
		// ExtractResult.StubDescriptor) is used - in practice that's always the
		// main archive, listed first; addon zips picked alongside it don't
		// normally carry their own descriptor at all.
		if stubDescriptor == nil && len(extracted.StubDescriptor) > 0 {
			stubDescriptor = extracted.StubDescriptor
		}
	}

	desc, hadOwnDescriptor, err := loverslabinstall.ResolveDescriptor(contentDir, stubDescriptor, file.Title, strconv.Itoa(file.ID))
	if err != nil {
		_ = os.RemoveAll(contentDir)
		return SaveResult{}, err
	}

	var written []string
	if !hadOwnDescriptor {
		descPath, writeErr := atomicfile.Write(contentDir, "descriptor.mod", mod.WriteClassicDescriptor(desc))
		if writeErr != nil {
			_ = os.RemoveAll(contentDir)
			log.Errorf("writing descriptor.mod for '%s' in '%s' failed: %v", file.Title, a.gameLabel(gameID), writeErr)
			return SaveResult{}, writeErr
		}
		written = append(written, descPath)
	}

	stubWritten, err := atomicfile.Write(filepath.Dir(stubPath), filepath.Base(stubPath), mod.WriteClassicDescriptor(desc))
	if err != nil {
		_ = os.RemoveAll(contentDir)
		log.Errorf("writing the stub for '%s' in '%s' failed: %v", file.Title, a.gameLabel(gameID), err)
		return SaveResult{}, err
	}
	written = append(written, stubWritten)

	modID := mod.LoversLabFilePrefix + strconv.Itoa(file.ID)
	installs, err := a.loverslabInstalls.Load(gameID)
	if err != nil {
		log.Warnf("could not read the LoversLab install tracking file, so this install was not recorded for update checks: %v", err)
	} else {
		installs, _ = loverslabtracking.With(installs, modID, loverslabtracking.Entry{
			FileURL:               file.URL,
			FileID:                file.ID,
			Title:                 file.Title,
			InstalledDateModified: dateModified,
			InstalledAt:           time.Now().Unix(),
		})
		if err := a.loverslabInstalls.Save(gameID, installs); err != nil {
			log.Warnf("could not save LoversLab install tracking for '%s': %v", file.Title, err)
		}
	}

	verb := "installed"
	if isUpdate {
		verb = "updated"
	}
	log.Infof("%s '%s' from LoversLab for '%s' (%d files across %d download(s))", verb, file.Title, a.gameLabel(gameID), fileCount, len(downloads))
	a.emit("mods-changed", gameID)
	return SaveResult{Files: written, SavedAt: time.Now().Unix()}, nil
}

// CancelLoversLabInstall stops the LoversLabInstall call tagged with requestID, if it
// is still running - a no-op otherwise (it may already be finished).
func (a *App) CancelLoversLabInstall(requestID string) {
	a.installMu.Lock()
	cancel := a.installCancel[requestID]
	a.installMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
