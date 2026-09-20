package gamelog

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidNameOnlyAllowsAPlainLogFileName(t *testing.T) {
	good := []string{"error.log", "game.log", "Setup.LOG", "script_profiling_summary.log"}
	bad := []string{"", "error.txt", "error", "../error.log", "sub/error.log", `sub\error.log`, "/etc/passwd", ".log", ".hidden.log", "..", "error.log/"}
	for _, n := range good {
		if !ValidName(n) {
			t.Errorf("ValidName(%q) = false, want true", n)
		}
	}
	for _, n := range bad {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true, want false", n)
		}
	}
}

func TestListOrdersTheUsefulLogsFirstAndSkipsEverythingElse(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"time.log": "t", "error.log": "eee", "zzz.log": "", "game.log": "gg", "setup.log": "s",
		"notes.txt": "not a log", "system.log": "y",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "script_documentation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir.log"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	want := []string{"error.log", "game.log", "setup.log", "system.log", "time.log", "zzz.log"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("List = %v, want %v", names, want)
	}
	if files[0].Size != 3 || files[0].Modified == 0 {
		t.Errorf("error.log entry = %+v, want its real size and time", files[0])
	}
}

func TestListOfAMissingFolderIsEmptyNotAnError(t *testing.T) {
	files, err := List(filepath.Join(t.TempDir(), "never-created"))
	if err != nil || files == nil || len(files) != 0 {
		t.Errorf("List(missing) = %v, %v, want an empty non-nil list and no error", files, err)
	}
}

// follow runs Follow on path with a fast poll and returns a channel of what it
// emits, and a stop function.
func follow(t *testing.T, path string, opts Options) (<-chan Batch, func()) {
	t.Helper()
	if opts.Interval == 0 {
		opts.Interval = 10 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Batch, 256)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Follow(ctx, path, opts, func(b Batch) { out <- b })
	}()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Follow did not return after its context was cancelled")
		}
	}
	t.Cleanup(stop)
	return out, stop
}

func next(t *testing.T, ch <-chan Batch) Batch {
	t.Helper()
	select {
	case b := <-ch:
		return b
	case <-time.After(2 * time.Second):
		t.Fatal("no batch arrived")
		return Batch{}
	}
}

func expectQuiet(t *testing.T, ch <-chan Batch) {
	t.Helper()
	select {
	case b := <-ch:
		t.Fatalf("unexpected batch: %+v", b)
	case <-time.After(120 * time.Millisecond):
	}
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestFollowStartsWithTheEndOfTheFileThenStreamsNewLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	appendTo(t, path, "one\ntwo\nthree\nfour\n")

	ch, _ := follow(t, path, Options{InitialLines: 3})
	first := next(t, ch)
	if !first.Reset || first.Missing || !reflect.DeepEqual(first.Lines, []string{"two", "three", "four"}) {
		t.Fatalf("first batch = %+v, want a reset holding the last 3 lines", first)
	}
	expectQuiet(t, ch)

	appendTo(t, path, "five\nsix\n")
	b := next(t, ch)
	if b.Reset || !reflect.DeepEqual(b.Lines, []string{"five", "six"}) {
		t.Fatalf("appended batch = %+v, want just the two new lines, not a reset", b)
	}
	expectQuiet(t, ch)
}

func TestFollowHoldsBackALineUntilItIsComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.log")
	appendTo(t, path, "start\n")
	ch, _ := follow(t, path, Options{})
	next(t, ch) // the initial batch

	appendTo(t, path, "half a li")
	expectQuiet(t, ch) // nothing yet: it is not a line

	appendTo(t, path, "ne, now whole\nand a second\nunfinished")
	b := next(t, ch)
	if !reflect.DeepEqual(b.Lines, []string{"half a line, now whole", "and a second"}) {
		t.Fatalf("lines = %q, want the completed line whole and the unfinished one still held", b.Lines)
	}
	appendTo(t, path, " ending\n")
	if b := next(t, ch); !reflect.DeepEqual(b.Lines, []string{"unfinished ending"}) {
		t.Fatalf("lines = %q", b.Lines)
	}
}

func TestFollowKeepsTheEndOfALargeFileAndDoesNotSplitTheFirstLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	var sb strings.Builder
	total := 30000
	for i := 0; i < total; i++ {
		sb.WriteString("line number ")
		sb.WriteString(strings.Repeat("x", 30))
		sb.WriteString(" ")
		sb.WriteString(itoa(i))
		sb.WriteString("\n")
	}
	appendTo(t, path, sb.String())

	ch, _ := follow(t, path, Options{InitialLines: 50})
	first := next(t, ch)
	if len(first.Lines) != 50 {
		t.Fatalf("got %d lines, want the last 50", len(first.Lines))
	}
	if !strings.HasSuffix(first.Lines[49], " "+itoa(total-1)) || !strings.HasSuffix(first.Lines[0], " "+itoa(total-50)) {
		t.Errorf("wrong lines: first %q, last %q", first.Lines[0], first.Lines[49])
	}
	for _, l := range first.Lines {
		if !strings.HasPrefix(l, "line number ") {
			t.Fatalf("a line was cut in half: %q", l)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestFollowResetsWhenTheGameTruncatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.log")
	appendTo(t, path, "old run line 1\nold run line 2\nold run line 3\n")
	ch, _ := follow(t, path, Options{})
	next(t, ch)

	// A new run starts the file over, and writes less than was there.
	if err := os.WriteFile(path, []byte("new run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := next(t, ch)
	if !b.Reset || !reflect.DeepEqual(b.Lines, []string{"new run"}) {
		t.Fatalf("after truncation: %+v, want a reset with the new content", b)
	}
	appendTo(t, path, "more\n")
	if b := next(t, ch); b.Reset || !reflect.DeepEqual(b.Lines, []string{"more"}) {
		t.Fatalf("after the reset: %+v, want the appended line as a plain update", b)
	}
}

func TestFollowNoticesARewriteThatEndedUpLongerThanTheOldFile(t *testing.T) {
	// The size alone can't show this: truncated and rewritten to more than was
	// there, all between two looks. Only the file's first bytes changing does.
	path := filepath.Join(t.TempDir(), "game.log")
	appendTo(t, path, "[23:45:43] old first line\nold second\n")
	ch, _ := follow(t, path, Options{Interval: 30 * time.Millisecond})
	next(t, ch)

	// Same file (same inode), longer than before, different start.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("[00:01:02] NEW first line\nnew second line\nnew third line\nnew fourth line\n")
	f.Close()

	b := next(t, ch)
	if !b.Reset {
		t.Fatalf("a rewritten file must reset the viewer, got %+v", b)
	}
	if got := strings.Join(b.Lines, "|"); !strings.HasPrefix(got, "[00:01:02] NEW first line|new second line") {
		t.Errorf("lines = %q, want the whole new content from its start", got)
	}
}

func TestFollowResetsWhenTheFileIsReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "error.log")
	appendTo(t, path, "a\nb\n")
	ch, _ := follow(t, path, Options{})
	next(t, ch)

	replacement := filepath.Join(dir, "error.log.new")
	if err := os.WriteFile(replacement, []byte("brand new file, longer than the old one\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	b := next(t, ch)
	if !b.Reset || len(b.Lines) != 2 || b.Lines[0] != "brand new file, longer than the old one" {
		t.Fatalf("after replacement: %+v", b)
	}
}

func TestFollowWaitsForAFileThatDoesNotExistYet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	ch, _ := follow(t, path, Options{})

	first := next(t, ch)
	if !first.Reset || !first.Missing || first.Lines == nil || len(first.Lines) != 0 {
		t.Fatalf("first batch = %+v, want a reset saying the file is missing, with an empty non-nil list", first)
	}
	expectQuiet(t, ch) // says so once, not on every look

	appendTo(t, path, "it exists now\n")
	b := next(t, ch)
	if !b.Reset || b.Missing || !reflect.DeepEqual(b.Lines, []string{"it exists now"}) {
		t.Fatalf("once created: %+v, want a reset with its content", b)
	}
}

func TestFollowSaysAgainWhenTheFileIsDeletedAndComesBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	appendTo(t, path, "x\n")
	ch, _ := follow(t, path, Options{})
	next(t, ch)

	os.Remove(path)
	if b := next(t, ch); !b.Missing || !b.Reset {
		t.Fatalf("after deleting: %+v, want the missing notice", b)
	}
	appendTo(t, path, "back\n")
	if b := next(t, ch); !b.Reset || !reflect.DeepEqual(b.Lines, []string{"back"}) {
		t.Fatalf("after it returned: %+v", b)
	}
}

func TestFollowSkipsToTheNewestWhenMoreArrivesThanItWillRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	appendTo(t, path, "before\n")
	ch, _ := follow(t, path, Options{MaxChunk: 400, Interval: 200 * time.Millisecond})
	next(t, ch)

	// Between two looks the game floods the file far past MaxChunk.
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("flood line ")
		sb.WriteString(itoa(i))
		sb.WriteString("\n")
	}
	appendTo(t, path, sb.String())

	b := next(t, ch)
	if b.Skipped <= 0 {
		t.Fatalf("Skipped = %d, want the bytes passed over reported", b.Skipped)
	}
	if b.Reset {
		t.Error("a skip is an update, not a reset: what the viewer holds is still valid")
	}
	if len(b.Lines) == 0 || b.Lines[len(b.Lines)-1] != "flood line 499" {
		t.Fatalf("last line = %q, want the newest", b.Lines)
	}
	for _, l := range b.Lines {
		if !strings.HasPrefix(l, "flood line ") {
			t.Fatalf("a line cut mid-way was emitted: %q", l)
		}
	}
	// What was skipped plus what was read accounts for everything that was written.
	read := int64(0)
	for _, l := range b.Lines {
		read += int64(len(l) + 1)
	}
	if b.Skipped+read != int64(sb.Len()) {
		t.Errorf("skipped %d + read %d = %d, want the %d bytes appended", b.Skipped, read, b.Skipped+read, sb.Len())
	}
}

func TestFollowHandlesWindowsLineEndingsAndBrokenEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	appendTo(t, path, "crlf line\r\nplain\nbad \xff\xfe bytes\nunicode 日本語\n")
	ch, _ := follow(t, path, Options{})
	b := next(t, ch)
	want := []string{"crlf line", "plain", "bad � bytes", "unicode 日本語"}
	if !reflect.DeepEqual(b.Lines, want) {
		t.Errorf("lines = %q, want %q", b.Lines, want)
	}
}

func TestFollowCutsAVeryLongLineWithoutSplittingACharacter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	appendTo(t, path, strings.Repeat("日", 50)+"\n")
	ch, _ := follow(t, path, Options{MaxLineLength: 10})
	b := next(t, ch)
	if len(b.Lines) != 1 || b.Lines[0] != strings.Repeat("日", 10)+"..." {
		t.Errorf("lines = %q", b.Lines)
	}
}

func TestFollowStopsWhenItsContextIsCancelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	appendTo(t, path, "x\n")
	ch, stop := follow(t, path, Options{})
	next(t, ch)
	stop() // fails the test itself if Follow doesn't return

	appendTo(t, path, "after stop\n")
	expectQuiet(t, ch)
}
