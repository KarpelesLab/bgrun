package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/KarpelesLab/bgrun/daemon"
)

var (
	stdinFlag  = flag.String("stdin", "null", "stdin mode: null, stream, or file path")
	stdoutFlag = flag.String("stdout", "log", "stdout mode: null, log, or file path")
	stderrFlag = flag.String("stderr", "log", "stderr mode: null, log, or file path")
	vtyFlag    = flag.Bool("vty", false, "run in VTY mode")
	helpFlag   = flag.Bool("help", false, "show help message")
)

func main() {
	flag.Parse()

	if *helpFlag {
		showHelp()
		os.Exit(0)
	}

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

	// Wait for signal
	<-sigCh
	log.Println("Received signal, shutting down...")
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
	fmt.Println("Usage: bgrun [options] <command> [args...]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -stdin <mode>   stdin mode: null, stream, or file path (default: null)")
	fmt.Println("  -stdout <mode>  stdout mode: null, log, or file path (default: log)")
	fmt.Println("  -stderr <mode>  stderr mode: null, log, or file path (default: log)")
	fmt.Println("  -vty            run in VTY mode")
	fmt.Println("  -help           show this help message")
	fmt.Println()
	fmt.Println("The daemon creates a runtime directory at:")
	fmt.Println("  $XDG_RUNTIME_DIR/<pid>  (if XDG_RUNTIME_DIR is set)")
	fmt.Println("  /tmp/.bgrun-<uid>/<pid> (otherwise)")
	fmt.Println()
	fmt.Println("In the runtime directory:")
	fmt.Println("  control.sock - Unix socket for control API")
	fmt.Println("  output.log   - Process output (when using 'log' mode)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  bgrun sleep 100")
	fmt.Println("  bgrun -stdin stream -stdout log bash")
	fmt.Println("  bgrun -vty -stdin stream vim myfile.txt")
}
