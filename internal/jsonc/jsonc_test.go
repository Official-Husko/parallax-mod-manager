package jsonc

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalStripsLineComments(t *testing.T) {
	data := []byte(`{
		// a leading comment
		"name": "Stellaris", // trailing comment
		"count": 1
	}`)
	var got struct {
		Name  string
		Count int
	}
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Name != "Stellaris" || got.Count != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestUnmarshalStripsBlockComments(t *testing.T) {
	data := []byte(`{
		/* a block comment
		   spanning multiple lines */
		"name": "Stellaris" /* inline */
	}`)
	var got struct{ Name string }
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Name != "Stellaris" {
		t.Errorf("got %+v", got)
	}
}

func TestUnmarshalPreservesSlashesInsideStrings(t *testing.T) {
	data := []byte(`{"path": "http://example.com/a/*b*/c"}`)
	var got struct{ Path string }
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := "http://example.com/a/*b*/c"
	if got.Path != want {
		t.Errorf("Path = %q, want %q", got.Path, want)
	}
}

func TestUnmarshalHandlesEscapedQuotesInStrings(t *testing.T) {
	data := []byte(`{"quote": "she said \"// not a comment\""}`)
	var got struct{ Quote string }
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := `she said "// not a comment"`
	if got.Quote != want {
		t.Errorf("Quote = %q, want %q", got.Quote, want)
	}
}

func TestUnmarshalRemovesTrailingCommaBeforeCloseBrace(t *testing.T) {
	data := []byte(`{"a": 1, "b": 2,}`)
	var got map[string]int
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["a"] != 1 || got["b"] != 2 {
		t.Errorf("got %+v", got)
	}
}

func TestUnmarshalRemovesTrailingCommaBeforeCloseBracket(t *testing.T) {
	data := []byte(`{"list": [1, 2, 3,]}`)
	var got struct{ List []int }
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got.List) != 3 {
		t.Errorf("List = %v, want 3 elements", got.List)
	}
}

func TestUnmarshalPreservesCommaInsideString(t *testing.T) {
	data := []byte(`{"note": "a, b, c,"}`)
	var got struct{ Note string }
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := "a, b, c,"
	if got.Note != want {
		t.Errorf("Note = %q, want %q", got.Note, want)
	}
}

func TestUnmarshalErrorsOnGenuinelyMalformedJSON(t *testing.T) {
	data := []byte(`{"a": }`)
	var got map[string]any
	if err := Unmarshal(data, &got); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestStripIsValidJSONForEncodingJSON(t *testing.T) {
	data := []byte(`{
		// comment
		"a": [1, 2,], // another
		"b": {"c": 3,},
	}`)
	stripped := Strip(data)
	var v any
	if err := json.Unmarshal(stripped, &v); err != nil {
		t.Fatalf("json.Unmarshal(Strip(data)): %v\nstripped:\n%s", err, stripped)
	}
}
