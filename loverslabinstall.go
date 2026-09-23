package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	stubPath = filepath.Join(modDir, mod.LoversLabFilePrefix+strconv.Itoa(fileID)+".mod")
	if data, readErr := os.ReadFile(stubPath); readErr == nil {
		desc, parseErr := mod.ParseDescriptor(data, mod.DescriptorClassic)
		if parseErr == nil && desc.Path != "" {
			dir := desc.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(modDir, dir)
			}
			return dir, stubPath, true, nil
		}
	}

	folderName, err := modedit.FolderName(strings.TrimSpace(title))
	if err != nil {
		return "", "", false, err
	}
	contentDir = filepath.Join(modDir, folderName)
	if _, statErr := os.Stat(contentDir); statErr == nil {
		// An unrelated mod already has this exact folder name - not this file's own
		// previous install (that was already handled above), just a naming
		// collision. Disambiguate with the file id rather than refusing outright.
		contentDir = filepath.Join(modDir, folderName+"-"+strconv.Itoa(fileID))
	}
	return contentDir, stubPath, false, nil
}

// LoversLabInstall downloads file from downloadURL (one of LoversLabDownloadDialog's
// own results) and installs it for gameID: extracted content first, then the stub
// that makes the game see it - see the package comment above for why, and
// resolveLoversLabInstallLocation for how an existing install of the same file is
// updated in place rather than duplicated. requestID tags the progress events this
// emits while it runs, and is what CancelLoversLabInstall stops.
func (a *App) LoversLabInstall(gameID, requestID string, file loverslab.FileSummary, downloadURL string) (SaveResult, error) {
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

	tmp, err := os.CreateTemp("", "parallax-loverslab-*.download")
	if err != nil {
		return SaveResult{}, fmt.Errorf("could not create a temporary file to download into: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	_, downloadErr := client.DownloadFile(ctx, downloadURL, tmp, func(done, total int64) {
		a.emit("loverslab-install-progress", LoversLabInstallProgress{RequestID: requestID, Stage: "downloading", Done: done, Total: total})
	})
	closeErr := tmp.Close()
	if downloadErr != nil {
		if errors.Is(downloadErr, context.Canceled) {
			log.Infof("downloading '%s' for '%s' was cancelled", file.Title, a.gameLabel(gameID))
			return SaveResult{}, errors.New("The download was cancelled.")
		}
		log.Errorf("downloading '%s' for '%s' failed: %v", file.Title, a.gameLabel(gameID), downloadErr)
		return SaveResult{}, downloadErr
	}
	if closeErr != nil {
		return SaveResult{}, fmt.Errorf("could not finish writing the download: %w", closeErr)
	}
	if ctx.Err() != nil {
		return SaveResult{}, errors.New("The download was cancelled.")
	}

	// isUpdate: wipe the previous install's content before re-extracting, so a file
	// the new version no longer ships doesn't linger - only now, after the download
	// is confirmed good, never before (a failed download must never touch what was
	// already installed).
	if isUpdate {
		if err := os.RemoveAll(contentDir); err != nil {
			return SaveResult{}, fmt.Errorf("could not clear the previous install: %w", err)
		}
	}

	a.emit("loverslab-install-progress", LoversLabInstallProgress{RequestID: requestID, Stage: "extracting", Done: 0, Total: -1})
	extracted, extractErr := loverslabinstall.ExtractZip(tmpPath, contentDir)
	if extractErr != nil {
		_ = os.RemoveAll(contentDir)
		log.Errorf("extracting '%s' for '%s' failed: %v", file.Title, a.gameLabel(gameID), extractErr)
		return SaveResult{}, extractErr
	}
	fileCount := extracted.Files

	desc, hadOwnDescriptor, err := loverslabinstall.ResolveDescriptor(contentDir, extracted.StubDescriptor, file.Title, strconv.Itoa(file.ID))
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
			FileURL:          file.URL,
			FileID:           file.ID,
			Title:            file.Title,
			InstalledUpdated: file.Updated,
			InstalledAt:      time.Now().Unix(),
		})
		if err := a.loverslabInstalls.Save(gameID, installs); err != nil {
			log.Warnf("could not save LoversLab install tracking for '%s': %v", file.Title, err)
		}
	}

	verb := "installed"
	if isUpdate {
		verb = "updated"
	}
	log.Infof("%s '%s' from LoversLab for '%s' (%d files)", verb, file.Title, a.gameLabel(gameID), fileCount)
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
