package ui

import (
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// GenerateQR returns the QR code as a string of ASCII blocks.
func GenerateQR(s string) (string, error) {
	qr, err := qrcode.New(s, qrcode.Medium)
	if err != nil {
		return "", err
	}

	qr.DisableBorder = true
	bm := qr.Bitmap()
	if len(bm) == 0 {
		return "", nil
	}

	w := len(bm[0])
	h := len(bm)

	var b strings.Builder

	// Render the QR code using half-blocks
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x++ {
			top := bm[y][x]
			bottom := false
			if y+1 < h {
				bottom = bm[y+1][x]
			}
			b.WriteRune(pixel(top, bottom))
		}
		b.WriteByte('\n')
	}

	return b.String(), nil
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
