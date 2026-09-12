// Package locale parses Paradox localization .yml files - a simple,
// line-based key/value format distinct from Clausewitz script. See
// docs/script-format.md.
package locale

import (
	"bufio"
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
func Parse(src []byte) (*Catalog, error) {
	src = bytes.TrimPrefix(src, utf8BOM)

	cat := &Catalog{}
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	lineNo := 0
	sawHeader := false
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

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
		cat.Entries = append(cat.Entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("locale: %w", err)
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
