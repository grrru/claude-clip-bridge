package bridge

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

type stubRunner struct {
	stdout []byte
	stderr []byte
	err    error
}

func (s stubRunner) Run(context.Context, string, ...string) ([]byte, []byte, error) {
	return s.stdout, s.stderr, s.err
}

func TestPNGPasteClipboardNoImage(t *testing.T) {
	t.Parallel()

	clipboard := NewPNGPasteClipboard("pngpaste", stubRunner{
		stderr: []byte("There is no image data in the clipboard"),
		err:    &exec.ExitError{},
	})

	_, err := clipboard.PNG(context.Background())
	if !errors.Is(err, ErrNoImage) {
		t.Fatalf("PNG() error = %v, want ErrNoImage", err)
	}
}

func TestPNGPasteClipboardRuntimeError(t *testing.T) {
	t.Parallel()

	clipboard := NewPNGPasteClipboard("pngpaste", stubRunner{
		err: errors.New("exec failed"),
	})

	_, err := clipboard.PNG(context.Background())
	if err == nil || errors.Is(err, ErrNoImage) {
		t.Fatalf("PNG() error = %v, want runtime error", err)
	}
}
