package daemon

import (
	"fmt"
	"io"
	"log"
	"os/exec"

	"github.com/creack/pty"
)

// startProcessVTY starts the process with a PTY
func (d *Daemon) startProcessVTY() error {
	d.cmd = exec.Command(d.config.Command[0], d.config.Command[1:]...)

	// Start the command with a PTY
	ptmx, err := pty.Start(d.cmd)
	if err != nil {
		return fmt.Errorf("failed to start command with PTY: %w", err)
	}

	// Store PTY as both stdin and stdout
	d.vtyPty = ptmx

	// Set initial PTY size (default to 24x80 if not specified)
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: 24,
		Cols: 80,
	}); err != nil {
		log.Printf("Warning: failed to set initial PTY size: %v", err)
	}

	d.mu.Lock()
	d.pid = d.cmd.Process.Pid
	d.running = true
	d.mu.Unlock()

	log.Printf("Started process %d with PTY: %v", d.pid, d.config.Command)

	return nil
}

// handleVTYOutput reads from PTY and broadcasts to clients and log
func (d *Daemon) handleVTYOutput() {
	if d.vtyPty == nil {
		return
	}

	defer d.vtyPty.Close()

	buf := make([]byte, 4096)
	for {
		n, err := d.vtyPty.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Write to log file
			if d.logFile != nil {
				d.logFile.Write(data)
			}

			// Broadcast to attached clients (as stdout stream)
			d.broadcastOutput(1, data) // 1 = stdout
		}

		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from PTY: %v", err)
			}
			return
		}
	}
}

// writeVTY writes data to the PTY
func (d *Daemon) writeVTY(data []byte) error {
	if d.vtyPty == nil {
		return fmt.Errorf("VTY is not available")
	}

	if _, err := d.vtyPty.Write(data); err != nil {
		return fmt.Errorf("failed to write to PTY: %w", err)
	}

	return nil
}

// resizeVTY resizes the PTY
func (d *Daemon) resizeVTY(rows, cols uint16) error {
	if d.vtyPty == nil {
		return fmt.Errorf("VTY is not available")
	}

	if err := pty.Setsize(d.vtyPty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	}); err != nil {
		return fmt.Errorf("failed to resize PTY: %w", err)
	}

	log.Printf("PTY resized to %dx%d", rows, cols)

	return nil
}
