package locale

import (
	"bytes"
	"sort"
	"strings"
)

// RenderFile renders entries (key -> value) as a full, valid Paradox
// localisation .yml file: the UTF-8 BOM real Paradox locale files carry
// (some non-English languages render incorrectly in-game without it -
// confirmed against a real Stellaris install, see
// internal/library.GeneratePatch's own patchContentFile, which established
// this same convention for the app's generated patch mod), the
// "l_<language>:" header line every loc file needs, and one "KEY:0
// "value"" line per entry, sorted by key for stable, diffable output.
//
// This is a genuinely different operation from patchContentFile: that
// function wraps already-formatted raw bytes copied verbatim from a real
// mod file (preserving an author's own original formatting exactly).
// RenderFile instead formats fresh entries from scratch (there are no
// "original bytes" for a machine-translated string) - built for
// internal/library's own translate_plan.go and translate_companion.go, but
// kept in this package since "produce a valid loc file" is this package's
// own concern, the direct counterpart to Parse.
func RenderFile(language string, entries map[string]string) []byte {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.Write(utf8BOM)
	buf.WriteString("l_")
	buf.WriteString(language)
	buf.WriteString(":\n")
	for _, key := range keys {
		buf.WriteString(" ")
		buf.WriteString(key)
		buf.WriteString(":0 \"")
		buf.WriteString(escapeValue(entries[key]))
		buf.WriteString("\"\n")
	}
	return buf.Bytes()
}

// escapeValue is parseQuoted's own inverse: a literal quote becomes \",
// and a real newline or carriage return (never valid inside one loc
// entry's single line) is flattened to a space rather than corrupting the
// line-based format - defensive against a translation service somehow
// returning a literal newline in its output, which none of the three
// real, confirmed response shapes this project has seen ever do, but
// nothing here should assume that forever.
func escapeValue(v string) string {
	v = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(v)
	return strings.ReplaceAll(v, `"`, `\"`)
}
