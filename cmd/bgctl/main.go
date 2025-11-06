package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/KarpelesLab/bgrun/client"
	"github.com/KarpelesLab/bgrun/protocol"
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

func cmdSignal(c *client.Client, sig syscall.Signal) error {
	if err := c.SendSignal(sig); err != nil {
		return err
	}

	fmt.Printf("Signal %d sent successfully\n", sig)
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
