package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"claude-clip-bridge/internal/xclip"
)

func main() {
	if err := run(); err != nil {
		var exitErr *xclip.ExitError
		if errors.As(err, &exitErr) {
			if message := exitErr.Message(); message != "" {
				fmt.Fprint(os.Stderr, message)
			}
			os.Exit(exitErr.Code)
		}

		fmt.Fprintf(os.Stderr, "xclip: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := xclip.DefaultPort
	if p := os.Getenv("CC_BRIDGE_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
	}

	return xclip.Run(os.Args[1:], xclip.Config{
		Stdin:          os.Stdin,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		Debug:          os.Getenv("CC_CLIP_DEBUG") == "1",
		PassthroughBin: "/usr/bin/xclip",
		Port:           port,
	})
}
