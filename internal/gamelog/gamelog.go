// Package gamelog reads the log files a Paradox game writes while it runs
// (error.log, game.log, setup.log... in the "logs" folder of its user data
// directory), and follows one of them as it grows.
//
// These files are not this app's own log (see internal/applog) and are not ours
// to keep tidy: the game truncates them each time it starts, writes to several
// at once, and can append megabytes in a second when a mod is broken. Following
// one therefore has to cope with the file shrinking or being replaced under it,
// with a line arriving in two pieces, and with far more than can usefully be
// shown.
package gamelog

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// File is one log file in the game's logs folder.
type File struct {
	Name string
	// Size is in bytes. Modified is Unix seconds.
	Size     int64
	Modified int64
}

// Listing is a logs folder and the log files in it.
type Listing struct {
	// Dir is the folder, whether or not it exists yet - a game that has never
	// run has none.
	Dir string
	// Files is never nil.
	Files []File
}

// ValidName reports whether name is a log file this package will open: a plain
// file name ending in ".log", with no folder in it. Anything else is refused, so
// a name handed in from outside can never reach a file outside the logs folder.
func ValidName(name string) bool {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return false
	}
	return strings.EqualFold(filepath.Ext(name), ".log")
}

// List returns the log files in dir, most useful first: error.log, game.log and
// the other logs a modder reads before the rest, then the remainder by name. A
// missing folder is not an error - the game just hasn't run yet - and gives an
// empty (non-nil) list.
func List(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []File{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, File{Name: e.Name(), Size: info.Size(), Modified: info.ModTime().Unix()})
	}
	sort.SliceStable(files, func(i, j int) bool {
		ri, rj := rank(files[i].Name), rank(files[j].Name)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

// preferred is the order of the logs worth reading first.
var preferred = []string{"error.log", "game.log", "debug.log", "setup.log", "system.log"}

func rank(name string) int {
	for i, p := range preferred {
		if strings.EqualFold(name, p) {
			return i
		}
	}
	return len(preferred)
}

// Batch is one update from a followed file.
type Batch struct {
	// Reset says the lines replace what the viewer holds instead of adding to it:
	// the first batch, and again whenever the file was truncated or replaced (a
	// new game run rewrites its logs from the top).
	Reset bool
	// Missing says the file doesn't exist (yet). Sent once, with Reset; the
	// lines follow as a Reset batch when it appears.
	Missing bool
	Lines   []string
	// Skipped is how many bytes were passed over because more than
	// Options.MaxChunk arrived between two looks. Only the newest were read.
	Skipped int64
}

// Options tunes Follow. The zero value is a sensible default.
type Options struct {
	// Interval is how often the file is looked at again. Default 500ms.
	Interval time.Duration
	// InitialLines is how many of the file's last lines the first batch holds.
	// Default 1000.
	InitialLines int
	// MaxChunk is the most new data read in one look. Default 1 MiB.
	MaxChunk int64
	// MaxLineLength cuts a line longer than this many characters. Default 4000.
	MaxLineLength int
}

const (
	defaultInterval     = 500 * time.Millisecond
	defaultInitialLines = 1000
	defaultMaxChunk     = 1 << 20
	defaultMaxLine      = 4000
	// initialWindow is how much of the end of the file is read to find the last
	// InitialLines lines. A file of very long lines gives fewer, which is fine.
	initialWindow = 512 << 10
	// headLen is how many of a file's first bytes are remembered to notice it
	// being rewritten (see follower.head).
	headLen = 64
	// maxPartial is the longest unfinished line held back waiting for its newline.
	maxPartial = 64 << 10
)

func (o Options) withDefaults() Options {
	if o.Interval <= 0 {
		o.Interval = defaultInterval
	}
	if o.InitialLines <= 0 {
		o.InitialLines = defaultInitialLines
	}
	if o.MaxChunk <= 0 {
		o.MaxChunk = defaultMaxChunk
	}
	if o.MaxLineLength <= 0 {
		o.MaxLineLength = defaultMaxLine
	}
	return o
}

// Follow reports the end of the file at path, and then every line added to it,
// by calling emit, until ctx is done. It blocks; run it in a goroutine. emit is
// called from that goroutine only, never concurrently.
//
// A line is reported only once complete: the game writes in pieces, and half a
// line shown now would be a different line from the whole one. If the file is
// truncated or replaced, the next batch has Reset set and holds its new end.
func Follow(ctx context.Context, path string, opts Options, emit func(Batch)) {
	f := &follower{path: path, opts: opts.withDefaults(), emit: emit}
	f.poll()
	t := time.NewTicker(f.opts.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.poll()
		}
	}
}

type follower struct {
	path string
	opts Options
	emit func(Batch)

	seen    os.FileInfo // the file as last looked at; nil before the first look, or when missing
	offset  int64       // bytes of the file already consumed
	partial []byte      // an unfinished line at the end of what was consumed
	// head is the file's first bytes as first read. A log only ever grows, so if
	// they change the file was rewritten - which the size alone can't show when
	// the game truncated it and wrote more than was there before, all between two
	// looks.
	head    []byte
	missing bool // whether a "missing" batch has been sent and not yet answered
}

func (f *follower) poll() {
	info, err := os.Stat(f.path)
	if err != nil {
		if !f.missing {
			f.missing = true
			f.seen, f.offset, f.partial = nil, 0, nil
			f.emit(Batch{Reset: true, Missing: true, Lines: []string{}})
		}
		return
	}
	f.missing = false

	if f.seen == nil || !os.SameFile(f.seen, info) || info.Size() < f.offset ||
		(info.Size() > f.offset && f.rewritten()) {
		// Only counts as seen once the read worked: otherwise the next look would
		// treat a file the viewer never received as one it could just append to.
		if f.restart() {
			f.seen = info
		}
		return
	}
	if info.Size() > f.offset {
		f.appended(info)
	}
	f.seen = info
}

// restart reads the end of the file afresh, as the first look or after the file
// shrank or was replaced. It reports whether that worked; if not (the file went
// again, or can't be read this time) the next look tries again.
func (f *follower) restart() bool {
	f.partial, f.head = nil, nil
	data, size, err := readTail(f.path, initialWindow)
	if err != nil {
		return false
	}
	f.offset = size
	lines, rest := splitLines(data, size > int64(len(data)))
	f.partial = rest
	if len(lines) > f.opts.InitialLines {
		lines = lines[len(lines)-f.opts.InitialLines:]
	}
	f.rememberHead()
	f.emit(Batch{Reset: true, Lines: f.clip(lines)})
	return true
}

// rewritten reports whether the start of the file is no longer what it was.
func (f *follower) rewritten() bool {
	if len(f.head) == 0 {
		return false
	}
	now, err := readRange(f.path, 0, int64(len(f.head)))
	return err == nil && !bytes.Equal(now, f.head)
}

// rememberHead records the first bytes of the file, once enough of it has been
// read to have them.
func (f *follower) rememberHead() {
	want := f.offset
	if want > headLen {
		want = headLen
	}
	if int64(len(f.head)) >= want {
		return
	}
	if b, err := readRange(f.path, 0, want); err == nil {
		f.head = b
	}
}

// appended reads what was added since the last look.
func (f *follower) appended(info os.FileInfo) {
	start := f.offset
	var skipped int64
	if info.Size()-start > f.opts.MaxChunk {
		// More arrived than can be shown: keep the newest. The cut is almost
		// certainly mid-line, so what comes before the first newline is dropped
		// along with everything skipped.
		start = info.Size() - f.opts.MaxChunk
		skipped = start - f.offset
		f.partial = nil
	}
	data, err := readRange(f.path, start, info.Size()-start)
	if err != nil {
		return
	}
	f.offset = start + int64(len(data))
	f.rememberHead()
	if skipped > 0 {
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			skipped += int64(i + 1)
			data = data[i+1:]
		}
	}
	lines, rest := splitLines(append(f.partial, data...), false)
	f.partial = rest
	if len(lines) > 0 || skipped > 0 {
		f.emit(Batch{Lines: f.clip(lines), Skipped: skipped})
	}
}

// clip shortens over-long lines and repairs invalid UTF-8, so what is emitted can
// always be shown.
func (f *follower) clip(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	for i, l := range lines {
		if !utf8.ValidString(l) {
			l = strings.ToValidUTF8(l, "�")
		}
		if utf8.RuneCountInString(l) > f.opts.MaxLineLength {
			l = string([]rune(l)[:f.opts.MaxLineLength]) + "..."
		}
		lines[i] = l
	}
	return lines
}

// splitLines splits data into its complete lines and the unfinished remainder
// after the last newline. A trailing carriage return is dropped from each line.
// If dropFirst, data began mid-line (it is the end of a larger file), so the
// first, partial, line is discarded.
func splitLines(data []byte, dropFirst bool) (lines []string, rest []byte) {
	if dropFirst {
		i := indexByte(data, '\n')
		if i < 0 {
			return nil, capPartial(data)
		}
		data = data[i+1:]
	}
	for {
		i := indexByte(data, '\n')
		if i < 0 {
			break
		}
		line := data[:i]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		lines = append(lines, string(line))
		data = data[i+1:]
	}
	return lines, capPartial(data)
}

// capPartial copies rest (so it doesn't pin the buffer it came from) and bounds
// it: a "line" this long with no newline isn't going to get one.
func capPartial(rest []byte) []byte {
	if len(rest) > maxPartial {
		rest = rest[len(rest)-maxPartial:]
	}
	return append([]byte(nil), rest...)
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// readTail returns the last n bytes of the file (all of it if smaller) and the
// file's size when read.
func readTail(path string, n int64) (data []byte, size int64, err error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, 0, err
	}
	size = info.Size()
	start := size - n
	if start < 0 {
		start = 0
	}
	data, err = readAt(fh, start, size-start)
	return data, start + int64(len(data)), err
}

// readRange reads up to length bytes from offset.
func readRange(path string, offset, length int64) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return readAt(fh, offset, length)
}

func readAt(fh *os.File, offset, length int64) ([]byte, error) {
	buf := make([]byte, length)
	n, err := fh.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:n], nil
}
