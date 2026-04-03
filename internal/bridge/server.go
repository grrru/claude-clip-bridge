package bridge

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type ServerConfig struct {
	Addr            string
	Token           [TokenSize]byte
	Clipboard       ClipboardProvider
	MonitorPID      int
	MonitorInterval time.Duration
	Logger          *log.Logger
	Alive           AliveFunc
}

type Server struct {
	addr            string
	token           [TokenSize]byte
	clipboard       ClipboardProvider
	monitorPID      int
	monitorInterval time.Duration
	logger          *log.Logger
	alive           AliveFunc

	mu       sync.Mutex
	listener net.Listener
	closed   bool
}

func NewServer(config ServerConfig) *Server {
	monitorInterval := config.MonitorInterval
	if monitorInterval <= 0 {
		monitorInterval = 5 * time.Second
	}

	logger := config.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}

	return &Server{
		addr:            config.Addr,
		token:           config.Token,
		clipboard:       config.Clipboard,
		monitorPID:      config.MonitorPID,
		monitorInterval: monitorInterval,
		logger:          logger,
		alive:           config.Alive,
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.addr == "" {
		return errors.New("addr is required")
	}
	if s.clipboard == nil {
		return errors.New("clipboard provider is required")
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}

	s.mu.Lock()
	s.listener = listener
	s.closed = false
	s.mu.Unlock()

	defer s.Close()

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	go MonitorProcess(ctx, s.monitorPID, s.monitorInterval, s.alive, func() {
		_ = s.Close()
	})

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.isClosed() || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept connection: %w", err)
		}

		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	listener := s.listener
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.listener = nil
	s.mu.Unlock()

	if listener != nil {
		return listener.Close()
	}
	return nil
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	request, token, err := ReadRequest(conn)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return
		}
		s.logf("read request: %v", err)
		return
	}

	if subtle.ConstantTimeCompare(token[:], s.token[:]) != 1 {
		// silently close - wrong token
		return
	}

	if request != RequestPNG {
		s.logf("unsupported request: 0x%02x", request)
		return
	}

	payload, err := s.clipboard.PNG(ctx)
	switch {
	case err == nil:
		if writeErr := WriteResponse(conn, payload); writeErr != nil {
			s.logf("write response: %v", writeErr)
		}
	case errors.Is(err, ErrNoImage):
		if writeErr := WriteResponse(conn, nil); writeErr != nil {
			s.logf("write empty response: %v", writeErr)
		}
	default:
		s.logf("clipboard read failed: %v", err)
	}
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Server) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}
