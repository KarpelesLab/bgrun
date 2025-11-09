package daemon

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/KarpelesLab/bgrun/protocol"
	"github.com/KarpelesLab/bgrun/termemu"
)

// startSocketServer starts the Unix socket server
func (d *Daemon) startSocketServer() error {
	// Remove existing socket if present
	os.Remove(d.socketPath)

	listener, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(d.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Store listener for cleanup
	d.listenerMu.Lock()
	d.listener = listener
	d.listenerMu.Unlock()

	go d.acceptConnections(listener)

	log.Printf("Socket server listening on %s", d.socketPath)

	return nil
}

// acceptConnections accepts incoming client connections
func (d *Daemon) acceptConnections(listener net.Listener) {
	defer listener.Close()

	for {
		select {
		case <-d.closeCh:
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-d.closeCh:
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		d.mu.Lock()
		d.clients[conn] = &client{
			conn:     conn,
			attached: false,
		}
		d.mu.Unlock()

		go d.handleClient(conn)
	}
}

// isNormalDisconnect checks if an error is a normal client disconnect
func isNormalDisconnect(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	// Check for connection reset by peer (ECONNRESET)
	if strings.Contains(err.Error(), "connection reset by peer") {
		return true
	}
	// Check for use of closed network connection
	if strings.Contains(err.Error(), "use of closed network connection") {
		return true
	}
	return false
}

// handleClient handles a client connection
func (d *Daemon) handleClient(conn net.Conn) {
	defer func() {
		conn.Close()
		d.mu.Lock()
		delete(d.clients, conn)
		d.mu.Unlock()
	}()

	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			if !isNormalDisconnect(err) {
				log.Printf("Read error from client: %v", err)
			}
			return
		}

		if err := d.handleMessage(conn, msg); err != nil {
			log.Printf("Error handling message: %v", err)
			protocol.WriteError(conn, err)
			if err == errShutdown {
				return
			}
		}
	}
}

var errShutdown = fmt.Errorf("shutdown requested")

// handleMessage processes a client message
func (d *Daemon) handleMessage(conn net.Conn, msg *protocol.Message) error {
	switch msg.Type {
	case protocol.MsgStatus:
		return d.handleStatus(conn)

	case protocol.MsgStdin:
		return d.handleStdin(msg.Payload)

	case protocol.MsgSignal:
		return d.handleSignal(conn, msg.Payload)

	case protocol.MsgResize:
		return d.handleResize(conn, msg.Payload)

	case protocol.MsgAttach:
		return d.handleAttach(conn, msg.Payload)

	case protocol.MsgDetach:
		return d.handleDetach(conn)

	case protocol.MsgCloseStdin:
		return d.handleCloseStdin(conn)

	case protocol.MsgWait:
		return d.handleWait(conn, msg.Payload)

	case protocol.MsgGetScreen:
		return d.handleGetScreen(conn)

	case protocol.MsgExport:
		return d.handleExport(conn, msg.Payload)

	case protocol.MsgShutdown:
		return d.handleShutdown(conn)

	default:
		return fmt.Errorf("unknown message type: 0x%02X", msg.Type)
	}
}

// handleStatus sends the current process status
func (d *Daemon) handleStatus(conn net.Conn) error {
	status := d.GetStatus()
	return protocol.WriteStatusResponse(conn, status)
}

// handleStdin writes data to the process stdin
func (d *Daemon) handleStdin(data []byte) error {
	// In VTY mode, write to PTY
	if d.config.UseVTY {
		return d.writeVTY(data)
	}

	// Standard mode
	if d.stdinPipe == nil {
		return fmt.Errorf("stdin is not available for streaming")
	}

	if _, err := d.stdinPipe.Write(data); err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}

	return nil
}

// handleSignal sends a signal to the process
func (d *Daemon) handleSignal(conn net.Conn, payload []byte) error {
	if len(payload) != 1 {
		return fmt.Errorf("invalid signal payload length")
	}

	sigNum := syscall.Signal(payload[0])

	d.mu.RLock()
	pid := d.pid
	running := d.running
	d.mu.RUnlock()

	if !running {
		return fmt.Errorf("process is not running")
	}

	// Send signal to the process
	if err := syscall.Kill(pid, sigNum); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}

	// Send acknowledgment
	return protocol.WriteMessage(conn, protocol.MsgSignalResponse, nil)
}

// handleResize resizes the VTY
func (d *Daemon) handleResize(conn net.Conn, payload []byte) error {
	if !d.config.UseVTY {
		return fmt.Errorf("VTY is not enabled")
	}

	if len(payload) != 4 {
		return fmt.Errorf("invalid resize payload length")
	}

	rows := binary.BigEndian.Uint16(payload[0:2])
	cols := binary.BigEndian.Uint16(payload[2:4])

	// Validate terminal size
	if rows == 0 || cols == 0 || rows > 500 || cols > 500 {
		return fmt.Errorf("invalid terminal size: %dx%d", rows, cols)
	}

	// Resize the PTY
	if err := d.resizeVTY(rows, cols); err != nil {
		return err
	}

	// Send acknowledgment
	return protocol.WriteMessage(conn, protocol.MsgResizeResponse, nil)
}

// handleAttach attaches the client to output streams
func (d *Daemon) handleAttach(conn net.Conn, payload []byte) error {
	if len(payload) != 1 {
		return fmt.Errorf("invalid attach payload length")
	}

	streams := payload[0]
	if streams == 0 || streams > protocol.StreamBoth {
		return fmt.Errorf("invalid stream selector: 0x%02X", streams)
	}

	d.mu.Lock()
	if client, ok := d.clients[conn]; ok {
		client.attached = true
		client.streams = streams
	}
	d.mu.Unlock()

	log.Printf("Client attached to streams: 0x%02X", streams)

	return nil
}

// handleDetach detaches the client from output streams
func (d *Daemon) handleDetach(conn net.Conn) error {
	d.mu.Lock()
	if client, ok := d.clients[conn]; ok {
		client.attached = false
	}
	d.mu.Unlock()

	log.Printf("Client detached from streams")

	return nil
}

// handleCloseStdin closes the stdin pipe
func (d *Daemon) handleCloseStdin(conn net.Conn) error {
	d.mu.Lock()
	if d.stdinPipe == nil || d.stdinClosed {
		d.mu.Unlock()
		return fmt.Errorf("stdin is not available for streaming")
	}
	pipe := d.stdinPipe
	d.stdinClosed = true
	d.mu.Unlock()

	if err := pipe.Close(); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
	}

	log.Printf("Stdin closed by client")

	// Send acknowledgment
	return protocol.WriteMessage(conn, protocol.MsgStatusResponse, []byte(`{"status":"stdin closed"}`))
}

// handleWait waits for a condition with timeout
func (d *Daemon) handleWait(conn net.Conn, payload []byte) error {
	timeoutSecs, waitType, err := protocol.ParseWait(payload)
	if err != nil {
		return err
	}

	log.Printf("Wait request: timeout=%ds, type=%d", timeoutSecs, waitType)

	// Execute the wait (this may block)
	status := d.waitForCondition(timeoutSecs, waitType)

	log.Printf("Wait completed with status: %d", status)

	// Send response
	return protocol.WriteWaitResponse(conn, status)
}

// handleGetScreen returns the current terminal screen state
func (d *Daemon) handleGetScreen(conn net.Conn) error {
	if !d.config.UseVTY {
		return fmt.Errorf("VTY is not enabled")
	}

	if d.vtyTermemu == nil {
		return fmt.Errorf("terminal emulator is not available")
	}

	// Get the screen buffer
	screen := d.vtyTermemu.GetScreen()
	cursorRow, cursorCol := d.vtyTermemu.GetCursor()

	// Check for empty screen
	if len(screen) == 0 {
		return fmt.Errorf("screen buffer is empty")
	}

	// Convert screen to string lines
	lines := make([]string, len(screen))
	for i, row := range screen {
		line := make([]rune, len(row))
		for j, cell := range row {
			if cell.Char == 0 {
				line[j] = ' '
			} else {
				line[j] = cell.Char
			}
		}
		lines[i] = string(line)
	}

	// Create response
	response := &protocol.ScreenResponse{
		Rows:      len(screen),
		Cols:      len(screen[0]),
		CursorRow: cursorRow,
		CursorCol: cursorCol,
		Lines:     lines,
	}

	return protocol.WriteScreenResponse(conn, response)
}

// handleExport exports terminal content in the specified format
func (d *Daemon) handleExport(conn net.Conn, payload []byte) error {
	// Parse export request
	req, err := protocol.ParseExportRequest(payload)
	if err != nil {
		return fmt.Errorf("failed to parse export request: %w", err)
	}

	if !d.config.UseVTY {
		return fmt.Errorf("VTY is not enabled")
	}

	if d.vtyTermemu == nil {
		return fmt.Errorf("terminal emulator is not available")
	}

	// Convert protocol format to termemu format
	var format termemu.ExportFormat
	switch req.Format {
	case protocol.ExportFormatPlainText:
		format = termemu.FormatPlainText
	case protocol.ExportFormatMarkdown:
		format = termemu.FormatMarkdown
	case protocol.ExportFormatHTML:
		format = termemu.FormatHTML
	default:
		return fmt.Errorf("unsupported export format: %d", req.Format)
	}

	// Export terminal content
	content := d.vtyTermemu.Export(termemu.ExportOptions{
		Format:                 format,
		IncludeScrollback:      req.IncludeScrollback,
		StartLine:              req.StartLine,
		EndLine:                req.EndLine,
		PreserveTrailingSpaces: req.PreserveTrailingSpaces,
	})

	// Create and send response
	response := &protocol.ExportResponse{
		Content: content,
		Format:  req.Format,
	}

	return protocol.WriteExportResponse(conn, response)
}

// handleShutdown shuts down the daemon
func (d *Daemon) handleShutdown(conn net.Conn) error {
	log.Printf("Shutdown requested by client")

	// Send acknowledgment before shutting down
	protocol.WriteMessage(conn, protocol.MsgStatusResponse, []byte(`{"status":"shutting down"}`))

	// Stop the daemon in a goroutine to allow the response to be sent
	go d.stop()

	return errShutdown
}

// handleStdout reads stdout and broadcasts to attached clients
func (d *Daemon) handleStdout() {
	if d.stdoutPipe == nil {
		return
	}

	defer d.stdoutPipe.Close()

	buf := make([]byte, 4096)
	for {
		n, err := d.stdoutPipe.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Write to log file
			if d.logFile != nil {
				d.logFile.Write(data)
			}

			// Broadcast to attached clients
			d.broadcastOutput(protocol.StreamStdout, data)
		}

		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "file already closed") {
				log.Printf("Error reading stdout: %v", err)
			}
			return
		}
	}
}

// handleStderr reads stderr and broadcasts to attached clients
func (d *Daemon) handleStderr() {
	if d.stderrPipe == nil {
		return
	}

	defer d.stderrPipe.Close()

	buf := make([]byte, 4096)
	for {
		n, err := d.stderrPipe.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Write to log file
			if d.logFile != nil {
				d.logFile.Write(data)
			}

			// Broadcast to attached clients
			d.broadcastOutput(protocol.StreamStderr, data)
		}

		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "file already closed") {
				log.Printf("Error reading stderr: %v", err)
			}
			return
		}
	}
}

// broadcastOutput sends output to all attached clients
func (d *Daemon) broadcastOutput(stream byte, data []byte) {
	d.mu.RLock()
	clients := make([]*client, 0, len(d.clients))
	for _, client := range d.clients {
		clients = append(clients, client)
	}
	d.mu.RUnlock()

	for _, client := range clients {
		if !client.attached {
			continue
		}

		// Check if client wants this stream
		wantStream := false
		if stream == protocol.StreamStdout && (client.streams&protocol.StreamStdout) != 0 {
			wantStream = true
		}
		if stream == protocol.StreamStderr && (client.streams&protocol.StreamStderr) != 0 {
			wantStream = true
		}

		if wantStream {
			client.writeMu.Lock()
			if err := protocol.WriteOutput(client.conn, stream, data); err != nil {
				log.Printf("Error writing output to client: %v", err)
			}
			client.writeMu.Unlock()
		}
	}
}
