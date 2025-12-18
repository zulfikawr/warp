package ui

import (
    "bufio"
    "fmt"
    "os"
    "strconv"
    "strings"

    qrcode "github.com/skip2/go-qrcode"
)

// PrintQR renders a QR code to the terminal as compact ASCII blocks.
func PrintQR(s string) error {
    // CHANGE 1: Use qrcode.Low instead of Medium.
    // This produces the smallest possible matrix dimension for the data.
    qr, err := qrcode.New(s, qrcode.Low)
    if err != nil {
        return err
    }
    
    // Remove library border.
    qr.DisableBorder = true

    bm := qr.Bitmap() 

    // CHANGE 2: Removed addQuietZone call.
    // We rely on the terminal's natural background for contrast to save space.

    w := len(bm[0])
    cols := detectTerminalColumns()
    
    if cols > 0 && w > cols {
        fmt.Fprintf(os.Stdout, "(QR width %d exceeds terminal columns %d)\n", w, cols)
    }

    out := bufio.NewWriter(os.Stdout)
    defer out.Flush()

    h := len(bm)
    
    // Render logic remains the same (Half-blocks)
    for y := 0; y < h; y += 2 {
        var b strings.Builder
        for x := 0; x < w; x++ {
            top := bm[y][x]
            bottom := false
            if y+1 < h {
                bottom = bm[y+1][x]
            }
            b.WriteRune(pixel(top, bottom))
        }
        b.WriteRune('\n')
        out.WriteString(b.String())
    }
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