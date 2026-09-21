package applog

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLineMatchesTheHouseFormatExactly(t *testing.T) {
	when := time.Date(2026, 7, 6, 0, 9, 37, 0, time.Local)
	timed := Entry{Time: when.UnixMilli(), Level: "info", Component: "DBLockManager", Message: "DBLockManager: granting DB lock to 'DatabaseWriter.NewDBWriteSession' (queue: 0)", Timed: true, DurationMs: 0}
	want := "2026/07/06 00:09:37 [DBLockManager] DBLockManager: granting DB lock to 'DatabaseWriter.NewDBWriteSession' (queue: 0) (0ms)"
	if got := timed.Line(); got != want {
		t.Errorf("Line() = %q\nwant      %q", got, want)
	}
	plain := Entry{Time: when.UnixMilli(), Level: "info", Component: "Scan", Message: "done"}
	if got := plain.Line(); got != "2026/07/06 00:09:37 [Scan] done" {
		t.Errorf("an untimed info line = %q", got)
	}
	for level, marker := range map[string]string{"debug": "DEBUG: ", "warn": "WARN: ", "error": "ERROR: "} {
		e := Entry{Time: when.UnixMilli(), Level: level, Component: "X", Message: "m"}
		if got := e.Line(); !strings.Contains(got, "[X] "+marker+"m") {
			t.Errorf("%s line = %q, want the %q marker", level, got, marker)
		}
	}
}

func TestRingKeepsTheNewestInOrderAndSeqIncreases(t *testing.T) {
	h := New(3)
	log := h.For("T")
	for i := 1; i <= 5; i++ {
		log.Infof("line %d", i)
	}
	got := h.Entries()
	if len(got) != 3 {
		t.Fatalf("kept %d lines, want the newest 3", len(got))
	}
	for i, want := range []string{"line 3", "line 4", "line 5"} {
		if got[i].Message != want {
			t.Errorf("entry %d = %q, want %q", i, got[i].Message, want)
		}
	}
	if got[0].Seq != 3 || got[2].Seq != 5 {
		t.Errorf("seq = %d..%d, want 3..5 (monotonic, dropped lines keep their numbers)", got[0].Seq, got[2].Seq)
	}
}

func TestTimerLogsElapsedAndFormatsAreOptional(t *testing.T) {
	h := New(10)
	log := h.For("Scan")
	timer := log.Begin()
	time.Sleep(15 * time.Millisecond)
	timer.Infof("100%% done: %d items", 3)
	log.Warnf("a literal %s stays safe with no args")
	e := h.Entries()
	if !e[0].Timed || e[0].DurationMs < 10 || e[0].Message != "100% done: 3 items" {
		t.Errorf("timed entry = %+v", e[0])
	}
	if e[1].Timed || e[1].Message != "a literal %s stays safe with no args" || e[1].Level != "warn" {
		t.Errorf("untimed entry = %+v", e[1])
	}
}

func TestMessagesAreFlattenedAndComponentsCleaned(t *testing.T) {
	h := New(10)
	h.Log(Error, "  [Bad]\x00Component-With-A-Very-Long-Name-Indeed  ", "first\nsecond\r\nthird", false, 0)
	h.Log(Info, "   ", "x", false, 0)
	e := h.Entries()
	if e[0].Message != "first | second | third" {
		t.Errorf("message = %q", e[0].Message)
	}
	if strings.ContainsAny(e[0].Component, "[]\x00") || len(e[0].Component) > 24 {
		t.Errorf("component = %q, want it printable and at most 24 chars", e[0].Component)
	}
	if e[1].Component != "App" {
		t.Errorf("an empty component = %q, want the App default", e[1].Component)
	}
}

func TestMinLevelDropsQuieterLines(t *testing.T) {
	h := New(10)
	h.SetMinLevel(Warn)
	log := h.For("T")
	log.Debugf("d")
	log.Infof("i")
	log.Warnf("w")
	if got := h.Entries(); len(got) != 1 || got[0].Message != "w" {
		t.Errorf("entries = %+v, want only the warning", got)
	}
}

func TestSubscribersSeeNewLinesAndCanStop(t *testing.T) {
	h := New(10)
	var got []string
	cancel := h.Subscribe(func(e Entry) { got = append(got, e.Message) })
	h.For("T").Infof("one")
	cancel()
	h.For("T").Infof("two")
	if len(got) != 1 || got[0] != "one" {
		t.Errorf("subscriber saw %v, want just [one]", got)
	}
}

func TestClearForgetsMemoryButNotTheFile(t *testing.T) {
	dir := t.TempDir()
	h := New(10)
	if err := h.SetFile(filepath.Join(dir, "app.log"), 1<<20); err != nil {
		t.Fatal(err)
	}
	h.For("T").Infof("kept on disk")
	h.Clear()
	h.Close()
	if len(h.Entries()) != 0 {
		t.Error("Clear must empty the in-memory lines")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "app.log"))
	if !strings.Contains(string(data), "[T] kept on disk") {
		t.Errorf("the file must survive Clear, got %q", data)
	}
}

func TestFileRotatesAtTheLimitKeepingOnePreviousFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	h := New(10)
	if err := h.SetFile(path, 200); err != nil {
		t.Fatal(err)
	}
	log := h.For("T")
	for i := 0; i < 30; i++ {
		log.Infof("a line of some length number %02d", i)
	}
	h.Close()
	cur, _ := os.ReadFile(path)
	old, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("expected a rotated file: %v", err)
	}
	if len(cur) == 0 || len(old) == 0 {
		t.Errorf("both files should hold lines, got %d and %d bytes", len(cur), len(old))
	}
	if len(cur) > 400 || len(old) > 400 {
		t.Errorf("files should stay near the limit, got %d and %d bytes", len(cur), len(old))
	}
	if _, err := os.Stat(path + ".2"); err == nil {
		t.Error("only one previous file may be kept")
	}
	if !strings.Contains(string(cur), "number 29") {
		t.Error("the newest line must be in the current file")
	}
}

func TestSetFileCreatesMissingFoldersAndReportsARealFailure(t *testing.T) {
	h := New(10)
	if err := h.SetFile(filepath.Join(t.TempDir(), "a", "b", "app.log"), 0); err != nil {
		t.Errorf("a nested new folder should be created: %v", err)
	}
	h.Close()
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("x"), 0o644)
	if err := New(10).SetFile(filepath.Join(blocker, "app.log"), 0); err == nil {
		t.Error("a path under a regular file must be reported as an error")
	}
}

func TestConcurrentLoggingIsSafe(t *testing.T) {
	h := New(50)
	h.Subscribe(func(Entry) {})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log := h.For("T")
			for i := 0; i < 200; i++ {
				log.Infof("x %d", i)
				_ = h.Entries()
			}
		}()
	}
	wg.Wait()
	e := h.Entries()
	if len(e) != 50 {
		t.Fatalf("kept %d lines, want 50", len(e))
	}
	for i := 1; i < len(e); i++ {
		if e[i].Seq != e[i-1].Seq+1 {
			t.Fatalf("seq gap at %d: %d then %d", i, e[i-1].Seq, e[i].Seq)
		}
	}
}

func TestDefaultHubIsUsableWithoutSetup(t *testing.T) {
	before := len(Default().Entries())
	For("Test").Infof("hello")
	if len(Default().Entries()) != before+1 && before < DefaultCapacity {
		t.Error("the package-level logger must work with no setup")
	}
}

func TestPinnedLinesSurviveTheRingFillingUp(t *testing.T) {
	h := New(5)
	head := h.For("System").Pin()
	head.Infof("OS: test")
	head.Infof("CPU: test")
	other := h.For("Scan")
	for i := 0; i < 20; i++ {
		other.Infof("line %d", i)
	}
	entries := h.Entries()
	if len(entries) != 7 {
		t.Fatalf("got %d entries, want the 2 pinned and the newest 5", len(entries))
	}
	if entries[0].Message != "OS: test" || entries[1].Message != "CPU: test" || entries[2].Message != "line 15" || entries[6].Message != "line 19" {
		t.Errorf("entries = %+v", entries)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Seq <= entries[i-1].Seq {
			t.Errorf("entries are not in order: %d then %d", entries[i-1].Seq, entries[i].Seq)
		}
	}
}

func TestPinnedLinesAreNotDuplicatedWhileTheRingStillHoldsThem(t *testing.T) {
	h := New(50)
	h.For("System").Pin().Infof("OS: test")
	h.For("Scan").Infof("scanning")
	entries := h.Entries()
	if len(entries) != 2 || entries[0].Message != "OS: test" || entries[1].Message != "scanning" {
		t.Errorf("entries = %+v, want each line once, in order", entries)
	}
}

func TestClearForgetsPinnedLinesAndOnlyOrdinaryLinesArePinnedByPin(t *testing.T) {
	h := New(3)
	h.For("System").Pin().Infof("kept")
	h.For("Scan").Infof("ordinary")
	for i := 0; i < 10; i++ {
		h.For("Scan").Infof("more %d", i)
	}
	for _, e := range h.Entries() {
		if e.Message == "ordinary" {
			t.Error("an ordinary line was pinned")
		}
	}
	h.Clear()
	if got := h.Entries(); len(got) != 0 {
		t.Errorf("after Clear: %+v", got)
	}
}

func TestPinnedLinesStillGoToTheFileAndSubscribers(t *testing.T) {
	h := New(5)
	var got []string
	cancel := h.Subscribe(func(e Entry) { got = append(got, e.Message) })
	defer cancel()
	h.For("System").Pin().Infof("hello")
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("subscriber saw %v", got)
	}
}

func TestOnlyAShortHeaderCanBePinned(t *testing.T) {
	h := New(2)
	p := h.For("System").Pin()
	for i := 0; i < 200; i++ {
		p.Infof("line %d", i)
	}
	pinned := 0
	for _, e := range h.Entries() {
		if e.Component == "System" {
			pinned++
		}
	}
	if pinned > maxPinned+2 {
		t.Errorf("%d pinned lines, want at most %d (plus what the ring holds)", pinned, maxPinned)
	}
}
