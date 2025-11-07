package bgclient

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/KarpelesLab/bgrun/protocol"
)

// ErrProcessTerminated is returned when attempting operations on a terminated process
var ErrProcessTerminated = errors.New("process has terminated")

// Client represents a connection to a bgrun daemon
type Client struct {
	conn       net.Conn
	pid        int
	runtimeDir string
	isZombie   bool
	status     *protocol.StatusResponse // cached status for zombie processes
}

// Connect connects to a bgrun daemon at the specified socket path
// Deprecated: Use New(pid) instead
func Connect(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket: %w", err)
	}

	return &Client{conn: conn}, nil
}

// New creates a client connection to a bgrun daemon by its PID
// If the daemon has terminated but left a status.json file (zombie state),
// most operations will return ErrProcessTerminated except Wait which will
// return immediately and clean up the zombie.
func New(pid int) (*Client, error) {
	runtimeDir, err := getRuntimeDirForPID(pid)
	if err != nil {
		return nil, err
	}

	socketPath := filepath.Join(runtimeDir, "control.sock")
	statusPath := filepath.Join(runtimeDir, "status.json")

	// Check if socket exists (daemon is running)
	if _, err := os.Stat(socketPath); err == nil {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to socket: %w", err)
		}
		return &Client{
			conn:       conn,
			pid:        pid,
			runtimeDir: runtimeDir,
			isZombie:   false,
		}, nil
	}

	// Socket doesn't exist, check for zombie (status.json exists)
	if _, err := os.Stat(statusPath); err == nil {
		// Read zombie status
		data, err := os.ReadFile(statusPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read zombie status: %w", err)
		}

		var status protocol.StatusResponse
		if err := json.Unmarshal(data, &status); err != nil {
			return nil, fmt.Errorf("failed to parse zombie status: %w", err)
		}

		return &Client{
			pid:        pid,
			runtimeDir: runtimeDir,
			isZombie:   true,
			status:     &status,
		}, nil
	}

	return nil, fmt.Errorf("process %d not found (no socket or status.json in %s)", pid, runtimeDir)
}

// getRuntimeDirForPID finds the runtime directory for a given daemon PID
func getRuntimeDirForPID(pid int) (string, error) {
	// Try XDG_RUNTIME_DIR first
	if xdgDir := os.Getenv("XDG_RUNTIME_DIR"); xdgDir != "" {
		dir := filepath.Join(xdgDir, "bgrun", strconv.Itoa(pid))
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	// Fall back to /tmp/.bgrun-<uid>/<pid>
	uid := os.Getuid()
	dir := filepath.Join("/tmp", ".bgrun-"+strconv.Itoa(uid), strconv.Itoa(pid))
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	return "", fmt.Errorf("runtime directory not found for PID %d (tried XDG_RUNTIME_DIR/bgrun and /tmp/.bgrun-%d)", pid, uid)
}

// Close closes the connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetStatus retrieves the current process status
func (c *Client) GetStatus() (*protocol.StatusResponse, error) {
	// Return cached status for zombie processes
	if c.isZombie {
		return c.status, nil
	}

	if err := protocol.WriteMessage(c.conn, protocol.MsgStatus, nil); err != nil {
		return nil, fmt.Errorf("failed to send status request: %w", err)
	}

	// We might receive a PROCESS_EXIT message before the status response
	// if the process just exited. Keep reading until we get a status response.
	for {
		msg, err := protocol.ReadMessage(c.conn)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		switch msg.Type {
		case protocol.MsgError:
			return nil, fmt.Errorf("server error: %s", string(msg.Payload))

		case protocol.MsgStatusResponse:
			status, err := protocol.ParseStatusResponse(msg.Payload)
			if err != nil {
				return nil, fmt.Errorf("failed to parse status: %w", err)
			}
			return status, nil

		case protocol.MsgProcessExit, protocol.MsgOutput:
			// Ignore these messages and keep reading
			continue

		default:
			return nil, fmt.Errorf("unexpected response type: 0x%02X", msg.Type)
		}
	}
}

// WriteStdin writes data to the process stdin
func (c *Client) WriteStdin(data []byte) error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	if err := protocol.WriteMessage(c.conn, protocol.MsgStdin, data); err != nil {
		return fmt.Errorf("failed to write stdin: %w", err)
	}
	return nil
}

// CloseStdin closes the process stdin pipe
func (c *Client) CloseStdin() error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	if err := protocol.WriteMessage(c.conn, protocol.MsgCloseStdin, nil); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
	}
	return nil
}

// SendSignal sends a signal to the process
func (c *Client) SendSignal(sig syscall.Signal) error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	payload := []byte{byte(sig)}
	if err := protocol.WriteMessage(c.conn, protocol.MsgSignal, payload); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}

	// Wait for acknowledgment
	msg, err := protocol.ReadMessage(c.conn)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if msg.Type == protocol.MsgError {
		return fmt.Errorf("server error: %s", string(msg.Payload))
	}

	if msg.Type != protocol.MsgSignalResponse {
		return fmt.Errorf("unexpected response type: 0x%02X", msg.Type)
	}

	return nil
}

// Resize resizes the VTY terminal
func (c *Client) Resize(rows, cols uint16) error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	payload := make([]byte, 4)
	payload[0] = byte(rows >> 8)
	payload[1] = byte(rows)
	payload[2] = byte(cols >> 8)
	payload[3] = byte(cols)

	if err := protocol.WriteMessage(c.conn, protocol.MsgResize, payload); err != nil {
		return fmt.Errorf("failed to send resize: %w", err)
	}

	// Wait for acknowledgment
	msg, err := protocol.ReadMessage(c.conn)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if msg.Type == protocol.MsgError {
		return fmt.Errorf("server error: %s", string(msg.Payload))
	}

	if msg.Type != protocol.MsgResizeResponse {
		return fmt.Errorf("unexpected response type: 0x%02X", msg.Type)
	}

	return nil
}

// Wait waits for a condition to be met with timeout
// waitType: protocol.WaitTypeExit (wait for process exit) or protocol.WaitTypeForeground (wait for foreground control)
// Returns: protocol.WaitStatusCompleted, protocol.WaitStatusTimeout, or protocol.WaitStatusNotApplicable
// For zombie processes, returns immediately with WaitStatusCompleted and cleans up the runtime directory
func (c *Client) Wait(timeoutSecs uint32, waitType byte) (byte, error) {
	// For zombie processes, return immediately and reap
	if c.isZombie {
		// Only reap on exit wait
		if waitType == protocol.WaitTypeExit {
			if err := c.reapZombie(); err != nil {
				return 0, fmt.Errorf("failed to reap zombie: %w", err)
			}
			return protocol.WaitStatusCompleted, nil
		}
		// For other wait types on zombies, not applicable
		return protocol.WaitStatusNotApplicable, nil
	}

	payload := make([]byte, 5)
	binary.BigEndian.PutUint32(payload[0:4], timeoutSecs)
	payload[4] = waitType

	if err := protocol.WriteMessage(c.conn, protocol.MsgWait, payload); err != nil {
		return 0, fmt.Errorf("failed to send wait: %w", err)
	}

	// Wait for response (may receive MsgProcessExit first)
	for {
		msg, err := protocol.ReadMessage(c.conn)
		if err != nil {
			return 0, fmt.Errorf("failed to read response: %w", err)
		}

		switch msg.Type {
		case protocol.MsgError:
			return 0, fmt.Errorf("server error: %s", string(msg.Payload))

		case protocol.MsgWaitResponse:
			status, err := protocol.ParseWaitResponse(msg.Payload)
			if err != nil {
				return 0, fmt.Errorf("failed to parse wait response: %w", err)
			}
			return status, nil

		case protocol.MsgProcessExit, protocol.MsgOutput:
			// Ignore these messages and keep reading
			continue

		default:
			return 0, fmt.Errorf("unexpected response type: 0x%02X", msg.Type)
		}
	}
}

// reapZombie cleans up the runtime directory for a terminated process
func (c *Client) reapZombie() error {
	if !c.isZombie {
		return fmt.Errorf("cannot reap non-zombie process")
	}
	return os.RemoveAll(c.runtimeDir)
}

// Attach attaches to output streams
// streams can be StreamStdout, StreamStderr, or StreamBoth
func (c *Client) Attach(streams byte) error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	payload := []byte{streams}
	if err := protocol.WriteMessage(c.conn, protocol.MsgAttach, payload); err != nil {
		return fmt.Errorf("failed to attach: %w", err)
	}
	return nil
}

// Detach detaches from output streams
func (c *Client) Detach() error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	if err := protocol.WriteMessage(c.conn, protocol.MsgDetach, nil); err != nil {
		return fmt.Errorf("failed to detach: %w", err)
	}
	return nil
}

// Shutdown requests the daemon to shut down
func (c *Client) Shutdown() error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	if err := protocol.WriteMessage(c.conn, protocol.MsgShutdown, nil); err != nil {
		return fmt.Errorf("failed to send shutdown: %w", err)
	}
	return nil
}

// OutputHandler is called when output is received
type OutputHandler func(stream byte, data []byte) error

// ExitHandler is called when the process exits
type ExitHandler func(exitCode int)

// ReadMessages reads and handles messages from the daemon
// This is typically run in a goroutine after calling Attach()
func (c *Client) ReadMessages(outputHandler OutputHandler, exitHandler ExitHandler) error {
	if c.isZombie {
		return ErrProcessTerminated
	}
	for {
		msg, err := protocol.ReadMessage(c.conn)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to read message: %w", err)
		}

		switch msg.Type {
		case protocol.MsgOutput:
			stream, data, err := protocol.ParseOutput(msg.Payload)
			if err != nil {
				return fmt.Errorf("failed to parse output: %w", err)
			}
			if outputHandler != nil {
				if err := outputHandler(stream, data); err != nil {
					return err
				}
			}

		case protocol.MsgProcessExit:
			exitCode, err := protocol.ParseProcessExit(msg.Payload)
			if err != nil {
				return fmt.Errorf("failed to parse exit code: %w", err)
			}
			if exitHandler != nil {
				exitHandler(exitCode)
			}
			return nil

		case protocol.MsgError:
			return fmt.Errorf("server error: %s", string(msg.Payload))

		default:
			// Ignore unknown message types
		}
	}
}
