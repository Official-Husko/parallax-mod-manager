// Package applog is this app's activity log: one line per notable thing the
// app does, tagged with the part of the app that did it, in the same shape
// Go's standard logger uses -
//
//	2026/07/06 00:09:37 [Scan] 'Stellaris': 86 mods, 4788 conflicts (2261ms)
//
// Every line is kept three ways: in a bounded in-memory ring (what the About
// page's log view shows, newest last), appended to a rotating file on disk
// (so there's something to attach to a bug report after a crash), and handed to
// any subscriber (how the UI gets new lines the moment they happen).
//
// Use it from anywhere with a component-scoped logger:
//
//	log := applog.For("Patch")
//	t := log.Begin()
//	... work ...
//	t.Infof("generation %d written: %d keys", gen, n) // logs "(1234ms)" too
//
// The package-level default hub means a package can log without being handed a
// logger, the same way the standard library's log package works; a test that
// wants isolation just makes its own with New. Logging never fails the caller:
// a file that can't be written is silently skipped, the in-memory ring keeps
// working.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level is how important a line is.
type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

// String is the lower-case name that crosses the Wails boundary (a bare int
// means nothing in JS - see internal/library's package comment).
func (l Level) String() string {
	switch l {
	case Debug:
		return "debug"
	case Warn:
		return "warn"
	case Error:
		return "error"
	default:
		return "info"
	}
}

// ParseLevel is the inverse of String; anything unrecognised is Info.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return Debug
	case "warn", "warning":
		return Warn
	case "error":
		return Error
	default:
		return Info
	}
}

// Entry is one log line.
type Entry struct {
	// Seq numbers lines in the order they were logged, from 1, so a consumer
	// that sees the same line twice (a snapshot and a live event overlapping)
	// can drop the duplicate.
	Seq int64
	// Time is when it was logged, Unix milliseconds - safe as a plain JS
	// number.
	Time      int64
	Level     string // "debug" | "info" | "warn" | "error"
	Component string
	Message   string
	// Timed is true when DurationMs is meaningful - a step that took 0ms still
	// reports "(0ms)", so absence has to be a separate flag.
	Timed      bool
	DurationMs int64
}

// Line renders e as one line of text, the format written to the log file and
// copied out of the UI: "2006/01/02 15:04:05 [Component] message (12ms)". An
// Info line has no level marker; the others carry "DEBUG:", "WARN:" or
// "ERROR:" so they stand out in a plain text file too.
func (e Entry) Line() string {
	var b strings.Builder
	b.WriteString(time.UnixMilli(e.Time).Format("2006/01/02 15:04:05"))
	b.WriteString(" [")
	b.WriteString(e.Component)
	b.WriteString("] ")
	switch ParseLevel(e.Level) {
	case Debug:
		b.WriteString("DEBUG: ")
	case Warn:
		b.WriteString("WARN: ")
	case Error:
		b.WriteString("ERROR: ")
	}
	b.WriteString(e.Message)
	if e.Timed {
		fmt.Fprintf(&b, " (%dms)", e.DurationMs)
	}
	return b.String()
}

// DefaultCapacity is how many lines the default hub keeps in memory.
const DefaultCapacity = 2000

// Hub collects log lines. The zero value is not usable; use New.
type Hub struct {
	mu       sync.Mutex
	ring     []Entry
	start    int // index of the oldest line in ring
	count    int
	seq      int64
	subs     map[int]func(Entry)
	nextSub  int
	file     *rotatingFile
	minLevel Level
}

// New returns a hub that remembers the most recent capacity lines.
func New(capacity int) *Hub {
	if capacity < 1 {
		capacity = 1
	}
	return &Hub{ring: make([]Entry, capacity), subs: map[int]func(Entry){}, minLevel: Debug}
}

var std = New(DefaultCapacity)

// Default is the package-level hub For logs to.
func Default() *Hub { return std }

// For returns a logger on the default hub, tagging every line with component.
func For(component string) Logger { return std.For(component) }

// SetMinLevel drops lines below level entirely (nothing is stored, written or
// sent). New hubs keep everything, Debug included.
func (h *Hub) SetMinLevel(level Level) {
	h.mu.Lock()
	h.minLevel = level
	h.mu.Unlock()
}

// SetFile starts appending every line to path (creating its folder), rotating
// once the file passes maxBytes: the current file becomes path+".1", replacing
// any older one, and a fresh file starts - so disk use stays under about twice
// maxBytes. A hub with no file just keeps memory.
func (h *Hub) SetFile(path string, maxBytes int64) error {
	f, err := openRotating(path, maxBytes)
	if err != nil {
		return err
	}
	h.mu.Lock()
	old := h.file
	h.file = f
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
	return nil
}

// FilePath reports the log file's path, or "" if there isn't one.
func (h *Hub) FilePath() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.file == nil {
		return ""
	}
	return h.file.path
}

// Close stops file logging. Lines logged afterwards still reach memory and
// subscribers.
func (h *Hub) Close() {
	h.mu.Lock()
	f := h.file
	h.file = nil
	h.mu.Unlock()
	if f != nil {
		f.close()
	}
}

// Entries returns a copy of every line still in memory, oldest first.
func (h *Hub) Entries() []Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Entry, 0, h.count)
	for i := 0; i < h.count; i++ {
		out = append(out, h.ring[(h.start+i)%len(h.ring)])
	}
	return out
}

// Clear forgets the in-memory lines. The file on disk is left alone: it's the
// record, and clearing a view shouldn't erase it.
func (h *Hub) Clear() {
	h.mu.Lock()
	h.start, h.count = 0, 0
	h.mu.Unlock()
}

// Subscribe calls fn for every line logged from now on and returns a function
// that stops it. fn runs while the hub's lock is held, so it must return
// quickly and must never call back into the hub - hand the entry to a channel
// (dropping it if full) and do the real work elsewhere.
func (h *Hub) Subscribe(fn func(Entry)) (cancel func()) {
	h.mu.Lock()
	id := h.nextSub
	h.nextSub++
	h.subs[id] = fn
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.subs, id)
		h.mu.Unlock()
	}
}

// Log records one line. duration is only shown when timed is true.
func (h *Hub) Log(level Level, component, message string, timed bool, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if level < h.minLevel {
		return
	}
	h.seq++
	e := Entry{
		Seq:        h.seq,
		Time:       time.Now().UnixMilli(),
		Level:      level.String(),
		Component:  cleanComponent(component),
		Message:    oneLine(message),
		Timed:      timed,
		DurationMs: duration.Milliseconds(),
	}
	if h.count < len(h.ring) {
		h.ring[(h.start+h.count)%len(h.ring)] = e
		h.count++
	} else {
		h.ring[h.start] = e
		h.start = (h.start + 1) % len(h.ring)
	}
	if h.file != nil {
		h.file.writeLine(e.Line())
	}
	for _, fn := range h.subs {
		fn(e)
	}
}

// oneLine flattens a message to a single line: the file is line-oriented and
// the viewer draws one row per entry, so an embedded newline (an error that
// wraps a multi-line message, say) becomes " | ".
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", " | ")
	s = strings.ReplaceAll(s, "\n", " | ")
	return strings.ReplaceAll(s, "\r", " ")
}

// cleanComponent keeps a component tag short and printable - it comes from the
// frontend too (see App.LogEvent), so it can't be trusted to be tidy.
func cleanComponent(c string) string {
	c = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '[' || r == ']' {
			return -1
		}
		return r
	}, strings.TrimSpace(c))
	if len(c) > 24 {
		c = c[:24]
	}
	if c == "" {
		return "App"
	}
	return c
}

// Logger logs on one hub under one component name.
type Logger struct {
	hub       *Hub
	component string
}

// For returns a logger on h tagging every line with component.
func (h *Hub) For(component string) Logger { return Logger{hub: h, component: component} }

func (l Logger) log(level Level, format string, args []any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.hub.Log(level, l.component, msg, false, 0)
}

// Debugf, Infof, Warnf and Errorf log a formatted line at that level. With no
// arguments the format is used as-is, so a message containing a literal "%"
// is safe.
func (l Logger) Debugf(format string, args ...any) { l.log(Debug, format, args) }
func (l Logger) Infof(format string, args ...any)  { l.log(Info, format, args) }
func (l Logger) Warnf(format string, args ...any)  { l.log(Warn, format, args) }
func (l Logger) Errorf(format string, args ...any) { l.log(Error, format, args) }

// Begin starts timing a step; finish it with one of the Timer's methods, which
// log the line with how long it took.
func (l Logger) Begin() Timer { return Timer{l: l, start: time.Now()} }

// Timer logs a line together with the time since Begin.
type Timer struct {
	l     Logger
	start time.Time
}

// Elapsed is the time since Begin.
func (t Timer) Elapsed() time.Duration { return time.Since(t.start) }

func (t Timer) log(level Level, format string, args []any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	t.l.hub.Log(level, t.l.component, msg, true, t.Elapsed())
}

func (t Timer) Debugf(format string, args ...any) { t.log(Debug, format, args) }
func (t Timer) Infof(format string, args ...any)  { t.log(Info, format, args) }
func (t Timer) Warnf(format string, args ...any)  { t.log(Warn, format, args) }
func (t Timer) Errorf(format string, args ...any) { t.log(Error, format, args) }

// rotatingFile appends lines to a file, rolling it over at a size limit.
type rotatingFile struct {
	path string
	max  int64
	f    *os.File
	size int64
}

func openRotating(path string, maxBytes int64) (*rotatingFile, error) {
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("applog: creating log folder: %w", err)
	}
	r := &rotatingFile{path: path, max: maxBytes}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("applog: opening log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("applog: reading log file size: %w", err)
	}
	r.f, r.size = f, info.Size()
	return nil
}

func (r *rotatingFile) writeLine(line string) {
	if r.f == nil {
		return
	}
	if r.size >= r.max {
		r.f.Close()
		r.f = nil
		os.Remove(r.path + ".1")
		os.Rename(r.path, r.path+".1")
		if err := r.open(); err != nil {
			return
		}
	}
	n, err := r.f.WriteString(line + "\n")
	if err != nil {
		return
	}
	r.size += int64(n)
}

func (r *rotatingFile) close() {
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
}
