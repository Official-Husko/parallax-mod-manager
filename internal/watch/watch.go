// Package watch provides a debounced, non-recursive watch on a single
// directory, used to notice mods being added, removed, or updated in a
// game's mod folder while the app is open.
package watch

import (
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
func New(dir string, debounceInterval time.Duration, onChange func()) (*FolderWatcher, error) {
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
	go fw.run(debounce.New(debounceInterval), onChange)
	return fw, nil
}

func (fw *FolderWatcher) run(debounced func(func()), onChange func()) {
	for {
		select {
		case _, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			debounced(onChange)
		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
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
