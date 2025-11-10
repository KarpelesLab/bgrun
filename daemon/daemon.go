package daemon

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/KarpelesLab/bgrun/protocol"
	"github.com/KarpelesLab/bgrun/termemu"
)

// StdinMode defines how stdin should be handled
type StdinMode int

const (
	StdinNull   StdinMode = iota // /dev/null
	StdinFile                    // read from file
	StdinStream                  // stream from socket
)

// IOMode defines how stdout/stderr should be handled
type IOMode int

const (
	IOModeNull IOMode = iota // /dev/null
	IOModeFile               // write to file
	IOModeLog                // write to output.log
)

// Config holds the daemon configuration
type Config struct {
	Command    []string
	StdinMode  StdinMode
	StdinPath  string // for StdinFile mode
	StdoutMode IOMode
	StdoutPath string // for IOModeFile
	StderrMode IOMode
	StderrPath string // for IOModeFile
	UseVTY     bool
	RuntimeDir string // if empty, will be auto-determined
}

// Daemon represents a background process manager
type Daemon struct {
	config     *Config
	runtimeDir string
	socketPath string
	logPath    string

	cmd       *exec.Cmd
	pid       int
	running   bool
	exitCode  *int
	startedAt time.Time
	endedAt   *time.Time

	stdinPipe   io.WriteCloser
	stdinClosed bool // tracks if stdin has been closed
	stdoutPipe  io.ReadCloser
	stderrPipe  io.ReadCloser

	// File descriptors for cleanup
	stdinFile  *os.File
	stdoutFile *os.File
	stderrFile *os.File

	vtyPty     *os.File          // PTY for VTY mode
	vtyTermemu *termemu.Terminal // Terminal emulator for VTY mode

	logFile *os.File

	listener   net.Listener
	listenerMu sync.Mutex

	mu      sync.RWMutex
	clients map[net.Conn]*client

	closeCh  chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
}

type client struct {
	conn     net.Conn
	attached bool
	streams  byte // which streams to send (StreamStdout, StreamStderr, StreamBoth)
	writeMu  sync.Mutex // protects writes to conn
}

// New creates a new daemon instance
func New(config *Config) (*Daemon, error) {
	if len(config.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	// Determine runtime directory
	runtimeDir := config.RuntimeDir
	if runtimeDir == "" {
		var err error
		runtimeDir, err = getRuntimeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine runtime directory: %w", err)
		}
	}

	d := &Daemon{
		config:     config,
		runtimeDir: runtimeDir,
		socketPath: filepath.Join(runtimeDir, "control.sock"),
		logPath:    filepath.Join(runtimeDir, "output.log"),
		clients:    make(map[net.Conn]*client),
		closeCh:    make(chan struct{}),
		doneCh:     make(chan struct{}),
	}

	return d, nil
}

// getRuntimeDir determines the runtime directory path
func getRuntimeDir() (string, error) {
	// Try XDG_RUNTIME_DIR first
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		dir := filepath.Join(xdgRuntime, "bgrun", strconv.Itoa(os.Getpid()))
		return dir, nil
	}

	// Fall back to /tmp/.bgrun-<uid>/<pid>
	uid := os.Getuid()
	dir := filepath.Join("/tmp", ".bgrun-"+strconv.Itoa(uid), strconv.Itoa(os.Getpid()))
	return dir, nil
}

// RuntimeDir returns the runtime directory path
func (d *Daemon) RuntimeDir() string {
	return d.runtimeDir
}

// SocketPath returns the control socket path
func (d *Daemon) SocketPath() string {
	return d.socketPath
}

// Done returns a channel that is closed when the process exits
func (d *Daemon) Done() <-chan struct{} {
	return d.doneCh
}

// Wait blocks until the process exits
func (d *Daemon) Wait() {
	<-d.doneCh
}

// Start starts the daemon and the managed process
func (d *Daemon) Start() error {
	// Create runtime directory
	if err := os.MkdirAll(d.runtimeDir, 0700); err != nil {
		return fmt.Errorf("failed to create runtime directory: %w", err)
	}

	// Open log file
	var err error
	d.logFile, err = os.OpenFile(d.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Start the process
	if err := d.startProcess(); err != nil {
		d.logFile.Close()
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Start socket server
	if err := d.startSocketServer(); err != nil {
		d.stop()
		return fmt.Errorf("failed to start socket server: %w", err)
	}

	// Start output handlers
	if d.config.UseVTY {
		go d.handleVTYOutput()
	} else {
		go d.handleStdout()
		go d.handleStderr()
	}
	go d.waitForProcess()

	return nil
}

// startProcess starts the managed process
func (d *Daemon) startProcess() error {
	// Use VTY mode if enabled
	if d.config.UseVTY {
		d.startedAt = time.Now()
		return d.startProcessVTY()
	}

	// Standard mode
	d.cmd = exec.Command(d.config.Command[0], d.config.Command[1:]...)

	// Setup stdin
	if err := d.setupStdin(); err != nil {
		return fmt.Errorf("failed to setup stdin: %w", err)
	}

	// Setup stdout
	if err := d.setupStdout(); err != nil {
		return fmt.Errorf("failed to setup stdout: %w", err)
	}

	// Setup stderr
	if err := d.setupStderr(); err != nil {
		return fmt.Errorf("failed to setup stderr: %w", err)
	}

	// Start process in new process group
	d.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	d.startedAt = time.Now()
	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	d.mu.Lock()
	d.pid = d.cmd.Process.Pid
	d.running = true
	d.mu.Unlock()

	log.Printf("Started process %d: %v", d.pid, d.config.Command)

	return nil
}

// setupStdin configures stdin for the process
func (d *Daemon) setupStdin() error {
	switch d.config.StdinMode {
	case StdinNull:
		devNull, err := os.Open("/dev/null")
		if err != nil {
			return err
		}
		d.stdinFile = devNull
		d.cmd.Stdin = devNull

	case StdinFile:
		f, err := os.Open(d.config.StdinPath)
		if err != nil {
			return err
		}
		d.stdinFile = f
		d.cmd.Stdin = f

	case StdinStream:
		pipe, err := d.cmd.StdinPipe()
		if err != nil {
			return err
		}
		d.stdinPipe = pipe
	}

	return nil
}

// setupStdout configures stdout for the process
func (d *Daemon) setupStdout() error {
	switch d.config.StdoutMode {
	case IOModeNull:
		devNull, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		d.stdoutFile = devNull
		d.cmd.Stdout = devNull

	case IOModeFile:
		f, err := os.OpenFile(d.config.StdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		d.stdoutFile = f
		d.cmd.Stdout = f

	case IOModeLog:
		pipe, err := d.cmd.StdoutPipe()
		if err != nil {
			return err
		}
		d.stdoutPipe = pipe
	}

	return nil
}

// setupStderr configures stderr for the process
func (d *Daemon) setupStderr() error {
	switch d.config.StderrMode {
	case IOModeNull:
		devNull, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		d.stderrFile = devNull
		d.cmd.Stderr = devNull

	case IOModeFile:
		f, err := os.OpenFile(d.config.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		d.stderrFile = f
		d.cmd.Stderr = f

	case IOModeLog:
		pipe, err := d.cmd.StderrPipe()
		if err != nil {
			return err
		}
		d.stderrPipe = pipe
	}

	return nil
}

// GetStatus returns the current process status
func (d *Daemon) GetStatus() *protocol.StatusResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := &protocol.StatusResponse{
		PID:       d.pid,
		Running:   d.running,
		ExitCode:  d.exitCode,
		StartedAt: d.startedAt.Format(time.RFC3339),
		Command:   d.config.Command,
		HasVTY:    d.config.UseVTY,
	}

	if d.endedAt != nil {
		endedStr := d.endedAt.Format(time.RFC3339)
		status.EndedAt = &endedStr
	}

	return status
}

// Stop stops the daemon and cleans up resources
func (d *Daemon) stop() {
	d.stopOnce.Do(func() {
		close(d.closeCh)

		// Close listener to unblock Accept()
		d.listenerMu.Lock()
		if d.listener != nil {
			if err := d.listener.Close(); err != nil {
				log.Printf("Error closing listener: %v", err)
			}
		}
		d.listenerMu.Unlock()

		// Close all client connections
		d.mu.Lock()
		conns := make([]net.Conn, 0, len(d.clients))
		for conn := range d.clients {
			conns = append(conns, conn)
		}
		d.mu.Unlock()

		for _, conn := range conns {
			if err := conn.Close(); err != nil {
				log.Printf("Error closing client connection: %v", err)
			}
		}

		// Close pipes
		if d.stdinPipe != nil {
			if err := d.stdinPipe.Close(); err != nil {
				log.Printf("Error closing stdin pipe: %v", err)
			}
		}
		if d.stdoutPipe != nil {
			if err := d.stdoutPipe.Close(); err != nil {
				log.Printf("Error closing stdout pipe: %v", err)
			}
		}
		if d.stderrPipe != nil {
			if err := d.stderrPipe.Close(); err != nil {
				log.Printf("Error closing stderr pipe: %v", err)
			}
		}

		// Close file descriptors
		if d.stdinFile != nil {
			if err := d.stdinFile.Close(); err != nil {
				log.Printf("Error closing stdin file: %v", err)
			}
		}
		if d.stdoutFile != nil {
			if err := d.stdoutFile.Close(); err != nil {
				log.Printf("Error closing stdout file: %v", err)
			}
		}
		if d.stderrFile != nil {
			if err := d.stderrFile.Close(); err != nil {
				log.Printf("Error closing stderr file: %v", err)
			}
		}

		// Close log file
		if d.logFile != nil {
			if err := d.logFile.Close(); err != nil {
				log.Printf("Error closing log file: %v", err)
			}
		}

		// Close VTY PTY
		if d.vtyPty != nil {
			if err := d.vtyPty.Close(); err != nil {
				log.Printf("Error closing VTY PTY: %v", err)
			}
		}

		// Clean up socket file
		if d.socketPath != "" {
			os.Remove(d.socketPath)
		}
	})
}

// waitForProcess waits for the process to exit
func (d *Daemon) waitForProcess() {
	err := d.cmd.Wait()

	d.mu.Lock()
	d.running = false
	now := time.Now()
	d.endedAt = &now

	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		d.exitCode = &code
	} else if err == nil {
		code := 0
		d.exitCode = &code
	} else {
		code := -1
		d.exitCode = &code
	}

	exitCode := *d.exitCode
	d.mu.Unlock()

	log.Printf("Process %d exited with code %d", d.pid, exitCode)

	// Notify all clients of process exit
	d.broadcastProcessExit(exitCode)

	// Remove the socket file to indicate daemon is shutting down
	// Leave status.json for zombie process handling
	if d.socketPath != "" {
		os.Remove(d.socketPath)
	}

	// Signal that the process has exited
	close(d.doneCh)
}

// broadcastProcessExit sends process exit notification to all clients
func (d *Daemon) broadcastProcessExit(exitCode int) {
	d.mu.RLock()
	clients := make([]*client, 0, len(d.clients))
	for _, client := range d.clients {
		clients = append(clients, client)
	}
	d.mu.RUnlock()

	for _, client := range clients {
		client.writeMu.Lock()
		if err := protocol.WriteProcessExit(client.conn, exitCode); err != nil {
			log.Printf("Error broadcasting exit to client: %v", err)
		}
		client.writeMu.Unlock()
	}
}
