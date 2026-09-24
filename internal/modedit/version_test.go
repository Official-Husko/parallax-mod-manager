package modedit

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"3.2", Version{Major: 3, Minor: 2}, true},
		{"1.0.0", Version{Major: 1, Minor: 0, Patch: 0, HadPatch: true}, true},
		{"v4.1", Version{Major: 4, Minor: 1}, true},
		{"V1.2.3", Version{Major: 1, Minor: 2, Patch: 3, HadPatch: true}, true},
		{" 2.5 ", Version{Major: 2, Minor: 5}, true},
		{"", Version{}, false},
		{"v4.*", Version{}, false},
		{"latest", Version{}, false},
		{"1", Version{}, false},
	}
	for _, c := range cases {
		got, ok := ParseVersion(c.in)
		if ok != c.ok {
			t.Errorf("ParseVersion(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

// TestBumpMatchesTheMockupsOwnWorkedExample: the redesign's own reference example for a mod
// saved at "3.2" - Patch -> 3.2.1, Minor -> 3.3 (kept two-number, since 3.2 was), Major -> 4.0.
func TestBumpMatchesTheMockupsOwnWorkedExample(t *testing.T) {
	v, ok := ParseVersion("3.2")
	if !ok {
		t.Fatal("ParseVersion(3.2) failed")
	}
	cases := []struct {
		kind BumpKind
		want string
	}{
		{BumpPatch, "3.2.1"},
		{BumpMinor, "3.3"},
		{BumpMajor, "4.0"},
	}
	for _, c := range cases {
		if got := v.Bump(c.kind).String(); got != c.want {
			t.Errorf("Bump(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestBumpOnAThreeNumberVersionKeepsThreeNumbers(t *testing.T) {
	v, _ := ParseVersion("1.2.7")
	if got := v.Bump(BumpMinor).String(); got != "1.3.0" {
		t.Errorf("Bump(minor) on 1.2.7 = %q, want %q", got, "1.3.0")
	}
	if got := v.Bump(BumpMajor).String(); got != "2.0.0" {
		t.Errorf("Bump(major) on 1.2.7 = %q, want %q", got, "2.0.0")
	}
	if got := v.Bump(BumpPatch).String(); got != "1.2.8" {
		t.Errorf("Bump(patch) on 1.2.7 = %q, want %q", got, "1.2.8")
	}
}
