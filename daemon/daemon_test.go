package daemon

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDaemonBasic(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"echo", "hello world"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Wait a bit for the process to start
	time.Sleep(100 * time.Millisecond)

	status := d.GetStatus()
	if status.PID == 0 {
		t.Error("PID should not be 0")
	}

	if len(status.Command) != 2 {
		t.Errorf("Expected 2 command args, got %d", len(status.Command))
	}

	// Wait for process to complete
	time.Sleep(500 * time.Millisecond)

	status = d.GetStatus()
	if status.Running {
		t.Error("Process should have stopped")
	}

	if status.ExitCode == nil {
		t.Error("Exit code should be set")
	} else if *status.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", *status.ExitCode)
	}
}

func TestDaemonLongRunning(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sleep", "2"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Process should be running
	time.Sleep(100 * time.Millisecond)
	status := d.GetStatus()
	if !status.Running {
		t.Error("Process should be running")
	}

	// Wait for completion
	d.Wait()

	status = d.GetStatus()
	if status.Running {
		t.Error("Process should have completed")
	}
}

func TestDaemonSignal(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sleep", "30"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Wait for process to start
	time.Sleep(100 * time.Millisecond)

	status := d.GetStatus()
	if !status.Running {
		t.Fatal("Process should be running")
	}

	// Send SIGTERM
	if err := syscall.Kill(status.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}

	// Wait for process to exit
	time.Sleep(500 * time.Millisecond)

	status = d.GetStatus()
	if status.Running {
		t.Error("Process should have exited")
	}
}

func TestRuntimeDir(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"echo", "test"},
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if d.RuntimeDir() != tmpDir {
		t.Errorf("Expected runtime dir %s, got %s", tmpDir, d.RuntimeDir())
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Check that socket was created
	if _, err := os.Stat(d.SocketPath()); os.IsNotExist(err) {
		t.Error("Socket file should exist")
	}

	expectedSocket := filepath.Join(tmpDir, "control.sock")
	if d.SocketPath() != expectedSocket {
		t.Errorf("Expected socket path %s, got %s", expectedSocket, d.SocketPath())
	}
}

func TestOutputLogging(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sh", "-c", "echo stdout; echo stderr >&2"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Wait for process to complete and output to be written
	time.Sleep(1 * time.Second)

	// Check that log file exists and has content
	logPath := filepath.Join(tmpDir, "output.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Error("Log file should not be empty")
	}

	// Should contain both stdout and stderr
	if !contains(contentStr, "stdout") {
		t.Error("Log should contain stdout output")
	}
	if !contains(contentStr, "stderr") {
		t.Error("Log should contain stderr output")
	}
}

func TestStdinStream(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"cat"},
		StdinMode:  StdinStream,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Wait for process to start
	time.Sleep(100 * time.Millisecond)

	// Write to stdin
	testData := []byte("hello from stdin\n")
	if stdinErr := d.handleStdin(testData); stdinErr != nil {
		t.Fatalf("Failed to write stdin: %v", stdinErr)
	}

	// Close stdin to let cat exit
	if d.stdinPipe != nil {
		d.stdinPipe.Close()
	}

	// Wait for process to complete
	time.Sleep(500 * time.Millisecond)

	// Check log file
	logPath := filepath.Join(tmpDir, "output.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !contains(string(content), "hello from stdin") {
		t.Error("Log should contain stdin data")
	}
}

func TestGetStatusResponse(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sleep", "1"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		RuntimeDir: tmpDir,
		UseVTY:     true,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer d.stop()

	// Wait for process to start
	time.Sleep(100 * time.Millisecond)

	status := d.GetStatus()

	// Verify status fields
	if status.PID <= 0 {
		t.Error("PID should be positive")
	}

	if !status.Running {
		t.Error("Process should be running")
	}

	if status.ExitCode != nil {
		t.Error("ExitCode should be nil while running")
	}

	if status.StartedAt == "" {
		t.Error("StartedAt should be set")
	}

	if status.EndedAt != nil {
		t.Error("EndedAt should be nil while running")
	}

	if len(status.Command) != 2 {
		t.Errorf("Expected 2 command parts, got %d", len(status.Command))
	}

	if !status.HasVTY {
		t.Error("HasVTY should be true")
	}

	// Wait for process to complete
	time.Sleep(1500 * time.Millisecond)

	status = d.GetStatus()
	if status.Running {
		t.Error("Process should not be running")
	}

	if status.ExitCode == nil {
		t.Error("ExitCode should be set")
	}

	if status.EndedAt == nil {
		t.Error("EndedAt should be set")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
