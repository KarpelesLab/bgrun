package main

import (
	"fmt"

	"github.com/KarpelesLab/bgrun/termemu"
)

// Example demonstrating OSC 8 hyperlink support in the terminal emulator
func main() {
	term := termemu.NewTerminal(24, 80)

	// Example 1: Basic hyperlink with ST terminator (ESC \)
	term.Write([]byte("\x1b]8;;https://github.com\x1b\\GitHub\x1b]8;;\x1b\\ - Code hosting platform\n"))

	// Example 2: Hyperlink with BEL terminator
	term.Write([]byte("\x1b]8;;https://golang.org\x07Go Language\x1b]8;;\x07 - Programming language\n"))

	// Example 3: Hyperlink with ID parameter (for linking cells across lines)
	term.Write([]byte("\x1b]8;id=link1;https://example.com\x1b\\Multi-line\nlink example\x1b]8;;\x1b\\\n"))

	// Example 4: Multiple hyperlinks in one line
	term.Write([]byte("Visit "))
	term.Write([]byte("\x1b]8;;https://anthropic.com\x1b\\Anthropic\x1b]8;;\x1b\\"))
	term.Write([]byte(" or "))
	term.Write([]byte("\x1b]8;;https://openai.com\x1b\\OpenAI\x1b]8;;\x1b\\"))
	term.Write([]byte(" for AI research\n"))

	// Get the screen and print it
	screen := term.GetScreen()

	fmt.Println("Terminal Screen Output:")
	fmt.Println("======================")

	for i, row := range screen {
		if i >= 5 { // Only show first 5 lines
			break
		}

		// Print the text content
		line := ""
		for _, cell := range row {
			if cell.Char != 0 {
				line += string(cell.Char)
			}
		}
		fmt.Printf("Line %d: %s\n", i, line)

		// Show hyperlinks on this line
		for j, cell := range row {
			if cell.HyperlinkURL != "" {
				fmt.Printf("  Cell %d: URL=%s", j, cell.HyperlinkURL)
				if cell.HyperlinkID != "" {
					fmt.Printf(", ID=%s", cell.HyperlinkID)
				}
				fmt.Println()
			}
		}
	}

	fmt.Println("\nOSC 8 Hyperlink Specification:")
	fmt.Println("===============================")
	fmt.Println("Opening:  ESC ] 8 ; params ; URI ST")
	fmt.Println("Closing:  ESC ] 8 ; ; ST")
	fmt.Println("Where ST can be: ESC \\ or BEL (\\x07)")
	fmt.Println("Parameters: Optional key=value pairs separated by ':'")
	fmt.Println("Example: id=unique-identifier")
}
