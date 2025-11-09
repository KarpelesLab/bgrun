package main

import (
	"fmt"
	"os"

	"github.com/KarpelesLab/bgrun/termemu"
)

func main() {
	// Create a terminal with some content
	term := termemu.NewTerminal(10, 80)

	// Add some content with hyperlinks and formatting
	term.Write([]byte("Terminal Export Examples\n"))
	term.Write([]byte("========================\n\n"))

	term.Write([]byte("Visit "))
	term.Write([]byte("\x1b]8;;https://github.com/KarpelesLab/bgrun\x1b\\bgrun on GitHub\x1b]8;;\x1b\\"))
	term.Write([]byte(" for source code\n\n"))

	term.Write([]byte("Documentation at "))
	term.Write([]byte("\x1b]8;id=docs;https://example.com/docs\x1b\\docs.example.com\x1b]8;;\x1b\\"))
	term.Write([]byte("\n\n"))

	term.Write([]byte("Special characters: *bold* _italic_ `code`\n"))
	term.Write([]byte("HTML entities: <tag> & \"quotes\"\n"))

	// Add some scrollback by writing more lines
	for i := 0; i < 5; i++ {
		term.Write([]byte(fmt.Sprintf("Scrollback line %d\n", i+1)))
	}

	fmt.Println("=== EXPORT EXAMPLES ===\n")

	// Example 1: Export current screen as plain text
	fmt.Println("1. Plain Text Export (Current Screen):")
	fmt.Println("---------------------------------------")
	plainText := term.ExportCurrentScreen(termemu.FormatPlainText)
	fmt.Println(plainText)

	// Example 2: Export as Markdown
	fmt.Println("\n2. Markdown Export (Current Screen):")
	fmt.Println("-------------------------------------")
	markdown := term.ExportCurrentScreen(termemu.FormatMarkdown)
	fmt.Println(markdown)

	// Example 3: Export specific range
	fmt.Println("\n3. Range Export (Lines 0-2):")
	fmt.Println("-----------------------------")
	rangeExport := term.ExportRange(termemu.FormatPlainText, 0, 2, false)
	fmt.Println(rangeExport)

	// Example 4: Export with scrollback
	fmt.Println("\n4. Export with Scrollback (first 5 lines):")
	fmt.Println("-------------------------------------------")
	withScrollback := term.ExportRange(termemu.FormatPlainText, 0, 4, true)
	fmt.Println(withScrollback)

	// Example 5: Export as HTML and save to file
	fmt.Println("\n5. HTML Export (saved to terminal_export.html):")
	fmt.Println("------------------------------------------------")
	html := term.ExportCurrentScreen(termemu.FormatHTML)

	err := os.WriteFile("terminal_export.html", []byte(html), 0644)
	if err != nil {
		fmt.Printf("Error writing HTML file: %v\n", err)
	} else {
		fmt.Println("HTML export saved to terminal_export.html")
		fmt.Println("Preview (first 500 chars):")
		if len(html) > 500 {
			fmt.Println(html[:500] + "...")
		} else {
			fmt.Println(html)
		}
	}

	// Example 6: Custom export with options
	fmt.Println("\n6. Custom Export with Options:")
	fmt.Println("-------------------------------")
	customExport := term.Export(termemu.ExportOptions{
		Format:                 termemu.FormatMarkdown,
		IncludeScrollback:      false,
		StartLine:              0,
		EndLine:                3,
		PreserveTrailingSpaces: false,
	})
	fmt.Println(customExport)

	fmt.Println("\n=== EXPORT FEATURES ===")
	fmt.Println("✓ Plain Text - Clean text output")
	fmt.Println("✓ Markdown - With clickable hyperlinks preserved")
	fmt.Println("✓ HTML - Styled output with hyperlinks and proper escaping")
	fmt.Println("✓ Range Selection - Export specific line ranges")
	fmt.Println("✓ Scrollback Support - Include or exclude scrollback buffer")
	fmt.Println("✓ Trailing Space Control - Preserve or trim trailing spaces")
}
