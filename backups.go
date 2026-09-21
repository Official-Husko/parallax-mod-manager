package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/browser"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sync/errgroup"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/backup"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modupdates"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// How often the Workshop is asked again, while the app is open, whether mods have
// been deleted: Steam removes a deleted mod's files on its own schedule, so the
// sooner the app finds out the more likely the files are still there to copy.
const backupRecheckInterval = 30 * time.Minute

// A backup that failed is not retried by the automatic checks for this long (a full
// disk stays full; the person is told once).
const backupRetryAfter = 30 * time.Minute

// backupState is mod preservation: the settings, what is being copied and what is
// known about each game's Workshop mods.
type backupState struct {
	// mu guards everything below.
	mu       sync.Mutex
	store    backup.Store
	settings backup.Settings
	// running holds a cancel function per game with copies in progress; inflight the
	// Workshop items being copied right now, so two paths never copy the same one.
	running  map[string]context.CancelFunc
	inflight map[string]bool
	// failedAt remembers when a copy of an item last failed.
	failedAt map[string]time.Time
	// states is what was last worked out for each game's Workshop mods (by game id);
	// filesGone says which flagged mods no longer have files on disk to copy.
	states    map[string][]WorkshopAvailability
	filesGone map[string]map[string]bool
	// blocked remembers, per Workshop item, that a copy was held back by a limit ("full":
	// the size cap, "lowspace": the free-space guard). limitState is the same for the
	// whole run ("size", "space" or ""), and is what the interface is told about once.
	blocked    map[string]string
	limitState string
	// copyMu serialises the copies themselves, so the limits are checked against what is
	// really there and two games' copies never race each other for the same space.
	copyMu sync.Mutex
	// defaultRoot is the backup folder used until the user picks one: inside the app's
	// settings folder (empty when that could not be found).
	defaultRoot string
	// ctx stops everything at shutdown.
	ctx    context.Context
	cancel context.CancelFunc
}

// rootLocked is the folder backups go in now. Callers hold mu.
func (b *backupState) rootLocked() string { return b.settings.Root(b.defaultRoot) }

// initBackups loads the backup settings and starts the background re-check. dir is the
// app's config folder ("" when it could not be found: the defaults are used and
// nothing is saved).
func (a *App) initBackups(dir string) {
	b := &a.backup
	b.mu.Lock()
	b.running = map[string]context.CancelFunc{}
	b.inflight = map[string]bool{}
	b.failedAt = map[string]time.Time{}
	b.blocked = map[string]string{}
	b.states = map[string][]WorkshopAvailability{}
	b.filesGone = map[string]map[string]bool{}
	b.defaultRoot = backup.DefaultRoot(dir)
	if dir != "" {
		b.store = backup.Store{Path: filepath.Join(dir, backup.SettingsFile)}
	}
	settings, err := b.store.Load()
	b.settings = settings
	b.ctx, b.cancel = context.WithCancel(context.Background())
	ctx := b.ctx
	root := b.rootLocked()
	b.mu.Unlock()

	log := applog.For("Backup")
	if err != nil {
		log.Warnf("the backup settings could not be read, so the defaults are used: %v", err)
	}
	log.Infof("mod backups: %s, folder '%s'", settings.Mode, root)

	if a.ctx != nil {
		go a.backupWatchdog(ctx)
	}
}

// stopBackups cancels copies in progress and the background re-check.
func (a *App) stopBackups() {
	a.backup.mu.Lock()
	cancel := a.backup.cancel
	a.backup.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// backupWatchdog asks the Workshop again every backupRecheckInterval, for the games
// looked at this run, so a mod deleted while the app is open is noticed - and copied -
// while Steam may not yet have removed it.
func (a *App) backupWatchdog(ctx context.Context) {
	ticker := time.NewTicker(backupRecheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		a.backup.mu.Lock()
		mode := a.backup.settings.Mode
		var games []string
		for id := range a.backup.states {
			games = append(games, id)
		}
		a.backup.mu.Unlock()
		if mode == backup.ModeOff {
			continue
		}
		for _, id := range games {
			a.recheckWorkshop(ctx, id)
		}
	}
}

// recheckWorkshop asks Steam again about gameID's Workshop mods, backs up any that are
// now deleted or private, and tells the interface when what it shows is out of date.
func (a *App) recheckWorkshop(ctx context.Context, gameID string) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return
	}
	details, err := a.workshopDetails.GetFresh(ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		applog.For("Backup").Warnf("couldn't check the Workshop for deleted '%s' mods: %v", cfg.DisplayName, err)
		return
	}
	result := classifyWorkshop(details, a.confirmWorkshopPages(details, true))

	a.backup.mu.Lock()
	previous := a.backup.states[gameID]
	a.backup.mu.Unlock()
	a.preserveWorkshopMods(gameID, result)
	if !sameAvailability(previous, result) {
		applog.For("Backup").Infof("'%s': the Workshop status of some mods changed", cfg.DisplayName)
		a.emit("workshop-availability-changed", gameID)
	}
}

func sameAvailability(a, b []WorkshopAvailability) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].RemoteFileID != b[i].RemoteFileID || a[i].State != b[i].State || a[i].Reason != b[i].Reason {
			return false
		}
	}
	return true
}

// backupJob is one mod to copy.
type backupJob struct {
	src    backup.Source
	reason string
}

// preserveWorkshopMods is what runs whenever the Workshop status of a game's mods has
// been worked out: it remembers the result, and - unless backups are off - starts
// copying every installed Workshop mod that needs it while its files are still there.
func (a *App) preserveWorkshopMods(gameID string, states []WorkshopAvailability) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return
	}
	b := &a.backup
	b.mu.Lock()
	if b.states == nil { // not started (tests)
		b.mu.Unlock()
		return
	}
	b.states[gameID] = states
	mode := b.settings.Mode
	root := b.rootLocked()
	b.mu.Unlock()

	byItem := make(map[string]string, len(states))
	for _, s := range states {
		byItem[s.RemoteFileID] = s.State
	}

	scanResult, err := scan.Scan(a.baseContext(), scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		return
	}
	gone := map[string]bool{}
	var jobs []backupJob
	for _, m := range scanResult.Mods {
		if m.Source != mod.SourceWorkshop || m.Descriptor.RemoteFileID == "" {
			continue
		}
		reason := ""
		switch byItem[m.Descriptor.RemoteFileID] {
		case string(steamapi.AvailabilityDeleted):
			reason = backup.ReasonDeleted
		case string(steamapi.AvailabilityPrivate):
			reason = backup.ReasonPrivate
		default:
			if mode == backup.ModeAll {
				reason = backup.ReasonAll
			}
		}
		if reason == "" {
			continue
		}
		if m.ContentMissing {
			// Flagged, and Steam has already taken the files: nothing left to copy.
			if reason != backup.ReasonAll {
				gone[m.Descriptor.RemoteFileID] = true
			}
			continue
		}
		jobs = append(jobs, backupJob{reason: reason, src: backup.Source{
			GameID:       gameID,
			ModID:        m.ID,
			RemoteFileID: m.Descriptor.RemoteFileID,
			Name:         m.Descriptor.Name,
			Version:      m.Descriptor.Version,
			ContentPath:  m.ContentPath,
			Reason:       reason,
		}})
	}
	b.mu.Lock()
	b.filesGone[gameID] = gone
	b.mu.Unlock()

	if mode == backup.ModeOff || len(jobs) == 0 || root == "" {
		return
	}
	// What is deleted or private goes first, so a size cap or a full disk is never spent
	// on mods that are in no danger while one that is waits.
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].reason != backup.ReasonAll && jobs[j].reason == backup.ReasonAll })
	a.startBackupJobs(gameID, cfg.DisplayName, root, jobs)
}

// startBackupJobs copies jobs one after another in the background, unless copies for
// this game are already running (the next check picks up whatever was missed).
func (a *App) startBackupJobs(gameID, gameName, root string, jobs []backupJob) {
	b := &a.backup
	b.mu.Lock()
	if _, busy := b.running[gameID]; busy {
		b.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(b.ctx)
	b.running[gameID] = cancel
	limits := b.settings.Limits()
	var todo []backupJob
	for _, j := range jobs {
		if t, failed := b.failedAt[j.src.RemoteFileID]; failed && time.Since(t) < backupRetryAfter {
			continue
		}
		if b.inflight[j.src.RemoteFileID] {
			continue
		}
		todo = append(todo, j)
	}
	b.mu.Unlock()
	if len(todo) == 0 {
		a.finishBackupRun(gameID)
		return
	}
	go func() {
		defer a.finishBackupRun(gameID)
		a.runBackupJobs(ctx, gameName, root, todo, limits)
	}()
}

func (a *App) finishBackupRun(gameID string) {
	a.backup.mu.Lock()
	if cancel, ok := a.backup.running[gameID]; ok {
		cancel()
		delete(a.backup.running, gameID)
	}
	a.backup.mu.Unlock()
}

// BackupResult is the outcome of one copy, sent to the interface as the "backup-done"
// event so it can say so.
type BackupResult struct {
	GameID       string
	ModID        string
	RemoteFileID string
	Name         string
	// Reason is why it was copied: "deleted", "private", "all" or "manual".
	Reason string
	// Status is "ok", "incomplete" or "failed".
	Status  string
	Message string
	Size    int64
}

func (a *App) runBackupJobs(ctx context.Context, gameName, root string, jobs []backupJob, limits backup.Limits) {
	copied, waiting := 0, 0
	blockedKind := ""
	for i, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		if blockedKind != "" {
			// A limit stopped the run: the rest wait, they are not tried one by one.
			a.markBlocked(j.src.RemoteFileID, blockedKind)
			waiting++
			continue
		}
		res, err := a.backupOne(ctx, root, j.src, gameName, limits, func() {
			a.emit("backup-progress", map[string]any{"GameID": j.src.GameID, "Done": i, "Total": len(jobs), "Name": j.src.Name})
		})
		switch {
		case errors.Is(err, backup.ErrOverLimit):
			blockedKind = "full"
		case errors.Is(err, backup.ErrLowSpace):
			blockedKind = "lowspace"
		case err == nil && !res.Skipped:
			copied++
		}
		if blockedKind != "" {
			a.markBlocked(j.src.RemoteFileID, blockedKind)
			waiting++
		}
	}
	if blockedKind != "" {
		a.noteLimit(blockedKind, waiting)
	} else {
		a.clearLimit()
	}
	if copied > 0 {
		a.emit("backups-changed")
	}
}

func (a *App) markBlocked(remoteFileID, kind string) {
	a.backup.mu.Lock()
	a.backup.blocked[remoteFileID] = kind
	a.backup.mu.Unlock()
}

// clearLimit forgets a limit that no longer holds back anything.
func (a *App) clearLimit() {
	b := &a.backup
	b.mu.Lock()
	b.limitState = ""
	b.blocked = map[string]string{}
	b.mu.Unlock()
}

// BackupLimitNotice is the "backup-limit" event: backups have stopped because of a limit
// the user set. It is sent once each time the state changes, not on every check.
type BackupLimitNotice struct {
	// Kind is "size" (the size cap is reached) or "space" (the free-space guard).
	Kind string
	// Message says it in words, with the numbers.
	Message string
	// Waiting is how many mods are waiting to be backed up.
	Waiting int
}

// noteLimit records that a limit stopped the run and, when that is news, tells the
// interface.
func (a *App) noteLimit(kind string, waiting int) {
	state := "size"
	if kind == "lowspace" {
		state = "space"
	}
	b := &a.backup
	b.mu.Lock()
	previous := b.limitState
	b.limitState = state
	settings := b.settings
	root := b.rootLocked()
	b.mu.Unlock()
	if previous == state {
		return
	}

	mods := fmt.Sprintf("%d mods are", waiting)
	if waiting == 1 {
		mods = "1 mod is"
	}
	var msg string
	if state == "size" {
		msg = fmt.Sprintf("Mod backups are paused: they have reached the %s size limit (%s used). %s waiting to be backed up.",
			humanSize(settings.LimitBytes), humanSize(backup.TotalSize(root)), capitalise(mods))
	} else {
		free := int64(0)
		if f, ok := backup.FreeSpace(root); ok {
			free = int64(f)
		}
		msg = fmt.Sprintf("Mod backups are paused: only %s is free on the drive holding the backup folder, and you asked to keep %s free. %s waiting to be backed up.",
			humanSize(free), humanSize(settings.KeepFreeBytes), capitalise(mods))
	}
	applog.For("Backup").Warnf("%s", msg)
	a.emit("backup-limit", BackupLimitNotice{Kind: state, Message: msg, Waiting: waiting})
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// backupOne copies one mod and reports how it went (log, and "backup-done" for the
// interface). The error is nil for a copy made or skipped; a limit that held it back
// is backup.ErrOverLimit or backup.ErrLowSpace, which the caller deals with (they are
// not failures to announce one by one).
func (a *App) backupOne(ctx context.Context, root string, src backup.Source, gameName string, limits backup.Limits, started func()) (backup.Result, error) {
	b := &a.backup
	b.mu.Lock()
	if b.inflight[src.RemoteFileID] {
		b.mu.Unlock()
		return backup.Result{Skipped: true}, nil
	}
	b.inflight[src.RemoteFileID] = true
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.inflight, src.RemoteFileID)
		b.mu.Unlock()
	}()

	log := applog.For("Backup")
	timer := log.Begin()
	var startedOnce sync.Once
	b.copyMu.Lock()
	res, err := backup.CopyWithLimits(ctx, root, src, func(done, total int64) {
		if started != nil {
			startedOnce.Do(started)
		}
	}, limits)
	b.copyMu.Unlock()
	out := BackupResult{GameID: src.GameID, ModID: src.ModID, RemoteFileID: src.RemoteFileID, Name: src.Name, Reason: src.Reason}
	switch {
	case err == nil && res.Skipped:
		return res, nil
	case err == nil && !res.Entry.Complete:
		out.Status, out.Size = "incomplete", res.Entry.Size
		out.Message = fmt.Sprintf("only part of it could be saved: %d files were removed while copying", res.Entry.Missing)
		timer.Warnf("'%s' (%s): %s", src.Name, gameName, out.Message)
	case err == nil:
		out.Status, out.Size = "ok", res.Entry.Size
		timer.Infof("'%s' (%s) backed up: %d files, %s (%s)", src.Name, gameName, res.Entry.Files, humanSize(res.Entry.Size), backupReasonText(src.Reason))
	case errors.Is(err, context.Canceled):
		return res, err
	case errors.Is(err, backup.ErrOverLimit), errors.Is(err, backup.ErrLowSpace):
		timer.Warnf("'%s' (%s) was not backed up: %v", src.Name, gameName, err)
		return res, err
	default:
		out.Status, out.Message = "failed", err.Error()
		timer.Warnf("couldn't back up '%s' (%s): %v", src.Name, gameName, err)
		b.mu.Lock()
		b.failedAt[src.RemoteFileID] = time.Now()
		b.mu.Unlock()
		a.emit("backup-done", out)
		return res, err
	}
	b.mu.Lock()
	delete(b.failedAt, src.RemoteFileID)
	delete(b.blocked, src.RemoteFileID)
	b.mu.Unlock()
	a.emit("backup-done", out)
	return res, nil
}

func backupReasonText(reason string) string {
	switch reason {
	case backup.ReasonDeleted:
		return "deleted from the Workshop"
	case backup.ReasonPrivate:
		return "made private"
	case backup.ReasonAll:
		return "every-mod backups"
	}
	return "on request"
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// attachBackupState fills in, on the flagged Workshop mods, whether each has a backup.
func (a *App) attachBackupState(gameID string, states []WorkshopAvailability) {
	b := &a.backup
	b.mu.Lock()
	mode := b.settings.Mode
	root := b.rootLocked()
	gone := b.filesGone[gameID]
	failed := map[string]bool{}
	for id, t := range b.failedAt {
		if time.Since(t) < backupRetryAfter {
			failed[id] = true
		}
	}
	blocked := map[string]string{}
	for id, kind := range b.blocked {
		blocked[id] = kind
	}
	b.mu.Unlock()

	for i := range states {
		s := &states[i]
		if e, ok := backup.Lookup(root, gameID, s.RemoteFileID); ok {
			s.BackedUpAt = e.BackedUpAt
			s.BackupState = "done"
			if !e.Complete {
				s.BackupState = "incomplete"
			}
			continue
		}
		atRisk := s.State == string(steamapi.AvailabilityDeleted) || s.State == string(steamapi.AvailabilityPrivate)
		switch {
		case !atRisk:
		case gone[s.RemoteFileID]:
			s.BackupState = "gone"
		case blocked[s.RemoteFileID] != "":
			s.BackupState = blocked[s.RemoteFileID] // "full" or "lowspace"
		case failed[s.RemoteFileID]:
			s.BackupState = "failed"
		case mode == backup.ModeOff:
			s.BackupState = "off"
		default:
			s.BackupState = "pending"
		}
	}
}

// --- Settings, for Settings > Backup ---------------------------------------

// BackupStatus is the backup settings as the panel shows them.
type BackupStatus struct {
	// Mode is "atrisk", "all" or "off".
	Mode string
	// CustomPath is the folder the user picked, empty for the default.
	CustomPath string
	// Root is the folder backups actually go in; DefaultRoot is the default one.
	Root        string
	DefaultRoot string
	// FreeBytes is the space left on the volume holding Root, 0 when unknown.
	FreeBytes int64
	// Running is true while copies are in progress.
	Running bool
	// LimitEnabled and LimitBytes are the cap on everything the backups take; KeepFreeEnabled
	// and KeepFreeBytes the free space that is left alone.
	LimitEnabled    bool
	LimitBytes      int64
	KeepFreeEnabled bool
	KeepFreeBytes   int64
	// UsedBytes is what all the backups take now, as recorded.
	UsedBytes int64
	// LimitState is "size" or "space" while a limit is holding backups back, "" otherwise.
	LimitState string
}

// BackupStatus reports the backup settings.
func (a *App) BackupStatus() BackupStatus {
	b := &a.backup
	b.mu.Lock()
	settings := b.settings
	root, defaultRoot := b.rootLocked(), b.defaultRoot
	running := len(b.running) > 0
	limitState := b.limitState
	b.mu.Unlock()
	st := BackupStatus{
		Mode: string(settings.Mode), CustomPath: settings.Path, Root: root, DefaultRoot: defaultRoot, Running: running,
		LimitEnabled: settings.LimitEnabled, LimitBytes: settings.LimitBytes, KeepFreeEnabled: settings.KeepFreeEnabled, KeepFreeBytes: settings.KeepFreeBytes,
		UsedBytes: backup.TotalSize(root), LimitState: limitState,
	}
	if st.Mode == "" {
		st.Mode = string(backup.ModeAtRisk)
	}
	if free, ok := backup.FreeSpace(st.Root); ok {
		st.FreeBytes = int64(free)
	}
	return st
}

// saveBackupSettings stores next and applies it: a change in what is backed up is
// acted on at once for the games looked at this run.
func (a *App) saveBackupSettings(next backup.Settings) error {
	b := &a.backup
	b.mu.Lock()
	if b.store.Path != "" {
		if err := b.store.Save(next); err != nil {
			b.mu.Unlock()
			return err
		}
	}
	b.settings = next
	// A changed setting deserves a fresh look: what a limit held back is tried again.
	b.limitState = ""
	b.blocked = map[string]string{}
	var cancels []context.CancelFunc
	if next.Mode == backup.ModeOff {
		for _, c := range b.running {
			cancels = append(cancels, c)
		}
	}
	games := map[string][]WorkshopAvailability{}
	for id, s := range b.states {
		games[id] = s
	}
	b.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	if next.Mode != backup.ModeOff {
		for id, s := range games {
			a.preserveWorkshopMods(id, s)
		}
	}
	a.emit("backups-changed")
	return nil
}

// SetBackupMode chooses which mods are backed up on their own: "atrisk", "all" or
// "off".
func (a *App) SetBackupMode(mode string) (BackupStatus, error) {
	m := backup.ParseMode(mode)
	a.backup.mu.Lock()
	next := a.backup.settings
	a.backup.mu.Unlock()
	next.Mode = m
	if err := a.saveBackupSettings(next); err != nil {
		return a.BackupStatus(), err
	}
	applog.For("Backup").Infof("mod backups set to %s", m)
	return a.BackupStatus(), nil
}

// SetBackupLimits sets the two rules a backup is held to: a cap on everything the backups
// take together, and the free space on the drive that is left alone. Each can be switched
// off; the sizes are in bytes. Backups a limit had held back are tried again at once.
func (a *App) SetBackupLimits(limitEnabled bool, limitBytes int64, keepFreeEnabled bool, keepFreeBytes int64) (BackupStatus, error) {
	if limitEnabled && limitBytes < backup.MinLimitBytes {
		return a.BackupStatus(), errors.New("the size limit must be at least 1 MB")
	}
	if keepFreeBytes < 0 {
		return a.BackupStatus(), errors.New("the free space to keep cannot be negative")
	}
	a.backup.mu.Lock()
	next := a.backup.settings
	a.backup.mu.Unlock()
	next.LimitEnabled, next.KeepFreeEnabled = limitEnabled, keepFreeEnabled
	if limitBytes >= backup.MinLimitBytes {
		next.LimitBytes = limitBytes
	}
	next.KeepFreeBytes = keepFreeBytes
	if err := a.saveBackupSettings(next); err != nil {
		return a.BackupStatus(), err
	}
	applog.For("Backup").Infof("backup limits: size cap %s, keep %s free", onOff(next.LimitEnabled, humanSize(next.LimitBytes)), onOff(next.KeepFreeEnabled, humanSize(next.KeepFreeBytes)))
	return a.BackupStatus(), nil
}

func onOff(on bool, value string) string {
	if !on {
		return "off"
	}
	return value
}

// SetBackupFolder chooses where backups are kept; "" goes back to the default folder.
// Backups already made stay where they are.
func (a *App) SetBackupFolder(path string) (BackupStatus, error) {
	a.backup.mu.Lock()
	next := a.backup.settings
	defaultRoot := a.backup.defaultRoot
	a.backup.mu.Unlock()
	next.Path = path
	root := next.Root(defaultRoot)
	if root == "" {
		return a.BackupStatus(), errors.New("the app's settings folder could not be found, so there is no default backup folder: choose one")
	}
	if err := backup.ValidateRoot(root); err != nil {
		return a.BackupStatus(), err
	}
	if err := a.saveBackupSettings(next); err != nil {
		return a.BackupStatus(), err
	}
	applog.For("Backup").Infof("backup folder set to '%s'", root)
	return a.BackupStatus(), nil
}

// BrowseForBackupFolder opens a folder picker and, if a folder is chosen, makes it the
// backup folder. A cancelled dialog changes nothing.
func (a *App) BrowseForBackupFolder() (BackupStatus, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select the folder to keep mod backups in",
	})
	if err != nil {
		return a.BackupStatus(), err
	}
	if dir == "" {
		return a.BackupStatus(), nil
	}
	return a.SetBackupFolder(dir)
}

// BackupOverview is what the panel says about a game's mods and backups.
type BackupOverview struct {
	// WorkshopMods and WorkshopBytes are the installed Workshop mods with files on
	// disk: what "every mod" mode would keep a copy of.
	WorkshopMods  int
	WorkshopBytes int64
	// AtRiskMods counts installed Workshop mods now known to be deleted or private.
	AtRiskMods int
	// BackedUpMods and BackedUpBytes are what is in the backup folder for this game.
	BackedUpMods  int
	BackedUpBytes int64
}

// BackupOverview counts gameID's installed Workshop mods and its backups.
func (a *App) BackupOverview(gameID string) (BackupOverview, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return BackupOverview{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	scanResult, err := scan.Scan(a.baseContext(), scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		return BackupOverview{}, err
	}
	var paths []string
	for _, m := range scanResult.Mods {
		if m.Source == mod.SourceWorkshop && m.Descriptor.RemoteFileID != "" && !m.ContentMissing {
			paths = append(paths, m.ContentPath)
		}
	}
	sizes := make([]int64, len(paths))
	g := errgroup.Group{}
	g.SetLimit(8)
	for i, p := range paths {
		g.Go(func() error {
			sizes[i] = modupdates.FingerprintDir(p).Size
			return nil
		})
	}
	_ = g.Wait()

	out := BackupOverview{WorkshopMods: len(paths)}
	for _, s := range sizes {
		out.WorkshopBytes += s
	}
	b := &a.backup
	b.mu.Lock()
	states := b.states[gameID]
	gone := b.filesGone[gameID]
	root := b.rootLocked()
	b.mu.Unlock()
	for _, s := range states {
		if (s.State == string(steamapi.AvailabilityDeleted) || s.State == string(steamapi.AvailabilityPrivate)) && !gone[s.RemoteFileID] {
			out.AtRiskMods++
		}
	}
	for _, e := range backup.List(root, gameID) {
		out.BackedUpMods++
		out.BackedUpBytes += e.Size
	}
	return out, nil
}

// ListBackups returns gameID's backups, newest first.
func (a *App) ListBackups(gameID string) ([]backup.Entry, error) {
	if _, ok := a.registry.Get(gameID); !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	a.backup.mu.Lock()
	root := a.backup.rootLocked()
	a.backup.mu.Unlock()
	entries := backup.List(root, gameID)
	if entries == nil {
		entries = []backup.Entry{}
	}
	return entries, nil
}

// OpenBackupFolder opens gameID's backup folder in the file manager.
func (a *App) OpenBackupFolder(gameID string) error {
	if _, ok := a.registry.Get(gameID); !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	a.backup.mu.Lock()
	root := a.backup.rootLocked()
	a.backup.mu.Unlock()
	if root == "" {
		return errors.New("there is no backup folder")
	}
	dir := backup.GameFolder(root, gameID)
	if err := backup.ValidateRoot(dir); err != nil {
		return err
	}
	if err := browser.OpenFile(dir); err != nil {
		applog.For("Files").Warnf("couldn't open '%s': %v", dir, err)
		return err
	}
	return nil
}

// BackupMod backs one Workshop mod up now, whatever the automatic mode is - for a mod
// the person wants kept safe. It waits for the copy to finish and says whether a copy
// was "saved" or an unchanged one was already there ("current").
func (a *App) BackupMod(gameID, modID string) (string, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return "", fmt.Errorf("app: unknown game %q", gameID)
	}
	scanResult, err := scan.Scan(a.baseContext(), scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		return "", err
	}
	for _, m := range scanResult.Mods {
		if m.ID != modID {
			continue
		}
		if m.Source != mod.SourceWorkshop || m.Descriptor.RemoteFileID == "" {
			return "", errors.New("only Steam Workshop mods can be backed up")
		}
		if m.ContentMissing {
			return "", errors.New("this mod's files are not on disk, so there is nothing to copy")
		}
		a.backup.mu.Lock()
		root := a.backup.rootLocked()
		a.backup.mu.Unlock()
		if root == "" {
			return "", errors.New("there is no backup folder")
		}
		src := backup.Source{GameID: gameID, ModID: m.ID, RemoteFileID: m.Descriptor.RemoteFileID, Name: m.Descriptor.Name, Version: m.Descriptor.Version, ContentPath: m.ContentPath, Reason: backup.ReasonManual}
		a.backup.mu.Lock()
		limits := a.backup.settings.Limits()
		a.backup.mu.Unlock()
		res, err := a.backupOne(a.baseContext(), root, src, cfg.DisplayName, limits, nil)
		switch {
		case errors.Is(err, backup.ErrOverLimit):
			return "", errors.New("the backup size limit would be exceeded, so nothing was copied. Raise the limit or delete backups under Settings > Backup")
		case errors.Is(err, backup.ErrLowSpace):
			return "", errors.New("the drive would be left with less free space than you asked to keep, so nothing was copied. Free some space or change that setting under Settings > Backup")
		case errors.Is(err, context.Canceled):
			return "", errors.New("the backup was cancelled")
		case err != nil:
			return "", errors.New("the backup failed; the activity log says why")
		}
		if res.Skipped {
			return "current", nil
		}
		a.emit("backups-changed")
		return "saved", nil
	}
	return "", fmt.Errorf("app: unknown mod %q", modID)
}
