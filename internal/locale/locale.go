// Package locale parses Paradox localization .yml files - a simple,
// line-based key/value format distinct from Clausewitz script. See
// docs/script-format.md.
package locale

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Entry is one localization line: KEY:VERSION "value".
type Entry struct {
	Key     string
	Version int
	Value   string
	Line    int
	// StartOffset/EndOffset are the exact byte range of this entry's whole
	// raw source line (including its original leading whitespace, before
	// any trimming), within the []byte passed to Parse - excluding the
	// line's own trailing \r\n or \n. Lets a caller copy this entry's
	// exact original bytes verbatim (see internal/library.GeneratePatch),
	// the same way script definitions already do via definition.Span.
	StartOffset, EndOffset int
}

// Catalog is one parsed .yml file.
type Catalog struct {
	Language string // e.g. "english", from the "l_english:" header
	Entries  []Entry
}

var headerPrefix = []byte("l_")

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Parse parses one localization .yml file's contents. Paradox loc files are
// conventionally saved with a UTF-8 BOM; it's stripped if present.
//
// Lines are split manually (not via bufio.Scanner) so each entry's exact
// byte range within src can be recorded - see Entry.StartOffset/EndOffset.
// A trailing "\r" before "\n" is stripped from the line's content, same as
// bufio.ScanLines already did before this was written by hand.
func Parse(src []byte) (*Catalog, error) {
	trimmedSrc := bytes.TrimPrefix(src, utf8BOM)
	// baseOffset corrects StartOffset/EndOffset back to be relative to the
	// original src the caller passed in (3 when a BOM was present, else
	// 0) - a real bug once shipped: without this, every offset was
	// relative to the BOM-stripped view, silently misaligning any byte
	// range a caller (internal/library.GeneratePatch) sliced out of the
	// real on-disk file it read itself, since that file still has its
	// BOM. Confirmed against real Stellaris locale files, which do carry
	// one.
	baseOffset := len(src) - len(trimmedSrc)
	src = trimmedSrc

	cat := &Catalog{}
	lineNo := 0
	sawHeader := false
	for pos := 0; pos < len(src); {
		lineNo++
		start := pos
		end := len(src)
		if nl := bytes.IndexByte(src[pos:], '\n'); nl >= 0 {
			end = pos + nl
			pos = end + 1
		} else {
			pos = len(src)
		}
		contentEnd := end
		if contentEnd > start && src[contentEnd-1] == '\r' {
			contentEnd--
		}

		trimmed := strings.TrimSpace(string(src[start:contentEnd]))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if !sawHeader {
			lang, ok := parseHeader(trimmed)
			if !ok {
				return nil, fmt.Errorf("locale: expected language header (e.g. \"l_english:\") on line %d, got %q", lineNo, trimmed)
			}
			cat.Language = lang
			sawHeader = true
			continue
		}

		entry, err := parseEntryLine(trimmed, lineNo)
		if err != nil {
			return nil, err
		}
		entry.StartOffset = start + baseOffset
		entry.EndOffset = contentEnd + baseOffset
		cat.Entries = append(cat.Entries, entry)
	}
	if !sawHeader {
		return nil, fmt.Errorf("locale: empty file, expected a language header")
	}
	return cat, nil
}

func parseHeader(line string) (lang string, ok bool) {
	if !bytes.HasPrefix([]byte(line), headerPrefix) {
		return "", false
	}
	if !strings.HasSuffix(line, ":") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(line, string(headerPrefix)), ":"), true
}

// parseEntryLine parses "KEY:VERSION \"value\"" (the version number is
// optional in some files and defaults to 0 when omitted).
func parseEntryLine(line string, lineNo int) (Entry, error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return Entry{}, fmt.Errorf("locale: malformed entry on line %d: missing ':': %q", lineNo, line)
	}
	key := line[:colon]
	rest := line[colon+1:]

	// rest is "0 \"value\"" or "0\"value\"" or just "\"value\"".
	rest = strings.TrimLeft(rest, " \t")
	version := 0
	if rest != "" && rest[0] != '"' {
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == 0 {
			return Entry{}, fmt.Errorf("locale: malformed entry on line %d: expected version number or quoted value: %q", lineNo, line)
		}
		v, err := strconv.Atoi(rest[:i])
		if err != nil {
			return Entry{}, fmt.Errorf("locale: malformed version on line %d: %w", lineNo, err)
		}
		version = v
		rest = strings.TrimLeft(rest[i:], " \t")
	}

	value, err := parseQuoted(rest)
	if err != nil {
		return Entry{}, fmt.Errorf("locale: line %d: %w", lineNo, err)
	}

	return Entry{Key: key, Version: version, Value: value, Line: lineNo}, nil
}

func parseQuoted(s string) (string, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("expected a quoted value, got %q", s)
	}
	inner := s[1 : len(s)-1]
	return strings.ReplaceAll(inner, `\"`, `"`), nil
}
