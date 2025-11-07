package termemu

import (
	"strings"
	"testing"
)

func TestNewTerminal(t *testing.T) {
	term := NewTerminal(24, 80)
	if term.rows != 24 {
		t.Errorf("Expected 24 rows, got %d", term.rows)
	}
	if term.cols != 80 {
		t.Errorf("Expected 80 cols, got %d", term.cols)
	}
	if term.cursorRow != 0 {
		t.Errorf("Expected cursor row 0, got %d", term.cursorRow)
	}
	if term.cursorCol != 0 {
		t.Errorf("Expected cursor col 0, got %d", term.cursorCol)
	}
	if term.maxScrollback != 1000 {
		t.Errorf("Expected maxScrollback 1000, got %d", term.maxScrollback)
	}
}

func TestBasicTextRendering(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Hello, World!"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")
	if !strings.HasPrefix(lines[0], "Hello, World!") {
		t.Errorf("Expected first line to start with 'Hello, World!', got: %s", lines[0])
	}

	row, col := term.GetCursor()
	if row != 0 {
		t.Errorf("Expected cursor row 0, got %d", row)
	}
	if col != 13 {
		t.Errorf("Expected cursor col 13, got %d", col)
	}
}

func TestLineFeed(t *testing.T) {
	term := NewTerminal(3, 10)
	// Use \r\n for proper newlines (CR+LF)
	term.Write([]byte("Line1\r\nLine2\r\nLine3"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	if !strings.HasPrefix(lines[0], "Line1") {
		t.Errorf("Expected line 0 to start with 'Line1', got: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "Line2") {
		t.Errorf("Expected line 1 to start with 'Line2', got: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "Line3") {
		t.Errorf("Expected line 2 to start with 'Line3', got: %s", lines[2])
	}

	row, col := term.GetCursor()
	if row != 2 {
		t.Errorf("Expected cursor row 2, got %d", row)
	}
	if col != 5 {
		t.Errorf("Expected cursor col 5, got %d", col)
	}
}

func TestCarriageReturn(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Hello\rWorld"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	// After "Hello\rWorld", cursor goes back to col 0 and writes "World"
	// Result should be "World" (World overwrites Hello)
	if !strings.HasPrefix(lines[0], "World") {
		t.Errorf("Expected line to start with 'World', got: %s", lines[0])
	}

	row, col := term.GetCursor()
	if row != 0 {
		t.Errorf("Expected cursor row 0, got %d", row)
	}
	if col != 5 {
		t.Errorf("Expected cursor col 5, got %d", col)
	}
}

func TestBackspace(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("Hello\bX"))

	_, col := term.GetCursor()
	if col != 5 {
		t.Errorf("Expected cursor col 5, got %d", col)
	}
}

func TestTab(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("A\tB"))

	_, col := term.GetCursor()
	// After "A" (col 1), tab moves to next 8-col boundary (col 8), then "B" at col 9
	if col != 9 {
		t.Errorf("Expected cursor col 9, got %d", col)
	}
}

func TestScrolling(t *testing.T) {
	term := NewTerminal(3, 10)

	// Fill screen and force scroll - use \r\n for proper newlines
	term.Write([]byte("Line1\r\nLine2\r\nLine3\r\nLine4"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	// Screen should show Line2, Line3, Line4 (Line1 scrolled to scrollback)
	if !strings.HasPrefix(lines[0], "Line2") {
		t.Errorf("Expected line 0 to start with 'Line2', got: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "Line3") {
		t.Errorf("Expected line 1 to start with 'Line3', got: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "Line4") {
		t.Errorf("Expected line 2 to start with 'Line4', got: %s", lines[2])
	}

	// Check scrollback
	scrollback := term.GetScrollback()
	if len(scrollback) != 1 {
		t.Errorf("Expected 1 line in scrollback, got %d", len(scrollback))
	}
	if len(scrollback) > 0 {
		firstLine := make([]rune, len(scrollback[0]))
		for i, cell := range scrollback[0] {
			if cell.Char == 0 {
				firstLine[i] = ' '
			} else {
				firstLine[i] = cell.Char
			}
		}
		scrollbackStr := strings.TrimSpace(string(firstLine))
		if scrollbackStr != "Line1" {
			t.Errorf("Expected scrollback to contain 'Line1', got: %s", scrollbackStr)
		}
	}
}

func TestResize(t *testing.T) {
	term := NewTerminal(10, 20)
	term.Write([]byte("Hello, World!"))

	// Resize to larger
	term.Resize(15, 30)
	if term.rows != 15 {
		t.Errorf("Expected 15 rows after resize, got %d", term.rows)
	}
	if term.cols != 30 {
		t.Errorf("Expected 30 cols after resize, got %d", term.cols)
	}

	// Content should be preserved
	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")
	if !strings.HasPrefix(lines[0], "Hello, World!") {
		t.Errorf("Expected content preserved after resize, got: %s", lines[0])
	}

	// Resize to smaller (cursor should adjust)
	term.cursorRow = 14
	term.cursorCol = 29
	term.Resize(5, 10)
	row, col := term.GetCursor()
	if row != 4 {
		t.Errorf("Expected cursor row adjusted to 4, got %d", row)
	}
	if col != 9 {
		t.Errorf("Expected cursor col adjusted to 9, got %d", col)
	}
}

func TestCursorMovement(t *testing.T) {
	term := NewTerminal(24, 80)

	// Test cursor up (ESC[A)
	term.cursorRow = 5
	term.Write([]byte("\x1b[3A"))
	row, _ := term.GetCursor()
	if row != 2 {
		t.Errorf("Expected cursor row 2 after up 3, got %d", row)
	}

	// Test cursor down (ESC[B)
	term.Write([]byte("\x1b[5B"))
	row, _ = term.GetCursor()
	if row != 7 {
		t.Errorf("Expected cursor row 7 after down 5, got %d", row)
	}

	// Test cursor forward (ESC[C)
	term.cursorCol = 10
	term.Write([]byte("\x1b[4C"))
	_, col := term.GetCursor()
	if col != 14 {
		t.Errorf("Expected cursor col 14 after forward 4, got %d", col)
	}

	// Test cursor back (ESC[D)
	term.Write([]byte("\x1b[6D"))
	_, col = term.GetCursor()
	if col != 8 {
		t.Errorf("Expected cursor col 8 after back 6, got %d", col)
	}
}

func TestCursorPosition(t *testing.T) {
	term := NewTerminal(24, 80)

	// Test ESC[H (home)
	term.cursorRow = 10
	term.cursorCol = 20
	term.Write([]byte("\x1b[H"))
	row, col := term.GetCursor()
	if row != 0 || col != 0 {
		t.Errorf("Expected cursor at 0,0 after ESC[H, got %d,%d", row, col)
	}

	// Test ESC[10;20H (absolute position)
	term.Write([]byte("\x1b[10;20H"))
	row, col = term.GetCursor()
	if row != 9 || col != 19 {
		t.Errorf("Expected cursor at 9,19 (1-indexed 10,20), got %d,%d", row, col)
	}

	// Test ESC[f (same as H)
	term.Write([]byte("\x1b[5;10f"))
	row, col = term.GetCursor()
	if row != 4 || col != 9 {
		t.Errorf("Expected cursor at 4,9, got %d,%d", row, col)
	}
}

func TestEraseInDisplay(t *testing.T) {
	term := NewTerminal(5, 10)
	term.Write([]byte("Line1\nLine2\nLine3\nLine4\nLine5"))

	// ESC[2J should clear entire screen
	term.Write([]byte("\x1b[2J"))

	screen := term.GetScreenAsString()
	// Screen should be all spaces now
	for _, line := range strings.Split(screen, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			t.Errorf("Expected empty screen after ESC[2J, got line: %s", line)
		}
	}

	row, col := term.GetCursor()
	if row != 0 || col != 0 {
		t.Errorf("Expected cursor at 0,0 after screen clear, got %d,%d", row, col)
	}
}

func TestEraseInLine(t *testing.T) {
	term := NewTerminal(3, 10)
	term.Write([]byte("0123456789"))

	// Move cursor to middle of line
	term.cursorCol = 5

	// ESC[K should clear from cursor to end of line
	term.Write([]byte("\x1b[K"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")
	firstLine := strings.TrimSpace(lines[0])

	if firstLine != "01234" {
		t.Errorf("Expected '01234' after erase to end of line, got: %s", firstLine)
	}
}

func TestEraseEntireLine(t *testing.T) {
	term := NewTerminal(3, 10)
	term.Write([]byte("AAAAAAAAAA\r\nBBBBBBBBBB\r\nCCCCCCCCCC"))

	// Move to middle line
	term.cursorRow = 1
	term.cursorCol = 5

	// ESC[2K should clear entire line
	term.Write([]byte("\x1b[2K"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	if strings.TrimSpace(lines[1]) != "" {
		t.Errorf("Expected empty middle line after ESC[2K, got: %s", lines[1])
	}

	// First and last lines should be unchanged
	if !strings.HasPrefix(lines[0], "AAAAAAAAAA") {
		t.Errorf("Expected first line unchanged, got: %s", lines[0])
	}
	if !strings.HasPrefix(lines[2], "CCCCCCCCCC") {
		t.Errorf("Expected last line unchanged, got: %s", lines[2])
	}
}

func TestComplexVT100Sequence(t *testing.T) {
	term := NewTerminal(10, 40)

	// Simulate a simple progress bar update
	term.Write([]byte("Progress: ["))
	for i := 0; i < 10; i++ {
		term.Write([]byte("="))
	}
	term.Write([]byte("] 100%"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	if !strings.Contains(lines[0], "Progress: [==========] 100%") {
		t.Errorf("Expected progress bar, got: %s", lines[0])
	}
}

func TestPrintableCharacters(t *testing.T) {
	term := NewTerminal(5, 80)

	// Test ASCII printable characters
	term.Write([]byte("ABC123!@#"))
	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	if !strings.HasPrefix(lines[0], "ABC123!@#") {
		t.Errorf("Expected 'ABC123!@#', got: %s", lines[0])
	}
}

func TestLongLineWrapping(t *testing.T) {
	term := NewTerminal(5, 10)

	// Write a line longer than terminal width
	term.Write([]byte("0123456789ABCDEF"))

	screen := term.GetScreenAsString()
	lines := strings.Split(screen, "\n")

	// Should wrap to second line
	if !strings.HasPrefix(lines[0], "0123456789") {
		t.Errorf("Expected first line '0123456789', got: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "ABCDEF") {
		t.Errorf("Expected second line to start with 'ABCDEF', got: %s", lines[1])
	}
}

func TestMultipleScrollback(t *testing.T) {
	term := NewTerminal(3, 10)
	term.maxScrollback = 2 // Limit to 2 lines for testing

	// Write 5 lines (will scroll 2 off the top)
	for i := 1; i <= 5; i++ {
		term.Write([]byte("Line"))
		term.Write([]byte{byte('0' + i)})
		term.Write([]byte("\r\n"))
	}

	scrollback := term.GetScrollback()
	// Should only keep last 2 scrolled lines
	if len(scrollback) != 2 {
		t.Errorf("Expected 2 lines in scrollback, got %d", len(scrollback))
	}
}

func TestCursorBoundaries(t *testing.T) {
	term := NewTerminal(10, 20)

	// Test cursor up at top boundary
	term.cursorRow = 0
	term.Write([]byte("\x1b[5A"))
	row, _ := term.GetCursor()
	if row != 0 {
		t.Errorf("Expected cursor to stay at row 0, got %d", row)
	}

	// Test cursor down at bottom boundary
	term.cursorRow = 9
	term.Write([]byte("\x1b[10B"))
	row, _ = term.GetCursor()
	if row != 9 {
		t.Errorf("Expected cursor to stay at row 9, got %d", row)
	}

	// Test cursor left at left boundary
	term.cursorCol = 0
	term.Write([]byte("\x1b[5D"))
	_, col := term.GetCursor()
	if col != 0 {
		t.Errorf("Expected cursor to stay at col 0, got %d", col)
	}

	// Test cursor right at right boundary
	term.cursorCol = 19
	term.Write([]byte("\x1b[10C"))
	_, col = term.GetCursor()
	if col != 19 {
		t.Errorf("Expected cursor to stay at col 19, got %d", col)
	}
}

func TestEmptyWrite(t *testing.T) {
	term := NewTerminal(10, 20)
	term.Write([]byte{})

	row, col := term.GetCursor()
	if row != 0 || col != 0 {
		t.Errorf("Expected cursor at 0,0 after empty write, got %d,%d", row, col)
	}
}

func TestFormat(t *testing.T) {
	term := NewTerminal(24, 80)
	term.Write([]byte("test\r\ntest\r\ntest"))

	format := term.Format()
	if !strings.Contains(format, "rows=24") {
		t.Errorf("Expected format to contain 'rows=24', got: %s", format)
	}
	if !strings.Contains(format, "cols=80") {
		t.Errorf("Expected format to contain 'cols=80', got: %s", format)
	}
	if !strings.Contains(format, "scrollback=0 lines") {
		t.Errorf("Expected format to contain 'scrollback=0 lines', got: %s", format)
	}
}
