package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/KarpelesLab/bgrun/bgclient"
	"github.com/KarpelesLab/bgrun/daemon"
	"github.com/KarpelesLab/bgrun/protocol"
)

// This example demonstrates using the export API via Unix socket
func main() {
	// Create a temporary directory for the daemon
	tmpDir, err := os.MkdirTemp("", "bgrun-export-api-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Configure daemon with VTY and some interesting content
	config := &daemon.Config{
		Command: []string{
			"bash", "-c",
			`echo "Export API Demo"
			echo "==============="
			echo ""
			echo "Visit \x1b]8;;https://github.com/KarpelesLab/bgrun\x1b\\bgrun on GitHub\x1b]8;;\x1b\\ for more info"
			echo ""
			echo "Features:"
			echo "- Plain text export"
			echo "- Markdown export with hyperlinks"
			echo "- HTML export with styling"
			sleep 30`,
		},
		StdinMode:  daemon.StdinStream,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		UseVTY:     true,
		RuntimeDir: tmpDir,
	}

	// Start the daemon
	d, err := daemon.New(config)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	// Get socket path
	socketPath := tmpDir + "/control.sock"

	// Wait for socket to be ready
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Connect to the daemon
	client, err := bgclient.Connect(socketPath)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Wait for output to be written
	time.Sleep(500 * time.Millisecond)

	fmt.Println("=== EXPORT API DEMO ===")

	// Example 1: Export as plain text
	fmt.Println("1. Plain Text Export (via API):")
	fmt.Println("--------------------------------")
	plainText, err := client.ExportPlainText(false)
	if err != nil {
		log.Fatalf("ExportPlainText failed: %v", err)
	}
	fmt.Println(plainText)

	// Example 2: Export as Markdown
	fmt.Println("\n2. Markdown Export (via API):")
	fmt.Println("------------------------------")
	markdown, err := client.ExportMarkdown(false)
	if err != nil {
		log.Fatalf("ExportMarkdown failed: %v", err)
	}
	fmt.Println(markdown)

	// Example 3: Export as HTML
	fmt.Println("\n3. HTML Export (via API):")
	fmt.Println("-------------------------")
	html, err := client.ExportHTML(false)
	if err != nil {
		log.Fatalf("ExportHTML failed: %v", err)
	}
	fmt.Println(html)

	// Example 4: Custom export with specific options
	fmt.Println("\n4. Custom Export (lines 0-3, with options):")
	fmt.Println("--------------------------------------------")
	resp, err := client.Export(&protocol.ExportRequest{
		Format:                 protocol.ExportFormatMarkdown,
		IncludeScrollback:      false,
		StartLine:              0,
		EndLine:                3,
		PreserveTrailingSpaces: false,
	})
	if err != nil {
		log.Fatalf("Custom export failed: %v", err)
	}
	fmt.Printf("Format: %d\n", resp.Format)
	fmt.Printf("Content:\n%s\n", resp.Content)

	fmt.Println("\n=== API Features ===")
	fmt.Println("✓ Export via Unix socket API")
	fmt.Println("✓ Three formats: PlainText, Markdown, HTML")
	fmt.Println("✓ Hyperlinks preserved in Markdown and HTML")
	fmt.Println("✓ Flexible export options (range, scrollback, spacing)")
	fmt.Println("✓ Convenience methods for quick exports")
	fmt.Println("✓ Custom options for fine-grained control")
}
