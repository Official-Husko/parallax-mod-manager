package backgrounds

import (
	"os"
	"path/filepath"
	"testing"
)

func writeImage(t *testing.T, dir, game, name, body string) {
	t.Helper()
	full := filepath.Join(dir, game, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidGameIDAndName(t *testing.T) {
	for _, id := range []string{"01a0963a-c214-75a3-908d-1b76b91ea7bf", "stellaris", "a"} {
		if !ValidGameID(id) {
			t.Errorf("ValidGameID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`, ".hidden", "a b", "-lead", string(make([]byte, 70))} {
		if ValidGameID(id) {
			t.Errorf("ValidGameID(%q) = true, want false", id)
		}
	}
	for _, name := range []string{"100 - nBRSxYy.jpg", "a.PNG", "x.webp", "spaces in name.jpeg"} {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false", name)
		}
	}
	for _, name := range []string{"", ".", "..", "../a.png", "a/b.png", `a\b.png`, "c:x.png", ".hidden.png", "a.png.part", "a.txt", "noext", "a\x00b.png", "a\nb.png"} {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true, want false", name)
		}
	}
}

func TestStorePathRefusesAnythingThatEscapes(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if p, ok := s.Path("game1", "a.png"); !ok || p != filepath.Join(s.Dir, "game1", "a.png") {
		t.Errorf("Path = %q, %v", p, ok)
	}
	for _, c := range [][2]string{{"..", "a.png"}, {"game1", "../a.png"}, {"a/b", "a.png"}, {"game1", "/etc/passwd"}, {"", "a.png"}} {
		if p, ok := s.Path(c[0], c[1]); ok {
			t.Errorf("Path(%q, %q) = %q, want refused", c[0], c[1], p)
		}
	}
	if _, ok := (Store{}).Path("game1", "a.png"); ok {
		t.Error("a store without a Dir must refuse everything")
	}
}

func TestStoreListHasUsageGames(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	writeImage(t, s.Dir, "g1", "b.jpg", "12345")
	writeImage(t, s.Dir, "g1", "a.png", "12")
	writeImage(t, s.Dir, "g1", "notes.txt", "ignored")
	writeImage(t, s.Dir, "g1", "c.png.part", "half")
	writeImage(t, s.Dir, "g2", "z.webp", "x")
	if err := os.Mkdir(filepath.Join(s.Dir, "not a game id"), 0o755); err != nil {
		t.Fatal(err)
	}

	list := s.List("g1")
	if len(list) != 2 || list[0] != (File{"a.png", 2}) || list[1] != (File{"b.jpg", 5}) {
		t.Errorf("List = %+v, want a.png and b.jpg, sorted, without the .txt or .part", list)
	}
	if !s.Has("g1", File{"a.png", 2}) || s.Has("g1", File{"a.png", 3}) || s.Has("g1", File{"missing.png", 1}) {
		t.Error("Has must be true only for a file that exists with exactly that size")
	}
	if files, bytes := s.Usage("g1"); files != 2 || bytes != 7 {
		t.Errorf("Usage = %d files, %d bytes", files, bytes)
	}
	if games := s.Games(); len(games) != 2 || games[0] != "g1" || games[1] != "g2" {
		t.Errorf("Games = %v", games)
	}
	if s.List("nope") != nil || (Store{}).List("g1") != nil {
		t.Error("an unknown game or an unset store lists nothing")
	}
}

func TestStoreRemovePackOnlyRemovesThatGame(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	writeImage(t, s.Dir, "g1", "a.png", "1")
	writeImage(t, s.Dir, "g2", "a.png", "1")
	if err := s.RemovePack("g1"); err != nil {
		t.Fatalf("RemovePack: %v", err)
	}
	if len(s.List("g1")) != 0 || len(s.List("g2")) != 1 {
		t.Error("only g1 should be gone")
	}
	for _, bad := range []string{"..", "", "a/b"} {
		if err := s.RemovePack(bad); err == nil {
			t.Errorf("RemovePack(%q) must be refused", bad)
		}
	}
	if _, err := os.Stat(s.Dir); err != nil {
		t.Error("the store folder itself must survive")
	}
}
