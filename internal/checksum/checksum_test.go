package checksum

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testdata/tree is a small game with four mods laid over it: overrides, a replaced folder, a
// dependency listed out of order, an archive, a file whose case differs from the game's, and files
// outside the manifest. The expected values below were produced by running the two games'
// independent reference implementations over exactly this tree, so a change that alters them
// changed what the checksum means, not just how it is computed.
const tree = "testdata/tree"

func treeMod(name, folder string, replace []string, deps ...string) Mod {
	return Mod{
		ID: folder, Name: name, RegistryID: "mod/" + folder + ".mod",
		Content: filepath.Join(tree, folder), ReplacePaths: replace, Dependencies: deps,
	}
}

func treeInput(algo Algorithm, mods ...Mod) Input {
	return Input{
		Algorithm:        algo,
		GameDir:          filepath.Join(tree, "game"),
		LauncherSettings: filepath.Join(tree, "game", "launcher-settings.json"),
		Mods:             mods,
	}
}

func TestGoldenAgainstTheReferenceImplementations(t *testing.T) {
	a := treeMod("ModA", "modA", nil)
	aReplacing := treeMod("ModA", "modA", []string{"common/sub"})
	b := treeMod("ModB", "modB", nil, "ModA")
	c := treeMod("ModC", "modC.zip", nil)
	d := treeMod("ModD", "modD", []string{"events/deep"})

	cases := []struct {
		name    string
		mods    []Mod
		stel    string
		stelN   int
		hoi     string
		hoiN    int
		mounted []string
	}{
		{"vanilla", nil, "6EC59486E4E926A818C7515AF3431E86", 12, "8E37BA515844D2B2B768B644D1825F9C", 12, nil},
		{"replace a subfolder", []Mod{aReplacing}, "CC0485CE52353DB56AAF1B0D729CE877", 12, "FE5B6D3983D11CD8190C3005EEFA16A5", 13, []string{"ModA"}},
		{"dependency listed first", []Mod{b, a}, "E9332512E428520209CB1834745EE859", 14, "7D60ECFE703C3E977B78BD96A4EC6F7E", 15, []string{"ModA", "ModB"}},
		{"directory, archive and a replaced folder", []Mod{a, c, d}, "A2B22FE67DBE908145B1D7C356849F93", 15, "092BEC7BB54F955090FA2CE4EDF414F6", 15, []string{"ModA", "ModC", "ModD"}},
		{"everything", []Mod{b, aReplacing, c, d}, "FF70BF3E88EE660298D3503D3A1A7A2A", 16, "7B4B3CF8DF33BC2BCF2030A2823E5C98", 17, []string{"ModA", "ModB", "ModC", "ModD"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Compute(context.Background(), treeInput(Stellaris, tc.mods...))
			if err != nil {
				t.Fatalf("Stellaris: %v", err)
			}
			if got.Full != tc.stel || got.Files != tc.stelN || got.Checksum != tc.stel[len(tc.stel)-4:] {
				t.Errorf("Stellaris: got %s (%d files), want %s (%d files)", got.Full, got.Files, tc.stel, tc.stelN)
			}
			if got.Salt != "Pegasus v1.2.3.0" {
				t.Errorf("Stellaris salt = %q", got.Salt)
			}
			if !reflect.DeepEqual(got.Order, tc.mounted) {
				t.Errorf("Stellaris mount order = %v, want %v", got.Order, tc.mounted)
			}

			got, err = Compute(context.Background(), treeInput(HOI4, tc.mods...))
			if err != nil {
				t.Fatalf("HOI4: %v", err)
			}
			if got.Full != tc.hoi || got.Files != tc.hoiN || got.Checksum != tc.hoi[len(tc.hoi)-4:] {
				t.Errorf("HOI4: got %s (%d files), want %s (%d files)", got.Full, got.Files, tc.hoi, tc.hoiN)
			}
			if got.Salt != "Pegasus v1.2.3.0.abcd" {
				t.Errorf("HOI4 salt = %q", got.Salt)
			}
			if !reflect.DeepEqual(got.Order, tc.mounted) {
				t.Errorf("HOI4 mount order = %v, want %v", got.Order, tc.mounted)
			}
		})
	}
}

func TestChecksumIsTheLastFourHexCharactersUpperCase(t *testing.T) {
	got, err := Compute(context.Background(), treeInput(Stellaris))
	if err != nil {
		t.Fatal(err)
	}
	if got.Checksum != "1E86" || got.Checksum != strings.ToUpper(got.Checksum) {
		t.Errorf("Checksum = %q, want 1E86", got.Checksum)
	}
}

func TestParseManifest(t *testing.T) {
	rules := ParseManifest("\xef\xbb\xbf\r\ndirectory \r\nname = \"common\"\r\nsub_directories = yes\r\nfile_extension = .txt\r\n\r\ndirectory\nname = map\nsub_directories = no\nfile_extension = .csv\n\ndirectory\nname = broken\n")
	want := []Rule{
		{Directory: "common", Extension: ".txt", Recursive: true},
		{Directory: "map", Extension: ".csv", Recursive: false},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("rules = %+v, want %+v", rules, want)
	}
}

func TestMissingContentIsReportedNotSkipped(t *testing.T) {
	in := treeInput(Stellaris, treeMod("ModA", "modA", nil), Mod{Name: "Gone", Content: filepath.Join(tree, "not-there")}, Mod{ID: "unnamed", Content: ""})
	_, err := Compute(context.Background(), in)
	var missing *MissingContentError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingContentError", err)
	}
	if !reflect.DeepEqual(missing.Mods, []string{"Gone", "unnamed"}) {
		t.Errorf("missing = %v", missing.Mods)
	}
}

func TestCancelledCalculationStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, algo := range []Algorithm{Stellaris, HOI4} {
		if _, err := Compute(ctx, treeInput(algo)); !errors.Is(err, context.Canceled) {
			t.Errorf("%s: err = %v, want context.Canceled", algo, err)
		}
	}
}

func TestUnknownAlgorithmAndMissingGameDir(t *testing.T) {
	if _, err := Compute(context.Background(), Input{Algorithm: "eu4", GameDir: "x"}); err == nil {
		t.Error("expected an error for an unknown algorithm")
	}
	if _, err := Compute(context.Background(), Input{Algorithm: Stellaris}); err == nil {
		t.Error("expected an error without a game folder")
	}
}

func TestSalts(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "launcher-settings.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got, err := stellarisSalt(write(`{"version":"Pegasus v4.4.6 (fdde)","rawVersion":"v4.4.6"}`)); err != nil || got != "Pegasus v4.4.6" {
		t.Errorf("stellaris salt = %q, %v", got, err)
	}
	// A raw version the version text does not repeat: the codename is what precedes the first number.
	if got, err := stellarisSalt(write(`{"version":"Orion 5.0.1 (abcd)","rawVersion":"9.9"}`)); err != nil || got != "Orion 9.9" {
		t.Errorf("fallback codename salt = %q, %v", got, err)
	}
	if _, err := stellarisSalt(write(`{"version":"Pegasus v4.4.6 (fdde)"}`)); err == nil {
		t.Error("expected an error without a rawVersion")
	}
	if got, err := hoi4Salt(write(`{"version":"Operation Postern v1.19.3.0.c01a (5632)","rawVersion":"1.19.3.0"}`)); err != nil || got != "Operation Postern v1.19.3.0.c01a" {
		t.Errorf("hoi4 salt = %q, %v", got, err)
	}
	if got, err := hoi4Salt(write(`{"version":"No suffix 1.0"}`)); err != nil || got != "No suffix 1.0" {
		t.Errorf("hoi4 salt without a suffix = %q, %v", got, err)
	}
	if _, err := hoi4Salt(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("expected an error for a missing launcher-settings.json")
	}
}

// Stellaris takes a mod's replace_path and dependencies from the descriptor.mod inside its
// folder when its registration does not have them.
func TestStellarisReadsTheNestedDescriptor(t *testing.T) {
	dir := t.TempDir()
	content := filepath.Join(dir, "content")
	if err := os.MkdirAll(filepath.Join(content, "common", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "descriptor.mod"), []byte("name=\"Nested\"\nreplace_path=\"common/sub\"\ndependencies={\n\t\"ModA\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := Mod{ID: "n", Content: content}
	a := treeMod("ModA", "modA", nil)

	got, err := Compute(context.Background(), treeInput(Stellaris, nested, a))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Order, []string{"ModA", "Nested"}) {
		t.Errorf("order = %v, want the nested dependency on ModA honoured", got.Order)
	}
	// Same result as spelling those fields out on the mod itself.
	explicit := Mod{ID: "n", Name: "Nested", Content: content, ReplacePaths: []string{"common/sub"}, Dependencies: []string{"ModA"}}
	want, err := Compute(context.Background(), treeInput(Stellaris, explicit, a))
	if err != nil {
		t.Fatal(err)
	}
	if got.Full != want.Full {
		t.Errorf("nested = %s, explicit = %s", got.Full, want.Full)
	}
}

func TestDependencyCycles(t *testing.T) {
	x := treeMod("X", "modA", nil, "Y")
	y := treeMod("Y", "modB", nil, "X")

	got, err := Compute(context.Background(), treeInput(Stellaris, x, y))
	if err != nil {
		t.Fatalf("Stellaris: %v", err)
	}
	if !reflect.DeepEqual(got.Order, []string{"X", "Y"}) || len(got.Warnings) == 0 {
		t.Errorf("Stellaris cycle: order %v, warnings %v; want the load order and a warning", got.Order, got.Warnings)
	}
	if _, err := Compute(context.Background(), treeInput(HOI4, x, y)); err == nil {
		t.Error("HOI4 cannot settle on an order for a cycle: expected an error")
	}
}

func TestMissingDependencyIsWarnedAbout(t *testing.T) {
	got, err := Compute(context.Background(), treeInput(Stellaris, treeMod("ModB", "modB", nil, "Not Enabled")))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "Not Enabled") {
		t.Errorf("warnings = %v", got.Warnings)
	}
}

// HOI4 reads only the first registration of a name; the second is ignored by the game, so it
// must not change the checksum either.
func TestHOI4IgnoresADuplicateNamedRegistration(t *testing.T) {
	modDir := t.TempDir()
	for name, body := range map[string]string{
		"a_first.mod":  "name=\"ModA\"\n",
		"b_second.mod": "name=\"ModA\"\n",
	} {
		if err := os.WriteFile(filepath.Join(modDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first := treeMod("ModA", "modA", nil)
	first.RegistryID = "mod/a_first.mod"
	second := treeMod("ModA", "modA", []string{"common/sub"})
	second.RegistryID = "mod/b_second.mod"

	withDup := treeInput(HOI4, first, second)
	withDup.ModDir = modDir
	got, err := Compute(context.Background(), withDup)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "b_second.mod") {
		t.Errorf("warnings = %v", got.Warnings)
	}

	alone := treeInput(HOI4, first)
	alone.ModDir = modDir
	want, err := Compute(context.Background(), alone)
	if err != nil {
		t.Fatal(err)
	}
	if got.Full != want.Full {
		t.Errorf("with the duplicate = %s, without = %s", got.Full, want.Full)
	}
}

func TestHOI4PathCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"common/z.txt", "common/a/x.txt", -1}, // a folder's own file before a subfolder's
		{"common/a/x.txt", "common/z.txt", 1},
		{"common/a.txt", "common/b.txt", -1},
		{"common/B.txt", "common/a.txt", -1}, // ordinal: upper case first
		{"common/a.txt", "common/a.txt", 0},
		{"common/a/x.txt", "common/a/y/z.txt", -1},
		{"common/\U0001F600.txt", "common/�.txt", -1}, // UTF-16 code units: a surrogate pair sorts before U+FFFD
	}
	for _, c := range cases {
		if got := hoi4PathCompare(c.a, c.b); got != c.want {
			t.Errorf("hoi4PathCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestStellarisRuleMatching(t *testing.T) {
	r := Rule{Directory: "map", Extension: ".txt"}
	for path, want := range map[string]bool{
		"map/a.txt": true, "map/sub/a.txt": false, "map/a.TXT": false, "map/": false, "other/a.txt": false, "map/a.csv": false,
	} {
		if got := r.matches(path); got != want {
			t.Errorf("non-recursive %q: %v, want %v", path, got, want)
		}
	}
	rec := Rule{Directory: "common", Extension: ".txt", Recursive: true}
	if !rec.matches("common/a/b/c.txt") {
		t.Error("a recursive rule should cover nested files")
	}
}

// HOI4 blanks the files of a replaced folder - including the replacing mod's own - and hashes a
// blank as an empty file, so what those files contain cannot change the checksum. Stellaris
// deletes the replaced files and mounts the mod's fresh, so there their content counts.
func TestReplacedFilesAreBlankInHOI4ButFreshInStellaris(t *testing.T) {
	copyTree := func(e2 string) Mod {
		dir := t.TempDir()
		for rel, body := range map[string]string{
			"events/deep/e2.txt": e2,
			"events/e9.txt":      "event nine from modD",
		} {
			p := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return Mod{ID: "d", Name: "ModD", Content: dir, ReplacePaths: []string{"events/deep"}}
	}
	one, two := copyTree("first"), copyTree("second")

	hoiOne, err := Compute(context.Background(), treeInput(HOI4, one))
	if err != nil {
		t.Fatal(err)
	}
	hoiTwo, err := Compute(context.Background(), treeInput(HOI4, two))
	if err != nil {
		t.Fatal(err)
	}
	if hoiOne.Full != hoiTwo.Full {
		t.Errorf("HOI4: a replaced file's content changed the checksum: %s vs %s", hoiOne.Full, hoiTwo.Full)
	}

	stelOne, err := Compute(context.Background(), treeInput(Stellaris, one))
	if err != nil {
		t.Fatal(err)
	}
	stelTwo, err := Compute(context.Background(), treeInput(Stellaris, two))
	if err != nil {
		t.Fatal(err)
	}
	if stelOne.Full == stelTwo.Full {
		t.Error("Stellaris: the replacing mod's own file should count")
	}

	if emptyMD5 != [16]byte{0xd4, 0x1d, 0x8c, 0xd9, 0x8f, 0x00, 0xb2, 0x04, 0xe9, 0x80, 0x09, 0x98, 0xec, 0xf8, 0x42, 0x7e} {
		t.Errorf("emptyMD5 = %x", emptyMD5)
	}
}
