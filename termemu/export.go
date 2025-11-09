package termemu

import (
	"html"
	"strings"
)

// ExportFormat represents the output format for export
type ExportFormat int

const (
	// FormatPlainText exports as plain text
	FormatPlainText ExportFormat = iota
	// FormatMarkdown exports as Markdown with hyperlinks
	FormatMarkdown
	// FormatHTML exports as HTML with hyperlinks and styling
	FormatHTML
)

// ExportOptions configures the export behavior
type ExportOptions struct {
	// Format specifies the output format
	Format ExportFormat

	// IncludeScrollback determines whether to include scrollback buffer
	IncludeScrollback bool

	// StartLine is the starting line number (0-indexed, negative values count from scrollback)
	// If IncludeScrollback is true, negative values reference scrollback lines
	// e.g., -10 means 10 lines from the end of scrollback
	StartLine int

	// EndLine is the ending line number (0-indexed, -1 means end of content)
	// If IncludeScrollback is true, this can reference screen lines
	EndLine int

	// PreserveTrailingSpaces keeps trailing spaces on each line
	PreserveTrailingSpaces bool
}

// Export exports the terminal content in the specified format
func (t *Terminal) Export(opts ExportOptions) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Determine which lines to export
	lines := t.getLinesForExport(opts)

	// Export based on format
	switch opts.Format {
	case FormatPlainText:
		return t.exportPlainText(lines, opts)
	case FormatMarkdown:
		return t.exportMarkdown(lines, opts)
	case FormatHTML:
		return t.exportHTML(lines, opts)
	default:
		return t.exportPlainText(lines, opts)
	}
}

// getLinesForExport extracts the lines to be exported based on options
func (t *Terminal) getLinesForExport(opts ExportOptions) [][]Cell {
	var allLines [][]Cell

	if opts.IncludeScrollback {
		// Include scrollback buffer
		allLines = make([][]Cell, 0, len(t.scrollback)+len(t.screen))
		allLines = append(allLines, t.scrollback...)
		allLines = append(allLines, t.screen...)
	} else {
		// Only current screen
		allLines = t.screen
	}

	if len(allLines) == 0 {
		return [][]Cell{}
	}

	// Determine start and end indices
	startIdx := opts.StartLine
	endIdx := opts.EndLine

	// Handle negative start (counting from beginning of scrollback)
	if startIdx < 0 {
		startIdx = 0
	}

	// Handle negative/unset end (use all remaining lines)
	if endIdx < 0 || endIdx >= len(allLines) {
		endIdx = len(allLines) - 1
	}

	// Validate range
	if startIdx > endIdx || startIdx >= len(allLines) {
		return [][]Cell{}
	}

	return allLines[startIdx : endIdx+1]
}

// exportPlainText exports as plain text
func (t *Terminal) exportPlainText(lines [][]Cell, opts ExportOptions) string {
	var sb strings.Builder

	for _, row := range lines {
		line := t.rowToPlainText(row, opts.PreserveTrailingSpaces)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// rowToPlainText converts a row of cells to plain text
func (t *Terminal) rowToPlainText(row []Cell, preserveTrailing bool) string {
	var sb strings.Builder

	for _, cell := range row {
		if cell.Char != 0 {
			sb.WriteRune(cell.Char)
		} else {
			sb.WriteByte(' ')
		}
	}

	str := sb.String()
	if !preserveTrailing {
		str = strings.TrimRight(str, " ")
	}
	return str
}

// exportMarkdown exports as Markdown with hyperlinks
func (t *Terminal) exportMarkdown(lines [][]Cell, opts ExportOptions) string {
	var sb strings.Builder

	for _, row := range lines {
		line := t.rowToMarkdown(row, opts.PreserveTrailingSpaces)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// rowToMarkdown converts a row of cells to Markdown with hyperlinks and formatting
func (t *Terminal) rowToMarkdown(row []Cell, preserveTrailing bool) string {
	if len(row) == 0 {
		return ""
	}

	var sb strings.Builder
	i := 0

	for i < len(row) {
		cell := row[i]

		// Collect consecutive cells with same formatting (ignoring colors) and hyperlink
		startI := i
		bold := cell.Attr.Bold
		italic := cell.Attr.Italic
		url := cell.HyperlinkURL

		for i < len(row) &&
			row[i].Attr.Bold == bold &&
			row[i].Attr.Italic == italic &&
			row[i].HyperlinkURL == url {
			i++
		}

		// Extract text from this span
		var text strings.Builder
		for j := startI; j < i; j++ {
			if row[j].Char != 0 {
				ch := row[j].Char
				// Only escape Markdown characters if not applying formatting
				// that uses those same characters
				if !bold && !italic {
					if ch == '*' || ch == '_' || ch == '`' || ch == '#' || ch == '\\' {
						text.WriteByte('\\')
					}
				} else {
					// When using bold/italic, escape backslashes
					if ch == '\\' {
						text.WriteByte('\\')
					}
				}
				text.WriteRune(ch)
			} else {
				text.WriteByte(' ')
			}
		}
		textStr := text.String()

		// Apply formatting
		var formatted string
		if bold && italic {
			formatted = "***" + textStr + "***"
		} else if bold {
			formatted = "**" + textStr + "**"
		} else if italic {
			formatted = "*" + textStr + "*"
		} else {
			formatted = textStr
		}

		// Apply hyperlink if present
		if url != "" {
			// Escape brackets in link text
			formatted = strings.ReplaceAll(formatted, "[", "\\[")
			formatted = strings.ReplaceAll(formatted, "]", "\\]")
			sb.WriteString("[")
			sb.WriteString(formatted)
			sb.WriteString("](")
			sb.WriteString(url)
			sb.WriteString(")")
		} else {
			sb.WriteString(formatted)
		}
	}

	str := sb.String()
	if !preserveTrailing {
		str = strings.TrimRight(str, " ")
	}
	return str
}

// exportHTML exports as HTML with hyperlinks and styling
func (t *Terminal) exportHTML(lines [][]Cell, opts ExportOptions) string {
	var sb strings.Builder

	// Write HTML header
	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString("<html>\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <style>\n")
	sb.WriteString("    body { font-family: monospace; background-color: #000; color: #fff; padding: 20px; }\n")
	sb.WriteString("    pre { margin: 0; line-height: 1.2; }\n")
	sb.WriteString("    a { color: #4af; text-decoration: underline; }\n")
	sb.WriteString("    a:hover { background-color: #333; }\n")
	sb.WriteString("  </style>\n")
	sb.WriteString("</head>\n<body>\n<pre>")

	// Export lines
	for _, row := range lines {
		line := t.rowToHTML(row, opts.PreserveTrailingSpaces)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	// Write HTML footer
	sb.WriteString("</pre>\n</body>\n</html>")

	return sb.String()
}

// rowToHTML converts a row of cells to HTML with hyperlinks and styling
func (t *Terminal) rowToHTML(row []Cell, preserveTrailing bool) string {
	if len(row) == 0 {
		return ""
	}

	var sb strings.Builder
	i := 0

	for i < len(row) {
		cell := row[i]

		// Check if we need styling for this cell
		hasStyle := cell.Attr.Bold || cell.Attr.Dim || cell.Attr.Italic ||
			cell.Attr.Underline || cell.Attr.Blink || cell.Attr.Reverse ||
			cell.Attr.Strike || cell.Attr.Fg != ColorDefault || cell.Attr.Bg != ColorDefault

		// Collect consecutive cells with same attributes and hyperlink
		startI := i
		attr := cell.Attr
		url := cell.HyperlinkURL
		linkID := cell.HyperlinkID

		for i < len(row) &&
			row[i].Attr == attr &&
			row[i].HyperlinkURL == url &&
			row[i].HyperlinkID == linkID {
			i++
		}

		// Extract text from this span
		var text strings.Builder
		for j := startI; j < i; j++ {
			if row[j].Char != 0 {
				text.WriteString(html.EscapeString(string(row[j].Char)))
			} else {
				text.WriteByte(' ')
			}
		}
		textStr := text.String()

		// Build the HTML for this span
		if url != "" {
			// Hyperlink
			sb.WriteString("<a href=\"")
			sb.WriteString(html.EscapeString(url))
			sb.WriteString("\"")
			if linkID != "" {
				sb.WriteString(" data-link-id=\"")
				sb.WriteString(html.EscapeString(linkID))
				sb.WriteString("\"")
			}
			if hasStyle {
				sb.WriteString(" style=\"")
				sb.WriteString(attributesToCSS(attr))
				sb.WriteString("\"")
			}
			sb.WriteString(">")
			sb.WriteString(textStr)
			sb.WriteString("</a>")
		} else if hasStyle {
			// Styled text without hyperlink
			sb.WriteString("<span style=\"")
			sb.WriteString(attributesToCSS(attr))
			sb.WriteString("\">")
			sb.WriteString(textStr)
			sb.WriteString("</span>")
		} else {
			// Plain text
			sb.WriteString(textStr)
		}
	}

	str := sb.String()
	if !preserveTrailing {
		str = strings.TrimRight(str, " ")
	}
	return str
}

// attributesToCSS converts terminal attributes to CSS style string
func attributesToCSS(attr Attributes) string {
	var styles []string

	// Text formatting
	if attr.Bold {
		styles = append(styles, "font-weight: bold")
	}
	if attr.Dim {
		styles = append(styles, "opacity: 0.5")
	}
	if attr.Italic {
		styles = append(styles, "font-style: italic")
	}
	if attr.Underline {
		styles = append(styles, "text-decoration: underline")
	}
	if attr.Strike {
		styles = append(styles, "text-decoration: line-through")
	}
	if attr.Blink {
		styles = append(styles, "animation: blink 1s step-start infinite")
	}

	// Colors
	var fgColor, bgColor string
	if attr.Reverse {
		// Swap foreground and background when reversed
		fgColor = colorToCSS(attr.Bg, false)
		bgColor = colorToCSS(attr.Fg, true)
	} else {
		fgColor = colorToCSS(attr.Fg, false)
		bgColor = colorToCSS(attr.Bg, true)
	}

	if fgColor != "" {
		styles = append(styles, "color: "+fgColor)
	}
	if bgColor != "" {
		styles = append(styles, "background-color: "+bgColor)
	}

	if attr.Hidden {
		styles = append(styles, "visibility: hidden")
	}

	return strings.Join(styles, "; ")
}

// colorToCSS converts a Color to CSS color value
func colorToCSS(c Color, isBackground bool) string {
	// Handle default color
	if c == ColorDefault {
		return ""
	}

	// Color palette (standard VGA colors)
	colors := map[Color]string{
		ColorBlack:         "#000000",
		ColorRed:           "#aa0000",
		ColorGreen:         "#00aa00",
		ColorYellow:        "#aa5500",
		ColorBlue:          "#0000aa",
		ColorMagenta:       "#aa00aa",
		ColorCyan:          "#00aaaa",
		ColorWhite:         "#aaaaaa",
		ColorBrightBlack:   "#555555",
		ColorBrightRed:     "#ff5555",
		ColorBrightGreen:   "#55ff55",
		ColorBrightYellow:  "#ffff55",
		ColorBrightBlue:    "#5555ff",
		ColorBrightMagenta: "#ff55ff",
		ColorBrightCyan:    "#55ffff",
		ColorBrightWhite:   "#ffffff",
	}

	if color, ok := colors[c]; ok {
		return color
	}

	// For extended 256 colors, return a generic representation
	// This would need to be expanded for full 256-color support
	return ""
}

// ExportCurrentScreen exports only the current screen view
func (t *Terminal) ExportCurrentScreen(format ExportFormat) string {
	return t.Export(ExportOptions{
		Format:            format,
		IncludeScrollback: false,
		StartLine:         0,
		EndLine:           -1,
	})
}

// ExportWithScrollback exports the entire content including scrollback
func (t *Terminal) ExportWithScrollback(format ExportFormat) string {
	return t.Export(ExportOptions{
		Format:            format,
		IncludeScrollback: true,
		StartLine:         0,
		EndLine:           -1,
	})
}

// ExportRange exports a specific range of lines
func (t *Terminal) ExportRange(format ExportFormat, startLine, endLine int, includeScrollback bool) string {
	return t.Export(ExportOptions{
		Format:            format,
		IncludeScrollback: includeScrollback,
		StartLine:         startLine,
		EndLine:           endLine,
	})
}
