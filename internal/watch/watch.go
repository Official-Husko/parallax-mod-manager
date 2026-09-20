// Package watch provides a debounced, non-recursive watch on a single
// directory, used to notice mods being added, removed, or updated in a
// game's mod folder while the app is open.
package watch

import (
	"errors"
	"sync"
	"time"

	"github.com/bep/debounce"
	"github.com/fsnotify/fsnotify"
)

// FolderWatcher watches one directory non-recursively for entries being
// added, removed, or rewritten in place, debouncing bursts of events (a
// bulk copy or archive extract fires many fsnotify events for one logical
// change) into a single onChange call after the directory goes quiet. Mods
// live as immediate children of a game's mod folder (a flat descriptor
// file per mod - see internal/scan), so watching just that one directory,
// not its subfolders, is deliberately the right scope: it catches a mod's
// descriptor appearing, disappearing, or being rewritten (a Workshop
// version bump touches the descriptor too), without turning into a full
// recursive watch of every file inside every mod's own content folder,
// which nothing here needs.
type FolderWatcher struct {
	watcher   *fsnotify.Watcher
	done      chan struct{}
	closeOnce sync.Once
}

// New starts watching dir. onChange is called from a background goroutine
// after events settle for the given debounce interval. dir must already
// exist.
//
// onError, if not nil, is told about every error the watch itself reports. An
// overflow of the operating system's event queue (a burst of changes bigger
// than it can hold) also counts as a change, since it means some events were
// lost: the folder may look different in ways nothing else will announce.
func New(dir string, debounceInterval time.Duration, onChange func(), onError func(error)) (*FolderWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}

	fw := &FolderWatcher{
		watcher: w,
		done:    make(chan struct{}),
	}
	go fw.run(debounce.New(debounceInterval), onChange, onError)
	return fw, nil
}

func (fw *FolderWatcher) run(debounced func(func()), onChange func(), onError func(error)) {
	for {
		select {
		case _, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			debounced(onChange)
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			if onError != nil {
				onError(err)
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				debounced(onChange)
			}
		case <-fw.done:
			return
		}
	}
}

// Close stops watching. Safe to call more than once, and safe on a nil
// *FolderWatcher.
func (fw *FolderWatcher) Close() error {
	if fw == nil {
		return nil
	}
	var err error
	fw.closeOnce.Do(func() {
		close(fw.done)
		err = fw.watcher.Close()
	})
	return err
}

// Mute lets the app tell its own folder watcher that the changes it is about
// to see are the app's doing. Generating the patch or purging empty mods writes
// into the watched folder and then refreshes the mod list itself, so a refresh
// prompted by the watcher as well would only repeat the same full scan.
//
// The zero value is ready to use, and it is safe for concurrent use.
type Mute struct {
	mu     sync.Mutex
	active int
	until  time.Time
	now    func() time.Time // time.Now unless a test says otherwise
}

// Begin mutes the watcher until the returned function is called, and for grace
// after that - long enough for the debounce to deliver the last of the events
// the work caused. Overlapping calls are fine: it stays muted until the last
// one has ended.
func (m *Mute) Begin(grace time.Duration) (end func()) {
	m.mu.Lock()
	m.active++
	m.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.active--
			if until := m.clock().Add(grace); until.After(m.until) {
				m.until = until
			}
		})
	}
}

// Muted reports whether a change reported now should be ignored.
func (m *Mute) Muted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active > 0 || m.clock().Before(m.until)
}

func (m *Mute) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}
