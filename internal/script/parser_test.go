package script

import (
	"errors"
	"testing"
)

func findEntry(t *testing.T, b Block, key string) Entry {
	t.Helper()
	for _, e := range b.Entries {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("no entry with key %q in block with %d entries", key, len(b.Entries))
	return Entry{}
}

func TestParseFlatKeyValues(t *testing.T) {
	src := []byte(`
namespace = dmm_mod
hide_window = yes
is_triggered_only = yes
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Root.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(f.Root.Entries))
	}

	ns := findEntry(t, f.Root, "namespace")
	if ns.Value.Kind != KindIdent || ns.Value.Raw != "dmm_mod" {
		t.Errorf("namespace = %+v", ns.Value)
	}

	hw := findEntry(t, f.Root, "hide_window")
	if hw.Value.Kind != KindIdent || hw.Value.Raw != "yes" {
		t.Errorf("hide_window = %+v", hw.Value)
	}
}

func TestParseNestedBlocks(t *testing.T) {
	src := []byte(`
country_event = {
	id = dmm_mod.1
	hide_window = yes

	trigger = {
		has_global_flag = dmm_mod_1
	}
}
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ev := findEntry(t, f.Root, "country_event")
	if ev.Value.Kind != KindBlock || ev.Value.Block == nil {
		t.Fatalf("country_event value = %+v, want block", ev.Value)
	}
	id := findEntry(t, *ev.Value.Block, "id")
	if id.Value.Raw != "dmm_mod.1" {
		t.Errorf("id = %q, want dmm_mod.1", id.Value.Raw)
	}
	trigger := findEntry(t, *ev.Value.Block, "trigger")
	if trigger.Value.Kind != KindBlock {
		t.Fatalf("trigger value = %+v, want block", trigger.Value)
	}
	flag := findEntry(t, *trigger.Value.Block, "has_global_flag")
	if flag.Value.Raw != "dmm_mod_1" {
		t.Errorf("has_global_flag = %q", flag.Value.Raw)
	}
}

func TestParseBareListItems(t *testing.T) {
	src := []byte(`
tags = {
	"Gameplay"
	"Fixes"
}
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tags := findEntry(t, f.Root, "tags")
	if tags.Value.Kind != KindBlock {
		t.Fatalf("tags value = %+v, want block", tags.Value)
	}
	items := tags.Value.Block.Entries
	if len(items) != 2 {
		t.Fatalf("expected 2 tag entries, got %d", len(items))
	}
	for i, want := range []string{"Gameplay", "Fixes"} {
		if items[i].Key != "" {
			t.Errorf("item %d has non-empty key %q", i, items[i].Key)
		}
		if items[i].Value.Kind != KindString || items[i].Value.Raw != want {
			t.Errorf("item %d = %+v, want string %q", i, items[i].Value, want)
		}
	}
}

func TestParseVariables(t *testing.T) {
	src := []byte(`
@test = 1

country_event = {
	chance = @test
}
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	v, ok := f.Variables["test"]
	if !ok {
		t.Fatalf("expected variable %q to be collected", "test")
	}
	if v.Kind != KindNumber || v.Raw != "1" {
		t.Errorf("@test = %+v, want number 1", v)
	}

	ev := findEntry(t, f.Root, "country_event")
	chance := findEntry(t, *ev.Value.Block, "chance")
	if chance.Value.Kind != KindIdent || chance.Value.Raw != "@test" {
		t.Errorf("chance = %+v, want ident \"@test\"", chance.Value)
	}
}

func TestParseComparisonOperators(t *testing.T) {
	src := []byte(`
trigger = {
	owner_species_pop_ethic_percentage >= 0.5
	planet_size <= 20
	some_flag != no
}
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	trigger := findEntry(t, f.Root, "trigger")
	block := trigger.Value.Block

	pct := findEntry(t, *block, "owner_species_pop_ethic_percentage")
	if pct.Op != ">=" || pct.Value.Raw != "0.5" || pct.Value.Kind != KindNumber {
		t.Errorf("pct entry = %+v", pct)
	}
	size := findEntry(t, *block, "planet_size")
	if size.Op != "<=" || size.Value.Raw != "20" {
		t.Errorf("size entry = %+v", size)
	}
	flag := findEntry(t, *block, "some_flag")
	if flag.Op != "!=" || flag.Value.Raw != "no" {
		t.Errorf("flag entry = %+v", flag)
	}
}

func TestParseCommentsIgnored(t *testing.T) {
	src := []byte(`
# this is a comment
key = value # trailing comment
# another comment
other = 5
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Root.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(f.Root.Entries), f.Root.Entries)
	}
}

func TestParseNegativeAndDecimalNumbers(t *testing.T) {
	src := []byte(`
min = -5
factor = -1.5
plain = 3.25
`)
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tests := map[string]string{"min": "-5", "factor": "-1.5", "plain": "3.25"}
	for key, want := range tests {
		e := findEntry(t, f.Root, key)
		if e.Value.Kind != KindNumber || e.Value.Raw != want {
			t.Errorf("%s = %+v, want number %q", key, e.Value, want)
		}
	}
}

func TestParseUnterminatedStringErrors(t *testing.T) {
	_, err := Parse([]byte(`name = "unterminated`))
	if err == nil {
		t.Fatal("expected an error for an unterminated string")
	}
	var syn *SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("expected a *SyntaxError, got %T", err)
	}
	if syn.Pos.Line != 1 {
		t.Errorf("expected line 1, got %d", syn.Pos.Line)
	}
}

func TestParseUnbalancedBraceErrors(t *testing.T) {
	_, err := Parse([]byte(`block = { key = value`))
	if err == nil {
		t.Fatal("expected an error for a missing closing brace")
	}
	var syn *SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("expected a *SyntaxError, got %T", err)
	}
}

func TestParseEmptyInput(t *testing.T) {
	f, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if len(f.Root.Entries) != 0 {
		t.Errorf("expected no entries, got %d", len(f.Root.Entries))
	}
}
