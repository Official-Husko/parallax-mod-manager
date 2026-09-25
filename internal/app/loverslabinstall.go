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

// locateLoversLabInstall finds modID's own stub descriptor for gameID, if one
// exists, and returns where its content actually lives on disk - the shared lookup
// resolveLoversLabInstallLocation (installing) and UninstallLoversLabMod (removing)
// both need: "is this mod already installed, and if so, where." found is false
// when there's no stub, or the stub exists but can't be read/parsed (never a reason
// to fail the caller outright - LoversLabInstall treats that as a fresh install,
// UninstallLoversLabMod treats it as "nothing to remove").
func locateLoversLabInstall(modDir, modID string) (contentDir, stubPath string, found bool) {
	stubPath = filepath.Join(modDir, modID+".mod")
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

// downloadBaseName strips a selected download's own file extension - e.g.
// "Tether_v1.2.zip" -> "Tether_v1.2" - used as a split-off mod's own default folder
// name/display name (see loversLabDownloadModID and LoversLabInstall) when more than
// one file is selected on the Files tab. Never empty: a download with no real Name at
// all (defensive - every real LoversLab download dialog entry has one) falls back to
// fallback instead of producing an unusable "_" folder.
func downloadBaseName(downloadName, fallback string) string {
	base := strings.TrimSuffix(downloadName, filepath.Ext(downloadName))
	if strings.TrimSpace(base) == "" {
		return fallback
	}
	return base
}

// loversLabDownloadModID builds the mod identity for one specific selected download,
// once more than one is being installed together (see LoversLabInstall) -
// mod.LoversLabFilePrefix+fileID alone is only ever used for the single-file case
// (unchanged from before this existed), since it can't tell two different downloads
// from the same page apart. Embedding the download's own (sanitized) name keeps this
// stable across reinstalls of that same file - same name in, same modID out - while
// still visibly grouping under the page's own fileID prefix.
func loversLabDownloadModID(fileID int, downloadName string) (string, error) {
	slug, err := modedit.FolderName(strings.TrimSpace(downloadBaseName(downloadName, strconv.Itoa(fileID))))
	if err != nil {
		return "", err
	}
	return mod.LoversLabFilePrefix + strconv.Itoa(fileID) + "-" + slug, nil
}

// resolveLoversLabInstallLocation works out where one mod identified by modID should
// be installed for gameID: if it's already installed, its own stub's declared path is
// reused, so a later download updates it in place rather than creating a second copy
// alongside it; otherwise a fresh folder name is picked from folderTitle,
// disambiguated with disambiguator in the rare case that collides with some unrelated
// mod's own folder name.
func (a *App) resolveLoversLabInstallLocation(gameID, modID, folderTitle, disambiguator string) (contentDir, stubPath string, isUpdate bool, err error) {
	locations, err := a.NewModLocations(gameID)
	if err != nil {
		return "", "", false, err
	}
	if len(locations) == 0 {
		return "", "", false, errors.New("no install location was found for this game")
	}
	modDir := locations[0].Path // NewModLocations always lists the game's own mod folder first, Default true

	if dir, stub, found := locateLoversLabInstall(modDir, modID); found {
		return dir, stub, true, nil
	}

	folderName, err := modedit.FolderName(strings.TrimSpace(folderTitle))
	if err != nil {
		return "", "", false, err
	}
	contentDir = filepath.Join(modDir, folderName)
	stubPath = filepath.Join(modDir, modID+".mod")
	if _, statErr := os.Stat(contentDir); statErr == nil {
		// An unrelated mod already has this exact folder name - not this mod's own
		// previous install (that was already handled above), just a naming
		// collision. Disambiguate rather than refusing outright.
		contentDir = filepath.Join(modDir, folderName+"-"+disambiguator)
	}
	return contentDir, stubPath, false, nil
}

// UninstallLoversLabMod removes one mod this app previously installed from LoversLab
// for gameID, identified by modID (LoversLabInstalledMod.ModID, the same identity
// LoversLabInstall itself uses - never just a fileID: since selecting several files at
// once installs each as its own separate mod, more than one can share a fileID):
// its content folder, its stub descriptor, and its entry in
// internal/loverslabtracking (so the update check stops looking for it) - mirrors
// LoversLabInstall's own conventions exactly (the shared modEditMu lock, muting the
// folder watcher while writing, emitting "mods-changed" once done). A mod that
// isn't actually installed returns a clear error rather than silently doing nothing.
func (a *App) UninstallLoversLabMod(gameID, modID string) error {
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

	contentDir, stubPath, found := locateLoversLabInstall(modDir, modID)
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
	// ModID is this entry's own real identity - internal/loverslabtracking's own map
	// key, and what UninstallLoversLabMod takes. Never assume it equals
	// "loverslab_<FileID>": selecting several files together on the Files tab installs
	// each as its own separate mod, so more than one entry can share a FileID (the
	// page they all came from) while each still has its own distinct ModID.
	ModID   string
	FileID  int
	Title   string
	FileURL string
	// ThumbnailURL is "" for anything installed before this field existed - see
	// loverslabtracking.Entry.ThumbnailURL, which this is read straight from.
	ThumbnailURL          string
	InstalledAt           int64
	InstalledDateModified string
	// ArchiveName/ArchivePosted/ContentDir are a permanent record of exactly what
	// was downloaded and where it went - see loverslabtracking.Entry's own fields of
	// the same name, which these are read straight from. Empty for anything
	// installed before they existed.
	ArchiveName   string
	ArchivePosted string
	ContentDir    string
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
	for modID, e := range installs {
		missing := true
		if modDir != "" {
			if _, _, found := locateLoversLabInstall(modDir, modID); found {
				missing = false
			}
		}
		out = append(out, LoversLabInstalledMod{
			ModID:                 modID,
			FileID:                e.FileID,
			Title:                 e.Title,
			FileURL:               e.FileURL,
			ThumbnailURL:          e.ThumbnailURL,
			InstalledAt:           e.InstalledAt,
			InstalledDateModified: e.InstalledDateModified,
			ArchiveName:           e.ArchiveName,
			ArchivePosted:         e.ArchivePosted,
			ContentDir:            e.ContentDir,
			ContentMissing:        missing,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstalledAt > out[j].InstalledAt })
	return out, nil
}

// loversLabInstallTarget is one mod LoversLabInstall will actually produce - a
// single-download install has exactly one, computed with the pre-split, unchanged
// identity (mod.LoversLabFilePrefix+fileID, folder name from the page's own title);
// selecting several files at once produces one target per file instead, each with its
// own identity derived from its own filename (see loversLabDownloadModID) - never
// merged, so each is its own separate mod: its own folder, its own descriptor, its
// own stub, separately uninstallable and separately tracked for updates.
type loversLabInstallTarget struct {
	dl            loverslab.FileDownload
	modID         string
	displayName   string
	disambiguator string
}

func loversLabInstallTargets(file loverslab.FileSummary, downloads []loverslab.FileDownload) ([]loversLabInstallTarget, error) {
	if len(downloads) == 1 {
		return []loversLabInstallTarget{{
			dl: downloads[0], modID: mod.LoversLabFilePrefix + strconv.Itoa(file.ID),
			displayName: file.Title, disambiguator: strconv.Itoa(file.ID),
		}}, nil
	}
	targets := make([]loversLabInstallTarget, len(downloads))
	for i, dl := range downloads {
		modID, err := loversLabDownloadModID(file.ID, dl.Name)
		if err != nil {
			return nil, err
		}
		targets[i] = loversLabInstallTarget{
			dl:            dl,
			modID:         modID,
			displayName:   downloadBaseName(dl.Name, fmt.Sprintf("%s %d", file.Title, i+1)),
			disambiguator: fmt.Sprintf("%d-%d", file.ID, i),
		}
	}
	return targets, nil
}

// LoversLabInstall downloads every file in downloads (LoversLabDownloadDialog's own
// results - the person's own checked selection on the Files tab, one or several) and
// installs them for gameID: extracted content first, then the stub that makes the
// game see it - see the package comment above for why, and
// resolveLoversLabInstallLocation for how an existing install of the same mod is
// updated in place rather than duplicated. Selecting exactly one file installs it the
// same way this always has; selecting several installs each as its own separate mod
// (see loversLabInstallTargets) - the whole call is still one all-or-nothing unit,
// though: if any one of several selected files fails partway through, every mod this
// same call already fully finished is rolled back too, not just the one that failed,
// the same "this batch either all lands or none of it does" guarantee a single-file
// install already gave. dateModified is the file's own current "dateModified"
// (loverslab.FileDetail, already fetched by the detail view this button lives on -
// not re-fetched here, since the frontend already has it) - recorded for the update
// check to later compare against; an empty string just means this install won't be
// checked for updates until the next one, never a reason to fail the install itself.
// requestID tags the progress events this emits while it runs, and is what
// CancelLoversLabInstall stops.
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

	targets, err := loversLabInstallTargets(file, downloads)
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

	installs, loadErr := a.loverslabInstalls.Load(gameID)
	if loadErr != nil {
		log.Warnf("could not read the LoversLab install tracking file before installing '%s': %v", file.Title, loadErr)
		installs = map[string]loverslabtracking.Entry{}
	}

	// doneDirs/doneStubs are every earlier target THIS call already fully finished -
	// rolled back alongside whichever one is currently failing, so a failure partway
	// through a multi-file selection never leaves some of the batch installed and the
	// rest missing.
	var doneDirs, doneStubs []string
	rollback := func(currentDir, currentStub string) {
		if currentDir != "" {
			_ = os.RemoveAll(currentDir)
		}
		if currentStub != "" {
			_ = os.Remove(currentStub)
		}
		for _, d := range doneDirs {
			_ = os.RemoveAll(d)
		}
		for _, s := range doneStubs {
			_ = os.Remove(s)
		}
	}

	var written []string
	anyUpdate := false
	totalFiles := 0
	for i, t := range targets {
		contentDir, stubPath, isUpdate, resolveErr := a.resolveLoversLabInstallLocation(gameID, t.modID, t.displayName, t.disambiguator)
		if resolveErr != nil {
			rollback("", "")
			return SaveResult{}, resolveErr
		}
		// isUpdate: wipe this one mod's own previous content before extracting the
		// newly selected download into it, so content it no longer ships doesn't
		// linger - only now, after resolving where to install, never before the
		// download is even attempted.
		if isUpdate {
			anyUpdate = true
			if err := os.RemoveAll(contentDir); err != nil {
				rollback("", "")
				return SaveResult{}, fmt.Errorf("could not clear the previous install: %w", err)
			}
		}

		tmp, tmpErr := os.CreateTemp("", "parallax-loverslab-*.download")
		if tmpErr != nil {
			rollback(contentDir, "")
			return SaveResult{}, fmt.Errorf("could not create a temporary file to download into: %w", tmpErr)
		}
		tmpPath := tmp.Name()

		_, downloadErr := client.DownloadFile(ctx, t.dl.URL, tmp, func(doneBytes, total int64) {
			a.emit("loverslab-install-progress", LoversLabInstallProgress{
				RequestID: requestID, Stage: "downloading", Done: doneBytes, Total: total,
				FileName: t.dl.Name, FileIndex: i + 1, FileCount: len(targets),
			})
		})
		closeErr := tmp.Close()
		if downloadErr != nil {
			os.Remove(tmpPath)
			rollback(contentDir, "")
			if errors.Is(downloadErr, context.Canceled) {
				log.Infof("downloading '%s' for '%s' was cancelled", file.Title, a.gameLabel(gameID))
				return SaveResult{}, errors.New("The download was cancelled.")
			}
			log.Errorf("downloading '%s' for '%s' failed: %v", t.dl.Name, a.gameLabel(gameID), downloadErr)
			return SaveResult{}, downloadErr
		}
		if closeErr != nil {
			os.Remove(tmpPath)
			rollback(contentDir, "")
			return SaveResult{}, fmt.Errorf("could not finish writing the download: %w", closeErr)
		}
		if ctx.Err() != nil {
			os.Remove(tmpPath)
			rollback(contentDir, "")
			return SaveResult{}, errors.New("The download was cancelled.")
		}

		a.emit("loverslab-install-progress", LoversLabInstallProgress{
			RequestID: requestID, Stage: "extracting", Done: 0, Total: -1,
			FileName: t.dl.Name, FileIndex: i + 1, FileCount: len(targets),
		})
		extracted, extractErr := loverslabinstall.ExtractZip(tmpPath, contentDir)
		os.Remove(tmpPath)
		if extractErr != nil {
			rollback(contentDir, "")
			log.Errorf("extracting '%s' for '%s' failed: %v", t.dl.Name, a.gameLabel(gameID), extractErr)
			return SaveResult{}, extractErr
		}
		totalFiles += extracted.Files

		desc, hadOwnDescriptor, descErr := loverslabinstall.ResolveDescriptor(contentDir, extracted.StubDescriptor, t.displayName, strconv.Itoa(file.ID))
		if descErr != nil {
			rollback(contentDir, "")
			return SaveResult{}, descErr
		}

		if !hadOwnDescriptor {
			descPath, writeErr := atomicfile.Write(contentDir, "descriptor.mod", mod.WriteClassicDescriptor(desc))
			if writeErr != nil {
				rollback(contentDir, "")
				log.Errorf("writing descriptor.mod for '%s' in '%s' failed: %v", t.displayName, a.gameLabel(gameID), writeErr)
				return SaveResult{}, writeErr
			}
			written = append(written, descPath)
		}

		stubWritten, stubErr := atomicfile.Write(filepath.Dir(stubPath), filepath.Base(stubPath), mod.WriteClassicDescriptor(desc))
		if stubErr != nil {
			rollback(contentDir, "")
			log.Errorf("writing the stub for '%s' in '%s' failed: %v", t.displayName, a.gameLabel(gameID), stubErr)
			return SaveResult{}, stubErr
		}
		written = append(written, stubWritten)
		doneDirs = append(doneDirs, contentDir)
		doneStubs = append(doneStubs, stubPath)

		installs, _ = loverslabtracking.With(installs, t.modID, loverslabtracking.Entry{
			FileURL:               file.URL,
			FileID:                file.ID,
			Title:                 t.displayName,
			ThumbnailURL:          file.ThumbnailURL,
			InstalledDateModified: dateModified,
			InstalledAt:           time.Now().Unix(),
			ArchiveName:           t.dl.Name,
			ArchivePosted:         t.dl.Posted,
			ContentDir:            contentDir,
		})
	}

	if err := a.loverslabInstalls.Save(gameID, installs); err != nil {
		log.Warnf("could not save LoversLab install tracking for '%s': %v", file.Title, err)
	}

	verb := "installed"
	if anyUpdate {
		verb = "updated"
	}
	log.Infof("%s %d mod(s) from '%s' on LoversLab for '%s' (%d files total)", verb, len(targets), file.Title, a.gameLabel(gameID), totalFiles)
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
