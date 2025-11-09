package bgclient

import (
	"strings"
	"testing"
	"time"

	"github.com/KarpelesLab/bgrun/daemon"
	"github.com/KarpelesLab/bgrun/protocol"
)

func TestExportAPI(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"bash", "-c", "echo 'Hello World'; echo '\x1b]8;;https://github.com\x1b\\GitHub\x1b]8;;\x1b\\'; sleep 10"},
		StdinMode:  daemon.StdinStream,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		UseVTY:     true,
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Wait for output
	time.Sleep(300 * time.Millisecond)

	t.Run("ExportPlainText", func(t *testing.T) {
		content, err := c.ExportPlainText(false)
		if err != nil {
			t.Fatalf("ExportPlainText failed: %v", err)
		}

		if !strings.Contains(content, "Hello World") {
			t.Errorf("Expected content to contain 'Hello World', got: %s", content)
		}

		if !strings.Contains(content, "GitHub") {
			t.Errorf("Expected content to contain 'GitHub', got: %s", content)
		}
	})

	t.Run("ExportMarkdown", func(t *testing.T) {
		content, err := c.ExportMarkdown(false)
		if err != nil {
			t.Fatalf("ExportMarkdown failed: %v", err)
		}

		if !strings.Contains(content, "Hello World") {
			t.Errorf("Expected content to contain 'Hello World', got: %s", content)
		}

		// Should have Markdown link
		if !strings.Contains(content, "[GitHub](https://github.com)") {
			t.Errorf("Expected Markdown link, got: %s", content)
		}
	})

	t.Run("ExportHTML", func(t *testing.T) {
		content, err := c.ExportHTML(false)
		if err != nil {
			t.Fatalf("ExportHTML failed: %v", err)
		}

		if !strings.Contains(content, "Hello World") {
			t.Errorf("Expected content to contain 'Hello World', got: %s", content)
		}

		// Should have HTML structure
		if !strings.Contains(content, "<!DOCTYPE html>") {
			t.Error("Expected HTML doctype")
		}

		// Should have HTML link
		if !strings.Contains(content, `<a href="https://github.com">`) {
			t.Errorf("Expected HTML link, got: %s", content)
		}
	})

	t.Run("ExportWithCustomOptions", func(t *testing.T) {
		resp, err := c.Export(&protocol.ExportRequest{
			Format:                 protocol.ExportFormatPlainText,
			IncludeScrollback:      false,
			StartLine:              0,
			EndLine:                2,
			PreserveTrailingSpaces: false,
		})
		if err != nil {
			t.Fatalf("Export with custom options failed: %v", err)
		}

		if resp.Format != protocol.ExportFormatPlainText {
			t.Errorf("Expected PlainText format, got: %d", resp.Format)
		}

		if resp.Content == "" {
			t.Error("Expected non-empty content")
		}
	})
}

func TestExportAPIWithoutVTY(t *testing.T) {
	config := &daemon.Config{
		Command:    []string{"echo", "test"},
		StdinMode:  daemon.StdinNull,
		StdoutMode: daemon.IOModeLog,
		StderrMode: daemon.IOModeLog,
		UseVTY:     false, // No VTY
	}
	_, socketPath := setupDaemon(t, config)

	c, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Export should fail without VTY
	_, err = c.ExportPlainText(false)
	if err == nil {
		t.Error("Expected error when exporting without VTY")
	}

	if !strings.Contains(err.Error(), "VTY") {
		t.Errorf("Expected VTY error, got: %v", err)
	}
}

func TestExportAPIZombie(t *testing.T) {
	// Test export on zombie process
	c := &Client{
		pid:      12345,
		isZombie: true,
	}

	_, err := c.ExportPlainText(false)
	if err != ErrProcessTerminated {
		t.Errorf("Expected ErrProcessTerminated, got: %v", err)
	}

	_, err = c.ExportMarkdown(false)
	if err != ErrProcessTerminated {
		t.Errorf("Expected ErrProcessTerminated, got: %v", err)
	}

	_, err = c.ExportHTML(false)
	if err != ErrProcessTerminated {
		t.Errorf("Expected ErrProcessTerminated, got: %v", err)
	}
}
