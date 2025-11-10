package main

import (
	"sync"
	"testing"
	"time"

	"github.com/KarpelesLab/bgrun/bgclient"
	"github.com/KarpelesLab/bgrun/daemon"
	"github.com/KarpelesLab/bgrun/protocol"
)

func TestClientServerIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Start daemon
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "echo hello; sleep 1; echo world"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := daemon.New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}

	// Connect client
	c, err := bgclient.Connect(d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer c.Close()

	// Get status
	status, err := c.GetStatus()
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if !status.Running {
		t.Error("Process should be running")
	}

	if status.PID <= 0 {
		t.Error("PID should be positive")
	}

	// Wait for process to complete
	d.Wait()

	// Get status again
	status, err = c.GetStatus()
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if status.Running {
		t.Error("Process should have completed")
	}

	if status.ExitCode == nil || *status.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %v", status.ExitCode)
	}
}

func TestClientAttachOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Start daemon that produces output - add initial delay to allow attach before output
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "sleep 0.2; for i in 1 2 3; do echo line $i; sleep 0.1; done"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := daemon.New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	// Wait for socket
	time.Sleep(100 * time.Millisecond)

	// Connect client
	c, err := bgclient.Connect(d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer c.Close()

	// Attach to output
	if err := c.Attach(protocol.StreamBoth); err != nil {
		t.Fatalf("Failed to attach: %v", err)
	}

	// Collect output
	var outputMu sync.Mutex
	var outputs []string
	exitCh := make(chan int, 1)

	go c.ReadMessages(
		func(stream byte, data []byte) error {
			outputMu.Lock()
			outputs = append(outputs, string(data))
			outputMu.Unlock()
			return nil
		},
		func(exitCode int) {
			exitCh <- exitCode
		},
	)

	// Wait for process to exit
	select {
	case code := <-exitCh:
		if code != 0 {
			t.Errorf("Expected exit code 0, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for process to exit")
	}

	// Check that we received output
	outputMu.Lock()
	defer outputMu.Unlock()

	if len(outputs) == 0 {
		t.Error("Should have received output")
	}

	// Combine all output
	combined := ""
	for _, out := range outputs {
		combined += out
	}

	// Should contain our output lines
	if !contains(combined, "line 1") || !contains(combined, "line 2") || !contains(combined, "line 3") {
		t.Errorf("Output should contain all lines, got: %s", combined)
	}
}

func TestClientStdinWrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Start daemon with cat (echoes stdin to stdout)
	config := &daemon.Config{
		Command:    []string{"cat"},
		StdinMode:  daemon.StdinStream,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := daemon.New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	// Wait for socket
	time.Sleep(100 * time.Millisecond)

	// Connect client
	c, err := bgclient.Connect(d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer c.Close()

	// Attach to output
	if err := c.Attach(protocol.StreamStdout); err != nil {
		t.Fatalf("Failed to attach: %v", err)
	}

	// Collect output
	var outputMu sync.Mutex
	var receivedData []byte
	exitCh := make(chan int, 1)

	go c.ReadMessages(
		func(stream byte, data []byte) error {
			outputMu.Lock()
			receivedData = append(receivedData, data...)
			outputMu.Unlock()
			return nil
		},
		func(exitCode int) {
			exitCh <- exitCode
		},
	)

	// Write to stdin
	testData := []byte("hello world from stdin\n")
	if err := c.WriteStdin(testData); err != nil {
		t.Fatalf("Failed to write stdin: %v", err)
	}

	// Give it a moment to echo back
	time.Sleep(200 * time.Millisecond)

	// Close stdin to let cat exit
	if err := c.CloseStdin(); err != nil {
		t.Fatalf("Failed to close stdin: %v", err)
	}

	// Wait for process to exit
	select {
	case <-exitCh:
		// Process exited
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for process to exit")
	}

	// Check received data
	outputMu.Lock()
	defer outputMu.Unlock()

	if !contains(string(receivedData), "hello world from stdin") {
		t.Errorf("Should have received echoed data, got: %s", string(receivedData))
	}
}

func TestMultipleClients(t *testing.T) {
	tmpDir := t.TempDir()

	// Start daemon - add initial delay to allow clients to attach before output
	config := &daemon.Config{
		Command:    []string{"sh", "-c", "sleep 0.2; for i in 1 2 3; do echo output $i; sleep 0.2; done"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		RuntimeDir: tmpDir,
	}

	d, err := daemon.New(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if startErr := d.Start(); startErr != nil {
		t.Fatalf("Failed to start daemon: %v", startErr)
	}
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	// Wait for socket
	time.Sleep(100 * time.Millisecond)

	// Connect multiple clients
	c1, err := bgclient.Connect(d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect client 1: %v", err)
	}
	defer c1.Close()

	c2, err := bgclient.Connect(d.SocketPath())
	if err != nil {
		t.Fatalf("Failed to connect client 2: %v", err)
	}
	defer c2.Close()

	// Both clients attach
	if err := c1.Attach(protocol.StreamStdout); err != nil {
		t.Fatalf("Failed to attach client 1: %v", err)
	}
	if err := c2.Attach(protocol.StreamStdout); err != nil {
		t.Fatalf("Failed to attach client 2: %v", err)
	}

	// Both clients should receive the same output
	var wg sync.WaitGroup
	wg.Add(2)

	var output1, output2 string
	var mu1, mu2 sync.Mutex

	go func() {
		defer wg.Done()
		c1.ReadMessages(
			func(stream byte, data []byte) error {
				mu1.Lock()
				output1 += string(data)
				mu1.Unlock()
				return nil
			},
			func(exitCode int) {},
		)
	}()

	go func() {
		defer wg.Done()
		c2.ReadMessages(
			func(stream byte, data []byte) error {
				mu2.Lock()
				output2 += string(data)
				mu2.Unlock()
				return nil
			},
			func(exitCode int) {},
		)
	}()

	// Wait for process to complete
	wg.Wait()

	// Both clients should have received output
	mu1.Lock()
	mu2.Lock()
	defer mu1.Unlock()
	defer mu2.Unlock()

	if len(output1) == 0 {
		t.Error("Client 1 should have received output")
	}
	if len(output2) == 0 {
		t.Error("Client 2 should have received output")
	}

	// Both should contain the same lines
	if !contains(output1, "output 1") || !contains(output2, "output 1") {
		t.Error("Both clients should have received 'output 1'")
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
