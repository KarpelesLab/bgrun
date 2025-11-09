package termemu

import (
	"strconv"
	"strings"
)

// vt100Parser handles VT100/ANSI escape sequence parsing
type vt100Parser struct {
	term  *Terminal
	state parserState
	buf   []byte
}

type parserState int

const (
	stateNormal parserState = iota
	stateEscape
	stateCSI
	stateOSC        // Operating System Command
	stateOSCEscape  // After ESC in OSC (expecting \)
)

func newVT100Parser(term *Terminal) *vt100Parser {
	return &vt100Parser{
		term:  term,
		state: stateNormal,
		buf:   make([]byte, 0, 32),
	}
}

func (p *vt100Parser) parse(data []byte) {
	for _, b := range data {
		p.processByte(b)
	}
}

func (p *vt100Parser) processByte(b byte) {
	switch p.state {
	case stateNormal:
		p.processNormal(b)
	case stateEscape:
		p.processEscape(b)
	case stateCSI:
		p.processCSI(b)
	case stateOSC:
		p.processOSC(b)
	case stateOSCEscape:
		p.processOSCEscape(b)
	}
}

func (p *vt100Parser) processNormal(b byte) {
	switch b {
	case '\x1b': // ESC
		p.state = stateEscape
		p.buf = p.buf[:0]
	case '\n': // Line feed
		p.term.lineFeed()
	case '\r': // Carriage return
		p.term.carriageReturn()
	case '\b': // Backspace
		p.term.backspace()
	case '\t': // Tab
		// Move to next tab stop (every 8 columns)
		nextTab := ((p.term.cursorCol / 8) + 1) * 8
		if nextTab < p.term.cols {
			p.term.cursorCol = nextTab
		}
	default:
		if b >= 32 && b < 127 || b >= 160 { // Printable characters
			p.term.putChar(rune(b))
		}
	}
}

func (p *vt100Parser) processEscape(b byte) {
	switch b {
	case '[': // CSI - Control Sequence Introducer
		p.state = stateCSI
		p.buf = p.buf[:0]
	case ']': // OSC - Operating System Command
		p.state = stateOSC
		p.buf = p.buf[:0]
	case 'M': // Reverse index (move up with scroll)
		if p.term.cursorRow > 0 {
			p.term.cursorRow--
		}
		p.state = stateNormal
	case '7': // Save cursor position (DECSC)
		// TODO: implement cursor save
		p.state = stateNormal
	case '8': // Restore cursor position (DECRC)
		// TODO: implement cursor restore
		p.state = stateNormal
	default:
		// Unknown escape sequence, back to normal
		p.state = stateNormal
	}
}

func (p *vt100Parser) processCSI(b byte) {
	// CSI sequences end with a letter (A-Z, a-z) or @, `, ~
	if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '@' || b == '`' || b == '~' {
		p.executeCSI(b)
		p.state = stateNormal
		return
	}

	// Accumulate parameters
	p.buf = append(p.buf, b)
}

func (p *vt100Parser) executeCSI(cmd byte) {
	params := p.parseParams(string(p.buf))

	switch cmd {
	case 'A': // Cursor up
		n := 1
		if len(params) > 0 {
			n = params[0]
		}
		p.term.cursorRow -= n
		if p.term.cursorRow < 0 {
			p.term.cursorRow = 0
		}

	case 'B': // Cursor down
		n := 1
		if len(params) > 0 {
			n = params[0]
		}
		p.term.cursorRow += n
		if p.term.cursorRow >= p.term.rows {
			p.term.cursorRow = p.term.rows - 1
		}

	case 'C': // Cursor forward
		n := 1
		if len(params) > 0 {
			n = params[0]
		}
		p.term.cursorCol += n
		if p.term.cursorCol >= p.term.cols {
			p.term.cursorCol = p.term.cols - 1
		}

	case 'D': // Cursor back
		n := 1
		if len(params) > 0 {
			n = params[0]
		}
		p.term.cursorCol -= n
		if p.term.cursorCol < 0 {
			p.term.cursorCol = 0
		}

	case 'H', 'f': // Cursor position
		row := 1
		col := 1
		if len(params) > 0 {
			row = params[0]
		}
		if len(params) > 1 {
			col = params[1]
		}
		// VT100 uses 1-indexed positions
		p.term.moveCursor(row-1, col-1)

	case 'J': // Erase in display
		mode := 0
		if len(params) > 0 {
			mode = params[0]
		}
		switch mode {
		case 0: // Clear from cursor to end of screen
			// TODO: implement partial clear
			p.term.clearScreen()
		case 1: // Clear from cursor to beginning of screen
			// TODO: implement partial clear
		case 2: // Clear entire screen
			p.term.clearScreen()
		}

	case 'K': // Erase in line
		mode := 0
		if len(params) > 0 {
			mode = params[0]
		}
		switch mode {
		case 0: // Clear from cursor to end of line
			for i := p.term.cursorCol; i < p.term.cols; i++ {
				p.term.screen[p.term.cursorRow][i] = Cell{}
			}
		case 1: // Clear from cursor to beginning of line
			for i := 0; i <= p.term.cursorCol && i < p.term.cols; i++ {
				p.term.screen[p.term.cursorRow][i] = Cell{}
			}
		case 2: // Clear entire line
			p.term.clearLine()
		}

	case 'm': // SGR - Select Graphic Rendition (colors, bold, etc.)
		// For now, just ignore color codes
		// TODO: implement color support

	case 'r': // Set scrolling region
		// TODO: implement scrolling regions

	case 'l', 'h': // Reset/Set mode
		// TODO: implement mode settings

	default:
		// Unknown CSI command, ignore
	}
}

func (p *vt100Parser) parseParams(s string) []int {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ";")
	params := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err == nil {
			params = append(params, n)
		}
	}
	return params
}

func (p *vt100Parser) processOSC(b byte) {
	// OSC sequences end with BEL (\x07) or ESC \ (ST - String Terminator)
	if b == '\x07' { // BEL
		p.executeOSC(string(p.buf))
		p.state = stateNormal
		return
	}
	if b == '\x1b' { // ESC (might be followed by \)
		p.state = stateOSCEscape
		return
	}
	// Accumulate OSC data
	p.buf = append(p.buf, b)
}

func (p *vt100Parser) processOSCEscape(b byte) {
	if b == '\\' { // ST - String Terminator
		p.executeOSC(string(p.buf))
		p.state = stateNormal
		return
	}
	// Not a valid ST, treat ESC as part of data and continue
	p.buf = append(p.buf, '\x1b', b)
	p.state = stateOSC
}

func (p *vt100Parser) executeOSC(data string) {
	// OSC format: command ; parameters
	// For OSC 8: "8;params;URI" or "8;;" to close hyperlink
	parts := strings.SplitN(data, ";", 2)
	if len(parts) < 1 {
		return
	}

	cmd := parts[0]
	if cmd != "8" {
		// Only handle OSC 8 (hyperlinks) for now
		return
	}

	if len(parts) < 2 {
		// Invalid OSC 8, ignore
		return
	}

	// Parse OSC 8: "8;params;URI"
	rest := parts[1]
	oscParts := strings.SplitN(rest, ";", 2)

	if len(oscParts) < 2 {
		// Invalid format, clear hyperlink
		p.term.hyperlink = nil
		return
	}

	params := oscParts[0]
	uri := oscParts[1]

	if uri == "" {
		// Empty URI means close/clear the hyperlink
		p.term.hyperlink = nil
		return
	}

	// Parse parameters (key=value pairs separated by :)
	// Currently only 'id' parameter is defined
	id := ""
	if params != "" {
		for _, param := range strings.Split(params, ":") {
			if strings.HasPrefix(param, "id=") {
				id = strings.TrimPrefix(param, "id=")
				break
			}
		}
	}

	// Set the current hyperlink
	p.term.hyperlink = &Hyperlink{
		URL: uri,
		ID:  id,
	}
}
