package terminal

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// State holds terminal state for restoration
type State struct {
	fd       int
	oldState *term.State
}

// IsTerminal returns true if the given file descriptor is a terminal
func IsTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

// MakeRaw puts the terminal into raw mode
func MakeRaw(fd int) (*State, error) {
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to make terminal raw: %w", err)
	}

	return &State{
		fd:       fd,
		oldState: oldState,
	}, nil
}

// Restore restores the terminal to its previous state
func (s *State) Restore() error {
	if s.oldState == nil {
		return nil
	}

	if err := term.Restore(s.fd, s.oldState); err != nil {
		return fmt.Errorf("failed to restore terminal: %w", err)
	}

	return nil
}

// GetSize returns the current terminal size
func GetSize(fd int) (rows, cols int, err error) {
	width, height, err := term.GetSize(fd)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get terminal size: %w", err)
	}
	return height, width, nil
}

// WatchResize watches for terminal resize signals
func WatchResize() chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	return sigCh
}

// StopWatchingResize stops watching for resize signals
func StopWatchingResize(sigCh chan os.Signal) {
	signal.Stop(sigCh)
	close(sigCh)
}
