package daemon

import (
	"net"
	"testing"
	"time"

	"github.com/KarpelesLab/bgrun/protocol"
)

func TestGetScreen(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sh", "-c", "echo 'Hello, World!'; echo 'Line 2'; sleep 10"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		UseVTY:     true,
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

	c, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	// Wait for output to be written
	time.Sleep(200 * time.Millisecond)

	// Send GetScreen request
	if writeErr := protocol.WriteMessage(c, protocol.MsgGetScreen, nil); writeErr != nil {
		t.Fatalf("Failed to send GetScreen: %v", writeErr)
	}

	// Read response
	msg, err := protocol.ReadMessage(c)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if msg.Type != protocol.MsgScreenResponse {
		t.Fatalf("Expected MsgScreenResponse, got 0x%02X", msg.Type)
	}

	screen, err := protocol.ParseScreenResponse(msg.Payload)
	if err != nil {
		t.Fatalf("Failed to parse screen response: %v", err)
	}

	// Verify screen dimensions
	if screen.Rows != 24 {
		t.Errorf("Expected 24 rows, got %d", screen.Rows)
	}
	if screen.Cols != 80 {
		t.Errorf("Expected 80 cols, got %d", screen.Cols)
	}

	// Verify we have lines
	if len(screen.Lines) != 24 {
		t.Errorf("Expected 24 lines, got %d", len(screen.Lines))
	}

	// Verify content contains our output
	found := false
	for _, line := range screen.Lines {
		if len(line) > 0 && (containsString(line, "Hello") || containsString(line, "World")) {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find 'Hello' or 'World' in screen output")
	}
}

func TestGetScreenWithoutVTY(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sleep", "10"},
		StdinMode:  StdinNull,
		StdoutMode: IOModeLog,
		StderrMode: IOModeLog,
		UseVTY:     false, // VTY disabled
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

	c, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	// Send GetScreen request (should fail)
	if writeErr := protocol.WriteMessage(c, protocol.MsgGetScreen, nil); writeErr != nil {
		t.Fatalf("Failed to send GetScreen: %v", writeErr)
	}

	// Read response - should be an error
	msg, err := protocol.ReadMessage(c)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if msg.Type != protocol.MsgError {
		t.Fatalf("Expected MsgError when VTY is disabled, got 0x%02X", msg.Type)
	}

	errorMsg := string(msg.Payload)
	if !containsString(errorMsg, "VTY") {
		t.Errorf("Expected error message to mention VTY, got: %s", errorMsg)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
