package atomicfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJSONRoundTrips(t *testing.T) {
	dir := t.TempDir()
	type payload struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	want := payload{A: "hello", B: 42}

	path, err := WriteJSON(dir, "test.json", want)
	if err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if path != filepath.Join(dir, "test.json") {
		t.Errorf("returned path = %q, want %q", path, filepath.Join(dir, "test.json"))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != want {
		t.Errorf("round-tripped = %+v, want %+v", got, want)
	}
}

func TestWriteJSONNoLeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteJSON(dir, "test.json", map[string]int{"x": 1}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "test.json" {
		t.Errorf("dir entries = %+v, want exactly [test.json]", entries)
	}
}

func TestWriteJSONOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteJSON(dir, "test.json", map[string]int{"x": 1}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := WriteJSON(dir, "test.json", map[string]int{"x": 2}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]int
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["x"] != 2 {
		t.Errorf("x = %d, want 2 (overwritten)", got["x"])
	}
}

func TestWriteJSONCreatesDirIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	if _, err := WriteJSON(dir, "test.json", map[string]int{"x": 1}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test.json")); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestWriteRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, "test.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if path != filepath.Join(dir, "test.txt") {
		t.Errorf("returned path = %q, want %q", path, filepath.Join(dir, "test.txt"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", data, "hello")
	}
}

func TestWriteNoLeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "test.txt", []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "test.txt" {
		t.Errorf("dir entries = %+v, want exactly [test.txt]", entries)
	}
}
