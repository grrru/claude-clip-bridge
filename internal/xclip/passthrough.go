package xclip

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type ExitError struct {
	Code int
	Text string
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func (e *ExitError) Message() string {
	if e.Text == "" {
		return ""
	}

	message := e.Text
	if len(message) == 0 || message[len(message)-1] == '\n' {
		return message
	}

	return message + "\n"
}

type PassthroughFunc func(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) error

func RunPassthrough(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			message := fmt.Sprintf("passthrough xclip not found: %s", path)
			return &ExitError{Code: 1, Text: message, Err: fmt.Errorf("%s", message)}
		}
		return err
	}

	command := exec.Command(path, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode()}
		}
		return err
	}

	return nil
}
