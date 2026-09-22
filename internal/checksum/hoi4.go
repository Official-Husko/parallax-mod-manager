package checksum

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// The Hearts of Iron IV scheme. The game's manifest files are laid out the same way as in
// Stellaris, but with differences that change the result:
//
//   - paths are compared without regard to case, and a mod's file replaces the game's when only
//     the case differs;
//   - a replace_path does not delete the files below it: they become blanks that later mods
//     cannot bring back, hashed as an empty file, and only the folder's direct files are
//     affected (a subfolder's files are kept unless the mod also replaces that subfolder);
//   - mods are laid over the game in dependency order, then by registry name - the launcher's
//     load order plays no part;
//   - every file is hashed on its own (bytes, then path) and the digests, followed by the game
//     version, go into one final MD5.

// hfile is one file of the laid-over tree.
type hfile struct {
	rule        int
	source      string
	path        string
	entry       *zip.File
	placeholder bool
	// modTime and size are the file's own, straight from the same stat (or, for an archive
	// entry, the same zip header) the walk already had to do to find it - see ResultCache. Zero
	// for a placeholder, which is fine: whether a path is a placeholder is itself derived from
	// the same walk, so it is exactly as stable a signal as a real file's size and time.
	modTime time.Time
	size    int64
}

func computeHOI4(ctx context.Context, in Input) (Result, error) {
	rules, err := readManifest(in.GameDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return Result{}, fmt.Errorf("checksum: reading the game's manifest: %w", err)
		}
		rules = hoi4FallbackRules()
	}
	for i := range rules {
		rules[i].Directory = strings.ToLower(rules[i].Directory)
		rules[i].Extension = strings.ToLower(rules[i].Extension)
	}
	salt, err := hoi4Salt(in.LauncherSettings)
	if err != nil {
		return Result{}, err
	}
	key := cacheKey(in)

	res := Result{Algorithm: HOI4, Salt: salt}
	mods, dropped := hoi4DropDuplicateNames(in.Mods, in.ModDir)
	res.Warnings = append(res.Warnings, dropped...)
	for i := range mods {
		mods[i].ReplacePaths = lowerAll(mods[i].ReplacePaths)
	}

	tree := &htree{exact: map[string]*hfile{}, folded: map[string]*hfile{}}
	var archives []*zip.ReadCloser
	defer func() {
		for _, a := range archives {
			a.Close()
		}
	}()

	if err := tree.mountDir(ctx, in.GameDir, "the game", rules, nil); err != nil {
		return Result{}, err
	}

	mounted, err := hoi4MountOrder(mods)
	if err != nil {
		return Result{}, err
	}
	for _, m := range mounted {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		res.Order = append(res.Order, displayName(m))
		for _, rp := range m.ReplacePaths {
			tree.applyReplacePath(rp, replacePathIsManifestLevel(rp, rules))
		}
		info, err := os.Stat(m.Content)
		switch {
		case err != nil:
			return Result{}, &MissingContentError{Mods: []string{displayName(m)}}
		case info.IsDir():
			if err := tree.mountDir(ctx, m.Content, displayName(m), rules, m.ReplacePaths); err != nil {
				return Result{}, err
			}
		default:
			zr, err := zip.OpenReader(m.Content)
			if err != nil {
				return Result{}, fmt.Errorf("checksum: opening the archive of '%s': %w", displayName(m), err)
			}
			archives = append(archives, zr)
			tree.mountArchive(zr, displayName(m), rules, m.ReplacePaths)
		}
	}

	items := tree.items()

	// Every file that will be hashed is already known here, with its size and modification time
	// (nothing extra was read to get them) - enough to tell whether the expensive part below,
	// reading and hashing every one's real bytes, would produce anything different from last
	// time. See ResultCache.
	fp := fingerprintOf(salt, items, func(it hitem) (string, int64, time.Time) {
		return it.path, it.file.size, it.file.modTime
	})
	if cached, ok := in.Cache.lookup(key, fp); ok {
		return cached, nil
	}

	digests := make([][md5.Size]byte, len(items))
	if err := hashAll(ctx, items, digests); err != nil {
		return Result{}, err
	}
	outer := md5.New()
	for _, d := range digests {
		outer.Write(d[:])
	}
	outer.Write([]byte(salt))

	res.Full = hexUpper(outer.Sum(nil))
	res.Checksum = res.Full[len(res.Full)-4:]
	res.Files = len(items)
	in.Cache.store(key, fp, res)
	return res, nil
}

// hoi4Salt is the version text the checksum is salted with: launcher-settings.json's version
// without the trailing " (code)", e.g. "Operation Postern v1.19.3.0.c01a".
func hoi4Salt(launcherSettings string) (string, error) {
	version, _, err := launcherVersion(launcherSettings)
	if err != nil {
		return "", err
	}
	if i := strings.LastIndex(version, " ("); i > 0 && strings.HasSuffix(version, ")") {
		version = version[:i]
	}
	if version == "" {
		return "", errors.New("checksum: the game's version text is empty")
	}
	return version, nil
}

func hoi4FallbackRules() []Rule {
	var rules []Rule
	for _, r := range [][2]string{
		{"common", ".txt"}, {"common", ".lua"}, {"events", ".txt"}, {"history", ".txt"},
		{"map", ".txt"}, {"map", ".map"}, {"map", ".bmp"}, {"map", ".csv"},
	} {
		rules = append(rules, Rule{Directory: r[0], Extension: r[1], Recursive: true})
	}
	return rules
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if n := strings.ToLower(normalizeVirtual(s)); n != "" && !contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// htree is the laid-over tree: exact keeps every path as spelled, folded points each
// lower-cased path at the file that currently owns it.
type htree struct {
	exact  map[string]*hfile
	folded map[string]*hfile
}

func (t *htree) set(path string, f *hfile) {
	t.exact[path] = f
	t.folded[strings.ToLower(path)] = f
}

// placeholder blanks out a path a mod replaced: an existing entry keeps its manifest rule, and
// the blank sticks.
func (t *htree) placeholder(path string, rule int) {
	f := t.exact[path]
	if f == nil {
		f = &hfile{rule: rule}
		t.exact[path] = f
	}
	f.placeholder = true
	t.folded[strings.ToLower(path)] = f
}

func isDirectChild(path, parent string) bool {
	prefix := strings.ToLower(normalizeVirtual(parent)) + "/"
	p := strings.ToLower(normalizeVirtual(path))
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	rest := p[len(prefix):]
	return rest != "" && !strings.Contains(rest, "/")
}

func underAnyReplace(path string, replacePaths []string) bool {
	for _, rp := range replacePaths {
		if isDirectChild(path, rp) {
			return true
		}
	}
	return false
}

// replacePathIsManifestLevel says whether a replace_path names a manifest folder itself or a
// folder directly below one: such a replacement removes the game's files outright instead of
// blanking them.
func replacePathIsManifestLevel(rp string, rules []Rule) bool {
	rp = strings.ToLower(normalizeVirtual(rp))
	for _, r := range rules {
		root := strings.ToLower(normalizeVirtual(r.Directory))
		if rp == root {
			return true
		}
		if rest, ok := strings.CutPrefix(rp, root+"/"); ok && rest != "" && !strings.Contains(rest, "/") {
			return true
		}
	}
	return false
}

func (t *htree) applyReplacePath(rp string, manifestLevel bool) {
	for path, f := range t.exact {
		if !isDirectChild(path, rp) {
			continue
		}
		if manifestLevel {
			delete(t.exact, path)
			delete(t.folded, strings.ToLower(path))
		} else {
			f.placeholder = true
			t.folded[strings.ToLower(path)] = f
		}
	}
}

func (t *htree) mountDir(ctx context.Context, base, source string, rules []Rule, replacePaths []string) error {
	for i, rule := range rules {
		root := filepath.Join(base, filepath.FromSlash(rule.Directory))
		err := walkRuleFiles(ctx, root, rule, func(phys string, info os.FileInfo) {
			rel, err := filepath.Rel(base, phys)
			if err != nil {
				return
			}
			virtual := normalizeVirtual(filepath.ToSlash(rel))
			if underAnyReplace(virtual, replacePaths) {
				t.placeholder(virtual, i)
				return
			}
			if old := t.exact[virtual]; old == nil || !old.placeholder {
				t.set(virtual, &hfile{rule: i, source: source, path: phys, modTime: info.ModTime(), size: info.Size()})
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// walkRuleFiles calls fn for the files of a rule below root, an extension compared without regard
// to case. A link to a folder is not entered; a link to a file counts as the file.
func walkRuleFiles(ctx context.Context, root string, rule Rule, fn func(path string, info os.FileInfo)) error {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && !rule.Recursive {
				return fs.SkipDir
			}
			return nil
		}
		var info os.FileInfo
		if d.Type()&fs.ModeSymlink != 0 {
			st, err := os.Stat(p)
			if err != nil || st.IsDir() {
				return nil
			}
			info = st
		} else if !d.Type().IsRegular() {
			return nil
		} else {
			st, err := d.Info()
			if err != nil {
				return nil
			}
			info = st
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), rule.Extension) {
			fn(p, info)
		}
		return nil
	})
	return err
}

func (t *htree) mountArchive(zr *zip.ReadCloser, source string, rules []Rule, replacePaths []string) {
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		virtual := normalizeVirtual(f.Name)
		if virtual == "" {
			continue
		}
		idx := -1
		for i, r := range rules {
			if hoi4RuleMatches(r, virtual) {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		if underAnyReplace(virtual, replacePaths) {
			t.placeholder(virtual, idx)
			continue
		}
		if old := t.exact[virtual]; old == nil || !old.placeholder {
			t.set(virtual, &hfile{rule: idx, source: source, entry: f, modTime: f.Modified, size: int64(f.UncompressedSize64)})
		}
	}
}

func hoi4RuleMatches(r Rule, virtual string) bool {
	p := strings.ToLower(normalizeVirtual(virtual))
	prefix := strings.ToLower(normalizeVirtual(r.Directory)) + "/"
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	if !r.Recursive && strings.Contains(p[len(prefix):], "/") {
		return false
	}
	return strings.HasSuffix(p, strings.ToLower(r.Extension))
}

type hitem struct {
	path string
	rule int
	file *hfile
}

// items lists the files in the order they are hashed: by manifest rule, then by path with a
// folder's own files before its subfolders.
func (t *htree) items() []hitem {
	out := make([]hitem, 0, len(t.exact))
	for path, exact := range t.exact {
		// The rule comes from the entry as spelled, the content from whichever file owns the
		// path once case is ignored.
		out = append(out, hitem{path: path, rule: exact.rule, file: t.folded[strings.ToLower(path)]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rule != out[j].rule {
			return out[i].rule < out[j].rule
		}
		return hoi4PathCompare(out[i].path, out[j].path) < 0
	})
	return out
}

// hoi4PathCompare orders paths component by component (ordinal, in UTF-16 code units), a file
// before anything inside a sibling folder.
func hoi4PathCompare(a, b string) int {
	aa, bb := strings.Split(a, "/"), strings.Split(b, "/")
	n := len(aa)
	if len(bb) < n {
		n = len(bb)
	}
	for i := 0; i < n; i++ {
		aFile, bFile := i == len(aa)-1, i == len(bb)-1
		if aFile == bFile {
			if c := ordinalCompare(aa[i], bb[i]); c != 0 {
				return c
			}
			continue
		}
		if aFile {
			return -1
		}
		return 1
	}
	switch {
	case len(aa) > len(bb):
		return 1
	case len(aa) < len(bb):
		return -1
	}
	return 0
}

// ordinalCompare compares two strings by UTF-16 code unit, the way the game sorts names.
func ordinalCompare(a, b string) int {
	if isASCII(a) && isASCII(b) {
		return strings.Compare(a, b)
	}
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			if ua[i] < ub[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ua) < len(ub):
		return -1
	case len(ua) > len(ub):
		return 1
	}
	return 0
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// hashAll fills digests with each item's own MD5 (bytes, then path; a blank is the MD5 of
// nothing), reading files on several workers - the digests do not depend on one another.
func hashAll(ctx context.Context, items []hitem, digests [][md5.Size]byte) error {
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				d, err := innerDigest(items[i])
				if err != nil {
					fail(err)
					continue
				}
				digests[i] = d
			}
		}()
	}
loop:
	for i := range items {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break loop
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	return firstErr
}

func innerDigest(it hitem) ([md5.Size]byte, error) {
	f := it.file
	if f.placeholder {
		return emptyMD5, nil
	}
	h := md5.New()
	var err error
	if f.entry != nil {
		var rc io.ReadCloser
		if rc, err = f.entry.Open(); err == nil {
			_, err = io.Copy(h, rc)
			rc.Close()
		}
	} else {
		err = hashFile(h, f.path)
	}
	if err != nil {
		return [md5.Size]byte{}, fmt.Errorf("checksum: reading %s (%s): %w", it.path, f.source, err)
	}
	h.Write([]byte(it.path))
	var out [md5.Size]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

// hoi4DropDuplicateNames drops a mod whose name another registration in the mod folder already
// took: the game reads only the first (in name order) registration of each name.
func hoi4DropDuplicateNames(mods []Mod, modDir string) ([]Mod, []string) {
	out := append([]Mod(nil), mods...)
	if modDir == "" {
		return out, nil
	}
	entries, err := os.ReadDir(modDir)
	if err != nil {
		return out, nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".mod") {
			files = append(files, e.Name())
		}
	}
	sort.Slice(files, func(i, j int) bool { return ordinalCompare(files[i], files[j]) < 0 })
	canonical := map[string]string{}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(modDir, name))
		if err != nil {
			continue
		}
		if n := descriptorName(readText(data)); strings.TrimSpace(n) != "" {
			if _, seen := canonical[n]; !seen {
				canonical[n] = "mod/" + name
			}
		}
	}

	kept := out[:0]
	var warnings []string
	for _, m := range out {
		expected, taken := canonical[m.Name]
		actual := strings.ReplaceAll(m.RegistryID, `\`, "/")
		if strings.TrimSpace(m.Name) != "" && taken && actual != expected {
			warnings = append(warnings, fmt.Sprintf("the game ignores '%s' at %s: %s registers the same name first", m.Name, m.RegistryID, expected))
			continue
		}
		kept = append(kept, m)
	}
	return kept, warnings
}

// hoi4MountOrder orders mods the way the game mounts them: by name, then a mod's priority drops
// below everything that depends on it, finally by registry name.
func hoi4MountOrder(mods []Mod) ([]Mod, error) {
	type entry struct {
		mod      Mod
		priority int
	}
	ordered := make([]*entry, len(mods))
	for i, m := range mods {
		ordered[i] = &entry{mod: m, priority: 10000}
	}
	registry := func(e *entry) string { return strings.ReplaceAll(e.mod.RegistryID, `\`, "/") }
	sort.SliceStable(ordered, func(i, j int) bool {
		if c := ordinalCompare(ordered[i].mod.Name, ordered[j].mod.Name); c != 0 {
			return c < 0
		}
		return ordinalCompare(registry(ordered[i]), registry(ordered[j])) < 0
	})

	iterations := 0
	for {
		changed := false
		for _, m := range ordered {
			for _, depName := range m.mod.Dependencies {
				for _, dep := range ordered {
					if dep.mod.Name == depName && dep.priority >= m.priority {
						dep.priority = m.priority - 1
						changed = true
					}
				}
			}
		}
		iterations++
		if changed && iterations > len(ordered)+1 {
			return nil, errors.New("checksum: the enabled mods depend on each other in a circle, so the game cannot settle on an order for them")
		}
		if !changed {
			break
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].priority != ordered[j].priority {
			return ordered[i].priority < ordered[j].priority
		}
		return ordinalCompare(registry(ordered[i]), registry(ordered[j])) < 0
	})
	out := make([]Mod, len(ordered))
	for i, e := range ordered {
		out[i] = e.mod
	}
	return out, nil
}
