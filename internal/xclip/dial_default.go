//go:build !linux

package xclip

import (
	"net"
	"time"
)

func dialTCP(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}
