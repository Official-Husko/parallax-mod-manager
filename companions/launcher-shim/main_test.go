package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeFixtureSettings(t *testing.T, dir string, settings launcherSettings) {
	t.Helper()
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "launcher-settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// touchExecutable creates an empty, executable file at dir/name - resolveLaunch only
// checks the resolved path exists, never that it's a real, runnable binary.
func touchExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestResolveLaunchReadsExePathAndArgs(t *testing.T) {
	dir := t.TempDir()
	want := touchExecutable(t, dir, "stellaris")
	writeFixtureSettings(t, dir, launcherSettings{ExePath: "./stellaris", ExeArgs: []string{"-gdpr-compliant"}})

	exe, args, err := resolveLaunch(dir)
	if err != nil {
		t.Fatalf("resolveLaunch: %v", err)
	}
	if exe != want {
		t.Errorf("exe = %q, want %q", exe, want)
	}
	if len(args) != 1 || args[0] != "-gdpr-compliant" {
		t.Errorf("args = %v, want [-gdpr-compliant]", args)
	}
}

func TestResolveLaunchMissingSettingsFile(t *testing.T) {
	if _, _, err := resolveLaunch(t.TempDir()); err == nil {
		t.Fatal("expected an error for a directory with no launcher-settings.json")
	}
}

func TestResolveLaunchMalformedSettingsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "launcher-settings.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveLaunch(dir); err == nil {
		t.Fatal("expected an error for malformed launcher-settings.json")
	}
}

func TestResolveLaunchNoExePath(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSettings(t, dir, launcherSettings{})
	if _, _, err := resolveLaunch(dir); err == nil {
		t.Fatal("expected an error when exePath is empty")
	}
}

func TestResolveLaunchExecutableDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	writeFixtureSettings(t, dir, launcherSettings{ExePath: "./stellaris"})
	if _, _, err := resolveLaunch(dir); err == nil {
		t.Fatal("expected an error when the resolved executable does not exist on disk")
	}
}

func TestResolveLaunchNestedLauncherFolder(t *testing.T) {
	// CK3/Imperator/Victoria3-style layout: launcher-settings.json (and, per this
	// shim's own design, dowser itself) sits in a nested folder, with exePath
	// pointing back up at the real binary elsewhere - see the module doc comment
	// on why this still resolves correctly without knowing anything game-specific.
	root := t.TempDir()
	launcherDir := filepath.Join(root, "launcher")
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := touchExecutable(t, root, "binaries/ck3.exe")
	writeFixtureSettings(t, launcherDir, launcherSettings{ExePath: "../binaries/ck3.exe"})

	exe, _, err := resolveLaunch(launcherDir)
	if err != nil {
		t.Fatalf("resolveLaunch: %v", err)
	}
	if exe != want {
		t.Errorf("exe = %q, want %q", exe, want)
	}
}

// fakeLaunch records what it was asked to run instead of actually running anything.
func fakeLaunch(calls *[]struct {
	Exe  string
	Args []string
}) func(exe string, args []string) error {
	return func(exe string, args []string) error {
		*calls = append(*calls, struct {
			Exe  string
			Args []string
		}{exe, args})
		return nil
	}
}

func TestRunInLaunchesTheResolvedExecutableWithForwardedArgs(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	// No one is listening on this port in a test - pingParallax must not block or
	// panic just because nothing answers.
	pingPort = 1 // a privileged port nothing in a test sandbox can be listening on

	dir := t.TempDir()
	want := touchExecutable(t, dir, "stellaris")
	writeFixtureSettings(t, dir, launcherSettings{ExePath: "./stellaris", ExeArgs: []string{"-gdpr-compliant"}})

	var calls []struct {
		Exe  string
		Args []string
	}
	launchFunc = fakeLaunch(&calls)
	defer func() { launchFunc = launch }()

	if err := runIn(dir, []string{"-mod=my_mod"}); err != nil {
		t.Fatalf("runIn: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("launchFunc called %d times, want 1", len(calls))
	}
	if calls[0].Exe != want {
		t.Errorf("launched exe = %q, want %q", calls[0].Exe, want)
	}
	wantArgs := []string{"-gdpr-compliant", "-mod=my_mod"}
	if len(calls[0].Args) != len(wantArgs) || calls[0].Args[0] != wantArgs[0] || calls[0].Args[1] != wantArgs[1] {
		t.Errorf("launched args = %v, want %v", calls[0].Args, wantArgs)
	}

	// The status file should reflect a successful run.
	data, err := os.ReadFile(filepath.Join(configDir, statusFileRelPath))
	if err != nil {
		t.Fatalf("reading status file: %v", err)
	}
	var got status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling status file: %v", err)
	}
	if !got.Success || got.ResolvedExe != want {
		t.Errorf("status = %+v, want Success=true ResolvedExe=%q", got, want)
	}
}

func TestRunInWritesAFailureStatusWhenResolutionFails(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	dir := t.TempDir() // no launcher-settings.json here at all

	if err := runIn(dir, nil); err == nil {
		t.Fatal("expected an error when launcher-settings.json is missing")
	}

	data, err := os.ReadFile(filepath.Join(configDir, statusFileRelPath))
	if err != nil {
		t.Fatalf("reading status file: %v", err)
	}
	var got status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshaling status file: %v", err)
	}
	if got.Success || got.Error == "" {
		t.Errorf("status = %+v, want Success=false with a non-empty Error", got)
	}
}

func TestPrintSourceMentionsTheRepoURL(t *testing.T) {
	var buf bytes.Buffer
	printSource(&buf)
	if !bytes.Contains(buf.Bytes(), []byte(repoURL)) {
		t.Errorf("printSource output %q does not contain the repo URL %q", buf.String(), repoURL)
	}
}
