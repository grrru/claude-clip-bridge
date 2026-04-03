package xclip

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"claude-clip-bridge/internal/bridge"
	"claude-clip-bridge/internal/testutil"
)

var shimTestToken = [bridge.TokenSize]byte{
	10, 20, 30, 40, 50, 60, 70, 80, 90, 100,
	11, 21, 31, 41, 51, 61, 71, 81, 91, 101,
	12, 22, 32, 42, 52, 62, 72, 82, 92, 102,
	13, 23,
}

type stubFinder struct {
	path string
	ok   bool
	err  error
}

func (s stubFinder) FindReachable() (string, bool, error) {
	return s.path, s.ok, s.err
}

func TestRunTargetsReturnsLocalCapabilities(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run([]string{"-selection", "clipboard", "-t", "TARGETS", "-o"}, Config{
		Stdout: stdoutWriter(&stdout),
		Stderr: io.Discard,
		Finder: stubFinder{path: "127.0.0.1:19876", ok: true},
		Passthrough: func(string, []string, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("passthrough should not be called")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "TARGETS\nimage/png\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunTargetsFallsBackWithoutBridge(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	called := false
	err := Run([]string{"-selection", "clipboard", "-t", "TARGETS", "-o"}, Config{
		Stdout: stdoutWriter(&stdout),
		Stderr: io.Discard,
		Finder: stubFinder{},
		Passthrough: func(string, []string, io.Reader, io.Writer, io.Writer) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatal("expected passthrough to be called")
	}
}

func TestRunDebugLogsStayOnStderr(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Run([]string{"-selection", "clipboard", "-t", "image/jpeg", "-o"}, Config{
		Stdout: stdoutWriter(&stdout),
		Stderr: &stderr,
		Debug:  true,
		Finder: stubFinder{path: "127.0.0.1:19876", ok: true},
		Passthrough: func(string, []string, io.Reader, io.Writer, io.Writer) error {
			_, writeErr := io.WriteString(&stdout, "payload")
			return writeErr
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if stdout.String() != "payload" {
		t.Fatalf("stdout = %q, want payload", stdout.String())
	}
	if got := stderr.String(); got == "" {
		t.Fatal("expected debug log on stderr")
	}
}

func TestRunNonPNGFallsBack(t *testing.T) {
	t.Parallel()

	called := false
	err := Run([]string{"-selection", "clipboard", "-t", "image/jpeg", "-o"}, Config{
		Stdout: io.Discard,
		Stderr: io.Discard,
		Finder: stubFinder{path: "127.0.0.1:19876", ok: true},
		Passthrough: func(string, []string, io.Reader, io.Writer, io.Writer) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatal("expected passthrough to be called")
	}
}

func TestRunPNGBridgeExchange(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	tokenFile := writeTokenFile(t, shimTestToken)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := bridge.NewServer(bridge.ServerConfig{
		Addr:      addr,
		Token:     shimTestToken,
		Clipboard: bridgeClipboard{payload: []byte("png-bytes")},
		Logger:    log.New(io.Discard, "", 0),
	})

	done := make(chan error, 1)
	go func() {
		done <- server.ListenAndServe(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	testutil.Eventually(t, time.Second, func() bool {
		return testutil.ReachableTCP(addr, 20*time.Millisecond)
	})

	var stdout bytes.Buffer
	err := Run([]string{"-selection", "clipboard", "-t", "image/png", "-o"}, Config{
		Stdout:    stdoutWriter(&stdout),
		Stderr:    io.Discard,
		TokenFile: tokenFile,
		Finder:    stubFinder{path: addr, ok: true},
		Passthrough: func(string, []string, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("passthrough should not be called")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := stdout.String(); got != "png-bytes" {
		t.Fatalf("stdout = %q, want %q", got, "png-bytes")
	}
}

func TestRunPNGFallsBackWithoutTokenFile(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := bridge.NewServer(bridge.ServerConfig{
		Addr:      addr,
		Token:     shimTestToken,
		Clipboard: bridgeClipboard{payload: []byte("png-bytes")},
		Logger:    log.New(io.Discard, "", 0),
	})
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe(ctx) }()
	defer func() { cancel(); <-done }()

	testutil.Eventually(t, time.Second, func() bool {
		return testutil.ReachableTCP(addr, 20*time.Millisecond)
	})

	called := false
	err := Run([]string{"-selection", "clipboard", "-t", "image/png", "-o"}, Config{
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		TokenFile: "/nonexistent/token",
		Finder:    stubFinder{path: addr, ok: true},
		Passthrough: func(string, []string, io.Reader, io.Writer, io.Writer) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !called {
		t.Fatal("expected passthrough when token file missing")
	}
}

type bridgeClipboard struct {
	payload []byte
}

func (b bridgeClipboard) PNG(context.Context) ([]byte, error) {
	return b.payload, nil
}

func stdoutWriter(buffer *bytes.Buffer) io.Writer {
	return buffer
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func writeTokenFile(t *testing.T, token [bridge.TokenSize]byte) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(token[:])), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
