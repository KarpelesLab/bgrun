package termemu

import (
	"bytes"
	"fmt"
	"sync"
)

// Cell represents a single terminal cell with character and attributes
type Cell struct {
	Char rune
	// Future: attributes like color, bold, etc.
}

// Terminal represents a terminal emulator with VT100 support
type Terminal struct {
	mu            sync.RWMutex
	rows          int
	cols          int
	screen        [][]Cell // Current screen buffer
	scrollback    [][]Cell // Scrollback buffer
	cursorRow     int      // Current cursor row (0-indexed)
	cursorCol     int      // Current cursor column (0-indexed)
	maxScrollback int      // Maximum scrollback lines
	parser        *vt100Parser
}

// NewTerminal creates a new terminal emulator
func NewTerminal(rows, cols int) *Terminal {
	t := &Terminal{
		rows:          rows,
		cols:          cols,
		screen:        make([][]Cell, rows),
		scrollback:    make([][]Cell, 0),
		maxScrollback: 1000, // Keep 1000 lines of scrollback
		cursorRow:     0,
		cursorCol:     0,
	}

	// Initialize screen
	for i := 0; i < rows; i++ {
		t.screen[i] = make([]Cell, cols)
	}

	t.parser = newVT100Parser(t)
	return t
}

// Write processes input and updates the terminal state
func (t *Terminal) Write(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.parser.parse(data)
}

// Resize changes the terminal size
func (t *Terminal) Resize(rows, cols int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create new screen buffer
	newScreen := make([][]Cell, rows)
	for i := 0; i < rows; i++ {
		newScreen[i] = make([]Cell, cols)
	}

	// Copy existing content
	copyRows := rows
	if t.rows < copyRows {
		copyRows = t.rows
	}
	for i := 0; i < copyRows; i++ {
		copyCols := cols
		if t.cols < copyCols {
			copyCols = t.cols
		}
		copy(newScreen[i][:copyCols], t.screen[i][:copyCols])
	}

	t.rows = rows
	t.cols = cols
	t.screen = newScreen

	// Adjust cursor position
	if t.cursorRow >= rows {
		t.cursorRow = rows - 1
	}
	if t.cursorCol >= cols {
		t.cursorCol = cols - 1
	}
}

// GetScreen returns a copy of the current screen buffer
func (t *Terminal) GetScreen() [][]Cell {
	t.mu.RLock()
	defer t.mu.RUnlock()

	screen := make([][]Cell, t.rows)
	for i := 0; i < t.rows; i++ {
		screen[i] = make([]Cell, t.cols)
		copy(screen[i], t.screen[i])
	}
	return screen
}

// GetScrollback returns a copy of the scrollback buffer
func (t *Terminal) GetScrollback() [][]Cell {
	t.mu.RLock()
	defer t.mu.RUnlock()

	scrollback := make([][]Cell, len(t.scrollback))
	for i := range t.scrollback {
		scrollback[i] = make([]Cell, len(t.scrollback[i]))
		copy(scrollback[i], t.scrollback[i])
	}
	return scrollback
}

// GetScreenAsString returns the screen as a string
func (t *Terminal) GetScreenAsString() string {
	screen := t.GetScreen()
	var buf bytes.Buffer
	for i, row := range screen {
		if i > 0 {
			buf.WriteByte('\n')
		}
		for _, cell := range row {
			if cell.Char == 0 {
				buf.WriteByte(' ')
			} else {
				buf.WriteRune(cell.Char)
			}
		}
	}
	return buf.String()
}

// GetCursor returns the current cursor position
func (t *Terminal) GetCursor() (row, col int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cursorRow, t.cursorCol
}

// Internal methods for terminal operations

func (t *Terminal) putChar(ch rune) {
	if t.cursorCol >= t.cols {
		t.lineFeed()
		t.cursorCol = 0
	}
	if t.cursorRow >= t.rows {
		t.cursorRow = t.rows - 1
	}
	t.screen[t.cursorRow][t.cursorCol] = Cell{Char: ch}
	t.cursorCol++
}

func (t *Terminal) lineFeed() {
	t.cursorRow++
	if t.cursorRow >= t.rows {
		// Scroll up - move top line to scrollback
		if len(t.screen) > 0 {
			topLine := make([]Cell, t.cols)
			copy(topLine, t.screen[0])
			t.scrollback = append(t.scrollback, topLine)

			// Trim scrollback if too long
			if len(t.scrollback) > t.maxScrollback {
				t.scrollback = t.scrollback[1:]
			}
		}

		// Shift screen up
		copy(t.screen[0:], t.screen[1:])

		// Clear bottom line
		t.screen[t.rows-1] = make([]Cell, t.cols)
		t.cursorRow = t.rows - 1
	}
}

func (t *Terminal) carriageReturn() {
	t.cursorCol = 0
}

func (t *Terminal) backspace() {
	if t.cursorCol > 0 {
		t.cursorCol--
	}
}

func (t *Terminal) moveCursor(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= t.rows {
		row = t.rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= t.cols {
		col = t.cols - 1
	}
	t.cursorRow = row
	t.cursorCol = col
}

func (t *Terminal) clearScreen() {
	for i := 0; i < t.rows; i++ {
		t.screen[i] = make([]Cell, t.cols)
	}
	t.cursorRow = 0
	t.cursorCol = 0
}

func (t *Terminal) clearLine() {
	t.screen[t.cursorRow] = make([]Cell, t.cols)
	t.cursorCol = 0
}

// Format returns a debug string representation
func (t *Terminal) Format() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return fmt.Sprintf("Terminal{rows=%d, cols=%d, cursor=(%d,%d), scrollback=%d lines}",
		t.rows, t.cols, t.cursorRow, t.cursorCol, len(t.scrollback))
}
