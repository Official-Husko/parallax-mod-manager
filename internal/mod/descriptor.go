package mod

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

// ParseDescriptor parses one mod descriptor file's content, dispatching on
// the game's configured descriptor format.
func ParseDescriptor(data []byte, kind DescriptorType) (Descriptor, error) {
	switch kind {
	case DescriptorClassic:
		return parseClassicDescriptor(data)
	case DescriptorJSONv1, DescriptorJSONv2:
		return parseJSONDescriptor(data)
	default:
		return Descriptor{}, fmt.Errorf("mod: unknown descriptor type %d", kind)
	}
}

// parseClassicDescriptor parses a Clausewitz-format descriptor.mod file. See
// docs/paradox-mod-format.md for the field table.
func parseClassicDescriptor(data []byte) (Descriptor, error) {
	f, err := script.Parse(data)
	if err != nil {
		return Descriptor{}, fmt.Errorf("mod: parsing classic descriptor: %w", err)
	}

	var d Descriptor
	for _, e := range f.Root.Entries {
		switch e.Key {
		case "name":
			d.Name = e.Value.Raw
		case "path", "archive":
			d.Path = e.Value.Raw
		case "version":
			d.Version = e.Value.Raw
		case "supported_version":
			d.SupportedVersion = e.Value.Raw
		case "remote_file_id":
			d.RemoteFileID = e.Value.Raw
		case "picture":
			d.Picture = e.Value.Raw
		case "user_dir":
			d.UserDir = e.Value.Raw
		case "replace_path":
			// Can appear multiple times, once per replaced path.
			d.ReplacePath = append(d.ReplacePath, e.Value.Raw)
		case "tags":
			d.Tags = append(d.Tags, blockStringList(e.Value)...)
		case "dependencies":
			d.Dependencies = append(d.Dependencies, blockStringList(e.Value)...)
		}
	}
	return d, nil
}

// blockStringList reads the bare scalar list items of a block value, e.g.
// tags = { "Gameplay" "Fixes" }.
func blockStringList(v script.Value) []string {
	if v.Kind != script.KindBlock || v.Block == nil {
		return nil
	}
	items := make([]string, 0, len(v.Block.Entries))
	for _, e := range v.Block.Entries {
		if e.Key == "" {
			items = append(items, e.Value.Raw)
		}
	}
	return items
}

// jsonDescriptor mirrors the Paradox Launcher JSON metadata shape (v1 and
// v2), permissively: v1 relationships are flat strings, v2 relationships are
// {resource_type, display_name} objects. See docs/paradox-mod-format.md.
type jsonDescriptor struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Path                 string            `json:"path"`
	Version              string            `json:"version"`
	SupportedGameVersion string            `json:"supported_game_version"`
	Tags                 []string          `json:"tags"`
	ShortDescription     string            `json:"short_description"`
	Relationships        []json.RawMessage `json:"relationships"`
	GameCustomData       map[string]any    `json:"game_custom_data"`
}

type jsonRelationshipV2 struct {
	ResourceType string `json:"resource_type"`
	DisplayName  string `json:"display_name"`
}

func parseJSONDescriptor(data []byte) (Descriptor, error) {
	var raw jsonDescriptor
	if err := json.Unmarshal(data, &raw); err != nil {
		return Descriptor{}, fmt.Errorf("mod: parsing JSON descriptor: %w", err)
	}

	d := Descriptor{
		ID:               raw.ID,
		Name:             raw.Name,
		Path:             raw.Path,
		Version:          raw.Version,
		SupportedVersion: raw.SupportedGameVersion,
		Tags:             raw.Tags,
		ShortDescription: raw.ShortDescription,
		GameCustomData:   raw.GameCustomData,
	}

	for _, rel := range raw.Relationships {
		// v1 shape: a flat string dependency name.
		var name string
		if err := json.Unmarshal(rel, &name); err == nil {
			d.Dependencies = append(d.Dependencies, name)
			continue
		}
		// v2 shape: {resource_type, display_name}.
		var v2 jsonRelationshipV2
		if err := json.Unmarshal(rel, &v2); err == nil && v2.DisplayName != "" {
			d.Dependencies = append(d.Dependencies, v2.DisplayName)
		}
	}

	return d, nil
}

// WriteClassicDescriptor serializes d as a classic Clausewitz descriptor.mod
// file (see docs/paradox-mod-format.md) - the inverse of
// parseClassicDescriptor, and round-trips every field that function reads.
// Used to mirror a Steam Workshop item's own self-contained descriptor.mod
// into a game's mod/ folder as its "path"-bearing stub when Steam or the
// Paradox Launcher hasn't created one yet (see internal/scan and
// docs/mod-sources.md) - not a general-purpose formatter, only ever needs to
// reproduce what that stub convention actually contains.
func WriteClassicDescriptor(d Descriptor) []byte {
	var b strings.Builder
	field := func(key, value string) {
		if value == "" {
			return
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(quoteClausewitz(value))
		b.WriteByte('\n')
	}
	block := func(key string, values []string) {
		if len(values) == 0 {
			return
		}
		b.WriteString(key)
		b.WriteString("={\n")
		for _, v := range values {
			b.WriteByte('\t')
			b.WriteString(quoteClausewitz(v))
			b.WriteByte('\n')
		}
		b.WriteString("}\n")
	}

	field("name", d.Name)
	field("path", d.Path)
	field("version", d.Version)
	field("supported_version", d.SupportedVersion)
	field("picture", d.Picture)
	block("tags", d.Tags)
	field("remote_file_id", d.RemoteFileID)
	field("user_dir", d.UserDir)
	// replace_path is parsed as a repeated scalar entry (one per replaced
	// path), not a block - see parseClassicDescriptor.
	for _, p := range d.ReplacePath {
		field("replace_path", p)
	}
	block("dependencies", d.Dependencies)

	return []byte(b.String())
}

// quoteClausewitz quotes s as a Clausewitz string literal, escaping the two
// characters the lexer treats specially inside one (see internal/script's
// lexString).
func quoteClausewitz(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
