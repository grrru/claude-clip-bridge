package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"claude-clip-bridge/internal/launcher"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "clip-bridge-start: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// SSH LocalCommand에서 호출: clip-bridge-start %h $PPID
	// $PPID는 shell의 부모(= SSH 클라이언트 프로세스) PID
	if len(os.Args) != 3 {
		return errors.New("usage: clip-bridge-start <hostname> <ssh_pid>")
	}

	hostname := os.Args[1]

	sshPID, err := strconv.Atoi(os.Args[2])
	if err != nil || sshPID <= 0 {
		return fmt.Errorf("invalid ssh_pid %q", os.Args[2])
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return launcher.Launch(ctx, launcher.Config{
		Hostname: hostname,
		SSHPID:   sshPID,
	})
}
