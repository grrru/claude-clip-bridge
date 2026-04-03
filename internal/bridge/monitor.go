package bridge

import (
	"context"
	"errors"
	"syscall"
	"time"
)

type AliveFunc func(pid int) bool

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func MonitorProcess(ctx context.Context, pid int, interval time.Duration, alive AliveFunc, onExit func()) {
	if pid <= 0 || onExit == nil {
		return
	}

	if interval <= 0 {
		interval = 5 * time.Second
	}
	if alive == nil {
		alive = ProcessAlive
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if alive(pid) {
				continue
			}

			onExit()
			return
		}
	}
}
