// Package qrpng renders WireGuard peer configurations as QR codes, for the
// UI (PNG) and for the serial console/SSH (Unicode half blocks).
package qrpng

import (
	"strings"

	"rsc.io/qr"
)

// PNG returns the QR code as a PNG image.
func PNG(text string) ([]byte, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	c.Scale = 6
	return c.PNG(), nil
}

// Terminal returns the QR code drawn with half blocks (two rows per line),
// dark modules on a light background with a quiet zone.
func Terminal(text string) (string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	n := c.Size
	black := func(x, y int) bool { return x >= 0 && y >= 0 && x < n && y < n && c.Black(x, y) }
	var b strings.Builder
	for y := -2; y < n+2; y += 2 {
		for x := -2; x < n+2; x++ {
			top, bot := black(x, y), black(x, y+1)
			switch {
			case top && bot:
				b.WriteString(" ")
			case top:
				b.WriteString("▄")
			case bot:
				b.WriteString("▀")
			default:
				b.WriteString("█")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
