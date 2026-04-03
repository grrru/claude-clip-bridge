package xclip

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"claude-clip-bridge/internal/bridge"
)

type Finder interface {
	FindReachable() (string, bool, error)
}

type Config struct {
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	Debug          bool
	OverrideAddr   string
	Port           int
	TokenFile      string
	DialTimeout    time.Duration
	PassthroughBin string
	Passthrough    PassthroughFunc
	Finder         Finder
}

func Run(args []string, config Config) error {
	if config.Stdin == nil {
		config.Stdin = os.Stdin
	}
	if config.Stdout == nil {
		config.Stdout = os.Stdout
	}
	if config.Stderr == nil {
		config.Stderr = os.Stderr
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 100 * time.Millisecond
	}
	if config.Passthrough == nil {
		config.Passthrough = RunPassthrough
	}
	if config.PassthroughBin == "" {
		config.PassthroughBin = "/usr/bin/xclip"
	}
	if config.OverrideAddr == "" {
		config.OverrideAddr = os.Getenv("CC_BRIDGE_ADDR")
	}
	if config.TokenFile == "" {
		if home, err := os.UserHomeDir(); err == nil {
			config.TokenFile = filepath.Join(home, ".config", "claude-clip-bridge", "token")
		}
	}
	if config.Finder == nil {
		config.Finder = Discoverer{
			Override:    config.OverrideAddr,
			Port:        config.Port,
			DialTimeout: config.DialTimeout,
		}
	}

	match := ParseArgs(args)
	if !match.IsClipboardRead() {
		debugf(config, "passthrough: non-bridge request")
		return config.Passthrough(config.PassthroughBin, args, config.Stdin, config.Stdout, config.Stderr)
	}

	if match.IsTargetsProbe() {
		return handleTargets(args, config)
	}

	if !match.IsPNGRequest() {
		debugf(config, "passthrough: unsupported image target %q", match.target)
		return config.Passthrough(config.PassthroughBin, args, config.Stdin, config.Stdout, config.Stderr)
	}

	addr, ok, err := config.Finder.FindReachable()
	if err != nil {
		return err
	}
	if !ok {
		debugf(config, "passthrough: no reachable bridge")
		return config.Passthrough(config.PassthroughBin, args, config.Stdin, config.Stdout, config.Stderr)
	}

	token, err := bridge.ReadTokenFile(config.TokenFile)
	if err != nil {
		debugf(config, "passthrough: token unavailable: %v", err)
		return config.Passthrough(config.PassthroughBin, args, config.Stdin, config.Stdout, config.Stderr)
	}

	debugf(config, "bridge fetch via %s", addr)

	conn, err := dialTCP(addr, config.DialTimeout)
	if err != nil {
		debugf(config, "passthrough: dial failed for %s: %v", addr, err)
		return config.Passthrough(config.PassthroughBin, args, config.Stdin, config.Stdout, config.Stderr)
	}
	defer conn.Close()

	if err := bridge.WritePNGRequest(conn, token); err != nil {
		return fmt.Errorf("write bridge request: %w", err)
	}

	payload, err := bridge.ReadResponse(conn)
	if err != nil {
		return fmt.Errorf("read bridge response: %w", err)
	}

	if len(payload) == 0 {
		return nil
	}

	_, err = config.Stdout.Write(payload)
	return err
}

func handleTargets(args []string, config Config) error {
	_, ok, err := config.Finder.FindReachable()
	if err != nil {
		return err
	}
	if !ok {
		debugf(config, "passthrough: bridge unavailable for TARGETS probe")
		return config.Passthrough(config.PassthroughBin, args, config.Stdin, config.Stdout, config.Stderr)
	}

	debugf(config, "local TARGETS response: bridge reachable")
	_, err = io.WriteString(config.Stdout, "TARGETS\nimage/png\n")
	return err
}

func debugf(config Config, format string, args ...any) {
	if !config.Debug {
		return
	}

	fmt.Fprintf(config.Stderr, "xclip-shim: "+format+"\n", args...)
}
