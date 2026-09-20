package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

func sampleRecord() FileRecord {
	defs := []definition.Definition{
		{Type: "common/buildings", ID: "building_a", ModID: "mod_x", FilePath: "common/buildings/a.txt", Hash: math.MaxUint64, Span: definition.Span{StartOffset: 0, EndOffset: 120, StartLine: 1, EndLine: 9}, Order: 0},
		{Type: "common/buildings", ID: "building_b", ModID: "mod_x", FilePath: "common/buildings/a.txt", Hash: 0, Span: definition.Span{StartOffset: 121, EndOffset: 300, StartLine: 10, EndLine: 22}, Order: 1},
		// A different type and file inside one record, non-ASCII text, an empty
		// ID and negative values must all survive.
		{Type: "localisation/korean", ID: "키_이름", ModID: "mod_ü", FilePath: "localisation/korean/한국어.yml", Hash: 1 << 40, Span: definition.Span{StartOffset: -1, EndOffset: 1 << 33, StartLine: 0, EndLine: -5}, Order: -2},
		{Type: "", ID: "", ModID: "", FilePath: "", Hash: 7, Order: 1 << 20},
	}
	return FileRecord{Path: "common/buildings/a.txt", ModTimeUnixNano: 1_700_000_000_123_456_789, Size: 4096, Hash: 0xdeadbeefcafef00d, Definitions: defs, ParseError: "locale: 2 lines skipped, first: line 3"}
}

func roundTrip(t *testing.T, r FileRecord) FileRecord {
	t.Helper()
	data, err := r.GobEncode()
	if err != nil {
		t.Fatalf("GobEncode: %v", err)
	}
	var back FileRecord
	if err := back.GobDecode(data); err != nil {
		t.Fatalf("GobDecode: %v", err)
	}
	return back
}

func TestFileRecordRoundTripsExactly(t *testing.T) {
	want := sampleRecord()
	if got := roundTrip(t, want); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the record\n got %+v\nwant %+v", got, want)
	}
}

func TestFileRecordWithNoDefinitionsRoundTripsToNil(t *testing.T) {
	want := FileRecord{Path: "empty.txt", Size: 3, Hash: 9}
	got := roundTrip(t, want)
	if got.Definitions != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v (a nil Definitions slice, as before)", got, want)
	}
	if got := roundTrip(t, FileRecord{}); !reflect.DeepEqual(got, FileRecord{}) {
		t.Errorf("the zero record must round trip, got %+v", got)
	}
}

func TestDefinitionsOfOneFileShareTheirRepeatedStrings(t *testing.T) {
	r := FileRecord{Path: "p"}
	for i := 0; i < 1000; i++ {
		r.Definitions = append(r.Definitions, definition.Definition{Type: "common/x", ID: "id_" + string(rune('a'+i%26)), ModID: "the_mod", FilePath: "common/x/f.txt"})
	}
	data, err := r.GobEncode()
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(data, []byte("common/x/f.txt")); n != 1 {
		t.Errorf("the file path appears %d times in the encoding, want it stored once", n)
	}
	var back FileRecord
	if err := back.GobDecode(data); err != nil {
		t.Fatal(err)
	}
	// Shared, not copied: equal strings point at the same bytes.
	a, b := back.Definitions[0], back.Definitions[999]
	if unsafeStringData(a.ModID) != unsafeStringData(b.ModID) || unsafeStringData(a.FilePath) != unsafeStringData(b.FilePath) {
		t.Error("definitions of one file should share the same ModID and FilePath strings")
	}
	if len(data) > 30*len(r.Definitions) {
		t.Errorf("encoded %d definitions in %d bytes - the compact form should be under 30 bytes each (17 fixed bytes plus the ID)", len(r.Definitions), len(data))
	}
}

func TestDecodingATruncatedRecordFailsAtEveryLength(t *testing.T) {
	data, err := sampleRecord().GobEncode()
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < len(data); n++ {
		var r FileRecord
		if err := r.GobDecode(data[:n]); err == nil {
			t.Fatalf("decoding the first %d of %d bytes succeeded - a truncated record must be rejected", n, len(data))
		}
	}
	var r FileRecord
	if err := r.GobDecode(append(append([]byte{}, data...), 0)); err == nil {
		t.Error("trailing bytes after a record must be rejected")
	}
}

func TestDecodingCorruptBytesNeverPanics(t *testing.T) {
	data, err := sampleRecord().GobEncode()
	if err != nil {
		t.Fatal(err)
	}
	seed := uint64(12345)
	next := func() uint64 { seed = seed*6364136223846793005 + 1442695040888963407; return seed >> 33 }
	for i := 0; i < 20000; i++ {
		mut := append([]byte{}, data...)
		for k := 0; k < 1+int(next()%4); k++ {
			mut[next()%uint64(len(mut))] = byte(next())
		}
		var r FileRecord
		_ = r.GobDecode(mut) // an error or a (different) record are both fine; a panic or a huge allocation is not
	}
	huge := []byte{recordCodecVersion, 0, 0, 0}
	huge = append(huge, make([]byte, 8)...)
	huge = append(huge, 0)                            // ParseError ""
	huge = append(huge, 0xff, 0xff, 0xff, 0xff, 0x0f) // a table of ~4 billion strings
	var r FileRecord
	if err := r.GobDecode(huge); err == nil {
		t.Error("an impossible string count must be rejected, not allocated")
	}
	if err := (&FileRecord{}).GobDecode([]byte{99}); err == nil {
		t.Error("an unknown record version must be rejected")
	}
}

// If Definition (or its Span) ever gains a field, the codec silently drops it
// from the cache. This fails first, so the codec and recordCodecVersion get
// updated together.
func TestCodecCoversEveryDefinitionField(t *testing.T) {
	if n := reflect.TypeOf(definition.Definition{}).NumField(); n != 7 {
		t.Errorf("definition.Definition has %d fields; the cache codec knows 7 (Type, ID, ModID, FilePath, Hash, Span, Order) - update record_codec.go and bump recordCodecVersion and FormatVersion", n)
	}
	if n := reflect.TypeOf(definition.Span{}).NumField(); n != 4 {
		t.Errorf("definition.Span has %d fields; the cache codec knows 4", n)
	}
	if n := reflect.TypeOf(FileRecord{}).NumField(); n != 6 {
		t.Errorf("FileRecord has %d fields; the cache codec knows 6 (Path, ModTimeUnixNano, Size, Hash, Definitions, ParseError)", n)
	}
}

func TestModCacheStillRoundTripsThroughTheStore(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	mc := &ModCache{Version: FormatVersion, ParserVersion: ParserVersion, ModID: "m", GameKey: "g", Files: map[string]FileRecord{
		"a.txt": sampleRecord(),
		"b.txt": {Path: "b.txt", Size: 1},
	}}
	if err := store.Save(context.Background(), mc); err != nil {
		t.Fatal(err)
	}
	back, err := store.Load(context.Background(), "g", "m")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, mc) {
		t.Errorf("the cache changed on its way through the store\n got %+v\nwant %+v", back, mc)
	}
}

func TestACacheInTheOldGobLayoutIsIgnored(t *testing.T) {
	// A file from before this format: a ModCache whose records were plain gob
	// structs. It carries the old format version, so Load must not try to read
	// its records with the new codec.
	type oldRecord struct {
		Path string
		Size int64
	}
	type oldCache struct {
		Version       int
		ParserVersion int
		ModID         string
		GameKey       string
		Files         map[string]oldRecord
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(oldCache{Version: FormatVersion - 1, ParserVersion: ParserVersion, ModID: "m", GameKey: "g", Files: map[string]oldRecord{"a": {"a", 1}}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store := FileStore{Dir: dir}
	if err := writeRaw(dir, "g", "m", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), "g", "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 0 {
		t.Errorf("an old-layout cache must be treated as absent, got %+v", got.Files)
	}
}

func writeRaw(dir, gameKey, modID string, data []byte) error {
	path := FileStore{Dir: dir}.path(gameKey, modID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// unsafeStringData is the address of a string's bytes, to tell shared strings
// from copies.
func unsafeStringData(s string) uintptr {
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}
