package terminal

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestIsTerminal(t *testing.T) {
	// Create a pipe which is not a terminal
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// Pipe should not be a terminal
	if IsTerminal(int(r.Fd())) {
		t.Error("pipe read end should not be a terminal")
	}

	if IsTerminal(int(w.Fd())) {
		t.Error("pipe write end should not be a terminal")
	}

	// Test with stdin/stdout/stderr (may or may not be terminals depending on environment)
	// We just verify the function doesn't crash
	IsTerminal(int(os.Stdin.Fd()))
	IsTerminal(int(os.Stdout.Fd()))
	IsTerminal(int(os.Stderr.Fd()))
}

func TestMakeRawNonTerminal(t *testing.T) {
	// Create a pipe which is not a terminal
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// MakeRaw should fail on a non-terminal
	_, err = MakeRaw(int(r.Fd()))
	if err == nil {
		t.Error("MakeRaw should fail on a non-terminal")
	}
}

func TestGetSizeNonTerminal(t *testing.T) {
	// Create a pipe which is not a terminal
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// GetSize should fail on a non-terminal
	_, _, err = GetSize(int(r.Fd()))
	if err == nil {
		t.Error("GetSize should fail on a non-terminal")
	}
}

func TestStateRestore(t *testing.T) {
	// Test that Restore on a nil state is safe
	s := &State{
		fd:       0,
		oldState: nil,
	}

	if err := s.Restore(); err != nil {
		t.Errorf("Restore with nil oldState should not error: %v", err)
	}
}

func TestWatchResize(t *testing.T) {
	// Test that WatchResize creates a channel and can receive signals
	sigCh := WatchResize()
	if sigCh == nil {
		t.Fatal("WatchResize should return a non-nil channel")
	}

	// Send a SIGWINCH signal to ourselves
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("failed to send SIGWINCH: %v", err)
	}

	// Wait for the signal with a timeout
	select {
	case sig := <-sigCh:
		if sig != syscall.SIGWINCH {
			t.Errorf("expected SIGWINCH, got %v", sig)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for SIGWINCH")
	}

	// Stop watching
	StopWatchingResize(sigCh)

	// Verify the channel is closed
	select {
	case _, ok := <-sigCh:
		if ok {
			t.Error("channel should be closed after StopWatchingResize")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel should be closed immediately")
	}
}

func TestMultipleWatchResize(t *testing.T) {
	// Test that multiple watchers can be created and stopped independently
	sigCh1 := WatchResize()
	sigCh2 := WatchResize()

	if sigCh1 == nil || sigCh2 == nil {
		t.Fatal("WatchResize should return non-nil channels")
	}

	// Stop the first one
	StopWatchingResize(sigCh1)

	// Second one should still work
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("failed to send SIGWINCH: %v", err)
	}

	select {
	case sig := <-sigCh2:
		if sig != syscall.SIGWINCH {
			t.Errorf("expected SIGWINCH on second channel, got %v", sig)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for SIGWINCH on second channel")
	}

	// Clean up
	StopWatchingResize(sigCh2)
}
