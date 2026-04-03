package launcher

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProcess struct {
	killed bool
}

func (f *fakeProcess) Kill() error {
	f.killed = true
	return nil
}

func TestLaunchSuccess(t *testing.T) {
	t.Parallel()

	process := &fakeProcess{}
	ready := false

	err := Launch(context.Background(), Config{
		Hostname:     "example",
		Addr:         "127.0.0.1:19876",
		SSHPID:       10,
		PollInterval: 5 * time.Millisecond,
		Timeout:      100 * time.Millisecond,
		StartBridge: func(Config) (Process, error) {
			go func() {
				time.Sleep(15 * time.Millisecond)
				ready = true
			}()
			return process, nil
		},
		Probe: func(string, time.Duration) bool {
			return ready
		},
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if process.killed {
		t.Fatal("process killed unexpectedly")
	}
}

func TestLaunchTimeoutKillsProcess(t *testing.T) {
	t.Parallel()

	process := &fakeProcess{}

	err := Launch(context.Background(), Config{
		Hostname:     "example",
		Addr:         "127.0.0.1:19876",
		SSHPID:       10,
		PollInterval: 5 * time.Millisecond,
		Timeout:      25 * time.Millisecond,
		StartBridge: func(Config) (Process, error) {
			return process, nil
		},
		Probe: func(string, time.Duration) bool {
			return false
		},
	})
	if err == nil {
		t.Fatal("Launch() error = nil, want timeout")
	}
	if !process.killed {
		t.Fatal("expected process to be killed on timeout")
	}
}

func TestLaunchStartError(t *testing.T) {
	t.Parallel()

	startErr := errors.New("start failed")
	err := Launch(context.Background(), Config{
		Hostname: "example",
		Addr:     "127.0.0.1:19876",
		SSHPID:   10,
		StartBridge: func(Config) (Process, error) {
			return nil, startErr
		},
	})
	if !errors.Is(err, startErr) {
		t.Fatalf("Launch() error = %v, want %v", err, startErr)
	}
}
