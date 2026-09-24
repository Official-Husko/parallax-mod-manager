package modedit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
)

// KindContent marks one of a template's own extra files (an event, a localisation file, and so
// on) - anything that is neither the descriptor nor the stub. EditorEdit.tsx's FileChange
// component already treats any Kind other than "stub" as "in the mod's folder", so this needs no
// frontend change to render correctly.
const KindContent = "content"

// stellarisGameID is Stellaris's own permanent registry UUID (data/games.jsonc) - the five
// Stellaris-specific templates below are gated on this exact game rather than a name or folder
// string, matching how the rest of this app already treats a game's UUID as its one stable
// identity (see docs/game-configuration.md). Every other registered game only gets Blank, until
// its own template shapes are worked out.
const stellarisGameID = "01a0963a-c214-75a3-908d-1b76b91ea7bf"

// TemplateFile is one extra file a template writes, beyond the descriptor (NewFiles), the stub
// (NewStub, only when the chosen location is the game's own mod folder) and the placeholder
// thumbnail (see Templates' own IncludeThumbnail) - all three of those stay handled separately,
// since whether a stub applies depends on the location the person picked, not on the template.
// RelPath is relative to the mod's own content folder, e.g. "events/foo_events.txt".
type TemplateFile struct {
	RelPath string
	Content string
}

// Template is one named starter-content preset offered on the New tab.
type Template struct {
	ID          string
	Name        string
	Description string
	// FileCount is a template-intrinsic estimate for the picker grid (descriptor + thumbnail,
	// if any, + the stub + this template's own extra files) - the picker's own badge number,
	// matching the redesign's own reference figures exactly. The live "what will be created"
	// preview is what's actually authoritative (it never shows a stub at all for a location
	// outside the game's own mod folder, for instance) - this is a fixed estimate for the tile.
	FileCount int
	// IncludeThumbnail says whether CreateMod also writes a placeholder thumbnail.png -
	// CreateMod otherwise writes no thumbnail at all, matching its behavior before templates
	// existed.
	IncludeThumbnail bool
	// Build returns this template's own extra content files for a mod named/tagged as in
	// fields. Nil for the Blank template, which adds nothing beyond descriptor/stub/thumbnail.
	Build func(fields Fields) []TemplateFile
}

// Templates lists the starter-content presets offered when creating a new mod for gameID.
// Every game gets Blank; only Stellaris (the one game this app's own templates are actually
// written against) gets the other five - Event chain, Localisation, Portrait set, Shipset and
// Game rule, real (if intentionally minimal) Clausewitz script skeletons rather than placeholder
// text, following the shapes docs/script-format.md documents. Every skeleton here is verified to
// parse cleanly through internal/script.Parse (see templates_test.go) - structurally valid, even
// though the exact field names inside are a reasonable starting point to build from, not
// confirmed against a real Stellaris load the way this app's own parsing/conflict logic is.
func Templates(gameID string) []Template {
	templates := []Template{blankTemplate}
	if gameID == stellarisGameID {
		templates = append(templates, eventChainTemplate, localisationTemplate, portraitSetTemplate, shipsetTemplate, gameRuleTemplate)
	}
	return templates
}

// TemplateByID finds one of Templates(gameID)'s own entries, falling back to Blank for an empty
// or unrecognized id - CreateMod/PreviewNewMod never fail outright over a stale/unknown template.
func TemplateByID(gameID, id string) Template {
	for _, t := range Templates(gameID) {
		if t.ID == id {
			return t
		}
	}
	return blankTemplate
}

var blankTemplate = Template{
	ID:               "blank",
	Name:             "Blank",
	Description:      "descriptor.mod and a thumbnail. Nothing else.",
	FileCount:        2,
	IncludeThumbnail: true,
}

var eventChainTemplate = Template{
	ID:               "event_chain",
	Name:             "Event chain",
	Description:      "An event file, on_actions hook and matching localisation.",
	FileCount:        6,
	IncludeThumbnail: true,
	Build:            buildEventChain,
}

var localisationTemplate = Template{
	ID:               "localisation",
	Name:             "Localisation",
	Description:      "Translation-only mod with the folder layout the game reads.",
	FileCount:        3,
	IncludeThumbnail: false,
	Build:            buildLocalisation,
}

var portraitSetTemplate = Template{
	ID:               "portrait_set",
	Name:             "Portrait set",
	Description:      "Species class, portrait group and asset selector stubs.",
	FileCount:        7,
	IncludeThumbnail: true,
	Build:            buildPortraitSet,
}

var shipsetTemplate = Template{
	ID:               "shipset",
	Name:             "Shipset",
	Description:      "Graphical culture entry and entity stubs for one shipset.",
	FileCount:        9,
	IncludeThumbnail: true,
	Build:            buildShipset,
}

var gameRuleTemplate = Template{
	ID:               "game_rule",
	Name:             "Game rule",
	Description:      "A single game rule with an on/off localisation pair.",
	FileCount:        4,
	IncludeThumbnail: true,
	Build:            buildGameRule,
}

// scriptID turns a mod's display name into a Clausewitz-safe identifier: lowercase ASCII
// letters/digits/underscores only, collapsing anything else into single underscores - the same
// job FolderName does for a filesystem path component, but script keys/namespaces/localisation
// keys have a stricter character set than a folder name does (no spaces, no most punctuation).
func scriptID(name string) string {
	var b strings.Builder
	lastUnderscore := true // true so a leading separator is dropped, not turned into "_word"
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	id := strings.TrimSuffix(b.String(), "_")
	if id == "" {
		id = "mod"
	}
	return id
}

func buildEventChain(fields Fields) []TemplateFile {
	id := scriptID(fields.Name)

	events := "namespace = " + id + "\n\n" +
		"country_event = {\n" +
		"\tid = " + id + ".1\n" +
		"\ttitle = " + id + ".1.t\n" +
		"\tdesc = " + id + ".1.d\n" +
		"\tpicture = GFX_evt_diplomacy\n\n" +
		"\tis_triggered_only = yes\n\n" +
		"\toption = {\n" +
		"\t\tname = " + id + ".1.a\n" +
		"\t}\n" +
		"}\n"

	onActions := "on_game_start = {\n" +
		"\tevents = {\n" +
		"\t\t" + id + ".1\n" +
		"\t}\n" +
		"}\n"

	loc := locale.RenderFile("english", map[string]string{
		id + ".1.t": fields.Name + ": a new beginning",
		id + ".1.d": "This is the first event in a chain started by " + fields.Name + ". Replace this text and add the events that follow it.",
		id + ".1.a": "Continue",
	})

	return []TemplateFile{
		{RelPath: "events/" + id + "_events.txt", Content: events},
		{RelPath: "common/on_actions/" + id + "_on_actions.txt", Content: onActions},
		{RelPath: "localisation/english/" + id + "_l_english.yml", Content: string(loc)},
	}
}

func buildLocalisation(fields Fields) []TemplateFile {
	id := scriptID(fields.Name)
	loc := locale.RenderFile("english", map[string]string{
		id + "_example": "Replace this with your own translated keys.",
	})
	return []TemplateFile{
		{RelPath: "localisation/english/" + id + "_l_english.yml", Content: string(loc)},
	}
}

func buildPortraitSet(fields Fields) []TemplateFile {
	id := scriptID(fields.Name)

	speciesClass := id + "_species_class = {\n" +
		"\tdefault_portrait = " + id + "_default\n" +
		"\tpossible_portraits = {\n" +
		"\t\t" + id + "_default\n" +
		"\t}\n" +
		"\tgraphical_culture = " + id + "_01\n" +
		"\tgraphical_culture_02 = " + id + "_01\n" +
		"}\n"

	portraits := "portrait_groups = {\n" +
		"\t" + id + " = {\n" +
		"\t\tdefault = \"" + id + "_default\"\n\n" +
		"\t\t" + id + "_default = {\n" +
		"\t\t\tentity = \"" + id + "_default_entity\"\n" +
		"\t\t}\n" +
		"\t}\n" +
		"}\n"

	assetSelectors := "asset_selectors = {\n" +
		"\t" + id + "_default = {\n" +
		"\t\tdefault = \"" + id + "_default_entity\"\n" +
		"\t}\n" +
		"}\n"

	return []TemplateFile{
		{RelPath: "common/species_classes/" + id + "_species_classes.txt", Content: speciesClass},
		{RelPath: "gfx/portraits/portraits/" + id + "_portraits.txt", Content: portraits},
		{RelPath: "gfx/portraits/asset_selectors/" + id + "_asset_selectors.txt", Content: assetSelectors},
	}
}

func buildShipset(fields Fields) []TemplateFile {
	id := scriptID(fields.Name)

	graphicalCulture := "graphical_culture = {\n" +
		"\t" + id + "_01 = {\n" +
		"\t\tcity_graphics = generic_01\n" +
		"\t\tship_graphics = " + id + "_01\n" +
		"\t\tfleet_graphics = " + id + "_01\n" +
		"\t}\n" +
		"}\n"

	// Real ship hulls are entity blocks (usually in a .asset file) referencing real 3D meshes
	// and textures this template cannot generate - this stays a plain comment file marking
	// where they belong, rather than a fabricated entity block pointing at meshes that don't
	// exist.
	entities := "# " + fields.Name + "'s hull entity stubs.\n" +
		"#\n" +
		"# A real ship hull is an \"entity\" block (usually in a .asset file) that references real\n" +
		"# 3D meshes and textures this template cannot create for you. Replace this file with your\n" +
		"# own " + id + "_01_hull_entities.asset once you have real ship art.\n"

	return []TemplateFile{
		{RelPath: "common/graphical_culture/" + id + "_graphical_culture.txt", Content: graphicalCulture},
		{RelPath: "gfx/models/ships/" + id + "/" + id + "_hull_entities.txt", Content: entities},
	}
}

func buildGameRule(fields Fields) []TemplateFile {
	id := scriptID(fields.Name)

	gameRule := id + "_game_rule = {\n" +
		"\toptions = {\n" +
		"\t\t" + id + "_on = {\n" +
		"\t\t\tdefault = yes\n" +
		"\t\t}\n" +
		"\t\t" + id + "_off = {\n" +
		"\t\t}\n" +
		"\t}\n" +
		"}\n"

	loc := locale.RenderFile("english", map[string]string{
		id + "_game_rule":      fields.Name,
		id + "_game_rule_desc": "What toggling " + fields.Name + " does.",
		id + "_on":             "On",
		id + "_off":            "Off",
	})

	return []TemplateFile{
		{RelPath: "common/game_rules/" + id + "_game_rules.txt", Content: gameRule},
		{RelPath: "localisation/english/" + id + "_l_english.yml", Content: string(loc)},
	}
}

// PlaceholderThumbnail is a fixed 512x384 PNG (Steam Workshop's classic preview aspect ratio) in
// this app's own two dark panel tones, diagonally striped to mirror the "no thumbnail yet"
// checkerboard motif already used in frontend/src/views/Editor.css's .editor-thumb-none - so a
// freshly created mod never looks broken before its author picks a real picture on the Edit tab.
// Exported since internal/app writes it (via CreateMod/PreviewNewMod), not this package.
func PlaceholderThumbnail() []byte {
	const w, h = 512, 384
	const stripe = 18
	dark1 := color.NRGBA{R: 0x1b, G: 0x23, B: 0x30, A: 0xff}
	dark2 := color.NRGBA{R: 0x19, G: 0x20, B: 0x2c, A: 0xff}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := dark1
			if ((x+y)/stripe)%2 != 0 {
				c = dark2
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	return buf.Bytes()
}
