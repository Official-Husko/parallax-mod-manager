// Package modedit edits the descriptor files and the thumbnail of a mod the person made.
//
// A classic-format mod is described by up to two files this app knows: a descriptor.mod inside the
// mod's own folder (which may not exist) and the stub in the game's mod folder that the game itself
// reads. An edit has to reach every one that exists, or the game keeps reading the old values, so the
// planning here works on a list of files.
//
// Files are edited in place, not regenerated: everything an edit does not name (other keys, path=,
// remote_file_id, comments, the byte order mark, the line endings) stays exactly as it was. Every
// result is read back with the descriptor parser before anything is written; an edit that would not
// read back as asked is refused.
package modedit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// Kinds of descriptor file.
const (
	KindDescriptor = "descriptor" // descriptor.mod inside the mod's folder
	KindStub       = "stub"       // <id>.mod in the game's mod folder
)

// Fields are the descriptor values the editor changes. Picture is separate (see Plan).
type Fields struct {
	Name             string
	Version          string
	SupportedVersion string
	Tags             []string
	Dependencies     []string
	ReplacePaths     []string
}

// File is one descriptor file as it is now.
type File struct {
	Path   string
	Kind   string
	Exists bool
	// Text is the current contents; empty when the file does not exist yet.
	Text string
}

// FileEdit is what a plan does to one file.
type FileEdit struct {
	Path   string
	Kind   string
	Create bool
	Before string
	After  string
}

// Changed says whether the edit alters the file.
func (e FileEdit) Changed() bool { return e.Before != e.After }

// Normalized returns f with its lists tidied: blank items dropped, each item trimmed, duplicates
// removed (first kept), and the scalars trimmed.
func (f Fields) Normalized() Fields {
	f.Name = strings.TrimSpace(f.Name)
	f.Version = strings.TrimSpace(f.Version)
	f.SupportedVersion = strings.TrimSpace(f.SupportedVersion)
	f.Tags = tidyList(f.Tags)
	f.Dependencies = tidyList(f.Dependencies)
	f.ReplacePaths = tidyList(f.ReplacePaths)
	return f
}

func tidyList(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Validate refuses what cannot be written: a mod needs a name, and no value may contain a line break.
func (f Fields) Validate() error {
	if f.Name == "" {
		return fmt.Errorf("modedit: a mod needs a name")
	}
	check := func(what, v string) error {
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("modedit: %s cannot contain a line break", what)
		}
		return nil
	}
	for _, s := range []struct{ what, v string }{{"the name", f.Name}, {"the version", f.Version}, {"the supported version", f.SupportedVersion}} {
		if err := check(s.what, s.v); err != nil {
			return err
		}
	}
	for _, list := range []struct {
		what  string
		items []string
	}{{"a tag", f.Tags}, {"a dependency", f.Dependencies}, {"a replace path", f.ReplacePaths}} {
		for _, v := range list.items {
			if err := check(list.what, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// Warnings lists things worth telling the person that do not stop the edit.
func (f Fields) Warnings() []string {
	var w []string
	if f.SupportedVersion != "" && !SupportedVersionShape.MatchString(f.SupportedVersion) {
		w = append(w, fmt.Sprintf("The supported version %q is not shaped like the game's versions (for example v4.*), so the game may treat the mod as made for another version.", f.SupportedVersion))
	}
	return w
}

// SupportedVersionShape is what a supported_version value should look like
// (e.g. "v4.*", "1.2.3"). Exported so internal/modcheck can flag the same
// shape problem for a mod's *saved* descriptor, not just a field being
// edited here.
var SupportedVersionShape = regexp.MustCompile(`^v?\d+(\.(\d+|\*))*$`)

// Plan works out what changes in each file for the wanted fields (and picture, when it is not
// empty; "" leaves picture= as it is). A file that does not exist is created (with every field, and
// no path=), which callers ask for by passing it with Exists false. The plan reports each file even
// when it is unchanged.
func Plan(files []File, want Fields, picture string) ([]FileEdit, error) {
	want = want.Normalized()
	if err := want.Validate(); err != nil {
		return nil, err
	}
	if strings.ContainsAny(picture, "\r\n") {
		return nil, fmt.Errorf("modedit: the picture name cannot contain a line break")
	}

	var edits []FileEdit
	for _, f := range files {
		var after string
		if f.Exists {
			after = editText(f.Text, want, picture)
		} else {
			after = newDescriptor(want, picture)
		}
		if err := verify(after, want, picture); err != nil {
			return nil, fmt.Errorf("modedit: the edit to %s would not read back as asked (%v); nothing was changed", f.Path, err)
		}
		edits = append(edits, FileEdit{Path: f.Path, Kind: f.Kind, Create: !f.Exists, Before: f.Text, After: after})
	}
	return edits, nil
}

// verify parses text and checks it holds exactly the wanted values.
func verify(text string, want Fields, picture string) error {
	d, err := mod.ParseDescriptor([]byte(strings.TrimPrefix(text, "\xef\xbb\xbf")), mod.DescriptorClassic)
	if err != nil {
		return err
	}
	if d.Name != want.Name || d.Version != want.Version || d.SupportedVersion != want.SupportedVersion {
		return fmt.Errorf("name, version or supported version differ")
	}
	if !equalLists(tidyList(d.Tags), want.Tags) || !equalLists(tidyList(d.Dependencies), want.Dependencies) || !equalLists(tidyList(d.ReplacePath), want.ReplacePaths) {
		return fmt.Errorf("a list differs")
	}
	if picture != "" && d.Picture != picture {
		return fmt.Errorf("the picture differs")
	}
	return nil
}

func equalLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newDescriptor writes a descriptor.mod from scratch: the fields, no path= (a descriptor inside
// the mod's own folder does not name its folder).
func newDescriptor(want Fields, picture string) string {
	return string(mod.WriteClassicDescriptor(mod.Descriptor{
		Name:             want.Name,
		Version:          want.Version,
		SupportedVersion: want.SupportedVersion,
		Picture:          picture,
		Tags:             want.Tags,
		Dependencies:     want.Dependencies,
		ReplacePath:      want.ReplacePaths,
	}))
}

// editText applies want to text in place.
func editText(text string, want Fields, picture string) string {
	bom := ""
	if strings.HasPrefix(text, "\xef\xbb\xbf") {
		bom, text = "\xef\xbb\xbf", strings.TrimPrefix(text, "\xef\xbb\xbf")
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}

	// What the file says now, so a field that already matches is left byte for byte alone.
	cur, _ := mod.ParseDescriptor([]byte(text), mod.DescriptorClassic)

	if cur.Name != want.Name {
		text = setScalar(text, "name", want.Name, false, eol)
	}
	if cur.Version != want.Version {
		text = setScalar(text, "version", want.Version, true, eol)
	}
	if cur.SupportedVersion != want.SupportedVersion {
		text = setScalar(text, "supported_version", want.SupportedVersion, true, eol)
	}
	if picture != "" && cur.Picture != picture {
		text = setScalar(text, "picture", picture, false, eol)
	}
	if !equalLists(tidyList(cur.Tags), want.Tags) {
		text = setBlock(text, "tags", want.Tags, eol)
	}
	if !equalLists(tidyList(cur.Dependencies), want.Dependencies) {
		text = setBlock(text, "dependencies", want.Dependencies, eol)
	}
	if !equalLists(tidyList(cur.ReplacePath), want.ReplacePaths) {
		text = setRepeated(text, "replace_path", want.ReplacePaths, eol)
	}
	return bom + text
}

// scalarLine matches key = value up to (not including) any comment after the value.
func scalarLine(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^([ \t]*` + regexp.QuoteMeta(key) + `[ \t]*=[ \t]*)("(?:[^"\\\r\n]|\\.)*"|[^\s#{}"]+)`)
}

// setScalar sets key="value" wherever the key is, adds it at the end when it is missing, and removes
// the line when value is empty and the key is optional.
func setScalar(text, key, value string, optional bool, eol string) string {
	re := scalarLine(key)
	if value == "" && optional {
		return removeLines(text, regexp.MustCompile(`(?m)^[ \t]*`+regexp.QuoteMeta(key)+`[ \t]*=[^\r\n]*(\r?\n|\z)`))
	}
	quoted := mod.QuoteClausewitz(value)
	if re.MatchString(text) {
		return re.ReplaceAllString(text, "${1}"+strings.ReplaceAll(quoted, "$", "$$"))
	}
	return appendLine(text, key+"="+quoted, eol)
}

// blockPattern matches key = { ... } (quoted items may contain braces).
func blockPattern(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(key) + `[ \t]*=[ \t]*\{(?:"(?:[^"\\]|\\.)*"|[^"}])*\}[ \t]*(\r?\n|\z)`)
}

// setBlock replaces the key's block with the items (the first block; any others are removed), adds
// one at the end when there was none, and removes it when there are no items.
func setBlock(text, key string, items []string, eol string) string {
	re := blockPattern(key)
	locs := re.FindAllStringSubmatchIndex(text, -1)
	rendered := ""
	if len(items) > 0 {
		var b strings.Builder
		b.WriteString(key + "={" + eol)
		for _, it := range items {
			b.WriteString("\t" + mod.QuoteClausewitz(it) + eol)
		}
		b.WriteString("}" + eol)
		rendered = b.String()
	}
	if len(locs) == 0 {
		if rendered == "" {
			return text
		}
		return appendRaw(text, rendered, eol)
	}
	var out strings.Builder
	last := 0
	for i, loc := range locs {
		out.WriteString(text[last:loc[0]])
		if i == 0 && rendered != "" {
			indent := text[loc[2]:loc[3]]
			out.WriteString(indent + rendered)
			// The match swallowed its own line ending; a block at the very end of the file with none
			// keeps the rendered one, which is harmless.
		}
		last = loc[1]
	}
	out.WriteString(text[last:])
	return out.String()
}

// setRepeated replaces every key="value" line with one per item, at the place of the first.
func setRepeated(text, key string, items []string, eol string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(key) + `[ \t]*=[^\r\n]*(\r?\n|\z)`)
	locs := re.FindAllStringIndex(text, -1)
	var lines strings.Builder
	for _, it := range items {
		lines.WriteString(key + "=" + mod.QuoteClausewitz(it) + eol)
	}
	if len(locs) == 0 {
		if len(items) == 0 {
			return text
		}
		return appendRaw(text, lines.String(), eol)
	}
	var out strings.Builder
	last := 0
	for i, loc := range locs {
		out.WriteString(text[last:loc[0]])
		if i == 0 {
			out.WriteString(lines.String())
		}
		last = loc[1]
	}
	out.WriteString(text[last:])
	return out.String()
}

// removeLines deletes every match of re (a pattern that includes its own line ending).
func removeLines(text string, re *regexp.Regexp) string {
	return re.ReplaceAllString(text, "")
}

// appendLine adds one line at the end of the text.
func appendLine(text, line, eol string) string {
	return appendRaw(text, line+eol, eol)
}

// appendRaw adds already line-ended text at the end, after a line ending if the text lacks one.
func appendRaw(text, add, eol string) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += eol
	}
	return text + add
}
