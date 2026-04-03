package bridge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var ErrNoImage = errors.New("no image data in clipboard")

type ClipboardProvider interface {
	PNG(ctx context.Context) ([]byte, error)
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout []byte, stderr []byte, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type PNGPasteClipboard struct {
	Path   string
	Runner CommandRunner
}

// pngpasteCandidates is the search order for the pngpaste binary.
// Background daemons launched via SSH LocalCommand inherit a minimal PATH
// that typically excludes Homebrew, so we probe known install locations.
var pngpasteCandidates = []string{
	"pngpaste",                   // PATH (works in interactive shells)
	"/opt/homebrew/bin/pngpaste", // Apple Silicon Homebrew
	"/usr/local/bin/pngpaste",    // Intel Homebrew
}

func NewPNGPasteClipboard(path string, runner CommandRunner) PNGPasteClipboard {
	if runner == nil {
		runner = ExecRunner{}
	}
	if path == "" {
		path = resolvePNGPaste()
	}

	return PNGPasteClipboard{Path: path, Runner: runner}
}

func resolvePNGPaste() string {
	for _, candidate := range pngpasteCandidates {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	// Absolute paths: check existence directly since LookPath skips them when not in PATH
	for _, candidate := range pngpasteCandidates {
		if len(candidate) > 0 && candidate[0] == '/' {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return "pngpaste" // last resort, will fail with a clear error
}

func (c PNGPasteClipboard) PNG(ctx context.Context) ([]byte, error) {
	stdout, stderr, err := c.Runner.Run(ctx, c.Path, "-")
	if err != nil {
		if looksLikeNoImage(stderr, err) {
			return nil, ErrNoImage
		}

		return nil, fmt.Errorf("run %s: %w", c.Path, err)
	}

	if len(stdout) == 0 {
		return nil, ErrNoImage
	}

	return stdout, nil
}

func looksLikeNoImage(stderr []byte, err error) bool {
	var execErr *exec.ExitError
	if !errors.As(err, &execErr) {
		return false
	}

	text := strings.ToLower(string(stderr))
	return strings.Contains(text, "no image") ||
		strings.Contains(text, "image data") ||
		strings.Contains(text, "png data")
}
