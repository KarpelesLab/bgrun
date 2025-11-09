package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/KarpelesLab/bgrun/bgclient"
	"github.com/KarpelesLab/bgrun/daemon"
	"github.com/KarpelesLab/bgrun/protocol"
	"github.com/KarpelesLab/bgrun/terminal"
)

var (
	// Daemon mode flags
	stdinFlag      = flag.String("stdin", "null", "stdin mode: null, stream, or file path")
	stdoutFlag     = flag.String("stdout", "log", "stdout mode: null, log, or file path")
	stderrFlag     = flag.String("stderr", "log", "stderr mode: null, log, or file path")
	vtyFlag        = flag.Bool("vty", false, "run in VTY mode")
	backgroundFlag = flag.Bool("background", false, "run daemon in background")

	// Control mode flags
	ctlFlag = flag.Bool("ctl", false, "run in control mode")
	pidFlag = flag.Int("pid", 0, "PID of bgrun daemon (for control mode)")

	helpFlag = flag.Bool("help", false, "show help message")
)

func main() {
	flag.Parse()

	if *helpFlag {
		showHelp()
		os.Exit(0)
	}

	// Handle background mode - re-exec without -background flag
	if *backgroundFlag {
		runInBackground()
		return
	}

	// Run in control mode if -ctl is specified
	if *ctlFlag {
		runControlMode()
		return
	}

	// Otherwise, run in daemon mode
	runDaemonMode()
}

func runInBackground() {
	// Build new args without -background flag
	var newArgs []string
	skipNext := false

	for i := 1; i < len(os.Args); i++ {
		if skipNext {
			skipNext = false
			continue
		}

		arg := os.Args[i]
		if arg == "-background" || arg == "--background" {
			continue
		}
		if arg == "-background=true" || arg == "--background=true" {
			continue
		}
		if arg == "-background=false" || arg == "--background=false" {
			continue
		}

		newArgs = append(newArgs, arg)
	}

	// Create command to run in background
	cmd := exec.Command(os.Args[0], newArgs...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Start the process
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start background process: %v\n", err)
		os.Exit(1)
	}

	// Output the PID for control operations
	fmt.Println(cmd.Process.Pid)

	// Exit parent process
	os.Exit(0)
}

func runControlMode() {
	if *pidFlag == 0 {
		fmt.Fprintln(os.Stderr, "Error: -pid flag is required for control mode")
		fmt.Fprintln(os.Stderr, "Usage: bgrun -ctl -pid <pid> <command> [args...]")
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

	// Connect to daemon by PID
	c, err := bgclient.New(*pidFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to PID %d: %v\n", *pidFlag, err)
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
			fmt.Fprintln(os.Stderr, "Usage: bgrun -ctl -pid <pid> wait <exit|foreground> <seconds>")
			os.Exit(1)
		}
		waitTypeStr := args[1]
		timeout, err := strconv.ParseUint(args[2], 10, 32)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid timeout: %v\n", err)
			os.Exit(1)
		}
		timeoutSecs := uint32(timeout)
		if err := cmdWait(c, waitTypeStr, timeoutSecs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "signal":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: signal number required")
			os.Exit(1)
		}
		signum, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
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

func runDaemonMode() {
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified")
		fmt.Fprintln(os.Stderr, "Use -help for usage information")
		os.Exit(1)
	}

	// Parse configuration
	config, err := parseConfig(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create daemon
	d, err := daemon.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create daemon: %v\n", err)
		os.Exit(1)
	}

	// Start daemon
	if err := d.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	// Print runtime information
	fmt.Printf("Process started successfully\n")
	fmt.Printf("Runtime directory: %s\n", d.RuntimeDir())
	fmt.Printf("Control socket: %s\n", d.SocketPath())

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for either signal or process exit
	select {
	case <-sigCh:
		log.Println("Received signal, shutting down...")
	case <-d.Done():
		log.Println("Process exited, shutting down...")
	}

	// Write final status to JSON file
	if err := writeFinalStatus(d); err != nil {
		log.Printf("Warning: failed to write final status: %v", err)
	}
}

func parseConfig(command []string) (*daemon.Config, error) {
	config := &daemon.Config{
		Command: command,
		UseVTY:  *vtyFlag,
	}

	// Parse stdin mode
	switch *stdinFlag {
	case "null":
		config.StdinMode = daemon.StdinNull
	case "stream":
		config.StdinMode = daemon.StdinStream
	default:
		// Treat as file path
		config.StdinMode = daemon.StdinFile
		config.StdinPath = *stdinFlag
	}

	// Parse stdout mode
	var err error
	config.StdoutMode, config.StdoutPath, err = parseIOMode(*stdoutFlag)
	if err != nil {
		return nil, fmt.Errorf("invalid stdout mode: %w", err)
	}

	// Parse stderr mode
	config.StderrMode, config.StderrPath, err = parseIOMode(*stderrFlag)
	if err != nil {
		return nil, fmt.Errorf("invalid stderr mode: %w", err)
	}

	return config, nil
}

func parseIOMode(mode string) (daemon.IOMode, string, error) {
	switch mode {
	case "null":
		return daemon.IOModeNull, "", nil
	case "log":
		return daemon.IOModeLog, "", nil
	default:
		// Treat as file path
		return daemon.IOModeFile, mode, nil
	}
}

func showHelp() {
	fmt.Println("bgrun - Background Process Runner")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  bgrun [daemon-options] <command> [args...]    Run daemon mode")
	fmt.Println("  bgrun -ctl -pid <pid> <command> [args...]     Run control mode")
	fmt.Println()
	fmt.Println("Daemon Options:")
	fmt.Println("  -stdin <mode>   stdin mode: null, stream, or file path (default: null)")
	fmt.Println("  -stdout <mode>  stdout mode: null, log, or file path (default: log)")
	fmt.Println("  -stderr <mode>  stderr mode: null, log, or file path (default: log)")
	fmt.Println("  -vty            run in VTY mode")
	fmt.Println("  -background     run daemon in background and output PID")
	fmt.Println()
	fmt.Println("Control Options:")
	fmt.Println("  -ctl         enable control mode")
	fmt.Println("  -pid <pid>   PID of bgrun daemon to control")
	fmt.Println()
	fmt.Println("Control Commands:")
	fmt.Println("  status              Show process status")
	fmt.Println("  attach              Attach to process output")
	fmt.Println("  wait <type> <secs>  Wait for condition (type: exit|foreground)")
	fmt.Println("  signal <signum>     Send signal to process")
	fmt.Println("  shutdown            Shutdown the daemon")
	fmt.Println()
	fmt.Println("General Options:")
	fmt.Println("  -help           show this help message")
	fmt.Println()
	fmt.Println("The daemon creates a runtime directory at:")
	fmt.Println("  $XDG_RUNTIME_DIR/bgrun/<pid>  (if XDG_RUNTIME_DIR is set)")
	fmt.Println("  /tmp/.bgrun-<uid>/<pid>       (otherwise)")
	fmt.Println()
	fmt.Println("In the runtime directory:")
	fmt.Println("  control.sock - Unix socket for control API")
	fmt.Println("  output.log   - Process output (when using 'log' mode)")
	fmt.Println("  status.json  - Final process status (written on exit)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Daemon mode:")
	fmt.Println("  bgrun sleep 100")
	fmt.Println("  bgrun -stdin stream -stdout log bash")
	fmt.Println("  bgrun -vty -stdin stream vim myfile.txt")
	fmt.Println()
	fmt.Println("  # Control mode:")
	fmt.Println("  bgrun -ctl -pid 12345 status")
	fmt.Println("  bgrun -ctl -pid 12345 attach")
	fmt.Println("  bgrun -ctl -pid 12345 wait exit 10")
}

// Control command functions

func cmdStatus(c *bgclient.Client) error {
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

func cmdAttach(c *bgclient.Client) error {
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

func trimTrailingSpaces(s string) string {
	i := len(s) - 1
	for i >= 0 && s[i] == ' ' {
		i--
	}
	return s[:i+1]
}

func cmdAttachNonInteractive(c *bgclient.Client) error {
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

func cmdAttachInteractive(c *bgclient.Client) error {
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

	// Send initial resize before getting screen (ensures screen is sized correctly)
	if err := c.Resize(uint16(rows), uint16(cols)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to resize terminal: %v\n", err)
	}

	// Get and display current screen state
	screen, err := c.GetScreen()
	if err != nil {
		// Non-fatal - just warn and continue
		fmt.Fprintf(os.Stderr, "Warning: failed to get screen state: %v\r\n", err)
	} else {
		// Clear screen and move to top-left
		fmt.Print("\x1b[2J\x1b[H")

		// Display the current screen
		// Trim trailing spaces from each line for better display
		for i, line := range screen.Lines {
			trimmed := trimTrailingSpaces(line)
			fmt.Print(trimmed)
			// Add newline unless it's the last line (to preserve cursor position)
			if i < len(screen.Lines)-1 {
				fmt.Print("\r\n")
			}
		}

		// Move cursor to the reported position
		if screen.CursorRow >= 0 && screen.CursorCol >= 0 {
			// ANSI escape: CSI row ; col H (positions are 1-indexed)
			fmt.Printf("\r\n\x1b[%d;%dH", screen.CursorRow+1, screen.CursorCol+1)
		}
	}

	// Attach to output
	if err := c.Attach(protocol.StreamBoth); err != nil {
		return err
	}

	// Watch for resize signals
	resizeCh := terminal.WatchResize()
	defer terminal.StopWatchingResize(resizeCh)

	// Channel for errors
	errCh := make(chan error, 2)
	doneCh := make(chan struct{})

	// Goroutine to read from stdin and send to server
	// Implements SSH-style escape sequence: <Enter>~. to detach
	detachCh := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		var lastByte byte // Track last byte for escape sequence detection

		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				data := buf[:n]

				// Process data looking for escape sequence: <newline>~.
				// We need to check if the sequence appears in the data
				for i := 0; i < len(data); i++ {
					// Check for escape sequence starting at position i
					// Pattern: previous byte was newline, current is ~, next is .
					if (lastByte == '\r' || lastByte == '\n') && data[i] == '~' {
						// Check if next byte is '.'
						if i+1 < len(data) && data[i+1] == '.' {
							// Found escape sequence - send data up to (but not including) ~.
							if i > 0 {
								c.WriteStdin(data[:i])
							}
							close(detachCh)
							return
						}
					}

					lastByte = data[i]
				}

				// No escape sequence found, send all data
				if writeErr := c.WriteStdin(data); writeErr != nil {
					errCh <- fmt.Errorf("failed to write stdin: %w", writeErr)
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

	// Main loop: handle resize events and detach signal
	for {
		select {
		case <-resizeCh:
			rows, cols, err := terminal.GetSize(fd)
			if err == nil {
				c.Resize(uint16(rows), uint16(cols))
			}

		case <-detachCh:
			// User pressed <Enter>~. to detach
			c.Detach()
			state.Restore()
			fmt.Println("\r\n[Detached]")
			return nil

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

func cmdSignal(c *bgclient.Client, sig syscall.Signal) error {
	if err := c.SendSignal(sig); err != nil {
		return err
	}

	fmt.Printf("Signal %d sent successfully\n", sig)
	return nil
}

func cmdWait(c *bgclient.Client, waitTypeStr string, timeoutSecs uint32) error {
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

func cmdShutdown(c *bgclient.Client) error {
	if err := c.Shutdown(); err != nil {
		// Connection might close before we get a response, which is OK
		if err != io.EOF {
			return err
		}
	}

	fmt.Println("Shutdown request sent")
	return nil
}

func writeFinalStatus(d *daemon.Daemon) error {
	status := d.GetStatus()

	// Write status to JSON file in runtime directory
	statusPath := filepath.Join(d.RuntimeDir(), "status.json")
	f, err := os.Create(statusPath)
	if err != nil {
		return fmt.Errorf("failed to create status file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(status); err != nil {
		return fmt.Errorf("failed to encode status: %w", err)
	}

	return nil
}
