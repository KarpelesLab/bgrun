package daemon

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"github.com/KarpelesLab/bgrun/termemu"
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
	rows := uint16(24)
	cols := uint16(80)
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	}); err != nil {
		log.Printf("Warning: failed to set initial PTY size: %v", err)
	}

	// Initialize terminal emulator
	d.vtyTermemu = termemu.NewTerminal(int(rows), int(cols))

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

			// Feed to terminal emulator
			if d.vtyTermemu != nil {
				d.vtyTermemu.Write(data)
			}

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

	// Resize terminal emulator
	if d.vtyTermemu != nil {
		d.vtyTermemu.Resize(int(rows), int(cols))
	}

	// Send SIGWINCH to the foreground process group
	// pty.Setsize should do this automatically, but let's be explicit
	d.mu.RLock()
	running := d.running
	d.mu.RUnlock()

	if running {
		// Get the actual foreground process group and send to it
		if pgrp, err := d.getForegroundPgrp(); err == nil && pgrp > 0 {
			if err := syscall.Kill(-pgrp, syscall.SIGWINCH); err != nil {
				log.Printf("Warning: failed to send SIGWINCH to pgrp %d: %v", pgrp, err)
			}
		}
	}

	log.Printf("PTY resized to %dx%d", rows, cols)

	return nil
}

// getForegroundPgrp gets the foreground process group of the PTY
func (d *Daemon) getForegroundPgrp() (int, error) {
	if d.vtyPty == nil {
		return 0, fmt.Errorf("VTY is not available")
	}

	// Use TIOCGPGRP ioctl to get the foreground process group
	var pgrp int
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		d.vtyPty.Fd(),
		syscall.TIOCGPGRP,
		uintptr(unsafe.Pointer(&pgrp)),
	)
	if errno != 0 {
		return 0, fmt.Errorf("failed to get foreground process group: %v", errno)
	}

	return pgrp, nil
}

// waitForCondition waits for a specific condition with timeout
func (d *Daemon) waitForCondition(timeoutSecs uint32, waitType byte) byte {
	// Import protocol package constants
	const (
		WaitTypeExit            byte = 0x00
		WaitTypeForeground      byte = 0x01
		WaitStatusCompleted     byte = 0x00
		WaitStatusTimeout       byte = 0x01
		WaitStatusNotApplicable byte = 0x02
	)

	switch waitType {
	case WaitTypeExit:
		// Wait for process to exit
		return d.waitForExit(timeoutSecs)

	case WaitTypeForeground:
		// Wait for foreground control to return to main process
		if d.vtyPty == nil {
			return WaitStatusNotApplicable
		}
		return d.waitForForeground(timeoutSecs)

	default:
		return WaitStatusNotApplicable
	}
}

// waitForExit waits for the process to exit
func (d *Daemon) waitForExit(timeoutSecs uint32) byte {
	const (
		WaitStatusCompleted byte = 0x00
		WaitStatusTimeout   byte = 0x01
	)

	// Create a channel to signal when process exits
	done := make(chan struct{})
	stop := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				d.mu.RLock()
				running := d.running
				d.mu.RUnlock()

				if !running {
					return
				}
			}
		}
	}()

	// Wait for exit or timeout
	select {
	case <-done:
		return WaitStatusCompleted
	case <-time.After(time.Duration(timeoutSecs) * time.Second):
		close(stop)
		return WaitStatusTimeout
	}
}

// waitForForeground waits for the foreground process group to return to main process
func (d *Daemon) waitForForeground(timeoutSecs uint32) byte {
	const (
		WaitStatusCompleted byte = 0x00
		WaitStatusTimeout   byte = 0x01
	)

	d.mu.RLock()
	targetPid := d.pid
	d.mu.RUnlock()

	// Create a channel to signal when condition is met
	done := make(chan struct{})
	stop := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				pgrp, err := d.getForegroundPgrp()
				if err != nil {
					// If we can't get the pgrp, continue polling
					continue
				}

				if pgrp == targetPid {
					return
				}
			}
		}
	}()

	// Wait for foreground control or timeout
	select {
	case <-done:
		return WaitStatusCompleted
	case <-time.After(time.Duration(timeoutSecs) * time.Second):
		close(stop)
		return WaitStatusTimeout
	}
}
