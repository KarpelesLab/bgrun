package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/KarpelesLab/bgrun/client"
	"github.com/KarpelesLab/bgrun/protocol"
	"github.com/KarpelesLab/bgrun/terminal"
)

var (
	socketFlag = flag.String("socket", "", "path to control socket (required)")
)

func main() {
	flag.Parse()

	if *socketFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: -socket flag is required")
		fmt.Fprintln(os.Stderr, "Usage: bgctl -socket <path> <command> [args...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  status              Show process status")
		fmt.Fprintln(os.Stderr, "  attach              Attach to process output")
		fmt.Fprintln(os.Stderr, "  wait <type> <secs>  Wait for condition (type: exit|foreground)")
		fmt.Fprintln(os.Stderr, "  signal <signum>     Send signal to process")
		fmt.Fprintln(os.Stderr, "  shutdown            Shutdown the daemon")
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified")
		os.Exit(1)
	}

	command := args[0]

	c, err := client.Connect(*socketFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch command {
	case "status":
		if err := cmdStatus(c); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "attach":
		if err := cmdAttach(c); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "wait":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: wait type and timeout required")
			fmt.Fprintln(os.Stderr, "Usage: bgctl -socket <path> wait <exit|foreground> <seconds>")
			os.Exit(1)
		}
		waitTypeStr := args[1]
		var timeoutSecs uint32
		if _, err := fmt.Sscanf(args[2], "%d", &timeoutSecs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid timeout: %v\n", err)
			os.Exit(1)
		}
		if err := cmdWait(c, waitTypeStr, timeoutSecs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "signal":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: signal number required")
			os.Exit(1)
		}
		var signum int
		if _, err := fmt.Sscanf(args[1], "%d", &signum); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid signal number: %v\n", err)
			os.Exit(1)
		}
		if err := cmdSignal(c, syscall.Signal(signum)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "shutdown":
		if err := cmdShutdown(c); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func cmdStatus(c *client.Client) error {
	status, err := c.GetStatus()
	if err != nil {
		return err
	}

	fmt.Printf("PID: %d\n", status.PID)
	fmt.Printf("Running: %v\n", status.Running)
	if status.ExitCode != nil {
		fmt.Printf("Exit Code: %d\n", *status.ExitCode)
	}
	fmt.Printf("Started: %s\n", status.StartedAt)
	if status.EndedAt != nil {
		fmt.Printf("Ended: %s\n", *status.EndedAt)
	}
	fmt.Printf("Command: %v\n", status.Command)
	fmt.Printf("Has VTY: %v\n", status.HasVTY)

	return nil
}

func cmdAttach(c *client.Client) error {
	// Check if we're running in a terminal
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		return cmdAttachNonInteractive(c)
	}

	// Get process status to check if it's VTY mode
	status, err := c.GetStatus()
	if err != nil {
		return err
	}

	if status.HasVTY {
		// Interactive VTY mode
		return cmdAttachInteractive(c)
	}

	// Non-VTY mode (just display output)
	return cmdAttachNonInteractive(c)
}

func cmdAttachNonInteractive(c *client.Client) error {
	// Attach to both stdout and stderr
	if err := c.Attach(protocol.StreamBoth); err != nil {
		return err
	}

	fmt.Println("Attached to process output (press Ctrl+C to detach)")
	fmt.Println("---")

	// Read and display output
	return c.ReadMessages(
		func(stream byte, data []byte) error {
			if stream == protocol.StreamStderr {
				os.Stderr.Write(data)
			} else {
				os.Stdout.Write(data)
			}
			return nil
		},
		func(exitCode int) {
			fmt.Printf("\n---\nProcess exited with code %d\n", exitCode)
		},
	)
}

func cmdAttachInteractive(c *client.Client) error {
	// Put terminal in raw mode
	fd := int(os.Stdin.Fd())
	state, err := terminal.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %w", err)
	}
	defer state.Restore()

	// Get current terminal size
	rows, cols, err := terminal.GetSize(fd)
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	// Attach to output
	if err := c.Attach(protocol.StreamBoth); err != nil {
		return err
	}

	// Send initial resize
	if err := c.Resize(uint16(rows), uint16(cols)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to resize terminal: %v\n", err)
	}

	// Watch for resize signals
	resizeCh := terminal.WatchResize()
	defer terminal.StopWatchingResize(resizeCh)

	// Channel for errors
	errCh := make(chan error, 2)
	doneCh := make(chan struct{})

	// Goroutine to read from stdin and send to server
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if err := c.WriteStdin(buf[:n]); err != nil {
					errCh <- fmt.Errorf("failed to write stdin: %w", err)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("failed to read stdin: %w", err)
				}
				return
			}
		}
	}()

	// Goroutine to read from server and write to stdout
	go func() {
		err := c.ReadMessages(
			func(stream byte, data []byte) error {
				os.Stdout.Write(data)
				return nil
			},
			func(exitCode int) {
				close(doneCh)
			},
		)
		if err != nil && err != io.EOF {
			errCh <- err
		}
	}()

	// Main loop: handle resize events
	for {
		select {
		case <-resizeCh:
			rows, cols, err := terminal.GetSize(fd)
			if err == nil {
				c.Resize(uint16(rows), uint16(cols))
			}

		case err := <-errCh:
			state.Restore()
			return err

		case <-doneCh:
			state.Restore()
			fmt.Println("\r\n[Process exited]")
			return nil
		}
	}
}

func cmdSignal(c *client.Client, sig syscall.Signal) error {
	if err := c.SendSignal(sig); err != nil {
		return err
	}

	fmt.Printf("Signal %d sent successfully\n", sig)
	return nil
}

func cmdWait(c *client.Client, waitTypeStr string, timeoutSecs uint32) error {
	var waitType byte
	switch waitTypeStr {
	case "exit":
		waitType = protocol.WaitTypeExit
	case "foreground":
		waitType = protocol.WaitTypeForeground
	default:
		return fmt.Errorf("invalid wait type: %s (must be 'exit' or 'foreground')", waitTypeStr)
	}

	fmt.Printf("Waiting for %s (timeout: %d seconds)...\n", waitTypeStr, timeoutSecs)

	status, err := c.Wait(timeoutSecs, waitType)
	if err != nil {
		return err
	}

	switch status {
	case protocol.WaitStatusCompleted:
		fmt.Println("Wait completed successfully")
	case protocol.WaitStatusTimeout:
		fmt.Println("Wait timed out")
	case protocol.WaitStatusNotApplicable:
		fmt.Println("Wait type not applicable (e.g., foreground wait on non-VTY process)")
	default:
		fmt.Printf("Unknown wait status: %d\n", status)
	}

	return nil
}

func cmdShutdown(c *client.Client) error {
	if err := c.Shutdown(); err != nil {
		// Connection might close before we get a response, which is OK
		if err != io.EOF {
			return err
		}
	}

	fmt.Println("Shutdown request sent")
	return nil
}
