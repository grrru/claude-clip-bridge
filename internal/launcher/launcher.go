package launcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"claude-clip-bridge/internal/bridge"
)

const DefaultPort = 19876

type Process interface {
	Kill() error
}

type StartBridgeFunc func(Config) (Process, error)
type ProbeFunc func(addr string, timeout time.Duration) bool

type Config struct {
	Hostname       string
	Addr           string // e.g. "127.0.0.1:19876"
	TokenFile      string
	BridgePath     string
	LogPath        string
	SSHPID         int
	PollInterval   time.Duration
	Timeout        time.Duration
	ConnectTimeout time.Duration
	StartBridge    StartBridgeFunc
	Probe          ProbeFunc
}

func Launch(ctx context.Context, config Config) error {
	if config.Hostname == "" {
		return errors.New("hostname is required")
	}
	if config.SSHPID <= 0 {
		return errors.New("ssh pid must be greater than zero")
	}

	if config.Addr == "" {
		port := DefaultPort
		if p := os.Getenv("CC_BRIDGE_PORT"); p != "" {
			if n, err := strconv.Atoi(p); err == nil && n > 0 {
				port = n
			}
		}
		config.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	}
	if config.TokenFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		config.TokenFile = filepath.Join(home, ".config", "claude-clip-bridge", "token")
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 100 * time.Millisecond
	}
	if config.Timeout <= 0 {
		config.Timeout = 2 * time.Second
	}
	if config.ConnectTimeout <= 0 {
		config.ConnectTimeout = 100 * time.Millisecond
	}
	if config.Probe == nil {
		config.Probe = ProbeTCP
	}
	if config.StartBridge == nil {
		config.StartBridge = startBridgeProcess
	}

	process, err := config.StartBridge(config)
	if err != nil {
		return err
	}

	timer := time.NewTimer(config.Timeout)
	defer timer.Stop()

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	for {
		if config.Probe(config.Addr, config.ConnectTimeout) {
			return nil
		}

		select {
		case <-ctx.Done():
			_ = process.Kill()
			return ctx.Err()
		case <-timer.C:
			_ = process.Kill()
			return fmt.Errorf("timed out waiting for bridge: %s", config.Addr)
		case <-ticker.C:
		}
	}
}

func ProbeTCP(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func DefaultLogPath(hostname string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%s.log", bridge.SanitizeSocketComponent(hostname))
	return filepath.Join(home, "Library", "Logs", "claude-clip-bridge", filename), nil
}

func ResolveBridgePath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	if envPath := os.Getenv("CLIP_BRIDGE_BIN"); envPath != "" {
		return envPath, nil
	}

	if executable, err := os.Executable(); err == nil {
		siblingPath := filepath.Join(filepath.Dir(executable), "clip-bridge")
		if info, statErr := os.Stat(siblingPath); statErr == nil && !info.IsDir() {
			return siblingPath, nil
		}
	}

	return exec.LookPath("clip-bridge")
}

type execProcess struct {
	process *os.Process
}

func (p execProcess) Kill() error {
	if p.process == nil {
		return nil
	}
	return p.process.Kill()
}

func startBridgeProcess(config Config) (Process, error) {
	bridgePath, err := ResolveBridgePath(config.BridgePath)
	if err != nil {
		return nil, fmt.Errorf("resolve clip-bridge binary: %w", err)
	}

	logPath := config.LogPath
	if logPath == "" {
		logPath, err = DefaultLogPath(config.Hostname)
		if err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	command := exec.Command(bridgePath,
		strconv.Itoa(config.SSHPID),
		config.Addr,
		config.TokenFile,
	)
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start clip-bridge: %w", err)
	}

	return execProcess{process: command.Process}, nil
}
