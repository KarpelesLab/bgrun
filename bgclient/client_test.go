package bgclient

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/KarpelesLab/bgrun/daemon"
	"github.com/KarpelesLab/bgrun/protocol"
)

func setupDaemon(t *testing.T, config *daemon.Config) (*daemon.Daemon, string) {
	tmpDir := t.TempDir()

	if config.RuntimeDir == "" {
		config.RuntimeDir = tmpDir
	}

	d, err := daemon.New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "control.sock")

	// Wait for socket to be ready
	maxRetries := 50
	for i := 0; i < maxRetries; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Cleanup(func() {
		d.GetStatus() // ensure daemon is still alive before stopping
	})

	return d, socketPath
}

func TestConnect(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "5"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	// Test successful connection
	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Test connection to non-existent socket
	_, err = Connect("/nonexistent/socket.sock")
	if err == nil {
		t.Fatal("Expected error connecting to non-existent socket")
	}
}

func TestClose(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "5"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Close should succeed
	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should fail gracefully
	err = c.Close()
	if err == nil {
		t.Log("Double close returned nil (acceptable)")
	}
}

func TestGetStatus(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "10"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Get status of running process
	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.PID <= 0 {
		t.Errorf("Invalid PID: %d", status.PID)
	}

	if !status.Running {
		t.Errorf("Process should be running")
	}

	if len(status.Command) == 0 {
		t.Errorf("Command should not be empty")
	}

	if status.Command[0] != "sleep" {
		t.Errorf("Expected command 'sleep', got %s", status.Command[0])
	}
}

func TestGetStatusAfterExit(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "sleep 0.2; exit 42"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	d, socketPath := setupDaemon(t, config)

	// Connect while process is still running
	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Verify process is running
	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.Running {
		t.Fatal("Process should be running")
	}

	// Wait for process to exit
	<-d.Done()
	time.Sleep(100 * time.Millisecond)

	// Get status after exit (connection still open, just process exited)
	status, err = c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Running {
		t.Errorf("Process should not be running")
	}

	if status.ExitCode == nil {
		t.Errorf("ExitCode should not be nil")
	} else if *status.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", *status.ExitCode)
	}
}

func TestWriteStdin(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"cat"},
		StdinMode:  daemon.StdinStream,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Write some data
	testData := []byte("hello world\n")
	if err := c.WriteStdin(testData); err != nil {
		t.Fatalf("WriteStdin failed: %v", err)
	}

	// Write should succeed
	t.Log("WriteStdin succeeded")
}

func TestCloseStdin(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"cat"},
		StdinMode:  daemon.StdinStream,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Close stdin
	if err := c.CloseStdin(); err != nil {
		t.Fatalf("CloseStdin failed: %v", err)
	}

	// Wait for cat to exit
	time.Sleep(500 * time.Millisecond)

	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Running {
		t.Errorf("Process should have exited after stdin closed")
	}
}

func TestSendSignal(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "60"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Send SIGTERM
	if err := c.SendSignal(syscall.SIGTERM); err != nil {
		t.Fatalf("SendSignal failed: %v", err)
	}

	// Wait for process to exit
	time.Sleep(500 * time.Millisecond)

	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Running {
		t.Errorf("Process should have exited after SIGTERM")
	}
}

func TestResize(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "10"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		UseVTY:     true,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Test resize
	if err := c.Resize(40, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}

	// Resize again
	if err := c.Resize(24, 80); err != nil {
		t.Fatalf("Second resize failed: %v", err)
	}

	t.Log("Resize operations succeeded")
}

func TestResizeWithoutVTY(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "10"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Resize without VTY should fail
	err = c.Resize(40, 120)
	if err == nil {
		t.Fatal("Expected error when resizing without VTY")
	}
}

func TestWaitForExit(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "1"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Wait for process to exit (should complete)
	status, err := c.Wait(5, protocol.WaitTypeExit)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if status != protocol.WaitStatusCompleted {
		t.Errorf("Expected WaitStatusCompleted, got %d", status)
	}
}

func TestWaitTimeout(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "10"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Wait with short timeout (should timeout)
	status, err := c.Wait(1, protocol.WaitTypeExit)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if status != protocol.WaitStatusTimeout {
		t.Errorf("Expected WaitStatusTimeout, got %d", status)
	}
}

func TestWaitForeground(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"bash"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		UseVTY:     true,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Wait for foreground (should complete quickly as bash is idle)
	status, err := c.Wait(5, protocol.WaitTypeForeground)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if status != protocol.WaitStatusCompleted {
		t.Errorf("Expected WaitStatusCompleted, got %d", status)
	}
}

func TestAttachDetach(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "echo hello; sleep 1; echo world"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Attach to stdout
	if err := c.Attach(protocol.StreamStdout); err != nil {
		t.Fatalf("Attach failed: %v", err)
	}

	// Detach
	if err := c.Detach(); err != nil {
		t.Fatalf("Detach failed: %v", err)
	}

	t.Log("Attach/Detach succeeded")
}

func TestShutdown(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sleep", "60"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Send shutdown - should succeed
	// Note: The shutdown handler calls os.Exit() which would terminate
	// the test process, so we just verify the message is sent successfully
	if err := c.Shutdown(); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Shutdown message was sent successfully
	t.Log("Shutdown message sent successfully")

	c.Close()
}

func TestReadMessages(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "echo line1; echo line2; echo line3"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Attach to output
	if err := c.Attach(protocol.StreamBoth); err != nil {
		t.Fatalf("Attach failed: %v", err)
	}

	var output bytes.Buffer
	var exitCode int
	exitReceived := false

	err = c.ReadMessages(
		func(stream byte, data []byte) error {
			output.Write(data)
			return nil
		},
		func(code int) {
			exitCode = code
			exitReceived = true
		},
	)

	if err != nil {
		t.Fatalf("ReadMessages failed: %v", err)
	}

	if !exitReceived {
		t.Fatal("Exit handler was not called")
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	outputStr := output.String()
	if len(outputStr) == 0 {
		t.Error("Expected some output")
	}

	t.Logf("Received output: %q", outputStr)
}

func TestReadMessagesWithError(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "echo test; sleep 0.1"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Attach to output
	if err := c.Attach(protocol.StreamBoth); err != nil {
		t.Fatalf("Attach failed: %v", err)
	}

	// Output handler that returns an error
	expectedErr := fmt.Errorf("test error")
	err = c.ReadMessages(
		func(stream byte, data []byte) error {
			return expectedErr
		},
		nil,
	)

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestReadMessagesEOF(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"true"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Don't attach, just try to read messages
	// Process will exit quickly and socket will close
	time.Sleep(200 * time.Millisecond)

	// Detach to trigger a clean close
	c.Detach()

	err = c.ReadMessages(nil, nil)
	if err != nil {
		// EOF is acceptable here
		t.Logf("ReadMessages returned: %v", err)
	}
}
