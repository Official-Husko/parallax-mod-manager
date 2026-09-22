package modedit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func parse(t *testing.T, text string) mod.Descriptor {
	t.Helper()
	d, err := mod.ParseDescriptor([]byte(strings.TrimPrefix(text, "\xef\xbb\xbf")), mod.DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v\n%s", err, text)
	}
	return d
}

func plan1(t *testing.T, text string, want Fields, picture string) FileEdit {
	t.Helper()
	edits, err := Plan([]File{{Path: "/x/descriptor.mod", Kind: KindDescriptor, Exists: true, Text: text}}, want, picture)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return edits[0]
}

const stub = `name="Lustful Void"
tags={
	"No more tags!"
}
supported_version="v4.0.*"
path="/library/Lustful Void"
`

func TestAnEditChangesOnlyWhatWasAsked(t *testing.T) {
	e := plan1(t, stub, Fields{Name: "Lustful Void 2", SupportedVersion: "v4.0.*", Tags: []string{"No more tags!"}}, "")
	want := strings.Replace(stub, `name="Lustful Void"`, `name="Lustful Void 2"`, 1)
	if e.After != want {
		t.Errorf("After:\n%s\nwant:\n%s", e.After, want)
	}
	if !e.Changed() {
		t.Error("Changed() = false")
	}
}

func TestNothingAskedMeansNothingChanges(t *testing.T) {
	e := plan1(t, stub, Fields{Name: "Lustful Void", SupportedVersion: "v4.0.*", Tags: []string{"No more tags!"}}, "")
	if e.Changed() || e.After != stub {
		t.Errorf("an unchanged edit rewrote the file:\n%s", e.After)
	}
}

func TestUnknownKeysPathAndCommentsAreKept(t *testing.T) {
	text := "# my mod\nname=\"A\" # the name\nremote_file_id=\"123\"\npath=\"/somewhere\"\nuser_dir=\"a_dir\"\nsomething_else={ x=1 y=2 }\n"
	e := plan1(t, text, Fields{Name: "B", Version: "1.2"}, "")
	for _, keep := range []string{"# my mod", "# the name", `remote_file_id="123"`, `path="/somewhere"`, `user_dir="a_dir"`, "something_else={ x=1 y=2 }"} {
		if !strings.Contains(e.After, keep) {
			t.Errorf("%q was lost:\n%s", keep, e.After)
		}
	}
	d := parse(t, e.After)
	if d.Name != "B" || d.Version != "1.2" || d.Path != "/somewhere" || d.RemoteFileID != "123" {
		t.Errorf("parsed = %+v", d)
	}
}

func TestEachFieldCanBeAddedChangedAndRemoved(t *testing.T) {
	full := Fields{
		Name: "Full", Version: "1.0", SupportedVersion: "v4.*",
		Tags:         []string{"Gameplay", "Fixes"},
		Dependencies: []string{"Other Mod", "Third"},
		ReplacePaths: []string{"common/buildings", "events"},
	}
	// Added to a file that has only a name.
	added := plan1(t, "name=\"Full\"\n", full, "thumbnail.png")
	d := parse(t, added.After)
	if d.Version != "1.0" || d.SupportedVersion != "v4.*" || d.Picture != "thumbnail.png" ||
		!reflect.DeepEqual(d.Tags, full.Tags) || !reflect.DeepEqual(d.Dependencies, full.Dependencies) || !reflect.DeepEqual(d.ReplacePath, full.ReplacePaths) {
		t.Fatalf("added: %+v\n%s", d, added.After)
	}

	// Changed.
	changed := full
	changed.Version, changed.SupportedVersion = "2.0", "v4.4.*"
	changed.Tags = []string{"Graphics"}
	changed.Dependencies = []string{"Other Mod"}
	changed.ReplacePaths = []string{"gfx"}
	e := plan1(t, added.After, changed, "")
	d = parse(t, e.After)
	if d.Version != "2.0" || d.SupportedVersion != "v4.4.*" || !reflect.DeepEqual(d.Tags, changed.Tags) || !reflect.DeepEqual(d.Dependencies, changed.Dependencies) || !reflect.DeepEqual(d.ReplacePath, changed.ReplacePaths) || d.Picture != "thumbnail.png" {
		t.Fatalf("changed: %+v\n%s", d, e.After)
	}

	// Removed.
	bare := Fields{Name: "Full"}
	e = plan1(t, e.After, bare, "")
	d = parse(t, e.After)
	if d.Version != "" || d.SupportedVersion != "" || len(d.Tags) != 0 || len(d.Dependencies) != 0 || len(d.ReplacePath) != 0 {
		t.Fatalf("removed: %+v\n%s", d, e.After)
	}
	if strings.Contains(e.After, "tags") || strings.Contains(e.After, "replace_path") || strings.Contains(e.After, "dependencies") {
		t.Errorf("empty fields left keys behind:\n%s", e.After)
	}
}

func TestByteOrderMarkAndLineEndingsAreKept(t *testing.T) {
	crlf := "\xef\xbb\xbfname=\"A\"\r\ntags={\r\n\t\"x\"\r\n}\r\npath=\"/p\"\r\n"
	e := plan1(t, crlf, Fields{Name: "B", Tags: []string{"x", "y"}, Dependencies: []string{"D"}}, "")
	if !strings.HasPrefix(e.After, "\xef\xbb\xbf") {
		t.Error("the byte order mark was lost")
	}
	if strings.Contains(strings.ReplaceAll(e.After, "\r\n", ""), "\n") {
		t.Errorf("a bare LF crept into a CRLF file:\n%q", e.After)
	}
	if d := parse(t, e.After); d.Name != "B" || !reflect.DeepEqual(d.Tags, []string{"x", "y"}) || !reflect.DeepEqual(d.Dependencies, []string{"D"}) || d.Path != "/p" {
		t.Errorf("parsed = %+v", d)
	}
}

func TestAwkwardTextIsQuotedSoItReadsBack(t *testing.T) {
	name := `The "Best" Mod \ 100% $1 ${x} ünïcödé ✓`
	e := plan1(t, "name=\"A\"\n", Fields{Name: name, Tags: []string{`a "quoted" tag`, "brace } inside", "back\\slash"}}, "")
	d := parse(t, e.After)
	if d.Name != name || !reflect.DeepEqual(d.Tags, []string{`a "quoted" tag`, "brace } inside", "back\\slash"}) {
		t.Errorf("did not read back: %+v\n%s", d, e.After)
	}
}

func TestListsAreTidiedBlankAndDuplicateItemsDropped(t *testing.T) {
	e := plan1(t, "name=\"A\"\n", Fields{Name: "  A  ", Tags: []string{" x ", "", "x", "y"}, ReplacePaths: []string{"", " a ", "a"}}, "")
	d := parse(t, e.After)
	if d.Name != "A" || !reflect.DeepEqual(d.Tags, []string{"x", "y"}) || !reflect.DeepEqual(d.ReplacePath, []string{"a"}) {
		t.Errorf("parsed = %+v", d)
	}
}

func TestSeveralBlocksAndDuplicateKeysAreCollapsed(t *testing.T) {
	text := "name=\"A\"\nname=\"B\"\ntags={\n\t\"one\"\n}\ntags={\n\t\"two\"\n}\nreplace_path=\"a\"\nreplace_path=\"b\"\n"
	e := plan1(t, text, Fields{Name: "C", Tags: []string{"z"}, ReplacePaths: []string{"c"}}, "")
	d := parse(t, e.After)
	if d.Name != "C" || !reflect.DeepEqual(d.Tags, []string{"z"}) || !reflect.DeepEqual(d.ReplacePath, []string{"c"}) {
		t.Errorf("parsed = %+v\n%s", d, e.After)
	}
	if strings.Count(e.After, "tags") != 1 || strings.Count(e.After, "replace_path") != 1 {
		t.Errorf("duplicates were kept:\n%s", e.After)
	}
}

func TestAMissingFileIsCreatedWithoutAPath(t *testing.T) {
	edits, err := Plan([]File{{Path: "/m/descriptor.mod", Kind: KindDescriptor, Exists: false}}, Fields{Name: "New", Version: "1", SupportedVersion: "v4.*", Tags: []string{"Gameplay"}}, "thumbnail.png")
	if err != nil {
		t.Fatal(err)
	}
	e := edits[0]
	if !e.Create || e.Before != "" {
		t.Errorf("edit = %+v", e)
	}
	d := parse(t, e.After)
	if d.Name != "New" || d.Path != "" || d.Picture != "thumbnail.png" || !reflect.DeepEqual(d.Tags, []string{"Gameplay"}) {
		t.Errorf("parsed = %+v\n%s", d, e.After)
	}
}

func TestEveryFileOfAModIsPlanned(t *testing.T) {
	files := []File{
		{Path: "/m/descriptor.mod", Kind: KindDescriptor, Exists: true, Text: "name=\"A\"\nversion=\"1\"\n"},
		{Path: "/u/mod/a.mod", Kind: KindStub, Exists: true, Text: "name=\"A\"\npath=\"/m\"\n"},
	}
	edits, err := Plan(files, Fields{Name: "B", Version: "1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 2 || !strings.Contains(edits[0].After, `name="B"`) || !strings.Contains(edits[1].After, `name="B"`) || !strings.Contains(edits[1].After, `path="/m"`) {
		t.Errorf("edits = %+v", edits)
	}
	if edits[0].Kind != KindDescriptor || edits[1].Kind != KindStub {
		t.Errorf("kinds = %q %q", edits[0].Kind, edits[1].Kind)
	}
}

func TestWhatCannotBeWrittenIsRefused(t *testing.T) {
	file := []File{{Path: "/x", Kind: KindStub, Exists: true, Text: "name=\"A\"\n"}}
	for name, want := range map[string]Fields{
		"no name":            {Name: "  "},
		"name with newline":  {Name: "a\nb"},
		"tag with newline":   {Name: "a", Tags: []string{"x\ny"}},
		"dep with carriage":  {Name: "a", Dependencies: []string{"x\ry"}},
		"version w newline":  {Name: "a", Version: "1\n2"},
		"replace w newline":  {Name: "a", ReplacePaths: []string{"a\nb"}},
		"supported w return": {Name: "a", SupportedVersion: "v4\r*"},
	} {
		if _, err := Plan(file, want, ""); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := Plan(file, Fields{Name: "a"}, "a\nb.png"); err == nil {
		t.Error("a picture name with a newline should be refused")
	}
}

// A file the parser cannot read is never written over with something that also would not read.
func TestAnUnreadableFileIsNotWrittenAsIs(t *testing.T) {
	_, err := Plan([]File{{Path: "/x", Kind: KindStub, Exists: true, Text: "name=\"A\"\nbroken={ \"unterminated\n"}}, Fields{Name: "B"}, "")
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("err = %v, want a refusal that says nothing was changed", err)
	}
}

func TestWarnings(t *testing.T) {
	if w := (Fields{Name: "a", SupportedVersion: "v4.*"}).Warnings(); len(w) != 0 {
		t.Errorf("a normal version warned: %v", w)
	}
	for _, ok := range []string{"v4.4.6", "4.*", "v3.9.*", "1.19.*", "v4"} {
		if w := (Fields{SupportedVersion: ok}).Warnings(); len(w) != 0 {
			t.Errorf("%q warned: %v", ok, w)
		}
	}
	for _, bad := range []string{"latest", "v4.x", "four"} {
		if w := (Fields{SupportedVersion: bad}).Warnings(); len(w) != 1 {
			t.Errorf("%q gave %v, want a warning", bad, w)
		}
	}
}

func TestSetScalarWithADollarSignInTheValue(t *testing.T) {
	e := plan1(t, "name=\"A\"\n", Fields{Name: "$1 and ${1} and $$"}, "")
	if d := parse(t, e.After); d.Name != "$1 and ${1} and $$" {
		t.Errorf("name = %q\n%s", d.Name, e.After)
	}
}
