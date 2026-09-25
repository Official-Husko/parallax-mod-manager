package toolmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
)

func TestRecordCreatesTheFileOnTheFirstCall(t *testing.T) {
	dir := t.TempDir()
	if err := Record(dir, "Edited in the Editor"); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("reading %s: %v", FileName, err)
	}
	content := string(data)
	if !strings.Contains(content, "# Made with Parallax Mod Manager") {
		t.Errorf("content = %q, want the heading", content)
	}
	if !strings.Contains(content, about.RepoURL) {
		t.Errorf("content = %q, want it to link the real repo URL", content)
	}
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(content, "- "+today+": Edited in the Editor") {
		t.Errorf("content = %q, want today's dated entry", content)
	}
}

func TestRecordAddsANewLineForADifferentAction(t *testing.T) {
	dir := t.TempDir()
	if err := Record(dir, "Edited in the Editor"); err != nil {
		t.Fatal(err)
	}
	if err := Record(dir, "Translated into German using Translanova"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Edited in the Editor") || !strings.Contains(content, "Translated into German using Translanova") {
		t.Errorf("content = %q, want both actions recorded", content)
	}
}

func TestRecordingTheSameActionTwiceOnTheSameDayIsANoOp(t *testing.T) {
	dir := t.TempDir()
	if err := Record(dir, "Edited in the Editor"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := Record(dir, "Edited in the Editor"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("recording the same action again on the same day changed the file:\nbefore: %q\nafter:  %q", first, second)
	}
	if n := strings.Count(string(second), "Edited in the Editor"); n != 1 {
		t.Errorf("action appears %d times, want exactly 1 (no duplicate line)", n)
	}
}

func TestRecordPreservesAnEarlierDifferentDatedEntryAndPutsTheNewestFirst(t *testing.T) {
	dir := t.TempDir()
	// Seed a fixture as if this mod had already been recorded on an earlier day - Record
	// itself always uses today's real date, so an older entry can only be set up directly
	// like this, not produced by calling Record twice in the same test run.
	seeded := header + "- 2020-01-01: Edited in the Editor\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Record(dir, "Generated as a load-order patch mod"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	lines := entryLines(t, string(data))
	if len(lines) != 2 {
		t.Fatalf("got %d entry lines, want 2: %v", len(lines), lines)
	}
	today := time.Now().Format("2006-01-02")
	if !strings.HasPrefix(lines[0], today+": Generated as a load-order patch mod") {
		t.Errorf("first line = %q, want the new entry first (newest first)", lines[0])
	}
	if !strings.HasPrefix(lines[1], "2020-01-01: Edited in the Editor") {
		t.Errorf("second line = %q, want the old entry preserved", lines[1])
	}
}

func TestRecordAddsANewLineForTheSameActionOnADifferentDay(t *testing.T) {
	dir := t.TempDir()
	seeded := header + "- 2020-01-01: Edited in the Editor\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Record(dir, "Edited in the Editor"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	lines := entryLines(t, string(data))
	if len(lines) != 2 {
		t.Fatalf("got %d entry lines, want 2 (the same action recorded on a different day is a new line, not a dedup): %v", len(lines), lines)
	}
}

func TestRecordIsANoOpWithNoModDirOrNoAction(t *testing.T) {
	dir := t.TempDir()
	if err := Record("", "Edited in the Editor"); err != nil {
		t.Errorf("Record() with no modDir error = %v, want nil", err)
	}
	if err := Record(dir, ""); err != nil {
		t.Errorf("Record() with no action error = %v, want nil", err)
	}
	if _, err := os.ReadFile(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Errorf("a marker file was written despite empty modDir/action")
	}
}

// entryLines returns content's own "- YYYY-MM-DD: ..." lines, in order.
func entryLines(t *testing.T, content string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if entryLine.MatchString(line) {
			out = append(out, strings.TrimPrefix(line, "- "))
		}
	}
	return out
}
