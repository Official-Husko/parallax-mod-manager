package cache

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// A FileRecord is stored with its own compact binary encoding (GobEncode and
// GobDecode below) rather than by letting encoding/gob walk its fields.
//
// Why: a file's definitions all repeat the same Type, ModID and FilePath, and
// gob writes - and, worse, on reading allocates - a separate copy of each string
// for every single definition. A real modlist has close to a million
// definitions, so that was several million small string allocations every time
// a cache was read, and reading the caches was the largest cost left in a warm
// rescan. Here the repeated strings are stored once per file and shared by
// every definition that uses them, and all the definitions' IDs are read out of
// one buffer as substrings of a single string, so decoding a file allocates
// three things (the strings table, the ID buffer, the definitions slice) instead
// of three per definition.
//
// The layout is a sequence of varints and length-prefixed byte strings:
//
//	byte      recordCodecVersion
//	string    Path
//	varint    ModTimeUnixNano
//	varint    Size
//	8 bytes   Hash (little-endian)
//	string    ParseError
//	uvarint   number of distinct strings, then that many strings (the table:
//	          every Type, ModID and FilePath the definitions use, each once)
//	uvarint   number of definitions
//	uvarint   total length of all IDs, then those bytes back to back
//	per definition:
//	          uvarint Type index, uvarint ModID index, uvarint FilePath index,
//	          uvarint ID length, 8 bytes Hash, varint StartOffset, varint
//	          EndOffset, varint StartLine, varint EndLine, varint Order
//
// Everything read is bounds-checked: a truncated or corrupt record is an error,
// which Load treats as "no cache" (see FileStore.Load), never a panic and never
// a wrongly-decoded record.
const recordCodecVersion = 1

var errCorruptRecord = errors.New("cache: corrupt file record")

// GobEncode implements gob.GobEncoder.
func (r FileRecord) GobEncode() ([]byte, error) {
	table := []string{}
	index := map[string]uint64{}
	intern := func(s string) uint64 {
		if i, ok := index[s]; ok {
			return i
		}
		i := uint64(len(table))
		table = append(table, s)
		index[s] = i
		return i
	}
	type refs struct{ typ, mod, file uint64 }
	defRefs := make([]refs, len(r.Definitions))
	idBytes := 0
	for i, d := range r.Definitions {
		defRefs[i] = refs{intern(string(d.Type)), intern(d.ModID), intern(d.FilePath)}
		idBytes += len(d.ID)
	}

	out := make([]byte, 0, 64+len(r.Path)+len(r.ParseError)+idBytes+len(r.Definitions)*24)
	out = append(out, recordCodecVersion)
	out = appendString(out, r.Path)
	out = binary.AppendVarint(out, r.ModTimeUnixNano)
	out = binary.AppendVarint(out, r.Size)
	out = binary.LittleEndian.AppendUint64(out, r.Hash)
	out = appendString(out, r.ParseError)

	out = binary.AppendUvarint(out, uint64(len(table)))
	for _, s := range table {
		out = appendString(out, s)
	}
	out = binary.AppendUvarint(out, uint64(len(r.Definitions)))
	out = binary.AppendUvarint(out, uint64(idBytes))
	for _, d := range r.Definitions {
		out = append(out, d.ID...)
	}
	for i, d := range r.Definitions {
		out = binary.AppendUvarint(out, defRefs[i].typ)
		out = binary.AppendUvarint(out, defRefs[i].mod)
		out = binary.AppendUvarint(out, defRefs[i].file)
		out = binary.AppendUvarint(out, uint64(len(d.ID)))
		out = binary.LittleEndian.AppendUint64(out, d.Hash)
		out = binary.AppendVarint(out, int64(d.Span.StartOffset))
		out = binary.AppendVarint(out, int64(d.Span.EndOffset))
		out = binary.AppendVarint(out, int64(d.Span.StartLine))
		out = binary.AppendVarint(out, int64(d.Span.EndLine))
		out = binary.AppendVarint(out, int64(d.Order))
	}
	return out, nil
}

// GobDecode implements gob.GobDecoder.
func (r *FileRecord) GobDecode(data []byte) error {
	rd := recordReader{b: data}
	if v := rd.byte(); v != recordCodecVersion {
		return fmt.Errorf("cache: unsupported file record version %d", v)
	}
	var out FileRecord
	out.Path = rd.string()
	out.ModTimeUnixNano = rd.varint()
	out.Size = rd.varint()
	out.Hash = rd.uint64()
	out.ParseError = rd.string()

	nTable := rd.count(1)
	table := make([]string, nTable)
	for i := range table {
		table[i] = rd.string()
	}
	nDefs := rd.count(minDefinitionBytes)
	idTotal := rd.count(1)
	ids := rd.take(idTotal)
	// One allocation for every ID in the file: each definition's ID is a slice
	// of this string, not a string of its own.
	idText := string(ids)

	if rd.err != nil {
		return rd.err
	}
	if nDefs > 0 {
		out.Definitions = make([]definition.Definition, nDefs)
	}
	offset := 0
	for i := range out.Definitions {
		typ, mod, file := rd.uvarint(), rd.uvarint(), rd.uvarint()
		idLen := int(rd.uvarint())
		hash := rd.uint64()
		start, end := rd.varint(), rd.varint()
		startLine, endLine := rd.varint(), rd.varint()
		order := rd.varint()
		if rd.err != nil {
			return rd.err
		}
		if typ >= uint64(len(table)) || mod >= uint64(len(table)) || file >= uint64(len(table)) ||
			idLen < 0 || idLen > len(idText)-offset {
			return errCorruptRecord
		}
		out.Definitions[i] = definition.Definition{
			Type:     definition.Type(table[typ]),
			ID:       idText[offset : offset+idLen],
			ModID:    table[mod],
			FilePath: table[file],
			Hash:     hash,
			Span: definition.Span{
				StartOffset: int(start), EndOffset: int(end), StartLine: int(startLine), EndLine: int(endLine),
			},
			Order: int(order),
		}
		offset += idLen
	}
	if offset != len(idText) || rd.pos != len(rd.b) {
		return errCorruptRecord
	}
	*r = out
	return nil
}

// minDefinitionBytes is the fewest bytes one encoded definition can take (three
// one-byte indexes and a length, the 8-byte hash, five one-byte varints), used
// to reject a claimed count that couldn't possibly fit in what's left.
const minDefinitionBytes = 3 + 1 + 8 + 5

func appendString(dst []byte, s string) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(s)))
	return append(dst, s...)
}

// recordReader reads a record's bytes, remembering the first thing that went
// wrong so decoding code can read straight through and check once.
type recordReader struct {
	b   []byte
	pos int
	err error
}

func (r *recordReader) fail() {
	if r.err == nil {
		r.err = errCorruptRecord
	}
}

func (r *recordReader) byte() byte {
	if r.err != nil || r.pos >= len(r.b) {
		r.fail()
		return 0
	}
	c := r.b[r.pos]
	r.pos++
	return c
}

func (r *recordReader) uvarint() uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b[r.pos:])
	if n <= 0 {
		r.fail()
		return 0
	}
	r.pos += n
	return v
}

func (r *recordReader) varint() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.b[r.pos:])
	if n <= 0 {
		r.fail()
		return 0
	}
	r.pos += n
	return v
}

func (r *recordReader) uint64() uint64 {
	b := r.take(8)
	if r.err != nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (r *recordReader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.pos {
		r.fail()
		return nil
	}
	b := r.b[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *recordReader) string() string {
	n := r.uvarint()
	if r.err != nil || n > uint64(len(r.b)-r.pos) {
		r.fail()
		return ""
	}
	return string(r.take(int(n)))
}

// count reads a claimed element count and checks that many elements, each at
// least minBytes long, could actually fit in the rest of the record - so a
// corrupt count can't make the decoder allocate a huge slice.
func (r *recordReader) count(minBytes int) int {
	n := r.uvarint()
	if r.err != nil {
		return 0
	}
	if minBytes < 1 {
		minBytes = 1
	}
	if n > uint64(len(r.b)-r.pos)/uint64(minBytes)+1 {
		r.fail()
		return 0
	}
	return int(n)
}
