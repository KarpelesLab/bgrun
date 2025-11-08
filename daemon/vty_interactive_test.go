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

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
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

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
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
	if writeErr := protocol.WriteMessage(conn, protocol.MsgSignal, payload); writeErr != nil {
		t.Fatalf("Failed to send signal: %v", writeErr)
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
// a full terminal setup and is better tested manually with bgrun -ctl attach

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

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
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

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
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

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
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

func TestWaitForExit(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a process that will run for 2 seconds
	config := &Config{
		Command:    []string{"sleep", "2"},
		UseVTY:     false, // Test works for both VTY and non-VTY
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

	time.Sleep(200 * time.Millisecond)

	// Test 1: Wait with sufficient timeout (should complete)
	conn, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for exit with 5 second timeout (process will exit in ~2 seconds)
	payload := make([]byte, 5)
	binary.BigEndian.PutUint32(payload[0:4], 5) // 5 second timeout
	payload[4] = protocol.WaitTypeExit

	if writeErr := protocol.WriteMessage(conn, protocol.MsgWait, payload); writeErr != nil {
		t.Fatalf("Failed to send wait: %v", writeErr)
	}

	// Read response (may receive MsgProcessExit first, ignore it and wait for MsgWaitResponse)
	var status byte
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			t.Fatalf("Failed to read wait response: %v", err)
		}

		if msg.Type == protocol.MsgError {
			t.Fatalf("Wait error: %s", string(msg.Payload))
		}

		if msg.Type == protocol.MsgProcessExit {
			// Ignore process exit message and continue reading
			continue
		}

		if msg.Type == protocol.MsgWaitResponse {
			status, err = protocol.ParseWaitResponse(msg.Payload)
			if err != nil {
				t.Fatalf("Failed to parse wait response: %v", err)
			}
			break
		}

		t.Fatalf("Unexpected message type: 0x%02X", msg.Type)
	}

	if status != protocol.WaitStatusCompleted {
		t.Errorf("Expected WaitStatusCompleted, got %d", status)
	}

	t.Log("Wait for exit completed successfully")
}

func TestWaitForExitTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a process that will run for 10 seconds
	config := &Config{
		Command:    []string{"sleep", "10"},
		UseVTY:     false,
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

	time.Sleep(200 * time.Millisecond)

	conn, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for exit with 1 second timeout (process will still be running)
	payload := make([]byte, 5)
	binary.BigEndian.PutUint32(payload[0:4], 1) // 1 second timeout
	payload[4] = protocol.WaitTypeExit

	if writeErr := protocol.WriteMessage(conn, protocol.MsgWait, payload); writeErr != nil {
		t.Fatalf("Failed to send wait: %v", writeErr)
	}

	// Read response
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read wait response: %v", err)
	}

	if msg.Type != protocol.MsgWaitResponse {
		t.Fatalf("Expected wait response, got 0x%02X", msg.Type)
	}

	status, err := protocol.ParseWaitResponse(msg.Payload)
	if err != nil {
		t.Fatalf("Failed to parse wait response: %v", err)
	}

	if status != protocol.WaitStatusTimeout {
		t.Errorf("Expected WaitStatusTimeout, got %d", status)
	}

	t.Log("Wait timeout test passed")
}

func TestWaitForForeground(t *testing.T) {
	tmpDir := t.TempDir()

	// Start bash in VTY mode
	config := &Config{
		Command:    []string{"bash"},
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

	time.Sleep(200 * time.Millisecond)

	conn, err := net.Dial("unix", d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Start a background sleep command in bash
	// This will put sleep in the foreground temporarily
	cmd := "sleep 2 &\n"
	if writeErr := protocol.WriteMessage(conn, protocol.MsgStdin, []byte(cmd)); writeErr != nil {
		t.Fatalf("Failed to write stdin: %v", writeErr)
	}

	// Give bash time to start the background process
	time.Sleep(300 * time.Millisecond)

	// At this point, bash should have regained foreground control
	// (since we used & to background the sleep command)

	// Wait for foreground control (bash should already have it)
	payload := make([]byte, 5)
	binary.BigEndian.PutUint32(payload[0:4], 5) // 5 second timeout
	payload[4] = protocol.WaitTypeForeground

	if writeErr := protocol.WriteMessage(conn, protocol.MsgWait, payload); writeErr != nil {
		t.Fatalf("Failed to send wait: %v", writeErr)
	}

	// Read response
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read wait response: %v", err)
	}

	if msg.Type != protocol.MsgWaitResponse {
		t.Fatalf("Expected wait response, got 0x%02X", msg.Type)
	}

	status, err := protocol.ParseWaitResponse(msg.Payload)
	if err != nil {
		t.Fatalf("Failed to parse wait response: %v", err)
	}

	if status != protocol.WaitStatusCompleted {
		t.Errorf("Expected WaitStatusCompleted, got %d", status)
	}

	t.Log("Wait for foreground completed successfully")

	// Clean up: send Ctrl-D (EOF) to bash
	if err := protocol.WriteMessage(conn, protocol.MsgStdin, []byte{0x04}); err != nil {
		t.Logf("Failed to write EOF: %v", err)
	}

	// Give bash a moment to exit gracefully
	time.Sleep(200 * time.Millisecond)
}
