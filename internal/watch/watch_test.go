package watch

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
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

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) })
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

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) })
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
	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) })
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

	w, err := New(dir, testDebounce, func() { atomic.AddInt32(&calls, 1) })
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
