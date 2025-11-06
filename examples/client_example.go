package main

import (
	"fmt"
	"log"
	"os"

	"github.com/KarpelesLab/bgrun/bgclient"
	"github.com/KarpelesLab/bgrun/protocol"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <socket-path>\n", os.Args[0])
		os.Exit(1)
	}

	socketPath := os.Args[1]

	// Connect to the daemon
	c, err := bgclient.Connect(socketPath)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	// Example 1: Get process status
	fmt.Println("=== Getting Process Status ===")
	status, err := c.GetStatus()
	if err != nil {
		log.Fatalf("Failed to get status: %v", err)
	}

	fmt.Printf("PID: %d\n", status.PID)
	fmt.Printf("Running: %v\n", status.Running)
	if status.ExitCode != nil {
		fmt.Printf("Exit Code: %d\n", *status.ExitCode)
	}
	fmt.Printf("Command: %v\n", status.Command)
	fmt.Printf("Started: %s\n", status.StartedAt)
	if status.EndedAt != nil {
		fmt.Printf("Ended: %s\n", *status.EndedAt)
	}
	fmt.Println()

	// Example 2: Attach to output (if process is still running)
	if status.Running {
		fmt.Println("=== Attaching to Output ===")
		if err := c.Attach(protocol.StreamBoth); err != nil {
			log.Fatalf("Failed to attach: %v", err)
		}

		// Read messages
		err = c.ReadMessages(
			func(stream byte, data []byte) error {
				switch stream {
				case protocol.StreamStdout:
					fmt.Print("[stdout] ")
					os.Stdout.Write(data)
				case protocol.StreamStderr:
					fmt.Print("[stderr] ")
					os.Stderr.Write(data)
				}
				return nil
			},
			func(exitCode int) {
				fmt.Printf("\n=== Process Exited: %d ===\n", exitCode)
			},
		)

		if err != nil {
			log.Printf("Error reading messages: %v", err)
		}
	}

	// Example 3: Writing to stdin (if stdin is available)
	// Uncomment if your bgrun was started with -stdin stream
	/*
		if status.Running {
			fmt.Println("=== Writing to stdin ===")
			testData := []byte("hello from client\n")
			if err := c.WriteStdin(testData); err != nil {
				log.Printf("Failed to write stdin: %v", err)
			}

			// Close stdin when done
			if err := c.CloseStdin(); err != nil {
				log.Printf("Failed to close stdin: %v", err)
			}
		}
	*/
}
