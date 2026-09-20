package watch

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

const testDebounce = 40 * time.Millisecond

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestSingleChangeTriggersOnChangeOnce(t *testing.T) {
	dir := t.TempDir()
	var calls int32

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) }, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(filepath.Join(dir, "mod.mod"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	waitFor(t, func() bool { return atomic.LoadInt32(&calls) == 1 }, "onChange was not called after a file was created")

	// Give it a bit longer to make sure it doesn't fire again on its own.
	time.Sleep(3 * testDebounce)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("onChange called %d times, want exactly 1", got)
	}
}

func TestBurstOfChangesCollapsesToOneCall(t *testing.T) {
	dir := t.TempDir()
	var calls int32

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) }, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	for i := 0; i < 20; i++ {
		name := filepath.Join(dir, "mod"+string(rune('a'+i))+".mod")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	waitFor(t, func() bool { return atomic.LoadInt32(&calls) == 1 }, "onChange was not called after a burst of changes")

	time.Sleep(3 * testDebounce)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("onChange called %d times for one burst, want exactly 1", got)
	}
}

func TestRemoveTriggersOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mod.mod")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var calls int32
	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) }, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	waitFor(t, func() bool { return atomic.LoadInt32(&calls) >= 1 }, "onChange was not called after a file was removed")
}

func TestCloseStopsFurtherCalls(t *testing.T) {
	dir := t.TempDir()
	var calls int32

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) }, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Closing twice must not panic.
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "mod.mod"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	time.Sleep(3 * testDebounce)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("onChange called %d times after Close, want 0", got)
	}
}

func TestCloseOnNilWatcherIsSafe(t *testing.T) {
	var w *FolderWatcher
	if err := w.Close(); err != nil {
		t.Fatalf("Close on nil watcher: %v", err)
	}
}

func TestWatchErrorsAreReportedAndAnOverflowCountsAsAChange(t *testing.T) {
	dir := t.TempDir()
	var calls int32
	errs := make(chan error, 4)

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) }, func(err error) { errs <- err })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	// Some other failure: reported, but it says nothing about the folder.
	other := errors.New("watch went sideways")
	w.watcher.Errors <- other
	select {
	case got := <-errs:
		if got != other {
			t.Fatalf("onError got %v, want %v", got, other)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onError was not called for a watch error")
	}
	time.Sleep(3 * testDebounce)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("an ordinary watch error triggered onChange %d times, want 0", got)
	}

	// The event queue overflowed, so events were lost: treat the folder as changed.
	w.watcher.Errors <- fsnotify.ErrEventOverflow
	waitFor(t, func() bool { return atomic.LoadInt32(&calls) == 1 }, "an event queue overflow did not trigger onChange")
	if got := <-errs; !errors.Is(got, fsnotify.ErrEventOverflow) {
		t.Errorf("onError got %v, want the overflow", got)
	}
}

func TestMuteCoversTheWorkAndTheGraceAfterIt(t *testing.T) {
	now := time.Unix(1000, 0)
	m := &Mute{now: func() time.Time { return now }}

	if m.Muted() {
		t.Fatal("a Mute nobody began must not mute anything")
	}
	end := m.Begin(time.Second)
	now = now.Add(time.Hour) // however long the work takes
	if !m.Muted() {
		t.Fatal("must stay muted for as long as the work is running")
	}
	end()
	if !m.Muted() {
		t.Fatal("must stay muted for the grace period after the work ends")
	}
	now = now.Add(999 * time.Millisecond)
	if !m.Muted() {
		t.Fatal("still inside the grace period")
	}
	now = now.Add(2 * time.Millisecond)
	if m.Muted() {
		t.Fatal("must stop muting once the grace period has passed")
	}
}

func TestMuteOverlappingWorkStaysMutedUntilTheLastEnds(t *testing.T) {
	now := time.Unix(1000, 0)
	m := &Mute{now: func() time.Time { return now }}

	endA := m.Begin(time.Second)
	endB := m.Begin(time.Second)
	endA()
	endA() // a second call must not release B's hold
	now = now.Add(time.Minute)
	if !m.Muted() {
		t.Fatal("one piece of work is still running, so the watcher must stay muted")
	}
	endB()
	now = now.Add(2 * time.Second)
	if m.Muted() {
		t.Fatal("all work has ended and its grace has passed")
	}
}

func TestMuteWorksWithTheZeroValueAndTheRealClock(t *testing.T) {
	var m Mute
	end := m.Begin(50 * time.Millisecond)
	if !m.Muted() {
		t.Fatal("expected muted while work is running")
	}
	end()
	time.Sleep(80 * time.Millisecond)
	if m.Muted() {
		t.Fatal("expected unmuted after the grace period")
	}
}
