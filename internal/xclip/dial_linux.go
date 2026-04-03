//go:build linux

package xclip

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// dialTCP connects to a TCP address. On Linux, some server environments return
// EINVAL when Go's net package tries to set TCP_NODELAY after connecting to an
// SSH-forwarded port. In that case we fall back to a raw syscall dialer which
// skips the TCP_NODELAY setsockopt.
func dialTCP(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err == nil {
		return conn, nil
	}
	if !strings.Contains(err.Error(), "TCP_NODELAY") {
		return nil, err
	}
	return dialTCPRaw(addr, timeout)
}

// dialTCPRaw creates a TCP connection via raw syscalls, bypassing Go's automatic
// TCP_NODELAY setsockopt which can fail with EINVAL on some Linux systems.
func dialTCPRaw(addr string, timeout time.Duration) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse addr %s: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %s", portStr)
	}

	ip := net.ParseIP(host).To4()
	if ip == nil {
		return nil, fmt.Errorf("IPv4 required for raw dial: %s", host)
	}

	fd, err := syscall.Socket(
		syscall.AF_INET,
		syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC,
		syscall.IPPROTO_TCP,
	)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}

	if timeout > 0 {
		tv := syscall.NsecToTimeval(timeout.Nanoseconds())
		_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
	}

	sa := &syscall.SockaddrInet4{Port: port}
	copy(sa.Addr[:], ip)

	if err := syscall.Connect(fd, sa); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}

	// net.FileConn dups the fd internally; close the original after.
	// net.FileConn calls newTCPConn which attempts setNoDelay but does not
	// propagate that error, so EINVAL is silently ignored.
	file := os.NewFile(uintptr(fd), addr)
	conn, err := net.FileConn(file)
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("wrap conn: %w", err)
	}
	return conn, nil
}
