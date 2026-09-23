package mod

import (
	"reflect"
	"testing"
)

func TestClassifySource(t *testing.T) {
	tests := []struct {
		filename string
		want     Source
	}{
		{"ugc_1830063425.mod", SourceWorkshop},
		{"pdx_00001.mod", SourceParadoxLauncher},
		{"my_local_mod.mod", SourceLocal},
		{"/some/dir/ugc_42.mod", SourceWorkshop},
		{"descriptor.mod", SourceLocal},
		{"loverslab_31347.mod", SourceLoversLab},
		{"/some/dir/loverslab_31347.mod", SourceLoversLab},
	}
	for _, tt := range tests {
		if got := ClassifySource(tt.filename); got != tt.want {
			t.Errorf("ClassifySource(%q) = %v, want %v", tt.filename, got, tt.want)
		}
	}
}

func TestParseClassicDescriptor(t *testing.T) {
	src := []byte(`
name="AI Species Limit"
path="path"
user_dir="dir"
replace_path="replace"
tags={
	"Gameplay"
	"Fixes"
}
picture="thumbnail.png"
supported_version="2.5.*"
remote_file_id="1830063425"
version = "version"
dependencies = {
	"fake"
}
`)
	d, err := ParseDescriptor(src, DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v", err)
	}

	want := Descriptor{
		Name:             "AI Species Limit",
		Path:             "path",
		UserDir:          "dir",
		ReplacePath:      []string{"replace"},
		Tags:             []string{"Gameplay", "Fixes"},
		Picture:          "thumbnail.png",
		SupportedVersion: "2.5.*",
		RemoteFileID:     "1830063425",
		Version:          "version",
		Dependencies:     []string{"fake"},
	}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("ParseDescriptor =\n%+v\nwant\n%+v", d, want)
	}
}

func TestParseClassicDescriptorRepeatedReplacePath(t *testing.T) {
	src := []byte(`
name = "Multi Replace"
replace_path = "common/buildings"
replace_path = "common/species_classes"
`)
	d, err := ParseDescriptor(src, DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v", err)
	}
	want := []string{"common/buildings", "common/species_classes"}
	if !reflect.DeepEqual(d.ReplacePath, want) {
		t.Errorf("ReplacePath = %v, want %v", d.ReplacePath, want)
	}
}

func TestWriteClassicDescriptorRoundTrips(t *testing.T) {
	d := Descriptor{
		Name:             "AI Species Limit",
		Path:             "/absolute/content/path",
		UserDir:          "dir",
		ReplacePath:      []string{"common/buildings", "common/species_classes"},
		Tags:             []string{"Gameplay", "Fixes"},
		Picture:          "thumbnail.png",
		SupportedVersion: "2.5.*",
		RemoteFileID:     "1830063425",
		Version:          "version",
		Dependencies:     []string{"fake"},
	}

	data := WriteClassicDescriptor(d)
	got, err := ParseDescriptor(data, DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor(WriteClassicDescriptor(d)): %v\ndata:\n%s", err, data)
	}
	if !reflect.DeepEqual(got, d) {
		t.Errorf("round-tripped =\n%+v\nwant\n%+v\ndata:\n%s", got, d, data)
	}
}

func TestWriteClassicDescriptorEscapesQuotesAndBackslashes(t *testing.T) {
	d := Descriptor{Name: `Say "hi" \ bye`}
	data := WriteClassicDescriptor(d)
	got, err := ParseDescriptor(data, DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v\ndata:\n%s", err, data)
	}
	if got.Name != d.Name {
		t.Errorf("Name = %q, want %q", got.Name, d.Name)
	}
}

func TestWriteClassicDescriptorOmitsEmptyFields(t *testing.T) {
	data := WriteClassicDescriptor(Descriptor{Name: "Bare"})
	got, err := ParseDescriptor(data, DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v\ndata:\n%s", err, data)
	}
	want := Descriptor{Name: "Bare"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseJSONDescriptorV1FlatRelationships(t *testing.T) {
	src := []byte(`{
		"id": "01234567-89ab-cdef-0123-456789abcdef",
		"name": "AI Species Limit",
		"path": "mod/ai-species-limit",
		"version": "1.0.0",
		"supported_game_version": "1.5.*",
		"tags": ["Gameplay"],
		"short_description": "Raises the AI species cap.",
		"relationships": ["Some Other Mod", "Another Dependency"]
	}`)
	d, err := ParseDescriptor(src, DescriptorJSONv1)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v", err)
	}
	if d.ID != "01234567-89ab-cdef-0123-456789abcdef" {
		t.Errorf("ID = %q", d.ID)
	}
	if d.Name != "AI Species Limit" || d.Path != "mod/ai-species-limit" {
		t.Errorf("Name/Path = %q/%q", d.Name, d.Path)
	}
	if d.SupportedVersion != "1.5.*" {
		t.Errorf("SupportedVersion = %q", d.SupportedVersion)
	}
	wantDeps := []string{"Some Other Mod", "Another Dependency"}
	if !reflect.DeepEqual(d.Dependencies, wantDeps) {
		t.Errorf("Dependencies = %v, want %v", d.Dependencies, wantDeps)
	}
}

func TestParseJSONDescriptorV2ObjectRelationships(t *testing.T) {
	src := []byte(`{
		"id": "abc",
		"name": "AI Species Limit",
		"relationships": [
			{ "resource_type": "mod", "display_name": "Some Other Mod" }
		],
		"game_custom_data": {
			"replace_path": ["common/species_classes"]
		}
	}`)
	d, err := ParseDescriptor(src, DescriptorJSONv2)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v", err)
	}
	wantDeps := []string{"Some Other Mod"}
	if !reflect.DeepEqual(d.Dependencies, wantDeps) {
		t.Errorf("Dependencies = %v, want %v", d.Dependencies, wantDeps)
	}
	if d.GameCustomData == nil {
		t.Fatal("expected GameCustomData to be populated")
	}
	if _, ok := d.GameCustomData["replace_path"]; !ok {
		t.Errorf("GameCustomData = %v, missing replace_path", d.GameCustomData)
	}
}

func TestParseDescriptorMalformedClassicErrors(t *testing.T) {
	_, err := ParseDescriptor([]byte(`name = "unterminated`), DescriptorClassic)
	if err == nil {
		t.Fatal("expected an error for a malformed classic descriptor")
	}
}

func TestParseDescriptorMalformedJSONErrors(t *testing.T) {
	_, err := ParseDescriptor([]byte(`{ not valid json`), DescriptorJSONv1)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestParseDescriptorUnknownTypeErrors(t *testing.T) {
	_, err := ParseDescriptor([]byte(`{}`), DescriptorType(99))
	if err == nil {
		t.Fatal("expected an error for an unknown descriptor type")
	}
}
