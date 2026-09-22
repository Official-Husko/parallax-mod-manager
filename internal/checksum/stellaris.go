package checksum

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The Stellaris scheme. Every file the manifest names is taken from the game folder, then each
// enabled mod is laid over it (a file with the same path replaces the earlier one, a replace_path
// first removes everything below it). The surviving files are walked in the manifest's order,
// each directory in name order, and one MD5 takes each file's bytes then its path; the game
// version goes in once at the end.

// sfile is one file of the laid-over tree.
type sfile struct {
	rule   int
	source string
	path   string    // on disk, or
	entry  *zip.File // inside an archive
	// modTime and size are the file's own, straight from the same stat (or, for an archive entry,
	// the same zip header) the walk already had to do to find it - see ResultCache.
	modTime time.Time
	size    int64
}

func (f *sfile) writeTo(w io.Writer) error {
	if f.entry == nil {
		return hashFile(w, f.path)
	}
	rc, err := f.entry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	return err
}

// matches is whether the virtual path is one this rule covers.
func (r Rule) matches(virtual string) bool {
	p := normalizeVirtual(virtual)
	prefix := normalizeVirtual(r.Directory) + "/"
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	rest := p[len(prefix):]
	if rest == "" {
		return false
	}
	if !r.Recursive && strings.Contains(rest, "/") {
		return false
	}
	return strings.HasSuffix(p, r.Extension)
}

func computeStellaris(ctx context.Context, in Input) (Result, error) {
	rules, err := readManifest(in.GameDir)
	if err != nil {
		return Result{}, fmt.Errorf("checksum: reading the game's manifest: %w", err)
	}
	salt, err := stellarisSalt(in.LauncherSettings)
	if err != nil {
		return Result{}, err
	}
	key := cacheKey(in)

	vfs := map[string]*sfile{}
	var archives []*zip.ReadCloser
	defer func() {
		for _, a := range archives {
			a.Close()
		}
	}()

	if err := mountDir(ctx, in.GameDir, "the game", rules, vfs); err != nil {
		return Result{}, err
	}

	ordered, warnings := stableDependencyOrder(in.Mods)
	res := Result{Algorithm: Stellaris, Salt: salt, Warnings: warnings}
	for _, m := range ordered {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		applyReplacePaths(vfs, m.ReplacePaths)
		res.Order = append(res.Order, displayName(m))

		info, err := os.Stat(m.Content)
		switch {
		case err != nil:
			return Result{}, &MissingContentError{Mods: []string{displayName(m)}}
		case info.IsDir():
			if err := mountDir(ctx, m.Content, displayName(m), rules, vfs); err != nil {
				return Result{}, err
			}
		default:
			zr, err := zip.OpenReader(m.Content)
			if err != nil {
				return Result{}, fmt.Errorf("checksum: opening the archive of '%s': %w", displayName(m), err)
			}
			archives = append(archives, zr)
			mountArchive(zr, displayName(m), rules, vfs)
		}
	}

	items := stellarisItems(vfs, rules)

	// Every file that will be hashed is already known here, with its size and modification time
	// (nothing extra was read to get them) - enough to tell whether the expensive part below,
	// reading and hashing every one's real bytes, would produce anything different from last
	// time. See ResultCache.
	fp := fingerprintOf(salt, items, func(it sitem) (string, int64, time.Time) {
		return it.path, it.file.size, it.file.modTime
	})
	if cached, ok := in.Cache.lookup(key, fp); ok {
		return cached, nil
	}

	h := md5.New()
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err := it.file.writeTo(h); err != nil {
			return Result{}, fmt.Errorf("checksum: reading %s (%s): %w", it.path, it.file.source, err)
		}
		h.Write([]byte(it.path))
	}
	// The version goes in once, after every file.
	h.Write([]byte(salt))

	full := hexUpper(h.Sum(nil))
	res.Full = full
	res.Checksum = full[len(full)-4:]
	res.Files = len(items)
	in.Cache.store(key, fp, res)
	return res, nil
}

var codenameRe = regexp.MustCompile(`^(.+?)\s+v?\d`)

// stellarisSalt is the version text the checksum is salted with: the game's codename and its
// raw version ("Pegasus v4.4.6"), from launcher-settings.json's "Pegasus v4.4.6 (fdde)".
func stellarisSalt(launcherSettings string) (string, error) {
	full, raw, err := launcherVersion(launcherSettings)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", errors.New("checksum: launcher-settings.json has no rawVersion")
	}
	codename := ""
	for _, marker := range []string{" v" + raw, " " + raw} {
		if i := strings.Index(full, marker); i > 0 {
			codename = strings.TrimSpace(full[:i])
			break
		}
	}
	if codename == "" {
		if m := codenameRe.FindStringSubmatch(full); m != nil {
			codename = strings.TrimSpace(m[1])
		}
	}
	if codename == "" {
		return "", fmt.Errorf("checksum: could not read the game's codename from version %q", full)
	}
	return codename + " " + raw, nil
}

// mountDir lays every manifest file below base over vfs.
func mountDir(ctx context.Context, base, source string, rules []Rule, vfs map[string]*sfile) error {
	for i, rule := range rules {
		root := filepath.Join(base, filepath.FromSlash(rule.Directory))
		err := walkFiles(ctx, root, rule.Extension, rule.Recursive, 0, func(phys string, info os.FileInfo) {
			rel, err := filepath.Rel(base, phys)
			if err != nil {
				return
			}
			vfs[normalizeVirtual(filepath.ToSlash(rel))] = &sfile{rule: i, source: source, path: phys, modTime: info.ModTime(), size: info.Size()}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// maxWalkDepth stops a folder that links back to itself from being walked forever.
const maxWalkDepth = 64

// walkFiles calls fn for every file below dir whose path ends in suffix, descending when
// recursive. A folder that cannot be read is skipped, as the game skips it.
func walkFiles(ctx context.Context, dir, suffix string, recursive bool, depth int, fn func(path string, info os.FileInfo)) error {
	if depth > maxWalkDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := filepath.Join(dir, e.Name())
		info, err := os.Stat(p) // follows links, like the game
		if err != nil {
			continue
		}
		switch {
		case info.Mode().IsRegular():
			if strings.HasSuffix(p, suffix) {
				fn(p, info)
			}
		case info.IsDir() && recursive:
			if err := walkFiles(ctx, p, suffix, recursive, depth+1, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func mountArchive(zr *zip.ReadCloser, source string, rules []Rule, vfs map[string]*sfile) {
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		virtual := normalizeVirtual(f.Name)
		idx := -1
		for i, r := range rules {
			if r.matches(virtual) {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		vfs[virtual] = &sfile{rule: idx, source: source, entry: f, modTime: f.Modified, size: int64(f.UncompressedSize64)}
	}
}

// applyReplacePaths removes everything below any of the paths.
func applyReplacePaths(vfs map[string]*sfile, replacePaths []string) {
	var roots []string
	for _, p := range replacePaths {
		if n := normalizeVirtual(p); n != "" {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		return
	}
	for virtual := range vfs {
		for _, root := range roots {
			if underPath(virtual, root) {
				delete(vfs, virtual)
				break
			}
		}
	}
}

type sitem struct {
	path string
	file *sfile
	key  []string
}

// stellarisItems lists the files in the order they are hashed: by manifest rule, then the way
// a depth-first walk of the sorted folders meets them.
func stellarisItems(vfs map[string]*sfile, rules []Rule) []sitem {
	var out []sitem
	for idx, rule := range rules {
		var group []sitem
		root := normalizeVirtual(rule.Directory)
		for p, f := range vfs {
			if f.rule != idx || !rule.matches(p) {
				continue
			}
			rest := strings.TrimLeft(strings.TrimPrefix(p, root), "/")
			group = append(group, sitem{path: p, file: f, key: strings.Split(rest, "/")})
		}
		sort.Slice(group, func(i, j int) bool { return lessKey(group[i].key, group[j].key) })
		out = append(out, group...)
	}
	return out
}

// lessKey orders path component lists element by element, a shorter list first when it is a
// prefix of the other.
func lessKey(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// withNestedDescriptor fills what a mod's registration leaves out from the descriptor.mod inside
// its own folder: the name, replace_path and dependencies (Stellaris reads them from either).
func withNestedDescriptor(m Mod) Mod {
	info, err := os.Stat(m.Content)
	if err != nil || !info.IsDir() {
		return m
	}
	data, err := os.ReadFile(filepath.Join(m.Content, "descriptor.mod"))
	if err != nil {
		return m
	}
	text := readText(data)
	if strings.TrimSpace(m.Name) == "" {
		m.Name = descriptorName(text)
	}
	if len(m.ReplacePaths) == 0 {
		m.ReplacePaths = descriptorReplacePaths(text, normalizeVirtual)
	}
	if len(m.Dependencies) == 0 {
		m.Dependencies = descriptorDependencies(text)
	}
	return m
}

// stableDependencyOrder keeps the launcher's order but moves a mod after every mod it depends
// on (matched by exact name; a dependency that is not enabled is ignored). A cycle falls back to
// the launcher's order.
func stableDependencyOrder(in []Mod) ([]Mod, []string) {
	if len(in) == 0 {
		return nil, nil
	}
	mods := make([]Mod, len(in))
	for i, m := range in {
		mods[i] = withNestedDescriptor(m)
	}
	var warnings []string

	byName := map[string][]int{}
	for i, m := range mods {
		if m.Name != "" {
			byName[m.Name] = append(byName[m.Name], i)
		}
	}
	edges := make([]map[int]bool, len(mods))
	for i := range edges {
		edges[i] = map[int]bool{}
	}
	indegree := make([]int, len(mods))
	for i, m := range mods {
		for _, dep := range m.Dependencies {
			matches := byName[dep]
			if len(matches) == 0 {
				warnings = append(warnings, fmt.Sprintf("'%s' depends on '%s', which is not enabled", displayName(m), dep))
				continue
			}
			d := matches[0]
			if d == i || edges[d][i] {
				continue
			}
			edges[d][i] = true
			indegree[i]++
		}
	}

	// Position in the launcher's list is the priority: of the mods free to go next, the
	// earliest in the list goes first.
	var ready []int
	for i, d := range indegree {
		if d == 0 {
			ready = append(ready, i)
		}
	}
	var order []int
	for len(ready) > 0 {
		best := 0
		for k, c := range ready {
			if c < ready[best] {
				best = k
			}
		}
		i := ready[best]
		ready = append(ready[:best], ready[best+1:]...)
		order = append(order, i)
		for n := range edges[i] {
			indegree[n]--
			if indegree[n] == 0 {
				ready = append(ready, n)
			}
		}
	}
	if len(order) != len(mods) {
		warnings = append(warnings, "the mods' dependencies form a cycle, so the load order was used as it is")
		return mods, warnings
	}
	out := make([]Mod, len(order))
	for k, i := range order {
		out[k] = mods[i]
	}
	return out, warnings
}
