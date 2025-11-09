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

// rowToMarkdown converts a row of cells to Markdown with hyperlinks
func (t *Terminal) rowToMarkdown(row []Cell, preserveTrailing bool) string {
	if len(row) == 0 {
		return ""
	}

	var sb strings.Builder
	i := 0

	for i < len(row) {
		cell := row[i]

		if cell.HyperlinkURL != "" {
			// Start of hyperlink - collect all consecutive cells with same URL
			url := cell.HyperlinkURL
			var linkText strings.Builder

			for i < len(row) && row[i].HyperlinkURL == url {
				if row[i].Char != 0 {
					linkText.WriteRune(row[i].Char)
				} else {
					linkText.WriteByte(' ')
				}
				i++
			}

			// Write Markdown link
			text := linkText.String()
			// Escape special Markdown characters in link text
			text = strings.ReplaceAll(text, "[", "\\[")
			text = strings.ReplaceAll(text, "]", "\\]")
			sb.WriteString("[")
			sb.WriteString(text)
			sb.WriteString("](")
			sb.WriteString(url)
			sb.WriteString(")")
		} else {
			// Regular character without hyperlink
			if cell.Char != 0 {
				// Escape special Markdown characters
				ch := cell.Char
				if ch == '*' || ch == '_' || ch == '`' || ch == '#' || ch == '\\' {
					sb.WriteByte('\\')
				}
				sb.WriteRune(ch)
			} else {
				sb.WriteByte(' ')
			}
			i++
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

// rowToHTML converts a row of cells to HTML with hyperlinks
func (t *Terminal) rowToHTML(row []Cell, preserveTrailing bool) string {
	if len(row) == 0 {
		return ""
	}

	var sb strings.Builder
	i := 0

	for i < len(row) {
		cell := row[i]

		if cell.HyperlinkURL != "" {
			// Start of hyperlink - collect all consecutive cells with same URL
			url := cell.HyperlinkURL
			id := cell.HyperlinkID
			var linkText strings.Builder

			for i < len(row) && row[i].HyperlinkURL == url {
				if row[i].Char != 0 {
					linkText.WriteString(html.EscapeString(string(row[i].Char)))
				} else {
					linkText.WriteByte(' ')
				}
				i++
			}

			// Write HTML link
			sb.WriteString("<a href=\"")
			sb.WriteString(html.EscapeString(url))
			sb.WriteString("\"")
			if id != "" {
				sb.WriteString(" data-link-id=\"")
				sb.WriteString(html.EscapeString(id))
				sb.WriteString("\"")
			}
			sb.WriteString(">")
			sb.WriteString(linkText.String())
			sb.WriteString("</a>")
		} else {
			// Regular character without hyperlink
			if cell.Char != 0 {
				sb.WriteString(html.EscapeString(string(cell.Char)))
			} else {
				sb.WriteByte(' ')
			}
			i++
		}
	}

	str := sb.String()
	if !preserveTrailing {
		str = strings.TrimRight(str, " ")
	}
	return str
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
