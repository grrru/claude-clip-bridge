package xclip

import (
	"fmt"
	"time"
)

const DefaultPort = 19876

type Discoverer struct {
	Override    string // override address, e.g. "127.0.0.1:19876"
	Port        int
	DialTimeout time.Duration
}

func (d Discoverer) FindReachable() (string, bool, error) {
	timeout := d.DialTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	if d.Override != "" {
		return d.Override, canReachTCP(d.Override, timeout), nil
	}

	port := d.Port
	if port <= 0 {
		port = DefaultPort
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return addr, canReachTCP(addr, timeout), nil
}

func canReachTCP(addr string, timeout time.Duration) bool {
	conn, err := dialTCP(addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
