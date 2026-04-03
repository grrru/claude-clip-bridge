package bridge

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"claude-clip-bridge/internal/testutil"
)

type stubClipboard struct {
	payload []byte
	err     error
}

func (s stubClipboard) PNG(context.Context) ([]byte, error) {
	return s.payload, s.err
}

func TestServerHappyPath(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	server, cancel, done := startServer(t, ServerConfig{
		Addr:      addr,
		Token:     testToken,
		Clipboard: stubClipboard{payload: []byte("png")},
		Logger:    log.New(io.Discard, "", 0),
	})
	defer func() {
		cancel()
		<-done
	}()

	payload := requestPayload(t, addr)
	if string(payload) != "png" {
		t.Fatalf("requestPayload() = %q, want %q", payload, "png")
	}

	if server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestServerEmptyClipboard(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	_, cancel, done := startServer(t, ServerConfig{
		Addr:      addr,
		Token:     testToken,
		Clipboard: stubClipboard{err: ErrNoImage},
		Logger:    log.New(io.Discard, "", 0),
	})
	defer func() {
		cancel()
		<-done
	}()

	payload := requestPayload(t, addr)
	if payload != nil {
		t.Fatalf("requestPayload() = %v, want nil", payload)
	}
}

func TestServerRuntimeFailureClosesConnection(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	_, cancel, done := startServer(t, ServerConfig{
		Addr:      addr,
		Token:     testToken,
		Clipboard: stubClipboard{err: errors.New("boom")},
		Logger:    log.New(io.Discard, "", 0),
	})
	defer func() {
		cancel()
		<-done
	}()

	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()

	if err := bridgeWriteRequest(conn); err != nil {
		t.Fatalf("bridgeWriteRequest() error = %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	if _, err := ReadResponse(conn); err == nil {
		t.Fatal("ReadResponse() error = nil, want non-nil")
	}
}

func TestServerRefusesWrongToken(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	_, cancel, done := startServer(t, ServerConfig{
		Addr:      addr,
		Token:     testToken,
		Clipboard: stubClipboard{payload: []byte("png")},
		Logger:    log.New(io.Discard, "", 0),
	})
	defer func() {
		cancel()
		<-done
	}()

	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()

	var wrongToken [TokenSize]byte
	if err := WritePNGRequest(conn, wrongToken); err != nil {
		t.Fatalf("WritePNGRequest() error = %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	// server closes silently on wrong token - expect EOF or deadline exceeded
	if _, err := ReadResponse(conn); err == nil {
		t.Fatal("ReadResponse() error = nil, want EOF or error")
	}
}

func TestServerMonitorClosesServer(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	aliveChecks := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := NewServer(ServerConfig{
		Addr:            addr,
		Token:           testToken,
		Clipboard:       stubClipboard{payload: []byte("png")},
		MonitorPID:      123,
		MonitorInterval: 10 * time.Millisecond,
		Alive: func(int) bool {
			aliveChecks++
			return aliveChecks < 2
		},
		Logger: log.New(io.Discard, "", 0),
	})

	done := make(chan error, 1)
	go func() {
		done <- server.ListenAndServe(ctx)
	}()

	testutil.Eventually(t, time.Second, func() bool {
		return testutil.ReachableTCP(addr, 20*time.Millisecond)
	})

	testutil.Eventually(t, time.Second, func() bool {
		return !testutil.ReachableTCP(addr, 20*time.Millisecond)
	})

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("ListenAndServe() error = %v", err)
	}
}

func startServer(t *testing.T, config ServerConfig) (*Server, context.CancelFunc, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(config)
	done := make(chan error, 1)

	go func() {
		done <- server.ListenAndServe(ctx)
	}()

	testutil.Eventually(t, time.Second, func() bool {
		return testutil.ReachableTCP(config.Addr, 20*time.Millisecond)
	})

	return server, cancel, done
}

func requestPayload(t *testing.T, addr string) []byte {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()

	if err := bridgeWriteRequest(conn); err != nil {
		t.Fatalf("bridgeWriteRequest() error = %v", err)
	}

	payload, err := ReadResponse(conn)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}

	return payload
}

func bridgeWriteRequest(conn net.Conn) error {
	return WritePNGRequest(conn, testToken)
}

func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}
