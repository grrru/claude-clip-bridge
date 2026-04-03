package xclip

import (
	"net"
	"testing"
	"time"
)

func TestDiscovererOverrideReachable(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer l.Close()

	discoverer := Discoverer{
		Override:    l.Addr().String(),
		DialTimeout: 20 * time.Millisecond,
	}

	got, ok, err := discoverer.FindReachable()
	if err != nil {
		t.Fatalf("FindReachable() error = %v", err)
	}
	if !ok {
		t.Fatal("FindReachable() ok = false, want true")
	}
	if got != l.Addr().String() {
		t.Fatalf("FindReachable() = %q, want %q", got, l.Addr().String())
	}
}

func TestDiscovererOverrideUnreachable(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := l.Addr().String()
	l.Close()

	discoverer := Discoverer{
		Override:    addr,
		DialTimeout: 20 * time.Millisecond,
	}

	_, ok, err := discoverer.FindReachable()
	if err != nil {
		t.Fatalf("FindReachable() error = %v", err)
	}
	if ok {
		t.Fatal("FindReachable() ok = true, want false")
	}
}

func TestDiscovererDefaultPort(t *testing.T) {
	t.Parallel()

	d := Discoverer{DialTimeout: 20 * time.Millisecond}
	got, _, err := d.FindReachable()
	if err != nil {
		t.Fatalf("FindReachable() error = %v", err)
	}

	want := "127.0.0.1:19876"
	if got != want {
		t.Fatalf("FindReachable() addr = %q, want %q", got, want)
	}
}

func TestDiscovererCustomPort(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port

	d := Discoverer{
		Port:        port,
		DialTimeout: 20 * time.Millisecond,
	}

	_, ok, err := d.FindReachable()
	if err != nil {
		t.Fatalf("FindReachable() error = %v", err)
	}
	if !ok {
		t.Fatal("FindReachable() ok = false, want true")
	}
}
