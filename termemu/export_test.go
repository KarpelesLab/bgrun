package termemu

import (
	"strings"
	"testing"
)

func TestExportPlainText(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Hello, World!\n"))
	term.Write([]byte("Line 2"))

	output := term.ExportCurrentScreen(FormatPlainText)
	lines := strings.Split(output, "\n")

	if len(lines) < 2 {
		t.Fatalf("Expected at least 2 lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "Hello, World!") {
		t.Errorf("Expected first line to contain 'Hello, World!', got: %s", lines[0])
	}

	if !strings.Contains(lines[1], "Line 2") {
		t.Errorf("Expected second line to contain 'Line 2', got: %s", lines[1])
	}
}

func TestExportPlainTextWithTrailingSpaces(t *testing.T) {
	term := NewTerminal(3, 10)
	term.Write([]byte("Test"))

	// Without preserving trailing spaces
	output := term.Export(ExportOptions{
		Format:                 FormatPlainText,
		IncludeScrollback:      false,
		PreserveTrailingSpaces: false,
	})
	lines := strings.Split(output, "\n")
	if lines[0] != "Test" {
		t.Errorf("Expected 'Test' without trailing spaces, got: %q", lines[0])
	}

	// With preserving trailing spaces
	output = term.Export(ExportOptions{
		Format:                 FormatPlainText,
		IncludeScrollback:      false,
		PreserveTrailingSpaces: true,
	})
	lines = strings.Split(output, "\n")
	if len(lines[0]) != 10 {
		t.Errorf("Expected line length 10 with trailing spaces, got: %d", len(lines[0]))
	}
}

func TestExportMarkdown(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Plain text\n"))
	term.Write([]byte("\x1b]8;;https://example.com\x1b\\Link\x1b]8;;\x1b\\ normal"))

	output := term.ExportCurrentScreen(FormatMarkdown)
	lines := strings.Split(output, "\n")

	if !strings.Contains(lines[0], "Plain text") {
		t.Errorf("Expected first line to contain 'Plain text', got: %s", lines[0])
	}

	if !strings.Contains(lines[1], "[Link](https://example.com)") {
		t.Errorf("Expected Markdown link in second line, got: %s", lines[1])
	}

	if !strings.Contains(lines[1], "normal") {
		t.Errorf("Expected 'normal' text after link, got: %s", lines[1])
	}
}

func TestExportMarkdownEscaping(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Text with *asterisks* and _underscores_\n"))
	term.Write([]byte("Brackets [test] and `backticks`"))

	output := term.ExportCurrentScreen(FormatMarkdown)
	lines := strings.Split(output, "\n")

	// Check that special characters are escaped
	if !strings.Contains(lines[0], "\\*") {
		t.Errorf("Expected escaped asterisks, got: %s", lines[0])
	}

	if !strings.Contains(lines[0], "\\_") {
		t.Errorf("Expected escaped underscores, got: %s", lines[0])
	}

	if !strings.Contains(lines[1], "\\`") {
		t.Errorf("Expected escaped backticks, got: %s", lines[1])
	}
}

func TestExportMarkdownLinkEscaping(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("\x1b]8;;https://example.com\x1b\\[Link]\x1b]8;;\x1b\\"))

	output := term.ExportCurrentScreen(FormatMarkdown)

	// Link text should have brackets escaped
	if !strings.Contains(output, "[\\[Link\\]](https://example.com)") {
		t.Errorf("Expected escaped brackets in link text, got: %s", output)
	}
}

func TestExportHTML(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Plain text\n"))
	term.Write([]byte("\x1b]8;;https://example.com\x1b\\Link\x1b]8;;\x1b\\ normal"))

	output := term.ExportCurrentScreen(FormatHTML)

	// Check HTML structure
	if !strings.Contains(output, "<!DOCTYPE html>") {
		t.Error("Expected HTML doctype")
	}

	if !strings.Contains(output, "<html>") {
		t.Error("Expected HTML tag")
	}

	if !strings.Contains(output, "<pre>") {
		t.Error("Expected pre tag")
	}

	// Check content
	if !strings.Contains(output, "Plain text") {
		t.Error("Expected plain text content")
	}

	if !strings.Contains(output, `<a href="https://example.com">Link</a>`) {
		t.Errorf("Expected HTML link, got: %s", output)
	}

	if !strings.Contains(output, "normal") {
		t.Error("Expected 'normal' text after link")
	}
}

func TestExportHTMLWithID(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("\x1b]8;id=link1;https://example.com\x1b\\Link\x1b]8;;\x1b\\"))

	output := term.ExportCurrentScreen(FormatHTML)

	if !strings.Contains(output, `data-link-id="link1"`) {
		t.Errorf("Expected data-link-id attribute, got: %s", output)
	}

	if !strings.Contains(output, `href="https://example.com"`) {
		t.Errorf("Expected href attribute, got: %s", output)
	}
}

func TestExportHTMLEscaping(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("<script>alert('xss')</script>\n"))
	term.Write([]byte("\x1b]8;;https://example.com?a=1&b=2\x1b\\Link<>\x1b]8;;\x1b\\"))

	output := term.ExportCurrentScreen(FormatHTML)

	// Check that HTML special characters are escaped
	if strings.Contains(output, "<script>") && !strings.Contains(output, "&lt;script&gt;") {
		t.Error("Expected escaped script tags")
	}

	// Check URL escaping
	if !strings.Contains(output, "&amp;") {
		t.Errorf("Expected escaped ampersand in URL, got: %s", output)
	}

	// Check link text escaping
	if strings.Contains(output, "Link<>") && !strings.Contains(output, "&lt;") {
		t.Error("Expected escaped angle brackets in link text")
	}
}

func TestExportWithScrollback(t *testing.T) {
	term := NewTerminal(3, 80)

	// Write more lines than screen height to create scrollback
	term.Write([]byte("Line 1\n"))
	term.Write([]byte("Line 2\n"))
	term.Write([]byte("Line 3\n"))
	term.Write([]byte("Line 4\n"))
	term.Write([]byte("Line 5"))

	// Export only current screen
	screenOnly := term.ExportCurrentScreen(FormatPlainText)
	screenLines := strings.Split(strings.TrimSpace(screenOnly), "\n")

	// Should have 3 lines (screen height)
	if len(screenLines) != 3 {
		t.Errorf("Expected 3 screen lines, got %d", len(screenLines))
	}

	// Export with scrollback
	withScrollback := term.ExportWithScrollback(FormatPlainText)
	allLines := strings.Split(strings.TrimSpace(withScrollback), "\n")

	// Should have 5 lines total (2 in scrollback + 3 on screen)
	if len(allLines) < 5 {
		t.Errorf("Expected at least 5 lines with scrollback, got %d", len(allLines))
	}

	// Check that scrollback lines are present
	if !strings.Contains(withScrollback, "Line 1") {
		t.Error("Expected scrollback to contain 'Line 1'")
	}

	if !strings.Contains(withScrollback, "Line 2") {
		t.Error("Expected scrollback to contain 'Line 2'")
	}
}

func TestExportRange(t *testing.T) {
	term := NewTerminal(10, 80)

	// Write numbered lines
	for i := 1; i <= 5; i++ {
		term.Write([]byte("Line " + string(rune('0'+i)) + "\n"))
	}

	// Export lines 1-3 (0-indexed)
	output := term.ExportRange(FormatPlainText, 1, 3, false)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "Line 2") {
		t.Errorf("Expected first line to contain 'Line 2', got: %s", lines[0])
	}

	if !strings.Contains(lines[2], "Line 4") {
		t.Errorf("Expected third line to contain 'Line 4', got: %s", lines[2])
	}
}

func TestExportRangeWithScrollback(t *testing.T) {
	term := NewTerminal(3, 80)

	// Create scrollback
	for i := 1; i <= 6; i++ {
		term.Write([]byte("Line " + string(rune('0'+i)) + "\n"))
	}

	// Export from scrollback (lines 0-2, which should be scrollback lines)
	output := term.ExportRange(FormatPlainText, 0, 2, true)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	// Should get the first scrollback lines
	if !strings.Contains(lines[0], "Line") {
		t.Errorf("Expected line to contain 'Line', got: %s", lines[0])
	}
}

func TestExportEmptyTerminal(t *testing.T) {
	term := NewTerminal(24, 80)

	output := term.ExportCurrentScreen(FormatPlainText)

	if output == "" {
		t.Error("Expected non-empty output even for empty terminal")
	}

	// Should have 24 lines (one per row)
	lines := strings.Split(output, "\n")
	if len(lines) < 24 {
		t.Errorf("Expected at least 24 lines, got %d", len(lines))
	}
}

func TestExportInvalidRange(t *testing.T) {
	term := NewTerminal(10, 80)
	term.Write([]byte("Test\n"))

	// Invalid range (start > end)
	output := term.ExportRange(FormatPlainText, 5, 2, false)

	// Should return empty or minimal output
	if strings.TrimSpace(output) != "" && len(strings.Split(output, "\n")) > 2 {
		t.Errorf("Expected minimal output for invalid range, got: %s", output)
	}

	// Start beyond available lines
	output = term.ExportRange(FormatPlainText, 100, 200, false)

	if strings.TrimSpace(output) != "" && len(strings.Split(output, "\n")) > 2 {
		t.Errorf("Expected minimal output for out-of-range start, got: %s", output)
	}
}

func TestExportMultipleHyperlinksMarkdown(t *testing.T) {
	term := NewTerminal(24, 80)

	term.Write([]byte("\x1b]8;;https://first.com\x1b\\First\x1b]8;;\x1b\\ "))
	term.Write([]byte("\x1b]8;;https://second.com\x1b\\Second\x1b]8;;\x1b\\"))

	output := term.ExportCurrentScreen(FormatMarkdown)

	if !strings.Contains(output, "[First](https://first.com)") {
		t.Errorf("Expected first link in Markdown, got: %s", output)
	}

	if !strings.Contains(output, "[Second](https://second.com)") {
		t.Errorf("Expected second link in Markdown, got: %s", output)
	}
}

func TestExportMultipleHyperlinksHTML(t *testing.T) {
	term := NewTerminal(24, 80)

	term.Write([]byte("\x1b]8;;https://first.com\x1b\\First\x1b]8;;\x1b\\ "))
	term.Write([]byte("\x1b]8;;https://second.com\x1b\\Second\x1b]8;;\x1b\\"))

	output := term.ExportCurrentScreen(FormatHTML)

	if !strings.Contains(output, `<a href="https://first.com">First</a>`) {
		t.Errorf("Expected first link in HTML, got: %s", output)
	}

	if !strings.Contains(output, `<a href="https://second.com">Second</a>`) {
		t.Errorf("Expected second link in HTML, got: %s", output)
	}
}

func TestExportMultilineHyperlink(t *testing.T) {
	term := NewTerminal(24, 80)

	term.Write([]byte("\x1b]8;id=multiline;https://example.com\x1b\\"))
	term.Write([]byte("Line 1\n"))
	term.Write([]byte("Line 2\x1b]8;;\x1b\\"))

	outputMD := term.ExportCurrentScreen(FormatMarkdown)
	outputHTML := term.ExportCurrentScreen(FormatHTML)

	// Markdown: each line should have its own link
	if !strings.Contains(outputMD, "[Line 1](https://example.com)") {
		t.Errorf("Expected Markdown link on first line, got: %s", outputMD)
	}

	if !strings.Contains(outputMD, "[Line 2](https://example.com)") {
		t.Errorf("Expected Markdown link on second line, got: %s", outputMD)
	}

	// HTML: should have the link ID
	if !strings.Contains(outputHTML, "data-link-id=\"multiline\"") {
		t.Errorf("Expected link ID in HTML, got: %s", outputHTML)
	}
}
