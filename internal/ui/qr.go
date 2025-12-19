package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// PrintQR renders a QR code to the terminal as compact ASCII blocks with a border.
func PrintQR(s string) error {
	// Use qrcode.Medium for better scannability (was Low for smallest size)
	qr, err := qrcode.New(s, qrcode.Medium)
	if err != nil {
		return err
	}

	// Remove library border - we'll add our own
	qr.DisableBorder = true

	bm := qr.Bitmap()

	w := len(bm[0])
	cols := detectTerminalColumns()

	if cols > 0 && w > cols {
		_, _ = fmt.Fprintf(os.Stdout, "(QR width %d exceeds terminal columns %d)\n", w, cols)
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	// Print top border
	border := strings.Repeat("─", w+2)
	_, _ = out.WriteString("┌" + border + "┐\n")

	h := len(bm)

	// Render with our custom border (Half-blocks)
	for y := 0; y < h; y += 2 {
		var b strings.Builder
		b.WriteString("│ ") // Left border with padding
		for x := 0; x < w; x++ {
			top := bm[y][x]
			bottom := false
			if y+1 < h {
				bottom = bm[y+1][x]
			}
			b.WriteRune(pixel(top, bottom))
		}
		b.WriteString(" │\n") // Right border with padding
		_, _ = out.WriteString(b.String())
	}

	// Print bottom border
	_, _ = out.WriteString("└" + border + "┘\n")

	return nil
}

func pixel(top, bottom bool) rune {
	switch {
	case top && bottom:
		return '█' // full block
	case top && !bottom:
		return '▀' // upper half
	case !top && bottom:
		return '▄' // lower half
	default:
		return ' ' // empty
	}
}

// detectTerminalColumns returns terminal columns via COLUMNS env var.
func detectTerminalColumns() int {
	s := os.Getenv("COLUMNS")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
