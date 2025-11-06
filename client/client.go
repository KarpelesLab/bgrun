package client

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"syscall"

	"github.com/KarpelesLab/bgrun/protocol"
)

// Client represents a connection to a bgrun daemon
type Client struct {
	conn net.Conn
}

// Connect connects to a bgrun daemon at the specified socket path
func Connect(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket: %w", err)
	}

	return &Client{conn: conn}, nil
}

// Close closes the connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetStatus retrieves the current process status
func (c *Client) GetStatus() (*protocol.StatusResponse, error) {
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
	if err := protocol.WriteMessage(c.conn, protocol.MsgStdin, data); err != nil {
		return fmt.Errorf("failed to write stdin: %w", err)
	}
	return nil
}

// CloseStdin closes the process stdin pipe
func (c *Client) CloseStdin() error {
	if err := protocol.WriteMessage(c.conn, protocol.MsgCloseStdin, nil); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
	}
	return nil
}

// SendSignal sends a signal to the process
func (c *Client) SendSignal(sig syscall.Signal) error {
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
func (c *Client) Wait(timeoutSecs uint32, waitType byte) (byte, error) {
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

// Attach attaches to output streams
// streams can be StreamStdout, StreamStderr, or StreamBoth
func (c *Client) Attach(streams byte) error {
	payload := []byte{streams}
	if err := protocol.WriteMessage(c.conn, protocol.MsgAttach, payload); err != nil {
		return fmt.Errorf("failed to attach: %w", err)
	}
	return nil
}

// Detach detaches from output streams
func (c *Client) Detach() error {
	if err := protocol.WriteMessage(c.conn, protocol.MsgDetach, nil); err != nil {
		return fmt.Errorf("failed to detach: %w", err)
	}
	return nil
}

// Shutdown requests the daemon to shut down
func (c *Client) Shutdown() error {
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
