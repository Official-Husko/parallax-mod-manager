package workshop

import (
	"bufio"
	"bytes"
	"context"
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

// helperRequest mirrors companions/parallax-steam-helper's own Request type
// exactly, field for field - hand-written here, not shared, since these are
// two separate Go modules (the same reason internal/launchershim mirrors
// companions/launcher-shim's own status type rather than importing it).
type helperRequest struct {
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

	payload, err := json.Marshal(helperRequest{
		LibraryPath:   req.LibraryPath,
		WorkDir:       workDir,
		AppID:         req.AppID,
		ItemID:        req.ItemID,
		Title:         req.Title,
		Description:   req.Description,
		ChangeNote:    req.ChangeNote,
		ContentFolder: req.ContentFolder,
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
