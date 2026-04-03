package xclip

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPassthroughPreservesArgs(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join(t.TempDir(), "xclip.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunPassthrough(scriptPath, []string{"-selection", "clipboard", "-o"}, nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunPassthrough() error = %v", err)
	}

	got := stdout.String()
	want := "-selection\nclipboard\n-o\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunPassthroughMissingBinary(t *testing.T) {
	t.Parallel()

	err := RunPassthrough("/does/not/exist", nil, nil, &bytes.Buffer{}, &bytes.Buffer{})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("RunPassthrough() error = %v, want ExitError code 1", err)
	}
}
