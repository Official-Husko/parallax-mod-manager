package gameproc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExecutableNamesCoversTheNativeAndTheProtonSpelling(t *testing.T) {
	cases := map[string][]string{
		"/games/Stellaris/stellaris":       {"stellaris", "stellaris.exe"},
		"./stellaris":                      {"stellaris", "stellaris.exe"},
		`C:\Games\Stellaris\stellaris.exe`: {"stellaris.exe", "stellaris"},
		"stellaris.EXE":                    {"stellaris.EXE", "stellaris"},
		"":                                 nil,
		"/":                                nil,
		".exe":                             {".exe"},
	}
	for in, want := range cases {
		if got := ExecutableNames(in); !reflect.DeepEqual(got, want) {
			t.Errorf("ExecutableNames(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchGoesByTheExecutableNameAndNeverByAnArgument(t *testing.T) {
	// Real processes from a machine with the game's mod folder open elsewhere:
	// the file manager and the terminal carry the game's name in their arguments,
	// not in their own name. They must never be taken for the game.
	procs := []Process{
		{PID: 1, Names: []string{"systemd"}},
		{PID: 640726, Names: []string{"dolphin"}},
		{PID: 646115, Names: []string{"konsole"}},
		{PID: 700, Names: []string{"stellaris"}},
		{PID: 701, Names: []string{"Stellaris.EXE"}}, // Wine keeps the installed case
		{PID: 702, Names: []string{"python3", "proton"}},
	}
	got := Match(procs, ExecutableNames("/games/Stellaris/stellaris"))
	if len(got) != 2 || got[0].PID != 700 || got[1].PID != 701 {
		t.Errorf("Match = %+v, want exactly the two processes named stellaris / stellaris.exe", got)
	}
	if got := Match(procs, nil); got != nil {
		t.Errorf("no names must match nothing, got %+v", got)
	}
	if got := Match(procs, []string{"stellaris-launcher"}); len(got) != 0 {
		t.Errorf("a different executable that merely shares a prefix matched: %+v", got)
	}
}

func TestMatchAcceptsEitherOfAProcessesNames(t *testing.T) {
	// The command line's own first word matches in full; the short name Linux keeps
	// is separate.
	p := Process{PID: 5, Names: []string{"someverylongname.exe", "someverylongnam"}}
	if len(Match([]Process{p}, []string{"someverylongname.exe"})) != 1 {
		t.Error("the full name should match")
	}
	if len(Match([]Process{p}, []string{"someverylongnam"})) != 1 {
		t.Error("the short name should match")
	}
}

func TestMatchRecognisesANameThatLinuxCutToFifteenCharacters(t *testing.T) {
	// A launcher script named "run_a_long_launcher" shows as its first 15 characters.
	cut := Process{PID: 9, Names: []string{"sh", "run_a_long_lau"[:14] + "n"}}
	if len(cut.Names[1]) != 15 {
		t.Fatalf("test setup: %q is %d characters", cut.Names[1], len(cut.Names[1]))
	}
	if len(Match([]Process{cut}, []string{"run_a_long_launcher"})) != 1 {
		t.Error("a name cut to 15 characters should match the longer name it is the front of")
	}
	if len(Match([]Process{cut}, []string{"run_a_long_other_one"})) != 0 {
		t.Error("a different long name must not match just because it is long")
	}
	// Under the cut-off length the name is whole, so a longer name is a different one.
	short := Process{PID: 10, Names: []string{"run_hoi4"}}
	if len(Match([]Process{short}, []string{"run_hoi4_extra_long_name"})) != 0 {
		t.Error("a whole (short) name must not match a longer name it merely starts")
	}
}

func TestBaseNameHandlesWindowsPathsFromWine(t *testing.T) {
	cases := map[string]string{
		`Z:\run\media\games\Stellaris\stellaris.exe`: "stellaris.exe",
		"/usr/bin/dolphin":                           "dolphin",
		"stellaris":                                  "stellaris",
		"  spaced  ":                                 "spaced",
		"":                                           "",
	}
	for in, want := range cases {
		if got := baseName(in); got != want {
			t.Errorf("baseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindNeverReturnsThisProcess(t *testing.T) {
	// The test binary is running, so a search by its own name would find it - and
	// must still come back empty.
	procs, err := Find(ExecutableNames("gameproc.test"))
	if err != nil {
		t.Skipf("can't list processes here: %v", err)
	}
	for _, p := range procs {
		if p.PID == selfPID() {
			t.Errorf("Find returned this very process: %+v", p)
		}
	}
}

func TestCheckReportsAnEmptyListNotNil(t *testing.T) {
	st, err := Check([]string{"a-name-no-process-has-9f3c1e"})
	if err != nil {
		t.Skipf("can't list processes here: %v", err)
	}
	if st.Running || st.PIDs == nil || len(st.PIDs) != 0 {
		t.Errorf("Check for nothing running = %+v, want not running with a non-nil empty list", st)
	}
}

func writeExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecutableNamesForIncludesTheProgramALauncherScriptStarts(t *testing.T) {
	// The shape of Hearts of Iron IV's real launcher script: the game itself is a
	// child of the script, not the script.
	dir := t.TempDir()
	writeExecutable(t, dir, "hoi4", "\x7fELF pretend binary")
	script := writeExecutable(t, dir, "run_hoi4", `#!/bin/sh

GAME_DIR=`+"`dirname \"$(realpath \"$0\")\"`"+`

export LD_LIBRARY_PATH="$GAME_DIR":"$LD_LIBRARY_PATH"
"$GAME_DIR/hoi4" "$@"
`)
	got := ExecutableNamesFor(script)
	want := []string{"run_hoi4", "run_hoi4.exe", "hoi4", "hoi4.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExecutableNamesFor(script) = %q, want %q", got, want)
	}
}

func TestExecutableNamesForIgnoresWhatTheScriptMentionsButIsNotTheGames(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "game", "binary")
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := writeExecutable(t, dir, "start.sh", `#!/usr/bin/env bash
# /usr/bin/env, /bin/sh and a folder must not count: none is a program next to the script
cd "$DIR/assets"
exec /usr/bin/nice -n 5 "$DIR/game" --flag
echo "$DIR/start.sh"
`)
	got := ExecutableNamesFor(script)
	want := []string{"start.sh", "start.sh.exe", "game", "game.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExecutableNamesFor = %q, want %q", got, want)
	}
}

func TestExecutableNamesForABinaryOrAMissingFileIsJustItsOwnName(t *testing.T) {
	dir := t.TempDir()
	other := writeExecutable(t, dir, "other", "binary")
	bin := writeExecutable(t, dir, "stellaris", "\x7fELF /other should not be looked for in a binary")
	if got := ExecutableNamesFor(bin); !reflect.DeepEqual(got, []string{"stellaris", "stellaris.exe"}) {
		t.Errorf("binary: %q", got)
	}
	_ = other
	if got := ExecutableNamesFor(filepath.Join(dir, "does-not-exist")); !reflect.DeepEqual(got, []string{"does-not-exist", "does-not-exist.exe"}) {
		t.Errorf("missing file: %q", got)
	}
	if got := ExecutableNamesFor(""); got != nil {
		t.Errorf("empty path: %q, want nil", got)
	}
}
