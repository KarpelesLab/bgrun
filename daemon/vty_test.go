package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVTYMode(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"bash", "-c", "echo 'Hello from VTY'; sleep 1; echo 'Goodbye'"},
		UseVTY:     true,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.stop()

	// Wait for process to start
	time.Sleep(100 * time.Millisecond)

	status := d.GetStatus()
	if !status.HasVTY {
		t.Error("Status should indicate VTY mode")
	}

	if !status.Running {
		t.Error("Process should be running")
	}

	// Wait for process to complete
	time.Sleep(2 * time.Second)

	status = d.GetStatus()
	if status.Running {
		t.Error("Process should have completed")
	}

	if status.ExitCode == nil || *status.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %v", status.ExitCode)
	}

	// Check that output was logged
	logPath := filepath.Join(tmpDir, "output.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Error("Log file should not be empty")
	}

	if !contains(contentStr, "Hello from VTY") {
		t.Error("Log should contain VTY output")
	}
}

func TestVTYResize(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sleep", "5"},
		UseVTY:     true,
		RuntimeDir: tmpDir,
	}

	d, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer d.stop()

	// Wait for process to start
	time.Sleep(100 * time.Millisecond)

	// Test resize
	if err := d.resizeVTY(40, 100); err != nil {
		t.Errorf("Failed to resize VTY: %v", err)
	}

	// Another resize
	if err := d.resizeVTY(24, 80); err != nil {
		t.Errorf("Failed to resize VTY: %v", err)
	}
}
