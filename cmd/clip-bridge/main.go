package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"claude-clip-bridge/internal/bridge"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "clip-bridge: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 4 {
		return errors.New("usage: clip-bridge <ssh_pid> <addr> <token_file>")
	}

	sshPID, err := strconv.Atoi(os.Args[1])
	if err != nil || sshPID <= 0 {
		return fmt.Errorf("invalid ssh pid %q", os.Args[1])
	}

	addr := os.Args[2]
	tokenFile := os.Args[3]

	token, err := bridge.ReadTokenFile(tokenFile)
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := bridge.NewServer(bridge.ServerConfig{
		Addr:       addr,
		Token:      token,
		MonitorPID: sshPID,
		Clipboard:  bridge.NewPNGPasteClipboard("pngpaste", bridge.ExecRunner{}),
		Logger:     log.New(os.Stderr, "clip-bridge: ", log.LstdFlags),
	})

	return server.ListenAndServe(ctx)
}
