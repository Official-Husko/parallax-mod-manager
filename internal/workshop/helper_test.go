package workshop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

func TestHelperPublisherIdentityReturnsThePersonaName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	script := "#!/bin/sh\ncat > /dev/null\necho '{\"stage\":\"opening\"}'\necho '{\"stage\":\"done\",\"personaName\":\"Kestrel_Admiral\"}'\n"
	path := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: path}
	result, err := pub.Identity(context.Background(), IdentityRequest{AppID: 281990})
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if result.PersonaName != "Kestrel_Admiral" {
		t.Errorf("PersonaName = %q, want %q", result.PersonaName, "Kestrel_Admiral")
	}
}

func TestHelperPublisherIdentityReturnsTheSteamIDAndDecodesTheAvatar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// A real, valid 1x1 PNG, base64-encoded - just enough to confirm decoding
	// end to end, not a claim about what a real avatar looks like.
	const avatarPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR42mPgEpEDAABoAD1q9XBbAAAAAElFTkSuQmCC"
	script := "#!/bin/sh\ncat > /dev/null\necho '{\"stage\":\"opening\"}'\n" +
		"echo '{\"stage\":\"done\",\"personaName\":\"Kestrel_Admiral\",\"steamId\":\"76561197989629849\",\"avatarPng\":\"" + avatarPNGBase64 + "\"}'\n"
	path := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: path}
	result, err := pub.Identity(context.Background(), IdentityRequest{AppID: 281990})
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if result.SteamID != "76561197989629849" {
		t.Errorf("SteamID = %q, want %q", result.SteamID, "76561197989629849")
	}
	wantPNG, _ := base64.StdEncoding.DecodeString(avatarPNGBase64)
	if !bytes.Equal(result.Avatar, wantPNG) {
		t.Errorf("Avatar = %d bytes, want the decoded %d-byte fixture PNG", len(result.Avatar), len(wantPNG))
	}
}

// TestHelperPublisherIdentityHandlesARealisticallyLargeAvatarLine is a real
// regression test: bufio.Scanner's default per-line limit is 64KB, and a
// real large avatar's base64 alone is already close to that on its own (a
// real 184x184 one, confirmed live, comes to ~70KB) - once folded into one
// JSON "done" line alongside personaName/steamId, the line silently
// exceeded the old default. Scan() just returned false with no error at
// all, so PersonaName/SteamID/Avatar all came back "" - not a crash, not a
// timeout, just quietly wrong data, and exactly what a real user actually
// hit before this test (and the buffer-size fix it guards) existed.
func TestHelperPublisherIdentityHandlesARealisticallyLargeAvatarLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// 90,000 raw bytes, well past a real avatar's own real size, so the
	// resulting base64 (~120,000 characters) comfortably exceeds 64KB on
	// its own, before any of the rest of the JSON line is even counted.
	raw := make([]byte, 90_000)
	for i := range raw {
		raw[i] = byte(i)
	}
	avatarPNGBase64 := base64.StdEncoding.EncodeToString(raw)

	script := "#!/bin/sh\ncat > /dev/null\necho '{\"stage\":\"opening\"}'\n" +
		"echo '{\"stage\":\"done\",\"personaName\":\"Kestrel_Admiral\",\"steamId\":\"76561197989629849\",\"avatarPng\":\"" + avatarPNGBase64 + "\"}'\n"
	path := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: path}
	result, err := pub.Identity(context.Background(), IdentityRequest{AppID: 281990})
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if result.PersonaName != "Kestrel_Admiral" {
		t.Errorf("PersonaName = %q, want %q (a too-long line must not silently blank out fields that came before it in the JSON)", result.PersonaName, "Kestrel_Admiral")
	}
	if result.SteamID != "76561197989629849" {
		t.Errorf("SteamID = %q, want %q", result.SteamID, "76561197989629849")
	}
	if !bytes.Equal(result.Avatar, raw) {
		t.Errorf("Avatar = %d bytes, want the real %d-byte fixture back unchanged", len(result.Avatar), len(raw))
	}
}

func TestHelperPublisherIdentityTreatsGarbageAvatarBase64AsNoneRatherThanFailing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	script := "#!/bin/sh\ncat > /dev/null\necho '{\"stage\":\"done\",\"personaName\":\"Kestrel_Admiral\",\"avatarPng\":\"not-valid-base64!!\"}'\n"
	path := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: path}
	result, err := pub.Identity(context.Background(), IdentityRequest{AppID: 281990})
	if err != nil {
		t.Fatalf("Identity() error = %v, want a corrupt avatar to be dropped, not fail the whole request", err)
	}
	if result.Avatar != nil {
		t.Errorf("Avatar = %v, want nil for undecodable base64", result.Avatar)
	}
	if result.PersonaName != "Kestrel_Admiral" {
		t.Errorf("PersonaName = %q, want %q even though the avatar was garbage", result.PersonaName, "Kestrel_Admiral")
	}
}

func TestHelperPublisherIdentityReturnsTheHelpersOwnErrorMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	script := "#!/bin/sh\ncat > /dev/null\necho '{\"stage\":\"error\",\"message\":\"Steam is not running\"}'\n"
	path := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: path}
	_, err := pub.Identity(context.Background(), IdentityRequest{AppID: 281990})
	if err == nil || !strings.Contains(err.Error(), "Steam is not running") {
		t.Errorf("Identity() error = %v, want it to mention the helper's own message", err)
	}
}

// TestHelperPublisherStagesAContentFolderExcludingChosenFiles is
// TestHelperPublisherEndToEndAgainstAFakeHelperProcess's own sibling, adding
// ExcludePaths. Publish's own defer removes the staged folder the moment it
// returns, so the fake helper inspects it itself - via `ls`, capturing that
// listing to a file - while it still exists, before ever emitting "done".
func TestHelperPublisherStagesAContentFolderExcludingChosenFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a Unix shell script - see the test's own doc comment")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // workDirFor resolves under os.UserConfigDir()

	contentFolder := t.TempDir()
	if err := os.WriteFile(filepath.Join(contentFolder, "descriptor.mod"), []byte(`name="Test"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentFolder, "notes.txt"), []byte("private dev notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	capturedRequest := filepath.Join(t.TempDir(), "captured-request.json")
	capturedListing := filepath.Join(t.TempDir(), "captured-listing.txt")
	script := "#!/bin/sh\n" +
		"REQ=$(cat)\n" +
		"echo \"$REQ\" > '" + capturedRequest + "'\n" +
		"CONTENT=$(echo \"$REQ\" | sed -n 's/.*\"contentFolder\":\"\\([^\"]*\\)\".*/\\1/p')\n" +
		"ls \"$CONTENT\" > '" + capturedListing + "'\n" +
		"echo '{\"stage\":\"done\",\"publishedFileId\":1}'\n"
	scriptPath := filepath.Join(t.TempDir(), "fake-helper.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pub := HelperPublisher{BinaryPath: scriptPath}
	var stages []string
	_, err := pub.Publish(context.Background(), PublishRequest{
		AppID:         281990,
		ContentFolder: contentFolder,
		ExcludePaths:  []string{"notes.txt"},
	}, func(p PublishProgress) { stages = append(stages, p.Stage) })
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if stages[0] != "staging" {
		t.Errorf("stages = %v, want a leading \"staging\" event", stages)
	}

	reqData, err := os.ReadFile(capturedRequest)
	if err != nil {
		t.Fatalf("reading the fake helper's captured request: %v", err)
	}
	var got helperRequest
	if err := json.Unmarshal(reqData, &got); err != nil {
		t.Fatalf("decoding the captured request: %v", err)
	}
	if got.ContentFolder == contentFolder {
		t.Fatalf("the helper was pointed at the real content folder %s directly, not a staged copy", contentFolder)
	}

	listing, err := os.ReadFile(capturedListing)
	if err != nil {
		t.Fatalf("reading the captured directory listing: %v", err)
	}
	if !strings.Contains(string(listing), "descriptor.mod") {
		t.Errorf("staged folder listing = %q, want it to contain descriptor.mod", listing)
	}
	if strings.Contains(string(listing), "notes.txt") {
		t.Errorf("staged folder listing = %q, want notes.txt excluded", listing)
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
