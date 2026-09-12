// Package jsonc reads JSON-with-Comments: plain JSON plus // line comments,
// /* */ block comments, and trailing commas before a closing } or ]. This
// project's on-disk data files (see CLAUDE.md) use this format so they stay
// hand-editable with an explanation next to every field, unlike the plain
// JSON internal/cache and internal/playset already write for their own
// disposable/generated data.
package jsonc

import (
	"bytes"
	"encoding/json"
)

// Strip removes comments and trailing commas from JSONC data, respecting
// string literals throughout (a // or /* inside a JSON string value, or a
// comma inside one, is left completely alone), returning bytes
// encoding/json can parse directly. Newlines are preserved wherever
// possible so a downstream JSON error still points at a sensible line.
func Strip(data []byte) []byte {
	return removeTrailingCommas(stripComments(data))
}

// Unmarshal parses JSONC by stripping comments and trailing commas, then
// delegating to encoding/json.Unmarshal.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(Strip(data), v)
}

func stripComments(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))

	inString := false
	i := 0
	for i < len(data) {
		c := data[i]

		if inString {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(data) {
				out.WriteByte(data[i+1])
				i += 2
				continue
			}
			if c == '"' {
				inString = false
			}
			i++
			continue
		}

		if c == '"' {
			inString = true
			out.WriteByte(c)
			i++
			continue
		}

		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			i += 2
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue // the '\n' itself (if any) is written on the next loop iteration
		}

		if c == '/' && i+1 < len(data) && data[i+1] == '*' {
			i += 2
			for i < len(data) && !(data[i] == '*' && i+1 < len(data) && data[i+1] == '/') {
				if data[i] == '\n' {
					out.WriteByte('\n')
				}
				i++
			}
			i += 2 // skip the closing "*/"
			continue
		}

		out.WriteByte(c)
		i++
	}

	return out.Bytes()
}

func removeTrailingCommas(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))

	inString := false
	i := 0
	for i < len(data) {
		c := data[i]

		if inString {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(data) {
				out.WriteByte(data[i+1])
				i += 2
				continue
			}
			if c == '"' {
				inString = false
			}
			i++
			continue
		}

		if c == '"' {
			inString = true
			out.WriteByte(c)
			i++
			continue
		}

		if c == ',' {
			j := i + 1
			for j < len(data) && isJSONWhitespace(data[j]) {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				i++ // drop the comma, keep the whitespace that follows it
				continue
			}
		}

		out.WriteByte(c)
		i++
	}

	return out.Bytes()
}

func isJSONWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
