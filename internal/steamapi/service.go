package steamapi

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Mode is how the optional Steam Web API key is used for Workshop details.
type Mode string

const (
	// ModeFree never uses a key: only the free endpoint is asked. The default.
	ModeFree Mode = "free"
	// ModeComplete asks the keyed endpoint for everything, and falls back to the
	// free one when the key is used up, rejected or unreachable.
	ModeComplete Mode = "complete"
	// ModeBackup asks the free endpoint first and the keyed one only for what the
	// free one could not answer: an item it failed on, or answered "not found" or
	// with another non-OK result (an unlisted item is the usual case).
	ModeBackup Mode = "backup"
)

// ParseMode reads a mode name; anything unknown or empty is ModeFree, the safe
// default (no key is ever used by mistake).
func ParseMode(s string) Mode {
	switch Mode(s) {
	case ModeComplete:
		return ModeComplete
	case ModeBackup:
		return ModeBackup
	}
	return ModeFree
}

// UsesKey says the mode may use a key.
func (m Mode) UsesKey() bool { return m == ModeComplete || m == ModeBackup }

const (
	// defaultCooldown is how long a key is left alone after Steam said it is out of
	// requests, when Steam did not say how long (its default quota is per day).
	defaultCooldown = time.Hour
	minCooldown     = 5 * time.Minute
	maxCooldown     = 24 * time.Hour
)

// Logger is where a Service writes what it does. It never receives the key.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

type discardLogger struct{}

func (discardLogger) Infof(string, ...any) {}
func (discardLogger) Warnf(string, ...any) {}

// Service decides, per request, which Steam API answers: the free one, the keyed
// one, or the free one after the keyed one gave out. It holds the key in memory
// only; storing it is not its business. Safe for concurrent use.
type Service struct {
	mu             sync.Mutex
	mode           Mode
	key            string
	rejected       bool
	exhaustedUntil time.Time
	lastError      string
	stats          Stats

	log Logger
	now func() time.Time
	// free and keyed default to the package's real requests; tests replace them.
	free  func(ctx context.Context, ids []string) (map[string]PublishedFileDetails, error)
	keyed func(ctx context.Context, key string, ids []string) (map[string]PublishedFileDetails, error)
}

// Stats counts what the Service has done this run.
type Stats struct {
	// ItemsFromKey and ItemsFromFree count the items each API answered.
	ItemsFromKey  int
	ItemsFromFree int
	// Rescued counts items only the key could return (Backup mode: the free API
	// failed or answered non-OK for them, the key returned them).
	Rescued int
}

// Status is a snapshot of a Service for the settings panel.
type Status struct {
	Mode   Mode
	HasKey bool
	// Rejected is true after Steam refused the key; the key is not used again until
	// it is saved anew or checked successfully.
	Rejected bool
	// ExhaustedUntil is when a used-up key is tried again; zero when it is not.
	ExhaustedUntil time.Time
	// KeyInUse is true when the next request would use the key.
	KeyInUse  bool
	LastError string
	Stats     Stats
}

// NewService is a Service in ModeFree. log may be nil.
func NewService(log Logger) *Service {
	if log == nil {
		log = discardLogger{}
	}
	return &Service{
		mode:  ModeFree,
		log:   log,
		now:   time.Now,
		free:  GetPublishedFileDetails,
		keyed: GetPublishedFileDetailsWithKey,
	}
}

// Configure sets the mode and the key together and clears any rejected or used-up
// state: a new key, or a new choice, deserves a fresh start. ModeFree drops the
// key from memory.
func (s *Service) Configure(mode Mode, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !mode.UsesKey() {
		mode, key = ModeFree, ""
	}
	s.mode, s.key = mode, key
	s.rejected = false
	s.exhaustedUntil = time.Time{}
	s.lastError = ""
}

// Key returns the key in memory, for the code that verifies or stores it.
func (s *Service) Key() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key
}

// MarkRejected records that Steam refused the key, so it is not used again until
// Configure is called.
func (s *Service) MarkRejected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejected = true
}

// Status snapshots the Service.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	return Status{
		Mode:           s.mode,
		HasKey:         s.key != "",
		Rejected:       s.rejected,
		ExhaustedUntil: s.exhaustedUntil,
		KeyInUse:       s.keyUsable(),
		LastError:      s.lastError,
		Stats:          s.stats,
	}
}

// expire forgets a used-up state whose time has passed. Callers hold mu.
func (s *Service) expire() {
	if !s.exhaustedUntil.IsZero() && !s.now().Before(s.exhaustedUntil) {
		s.exhaustedUntil = time.Time{}
	}
}

// keyUsable says whether the key would be used right now. Callers hold mu.
func (s *Service) keyUsable() bool {
	return s.mode.UsesKey() && s.key != "" && !s.rejected && s.exhaustedUntil.IsZero()
}

// Fetch returns Workshop details for ids according to the mode. It matches the
// signature of GetPublishedFileDetails, so it can stand in for it.
//
// An id neither API answered is absent from the result (the details cache asks
// again next time). An error is returned only when there is nothing to return at
// all.
func (s *Service) Fetch(ctx context.Context, ids []string) (map[string]PublishedFileDetails, error) {
	if len(ids) == 0 {
		return map[string]PublishedFileDetails{}, nil
	}
	s.mu.Lock()
	s.expire()
	mode, key, usable := s.mode, s.key, s.keyUsable()
	s.mu.Unlock()

	switch {
	case !usable:
		return s.fetchFree(ctx, ids)
	case mode == ModeComplete:
		return s.fetchComplete(ctx, key, ids)
	default:
		return s.fetchBackup(ctx, key, ids)
	}
}

// fetchFree asks the free endpoint only.
func (s *Service) fetchFree(ctx context.Context, ids []string) (map[string]PublishedFileDetails, error) {
	res, err := s.free(ctx, ids)
	s.mu.Lock()
	s.stats.ItemsFromFree += len(res)
	s.mu.Unlock()
	return res, err
}

// fetchComplete asks the key for everything, then the free endpoint for whatever
// the key did not return.
func (s *Service) fetchComplete(ctx context.Context, key string, ids []string) (map[string]PublishedFileDetails, error) {
	res, err := s.keyed(ctx, key, ids)
	s.noteKeyed(res, err)
	if res == nil {
		res = map[string]PublishedFileDetails{}
	}

	var missing []string
	for _, id := range ids {
		if _, ok := res[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return res, nil
	}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	freeRes, freeErr := s.free(ctx, missing)
	s.mu.Lock()
	s.stats.ItemsFromFree += len(freeRes)
	s.mu.Unlock()
	for id, d := range freeRes {
		res[id] = d
	}
	if len(res) == 0 && freeErr != nil {
		return nil, freeErr
	}
	return res, nil
}

// fetchBackup asks the free endpoint first, then the key for what it could not
// answer well.
func (s *Service) fetchBackup(ctx context.Context, key string, ids []string) (map[string]PublishedFileDetails, error) {
	res, freeErr := s.free(ctx, ids)
	if res == nil {
		res = map[string]PublishedFileDetails{}
	}
	s.mu.Lock()
	s.stats.ItemsFromFree += len(res)
	s.mu.Unlock()

	var need []string
	for _, id := range ids {
		if d, ok := res[id]; !ok || d.Result != int(ResultOK) {
			need = append(need, id)
		}
	}
	if len(need) == 0 {
		return res, nil
	}
	if ctx.Err() != nil {
		if len(res) == 0 {
			return nil, ctx.Err()
		}
		return res, nil
	}

	keyedRes, err := s.keyed(ctx, key, need)
	s.noteKeyed(keyedRes, err)
	rescued := 0
	for id, d := range keyedRes {
		// The keyed answer replaces the free one either way: for an item that is
		// still not OK it is the more precise (ItemDeleted, not FileNotFound).
		if old, had := res[id]; !had || old.Result != int(ResultOK) {
			if d.Result == int(ResultOK) {
				rescued++
			}
			res[id] = d
		}
	}
	if rescued > 0 {
		s.mu.Lock()
		s.stats.Rescued += rescued
		s.mu.Unlock()
		s.log.Infof("the Steam API key returned %d Workshop items the free API could not (of %d asked for)", rescued, len(need))
	}
	if len(res) == 0 {
		if freeErr != nil {
			return nil, freeErr
		}
		return nil, err
	}
	return res, nil
}

// noteKeyed records the outcome of a keyed request: the counters, and the state a
// rejected or used-up key puts the Service in.
func (s *Service) noteKeyed(res map[string]PublishedFileDetails, err error) {
	s.mu.Lock()
	s.stats.ItemsFromKey += len(res)
	var rl *RateLimitedError
	switch {
	case err == nil:
		s.lastError = ""
		s.mu.Unlock()
	case errors.Is(err, ErrKeyRejected):
		s.rejected = true
		s.lastError = "Steam rejected the API key"
		s.mu.Unlock()
		s.log.Warnf("Steam rejected the API key, so the free API is used until a key is saved again")
	case errors.As(err, &rl):
		cooldown := rl.RetryAfter
		if cooldown <= 0 {
			cooldown = defaultCooldown
		}
		cooldown = min(max(cooldown, minCooldown), maxCooldown)
		s.exhaustedUntil = s.now().Add(cooldown)
		s.lastError = "the API key is out of requests for now"
		until := s.exhaustedUntil
		s.mu.Unlock()
		s.log.Warnf("the Steam API key is out of requests, so the free API is used until %s", until.Format("15:04"))
	default:
		s.lastError = err.Error()
		s.mu.Unlock()
		s.log.Warnf("the keyed Steam API failed, so the free API is used for this request: %v", err)
	}
}
