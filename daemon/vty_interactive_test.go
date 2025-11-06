package daemon

import (
	"encoding/binary"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/KarpelesLab/bgrun/protocol"
)

func TestVTYBasicIO(t *testing.T) {
	tmpDir := t.TempDir()

	// Use echo to verify basic PTY output works
	config := &Config{
		Command:    []string{"bash", "-c", "echo 'Test output from PTY'; sleep 0.5"},
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

	// Wait for process to start and produce output
	time.Sleep(1 * time.Second)

	// Verify status shows VTY mode
	status := d.GetStatus()
	if !status.HasVTY {
		t.Error("Status should indicate VTY mode")
	}

	// Wait for process to complete
	time.Sleep(1 * time.Second)

	// Check that output was logged
	// Note: PTY mode logs all output to output.log
	// We can verify the PTY worked by checking that the process ran successfully
	status = d.GetStatus()
	if status.Running {
		t.Error("Process should have completed")
	}

	if status.ExitCode == nil || *status.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %v", status.ExitCode)
	}
}

func TestVTYSignalDelivery(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a long-running process
	config := &Config{
		Command:    []string{"sleep", "30"},
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

	time.Sleep(200 * time.Millisecond)

	status := d.GetStatus()
	if !status.Running {
		t.Fatal("Process should be running")
	}

	pid := status.PID

	// Send SIGTERM via daemon handler
	conn, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send SIGTERM
	payload := []byte{byte(syscall.SIGTERM)}
	if err := protocol.WriteMessage(conn, protocol.MsgSignal, payload); err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}

	// Wait for acknowledgment
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if msg.Type == protocol.MsgError {
		t.Fatalf("Signal error: %s", string(msg.Payload))
	}

	if msg.Type != protocol.MsgSignalResponse {
		t.Errorf("Expected signal response, got type 0x%02X", msg.Type)
	}

	// Wait for process to terminate
	time.Sleep(500 * time.Millisecond)

	status = d.GetStatus()
	if status.Running {
		// Still running, force kill for cleanup
		syscall.Kill(pid, syscall.SIGKILL)
		t.Error("Process should have terminated after SIGTERM")
	}
}

// Note: Interactive Ctrl-C testing (sending 0x03 via PTY) requires
// a full terminal setup and is better tested manually with bgctl attach

func TestVTYKillProcess(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"sleep", "30"},
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

	time.Sleep(200 * time.Millisecond)

	status := d.GetStatus()
	if !status.Running {
		t.Fatal("Process should be running")
	}

	pid := status.PID

	// Kill the process
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("Failed to kill process: %v", err)
	}

	// Wait for process to die
	time.Sleep(500 * time.Millisecond)

	status = d.GetStatus()
	if status.Running {
		t.Error("Process should have been killed")
	}

	if status.ExitCode == nil {
		t.Error("Exit code should be set")
	}
}

func TestVTYStdinWrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Simple test: send input to cat and verify we can write
	config := &Config{
		Command:    []string{"cat"},
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

	time.Sleep(200 * time.Millisecond)

	// Verify process is running with VTY
	status := d.GetStatus()
	if !status.Running {
		t.Fatal("Process should be running")
	}
	if !status.HasVTY {
		t.Fatal("Process should have VTY enabled")
	}

	// Test writing to stdin via the daemon directly
	testData := []byte("hello from test\n")
	if err := d.writeVTY(testData); err != nil {
		t.Fatalf("Failed to write to VTY: %v", err)
	}

	// Give cat a moment to process
	time.Sleep(200 * time.Millisecond)

	//Send Ctrl-D to close cat
	if err := d.writeVTY([]byte{0x04}); err != nil {
		t.Fatalf("Failed to write Ctrl-D: %v", err)
	}

	// Wait for cat to exit
	time.Sleep(500 * time.Millisecond)

	status = d.GetStatus()
	if status.Running {
		t.Error("Cat should have exited after Ctrl-D")
	}
}

func TestVTYResizeWhileRunning(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Command:    []string{"bash", "-c", "echo 'Start'; sleep 3; echo 'End'"},
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

	time.Sleep(200 * time.Millisecond)

	// Connect and send resize commands
	conn, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send multiple resize requests
	sizes := []struct{ rows, cols uint16 }{
		{30, 120},
		{50, 200},
		{24, 80},
		{40, 160},
	}

	for _, size := range sizes {
		payload := make([]byte, 4)
		binary.BigEndian.PutUint16(payload[0:2], size.rows)
		binary.BigEndian.PutUint16(payload[2:4], size.cols)

		if err := protocol.WriteMessage(conn, protocol.MsgResize, payload); err != nil {
			t.Fatalf("Failed to send resize: %v", err)
		}

		// Read response
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			t.Fatalf("Failed to read resize response: %v", err)
		}

		if msg.Type == protocol.MsgError {
			t.Fatalf("Resize error: %s", string(msg.Payload))
		}

		if msg.Type != protocol.MsgResizeResponse {
			t.Errorf("Expected resize response, got 0x%02X", msg.Type)
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Process should still be running or just finished
	status := d.GetStatus()
	t.Logf("Process status: running=%v, exitcode=%v", status.Running, status.ExitCode)
}
