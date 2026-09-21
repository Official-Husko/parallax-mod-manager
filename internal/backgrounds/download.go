package backgrounds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// Job is one game's images to fetch.
type Job struct {
	GameID string
	Files  []File
}

// Progress is one snapshot of a running download, for the UI's progress bar.
type Progress struct {
	// State is "running" while it works, then exactly one of "done" (finished,
	// possibly with some files failed), "cancelled" or "error".
	State string
	// GameID and File name what is being fetched right now.
	GameID string
	File   string
	// FilesDone/FilesTotal and BytesDone/BytesTotal count only what has to be
	// downloaded - images already on disk with the right size are not in them.
	FilesDone  int
	FilesTotal int
	BytesDone  int64
	BytesTotal int64
	// Skipped is how many images were already there.
	Skipped int
	// Failed is how many gave up after their retries.
	Failed         int
	BytesPerSecond int64
	Error          string
}

// Result is what a finished Run did.
type Result struct {
	Downloaded  int
	Skipped     int
	Failed      int
	FailedFiles []string
	Bytes       int64
}

// FileResult is how one image ended, for callers that want to log each.
type FileResult struct {
	GameID string
	Name   string
	// Size is the image's size in bytes as listed.
	Size int64
	// Took is how long it took, retries included.
	Took time.Duration
	// Err is why it failed after its retries; nil when it was downloaded. An image
	// that stopped because the run was cancelled is not reported at all.
	Err error
}

// Downloader fetches images into a Store.
type Downloader struct {
	Client *http.Client
	Store  Store
	// URL says where one image comes from.
	URL func(gameID, name string) string
	// Concurrency is how many images are fetched at once (default 3).
	Concurrency int
	// Attempts is how many times one image is tried before it counts as failed
	// (default 3); RetryDelay is the pause before the next try (default 300ms).
	Attempts   int
	RetryDelay time.Duration
	// UserAgent is sent with every request.
	UserAgent string
	// OnFile, when set, is called as each image ends (from the worker that
	// fetched it, so it must be safe to call concurrently).
	OnFile func(FileResult)
}

// reportEvery is how often byte progress is reported while data is flowing; the
// start and end of every image are always reported.
const reportEvery = 150 * time.Millisecond

type task struct {
	gameID string
	file   File
}

// Run downloads what is missing from jobs, reporting progress as it goes. Each
// image is written to a temporary file and renamed only when complete and the
// right size, so an interrupted run never leaves a half image that looks whole,
// and running it again resumes: what is already there is skipped. Cancelling ctx
// stops it and removes the partial file. A failed image does not stop the others.
func (d Downloader) Run(ctx context.Context, jobs []Job, report func(Progress)) (Result, error) {
	if d.Store.Dir == "" || d.URL == nil {
		return Result{}, errors.New("backgrounds: downloader is not configured")
	}
	var (
		todo    []task
		res     Result
		p       Progress
		started = time.Now()
	)
	for _, j := range jobs {
		if !ValidGameID(j.GameID) {
			return Result{}, fmt.Errorf("backgrounds: %q is not a valid game id", j.GameID)
		}
		for _, f := range j.Files {
			if !ValidName(f.Name) {
				continue // never create a file for a name that is not a plain image name
			}
			if d.Store.Has(j.GameID, f) {
				res.Skipped++
				continue
			}
			todo = append(todo, task{j.GameID, f})
			p.BytesTotal += f.Size
		}
	}
	p.FilesTotal = len(todo)
	p.Skipped = res.Skipped
	p.State = "running"

	var (
		mu       sync.Mutex
		lastSent time.Time
		active   []task
	)
	// emit sends a snapshot; the caller holds mu. force skips the rate limit.
	emit := func(force bool) {
		if report == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastSent) < reportEvery {
			return
		}
		lastSent = now
		if secs := now.Sub(started).Seconds(); secs > 0 {
			p.BytesPerSecond = int64(float64(p.BytesDone) / secs)
		}
		if len(active) > 0 {
			p.GameID, p.File = active[0].gameID, active[0].file.Name
		}
		report(p)
	}

	mu.Lock()
	emit(true)
	mu.Unlock()

	workers := d.Concurrency
	if workers <= 0 {
		workers = 3
	}
	queue := make(chan task)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range queue {
				mu.Lock()
				active = append(active, t)
				emit(true)
				mu.Unlock()

				var counted int64
				began := time.Now()
				err := d.fetchWithRetries(ctx, t, func(n int64) {
					mu.Lock()
					p.BytesDone += n
					counted += n
					emit(false)
					mu.Unlock()
				}, func() {
					// A try failed part-way: what it counted is not progress any more.
					mu.Lock()
					p.BytesDone -= counted
					counted = 0
					mu.Unlock()
				})

				mu.Lock()
				for i, a := range active {
					if a == t {
						active = append(active[:i], active[i+1:]...)
						break
					}
				}
				switch {
				case err == nil:
					p.FilesDone++
					res.Downloaded++
					res.Bytes += t.file.Size
				case ctx.Err() != nil:
					p.BytesDone -= counted
				default:
					p.Failed++
					p.BytesDone -= counted
					res.Failed++
					res.FailedFiles = append(res.FailedFiles, t.gameID+"/"+t.file.Name)
				}
				emit(true)
				mu.Unlock()
				if d.OnFile != nil && (err == nil || ctx.Err() == nil) {
					d.OnFile(FileResult{GameID: t.gameID, Name: t.file.Name, Size: t.file.Size, Took: time.Since(began), Err: err})
				}
			}
		}()
	}
feed:
	for _, t := range todo {
		select {
		case queue <- t:
		case <-ctx.Done():
			break feed
		}
	}
	close(queue)
	wg.Wait()

	res.Skipped = p.Skipped
	mu.Lock()
	defer mu.Unlock()
	active = nil
	if err := ctx.Err(); err != nil {
		p.State = "cancelled"
		emit(true)
		return res, err
	}
	p.State = "done"
	emit(true)
	return res, nil
}

// fetchWithRetries tries one image up to Attempts times. counted is told about
// every byte received; undo is called when a try is abandoned.
func (d Downloader) fetchWithRetries(ctx context.Context, t task, counted func(int64), undo func()) error {
	attempts := d.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	delay := d.RetryDelay
	if delay <= 0 {
		delay = 300 * time.Millisecond
	}
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = d.fetchOnce(ctx, t, counted); err == nil {
			return nil
		}
		undo()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
}

func (d Downloader) fetchOnce(ctx context.Context, t task, counted func(int64)) error {
	final, ok := d.Store.Path(t.gameID, t.file.Name)
	if !ok {
		return fmt.Errorf("backgrounds: %s/%s is not a storable image", t.gameID, t.file.Name)
	}
	if err := os.MkdirAll(d.Store.GameDir(t.gameID), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.URL(t.gameID, t.file.Name), nil)
	if err != nil {
		return err
	}
	if d.UserAgent != "" {
		req.Header.Set("User-Agent", d.UserAgent)
	}
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	part := final + partSuffix
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, &countingReader{r: resp.Body, on: counted})
	closeErr := out.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil && t.file.Size > 0 && written != t.file.Size {
		copyErr = fmt.Errorf("size mismatch: got %d bytes, expected %d", written, t.file.Size)
	}
	if copyErr != nil {
		os.Remove(part)
		return copyErr
	}
	if err := os.Rename(part, final); err != nil {
		os.Remove(part)
		return err
	}
	return nil
}

type countingReader struct {
	r  io.Reader
	on func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.on(int64(n))
	}
	return n, err
}
