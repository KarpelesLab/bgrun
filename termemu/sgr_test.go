package termemu

import (
	"strings"
	"testing"
)

func TestSGR_Bold(t *testing.T) {
	term := NewTerminal(5, 40)
	term.Write([]byte("\x1b[1mBold Text\x1b[0m Normal"))

	screen := term.GetScreen()
	if screen[0][0].Attr.Bold != true {
		t.Error("Expected first character to be bold")
	}
	if screen[0][0].Char != 'B' {
		t.Errorf("Expected 'B', got %c", screen[0][0].Char)
	}

	// Check that bold is reset
	normalIdx := 9 // After "Bold Text"
	if screen[0][normalIdx].Attr.Bold != false {
		t.Error("Expected character after reset to not be bold")
	}
}

func TestSGR_Italic(t *testing.T) {
	term := NewTerminal(5, 40)
	term.Write([]byte("\x1b[3mItalic\x1b[23m"))

	screen := term.GetScreen()
	if screen[0][0].Attr.Italic != true {
		t.Error("Expected first character to be italic")
	}
}

func TestSGR_Underline(t *testing.T) {
	term := NewTerminal(5, 40)
	term.Write([]byte("\x1b[4mUnderlined\x1b[24m"))

	screen := term.GetScreen()
	if screen[0][0].Attr.Underline != true {
		t.Error("Expected first character to be underlined")
	}
}

func TestSGR_ForegroundColors(t *testing.T) {
	term := NewTerminal(5, 80)

	// Test standard colors (30-37)
	term.Write([]byte("\x1b[31mRed\x1b[0m"))
	screen := term.GetScreen()
	if screen[0][0].Attr.Fg != ColorRed {
		t.Errorf("Expected red foreground, got %d", screen[0][0].Attr.Fg)
	}

	// Test bright colors (90-97)
	term.Write([]byte("\x1b[92mBright Green\x1b[0m"))
	screen = term.GetScreen()
	if screen[0][3].Attr.Fg != ColorBrightGreen {
		t.Errorf("Expected bright green foreground, got %d", screen[0][3].Attr.Fg)
	}
}

func TestSGR_BackgroundColors(t *testing.T) {
	term := NewTerminal(5, 80)

	// Test standard background colors (40-47)
	term.Write([]byte("\x1b[44mBlue BG\x1b[0m"))
	screen := term.GetScreen()
	if screen[0][0].Attr.Bg != ColorBlue {
		t.Errorf("Expected blue background, got %d", screen[0][0].Attr.Bg)
	}

	// Test bright background colors (100-107)
	term.Write([]byte("\x1b[103mBright Yellow BG\x1b[0m"))
	screen = term.GetScreen()
	if screen[0][7].Attr.Bg != ColorBrightYellow {
		t.Errorf("Expected bright yellow background, got %d", screen[0][7].Attr.Bg)
	}
}

func TestSGR_CombinedAttributes(t *testing.T) {
	term := NewTerminal(5, 80)

	// Test bold + italic
	term.Write([]byte("\x1b[1;3mBold Italic\x1b[0m"))
	screen := term.GetScreen()
	if !screen[0][0].Attr.Bold || !screen[0][0].Attr.Italic {
		t.Error("Expected first character to be both bold and italic")
	}

	// Test bold + red foreground + blue background
	term.Write([]byte("\x1b[1;31;44mStyled\x1b[0m"))
	screen = term.GetScreen()
	if !screen[0][11].Attr.Bold {
		t.Error("Expected bold")
	}
	if screen[0][11].Attr.Fg != ColorRed {
		t.Error("Expected red foreground")
	}
	if screen[0][11].Attr.Bg != ColorBlue {
		t.Error("Expected blue background")
	}
}

func TestSGR_Reset(t *testing.T) {
	term := NewTerminal(5, 80)

	// Apply multiple attributes
	term.Write([]byte("\x1b[1;3;4;31;44mStyled\x1b[0mNormal"))
	screen := term.GetScreen()

	// Check styled text
	if !screen[0][0].Attr.Bold {
		t.Error("Expected bold before reset")
	}

	// Check reset text
	normalIdx := 6
	if screen[0][normalIdx].Attr.Bold {
		t.Error("Expected bold to be reset")
	}
	if screen[0][normalIdx].Attr.Italic {
		t.Error("Expected italic to be reset")
	}
	if screen[0][normalIdx].Attr.Underline {
		t.Error("Expected underline to be reset")
	}
	if screen[0][normalIdx].Attr.Fg != ColorDefault {
		t.Error("Expected default foreground color")
	}
	if screen[0][normalIdx].Attr.Bg != ColorDefault {
		t.Error("Expected default background color")
	}
}

func TestSGR_Reverse(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[7mReversed\x1b[27m"))

	screen := term.GetScreen()
	if !screen[0][0].Attr.Reverse {
		t.Error("Expected reverse attribute")
	}

	// Check that reverse is cleared
	if screen[0][8].Attr.Reverse {
		t.Error("Expected reverse to be cleared")
	}
}

func TestSGR_Strike(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[9mStrikethrough\x1b[29m"))

	screen := term.GetScreen()
	if !screen[0][0].Attr.Strike {
		t.Error("Expected strikethrough attribute")
	}
}

func TestSGR_Dim(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[2mDim\x1b[22m"))

	screen := term.GetScreen()
	if !screen[0][0].Attr.Dim {
		t.Error("Expected dim attribute")
	}

	// SGR 22 clears both bold and dim
	if screen[0][3].Attr.Dim {
		t.Error("Expected dim to be cleared")
	}
}

func TestExportHTML_WithFormatting(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[1mBold\x1b[0m \x1b[3mItalic\x1b[0m \x1b[31mRed\x1b[0m"))

	html := term.Export(ExportOptions{
		Format:                 FormatHTML,
		IncludeScrollback:      false,
		StartLine:              0,
		EndLine:                -1,
		PreserveTrailingSpaces: false,
	})

	// Check for bold styling
	if !strings.Contains(html, "font-weight: bold") {
		t.Error("Expected bold styling in HTML")
	}

	// Check for italic styling
	if !strings.Contains(html, "font-style: italic") {
		t.Error("Expected italic styling in HTML")
	}

	// Check for red color
	if !strings.Contains(html, "color: #aa0000") {
		t.Error("Expected red color in HTML")
	}
}

func TestExportHTML_WithColors(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[32;44mGreen on Blue\x1b[0m"))

	html := term.Export(ExportOptions{
		Format: FormatHTML,
	})

	// Check for green foreground
	if !strings.Contains(html, "color: #00aa00") {
		t.Error("Expected green foreground color in HTML")
	}

	// Check for blue background
	if !strings.Contains(html, "background-color: #0000aa") {
		t.Error("Expected blue background color in HTML")
	}
}

func TestExportHTML_Reverse(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[31;44;7mReversed\x1b[0m"))

	html := term.Export(ExportOptions{
		Format: FormatHTML,
	})

	// When reversed, red foreground should become background
	// and blue background should become foreground
	if !strings.Contains(html, "color: #0000aa") {
		t.Error("Expected blue foreground (reversed from background)")
	}
	if !strings.Contains(html, "background-color: #aa0000") {
		t.Error("Expected red background (reversed from foreground)")
	}
}

func TestExportMarkdown_WithFormatting(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[1mBold\x1b[0m \x1b[3mItalic\x1b[0m \x1b[1;3mBoth\x1b[0m"))

	markdown := term.Export(ExportOptions{
		Format: FormatMarkdown,
	})

	// Check for bold (should be **text**)
	if !strings.Contains(markdown, "**Bold**") {
		t.Errorf("Expected **Bold** in markdown, got: %s", markdown)
	}

	// Check for italic (should be *text*)
	if !strings.Contains(markdown, "*Italic*") {
		t.Errorf("Expected *Italic* in markdown, got: %s", markdown)
	}

	// Check for bold+italic (should be ***text***)
	if !strings.Contains(markdown, "***Both***") {
		t.Errorf("Expected ***Both*** in markdown, got: %s", markdown)
	}
}

func TestExportMarkdown_StripsColors(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[31mRed Text\x1b[0m"))

	markdown := term.Export(ExportOptions{
		Format: FormatMarkdown,
	})

	// Colors should be stripped, just plain text
	if !strings.Contains(markdown, "Red Text") {
		t.Error("Expected 'Red Text' in markdown")
	}

	// Should not contain any color codes
	if strings.Contains(markdown, "color:") || strings.Contains(markdown, "#aa0000") {
		t.Error("Markdown should not contain color styling")
	}
}

func TestExportMarkdown_BoldWithHyperlink(t *testing.T) {
	term := NewTerminal(5, 80)
	term.Write([]byte("\x1b[1m\x1b]8;;https://example.com\x1b\\Bold Link\x1b]8;;\x1b\\\x1b[0m"))

	markdown := term.Export(ExportOptions{
		Format: FormatMarkdown,
	})

	// Should have both bold and link
	// Format: [**Bold Link**](https://example.com)
	if !strings.Contains(markdown, "[**Bold Link**](https://example.com)") {
		t.Errorf("Expected bold link in markdown, got: %s", markdown)
	}
}

func TestSGR_256ColorMode(t *testing.T) {
	term := NewTerminal(5, 80)

	// Test 256 color mode: ESC[38;5;Nm for foreground
	term.Write([]byte("\x1b[38;5;1mColor\x1b[0m"))
	screen := term.GetScreen()

	// Color index 1 should map to ColorRed
	if screen[0][0].Attr.Fg != ColorRed {
		t.Errorf("Expected color index 1 to map to red, got %d", screen[0][0].Attr.Fg)
	}
}

func TestSGR_DefaultColors(t *testing.T) {
	term := NewTerminal(5, 80)

	// Set a color, then reset to default
	term.Write([]byte("\x1b[31mRed\x1b[39mDefault"))
	screen := term.GetScreen()

	// First chars should be red
	if screen[0][0].Attr.Fg != ColorRed {
		t.Error("Expected red foreground")
	}

	// After SGR 39, should be default
	if screen[0][3].Attr.Fg != ColorDefault {
		t.Errorf("Expected default foreground, got %d", screen[0][3].Attr.Fg)
	}
}

func TestColorToCSS(t *testing.T) {
	tests := []struct {
		color    Color
		expected string
	}{
		{ColorDefault, ""},
		{ColorRed, "#aa0000"},
		{ColorGreen, "#00aa00"},
		{ColorBrightRed, "#ff5555"},
		{ColorBrightWhite, "#ffffff"},
		{ColorBlack, "#000000"},
	}

	for _, test := range tests {
		result := colorToCSS(test.color, false)
		if result != test.expected {
			t.Errorf("colorToCSS(%d) = %s, expected %s", test.color, result, test.expected)
		}
	}
}
