package workshop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/fsutil"
)

// helperBinaryName names companions/parallax-steam-helper's own pre-built
// output for the current OS - see the root build.sh, which places every
// companion for both linux/amd64 and windows/amd64 side by side in a
// "companions" folder next to this app's own executable, regardless of
// which OS this app itself happens to be running on (matching
// internal/launchershim's own shimBinaryName).
func helperBinaryName() string {
	if runtime.GOOS == "windows" {
		return "parallax-steam-helper-windows-amd64.exe"
	}
	return "parallax-steam-helper-linux-amd64"
}

// helperPath resolves companions/parallax-steam-helper's own pre-built
// binary, relative to this app's own executable (see helperBinaryName) -
// mirroring internal/launchershim's own shimSourcePath.
func helperPath() (string, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("workshop: finding this app's own path: %w", err)
	}
	return filepath.Join(filepath.Dir(selfPath), "companions", helperBinaryName()), nil
}

// workDirFor is where the helper writes its own steam_appid.txt for one
// publish - a fresh, dedicated folder under this app's own config directory,
// scoped by AppID so two different games' publishes (unlikely to ever
// actually overlap in time, but cheap to keep separate) never share one.
// Never the target game's own install directory.
func workDirFor(appID uint32) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("workshop: finding the user config directory: %w", err)
	}
	return filepath.Join(configDir, "parallax-mod-manager", "steam_publish_work", strconv.FormatUint(uint64(appID), 10)), nil
}

// stageExcluding copies contentFolder into a fresh "staged-content" folder
// under workDir, leaving out every path in excludePaths (see
// PublishRequest.ExcludePaths), and returns that copy's path plus a cleanup
// func that removes it - called unconditionally via defer once Publish is
// done with it, whether the publish itself succeeded or not. The real
// content folder is never modified or read destructively; this only ever
// adds a temporary copy elsewhere.
func stageExcluding(ctx context.Context, contentFolder, workDir string, excludePaths []string) (staged string, stats fsutil.Stats, cleanup func(), err error) {
	exclude := make(map[string]bool, len(excludePaths))
	for _, p := range excludePaths {
		exclude[strings.TrimRight(filepath.ToSlash(p), "/")] = true
	}

	staged = filepath.Join(workDir, "staged-content")
	if err := os.RemoveAll(staged); err != nil {
		return "", fsutil.Stats{}, nil, fmt.Errorf("clearing an old staging folder at %s: %w", staged, err)
	}
	if err := os.MkdirAll(staged, 0o755); err != nil {
		return "", fsutil.Stats{}, nil, fmt.Errorf("creating a staging folder at %s: %w", staged, err)
	}

	stats, err = fsutil.CopyTreeExcluding(ctx, contentFolder, staged, exclude, 0, nil)
	if err != nil {
		_ = os.RemoveAll(staged)
		return "", fsutil.Stats{}, nil, fmt.Errorf("copying %s to %s without the excluded files: %w", contentFolder, staged, err)
	}
	return staged, stats, func() { _ = os.RemoveAll(staged) }, nil
}

// helperRequest mirrors companions/parallax-steam-helper's own Request type
// exactly, field for field - hand-written here, not shared, since these are
// two separate Go modules (the same reason internal/launchershim mirrors
// companions/launcher-shim's own status type rather than importing it).
type helperRequest struct {
	Mode          string `json:"mode,omitempty"`
	LibraryPath   string `json:"libraryPath"`
	WorkDir       string `json:"workDir"`
	AppID         uint32 `json:"appId"`
	ItemID        uint64 `json:"itemId"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	ChangeNote    string `json:"changeNote"`
	ContentFolder string `json:"contentFolder"`
	PreviewFile   string `json:"previewFile,omitempty"`
	Visibility    string `json:"visibility"`
}

// helperEvent mirrors companions/parallax-steam-helper's own Event type.
type helperEvent struct {
	Stage           string `json:"stage"`
	Message         string `json:"message,omitempty"`
	Processed       uint64 `json:"processed,omitempty"`
	Total           uint64 `json:"total,omitempty"`
	PublishedFileID uint64 `json:"publishedFileId,omitempty"`
	PersonaName     string `json:"personaName,omitempty"`
	SteamID         string `json:"steamId,omitempty"`
	// AvatarPNG is still base64-encoded here, exactly as the helper wrote
	// it - decoded to real bytes only once, in Identity below.
	AvatarPNG string `json:"avatarPng,omitempty"`
}

// HelperPublisher is the real Publisher, driving a real
// companions/parallax-steam-helper process. The zero value resolves the
// helper's path automatically (see helperPath); BinaryPath overrides that,
// for tests only.
type HelperPublisher struct {
	BinaryPath string
}

func (h HelperPublisher) binaryPath() (string, error) {
	if h.BinaryPath != "" {
		return h.BinaryPath, nil
	}
	return helperPath()
}

// Publish starts the helper, writes req as JSON on its stdin, and streams
// its stdout back through onProgress - see decodeEvents for the actual
// line-by-line protocol handling, kept separate so it is testable without a
// real subprocess.
func (h HelperPublisher) Publish(ctx context.Context, req PublishRequest, onProgress func(PublishProgress)) (PublishResult, error) {
	binPath, err := h.binaryPath()
	if err != nil {
		return PublishResult{}, err
	}
	if _, err := os.Stat(binPath); err != nil {
		return PublishResult{}, fmt.Errorf("workshop: helper binary not found at %s (has it been installed with this build?): %w", binPath, err)
	}

	workDir, err := workDirFor(req.AppID)
	if err != nil {
		return PublishResult{}, err
	}

	contentFolder := req.ContentFolder
	if len(req.ExcludePaths) > 0 && contentFolder != "" {
		if onProgress != nil {
			onProgress(PublishProgress{Stage: "staging", Message: fmt.Sprintf("leaving out %d item(s) you chose to exclude", len(req.ExcludePaths))})
		}
		staged, stats, cleanup, err := stageExcluding(ctx, contentFolder, workDir, req.ExcludePaths)
		if err != nil {
			return PublishResult{}, fmt.Errorf("workshop: preparing files to upload: %w", err)
		}
		defer cleanup()
		contentFolder = staged
		if onProgress != nil {
			onProgress(PublishProgress{Stage: "staged", Processed: uint64(stats.Files)})
		}
	}

	payload, err := json.Marshal(helperRequest{
		LibraryPath:   req.LibraryPath,
		WorkDir:       workDir,
		AppID:         req.AppID,
		ItemID:        req.ItemID,
		Title:         req.Title,
		Description:   req.Description,
		ChangeNote:    req.ChangeNote,
		ContentFolder: contentFolder,
		PreviewFile:   req.PreviewFile,
		Visibility:    req.Visibility,
	})
	if err != nil {
		return PublishResult{}, fmt.Errorf("workshop: encoding the helper request: %w", err)
	}

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdin = bytes.NewReader(payload)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return PublishResult{}, fmt.Errorf("workshop: opening the helper's stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return PublishResult{}, fmt.Errorf("workshop: starting the helper: %w", err)
	}

	result, decodeErr := decodeEvents(stdout, onProgress)
	waitErr := cmd.Wait()

	if decodeErr != nil {
		return PublishResult{}, fmt.Errorf("workshop: %w", decodeErr)
	}
	if waitErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return PublishResult{}, fmt.Errorf("workshop: the helper exited with an error: %s", msg)
		}
		return PublishResult{}, fmt.Errorf("workshop: the helper exited with an error: %w", waitErr)
	}
	return result, nil
}

// Identity starts the helper in "identity" mode - no item is created or
// touched, and no local staging happens (there is no content to stage) - and
// returns whatever persona name its final "done" event reports.
func (h HelperPublisher) Identity(ctx context.Context, req IdentityRequest) (IdentityResult, error) {
	binPath, err := h.binaryPath()
	if err != nil {
		return IdentityResult{}, err
	}
	if _, err := os.Stat(binPath); err != nil {
		return IdentityResult{}, fmt.Errorf("workshop: helper binary not found at %s (has it been installed with this build?): %w", binPath, err)
	}

	workDir, err := workDirFor(req.AppID)
	if err != nil {
		return IdentityResult{}, err
	}

	payload, err := json.Marshal(helperRequest{Mode: "identity", LibraryPath: req.LibraryPath, WorkDir: workDir, AppID: req.AppID})
	if err != nil {
		return IdentityResult{}, fmt.Errorf("workshop: encoding the helper request: %w", err)
	}

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Stdin = bytes.NewReader(payload)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return IdentityResult{}, fmt.Errorf("workshop: opening the helper's stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return IdentityResult{}, fmt.Errorf("workshop: starting the helper: %w", err)
	}

	var personaName, steamID, avatarPNG string
	var decodeErr error
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var e helperEvent
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		switch e.Stage {
		case "done":
			personaName = e.PersonaName
			steamID = e.SteamID
			avatarPNG = e.AvatarPNG
		case "error":
			decodeErr = errors.New(e.Message)
		}
	}
	waitErr := cmd.Wait()

	if decodeErr != nil {
		return IdentityResult{}, fmt.Errorf("workshop: %w", decodeErr)
	}
	if waitErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return IdentityResult{}, fmt.Errorf("workshop: the helper exited with an error: %s", msg)
		}
		return IdentityResult{}, fmt.Errorf("workshop: the helper exited with an error: %w", waitErr)
	}
	// A corrupt/truncated base64 avatar is treated the same as none at all -
	// this whole identity request has otherwise genuinely succeeded, and an
	// initial-letter avatar is a perfectly fine fallback, not worth failing
	// over.
	var avatar []byte
	if avatarPNG != "" {
		if decoded, err := base64.StdEncoding.DecodeString(avatarPNG); err == nil {
			avatar = decoded
		}
	}
	return IdentityResult{PersonaName: personaName, SteamID: steamID, Avatar: avatar}, nil
}

// errNoFinalEvent is decodeEvents' own error for a helper that exits
// (cleanly, from cmd.Wait's point of view) without ever reporting "done" or
// "error" - a helper bug, not a normal outcome, so it is always surfaced
// rather than silently treated as success.
var errNoFinalEvent = errors.New("the helper exited without reporting a final result")

// decodeEvents reads r as newline-delimited JSON helperEvents, forwarding
// each as a PublishProgress to onProgress (which may be nil), until EOF.
// Pulled out of Publish so this - the actual protocol logic - is testable
// directly against a fixed byte stream, without a real subprocess.
func decodeEvents(r io.Reader, onProgress func(PublishProgress)) (PublishResult, error) {
	var result PublishResult
	var publishErr error
	sawFinal := false

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var e helperEvent
		if err := json.Unmarshal(line, &e); err != nil {
			// A malformed line is surprising but not, on its own, worth
			// aborting an otherwise-succeeding publish over - cmd.Wait
			// (in Publish) still reports the real process outcome.
			continue
		}

		if onProgress != nil {
			onProgress(PublishProgress{
				Stage:           e.Stage,
				Message:         e.Message,
				Processed:       e.Processed,
				Total:           e.Total,
				PublishedFileID: e.PublishedFileID,
			})
		}

		switch e.Stage {
		case "done":
			result = PublishResult{PublishedFileID: e.PublishedFileID}
			sawFinal = true
		case "error":
			publishErr = errors.New(e.Message)
			sawFinal = true
		}
	}
	if err := scanner.Err(); err != nil {
		return PublishResult{}, fmt.Errorf("reading the helper's output: %w", err)
	}
	if publishErr != nil {
		return PublishResult{}, publishErr
	}
	if !sawFinal {
		return PublishResult{}, errNoFinalEvent
	}
	return result, nil
}
