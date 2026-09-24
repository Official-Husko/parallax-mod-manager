package workshop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDecodeEventsReturnsTheFinalPublishedFileIDOnDone(t *testing.T) {
	stream := strings.NewReader(`{"stage":"opening"}
{"stage":"initialized"}
{"stage":"creating"}
{"stage":"created","publishedFileId":42}
{"stage":"updating","publishedFileId":42}
{"stage":"uploading","publishedFileId":42,"processed":50,"total":100}
{"stage":"done","publishedFileId":42}
`)
	var progress []PublishProgress
	result, err := decodeEvents(stream, func(p PublishProgress) { progress = append(progress, p) })
	if err != nil {
		t.Fatalf("decodeEvents() error = %v", err)
	}
	if result.PublishedFileID != 42 {
		t.Errorf("PublishedFileID = %d, want 42", result.PublishedFileID)
	}
	if len(progress) != 7 {
		t.Errorf("got %d progress callbacks, want 7 (one per line)", len(progress))
	}
}

func TestDecodeEventsReturnsTheHelpersOwnErrorMessage(t *testing.T) {
	stream := strings.NewReader(`{"stage":"opening"}
{"stage":"error","message":"SteamAPI_Init failed - is Steam running?"}
`)
	_, err := decodeEvents(stream, nil)
	if err == nil {
		t.Fatal("want an error from a stage=error event")
	}
	if !strings.Contains(err.Error(), "SteamAPI_Init failed") {
		t.Errorf("error = %v, want it to carry the helper's own message", err)
	}
}

func TestDecodeEventsFailsWhenNoFinalEventIsEverSeen(t *testing.T) {
	stream := strings.NewReader(`{"stage":"opening"}
{"stage":"initialized"}
`)
	_, err := decodeEvents(stream, nil)
	if !errors.Is(err, errNoFinalEvent) {
		t.Errorf("err = %v, want errNoFinalEvent", err)
	}
}

func TestDecodeEventsToleratesAMalformedLineWithoutAborting(t *testing.T) {
	stream := strings.NewReader("{\"stage\":\"opening\"}\nnot json at all\n{\"stage\":\"done\",\"publishedFileId\":7}\n")
	result, err := decodeEvents(stream, nil)
	if err != nil {
		t.Fatalf("decodeEvents() error = %v, want it to skip the bad line and still see stage=done", err)
	}
	if result.PublishedFileID != 7 {
		t.Errorf("PublishedFileID = %d, want 7", result.PublishedFileID)
	}
}

func TestDecodeEventsWorksWithANilProgressCallback(t *testing.T) {
	stream := strings.NewReader(`{"stage":"done","publishedFileId":1}` + "\n")
	if _, err := decodeEvents(stream, nil); err != nil {
		t.Fatalf("decodeEvents() error = %v", err)
	}
}

// TestHelperPublisherEndToEndAgainstAFakeHelperProcess exercises the real
// exec.Command/stdin/stdout wiring in Publish - decodeEvents' own logic is
// already covered directly above; this is only about the process plumbing
// around it. Unix-only: the fixture is a shell script, and this project's
// Windows companion build is verified separately by build.sh's own
// cross-compile (see companions/parallax-steam-helper's own tests for the
// helper's real behavior).
func TestHelperPublisherEndToEndAgainstAFakeHelperProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}

	script := "#!/bin/sh\ncat > /dev/null\necho '{\"stage\":\"opening\"}'\necho '{\"stage\":\"done\",\"publishedFileId\":99}'\n"
	path := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: path}
	var stages []string
	result, err := pub.Publish(context.Background(), PublishRequest{AppID: 281990}, func(p PublishProgress) {
		stages = append(stages, p.Stage)
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if result.PublishedFileID != 99 {
		t.Errorf("PublishedFileID = %d, want 99", result.PublishedFileID)
	}
	if len(stages) != 2 || stages[0] != "opening" || stages[1] != "done" {
		t.Errorf("stages = %v, want [opening done]", stages)
	}
}

func TestHelperPublisherFailsCleanlyWhenTheBinaryDoesNotExist(t *testing.T) {
	pub := HelperPublisher{BinaryPath: filepath.Join(t.TempDir(), "does-not-exist")}
	_, err := pub.Publish(context.Background(), PublishRequest{AppID: 281990}, nil)
	if err == nil {
		t.Fatal("want an error when the helper binary is missing")
	}
}

func TestHelperBinaryNameMatchesGOOS(t *testing.T) {
	name := helperBinaryName()
	if runtime.GOOS == "windows" {
		if name != "parallax-steam-helper-windows-amd64.exe" {
			t.Errorf("helperBinaryName() = %q on windows", name)
		}
	} else if name != "parallax-steam-helper-linux-amd64" {
		t.Errorf("helperBinaryName() = %q on %s", name, runtime.GOOS)
	}
}

func TestWorkDirForIsScopedByAppID(t *testing.T) {
	dirA, err := workDirFor(281990)
	if err != nil {
		t.Fatal(err)
	}
	dirB, err := workDirFor(394360)
	if err != nil {
		t.Fatal(err)
	}
	if dirA == dirB {
		t.Errorf("workDirFor returned the same directory for two different AppIDs: %s", dirA)
	}
	if !strings.Contains(dirA, "281990") {
		t.Errorf("workDirFor(281990) = %q, want it to mention the AppID", dirA)
	}
}
