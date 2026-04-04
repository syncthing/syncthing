// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
	"strings"

	"github.com/vitrun/qart/qr"
)

// renderQR generates an ASCII QR code for the given text using Unicode
// half-block characters. Each terminal row encodes two QR rows, keeping
// the output compact. On dark terminals, the rendering is inverted so
// that the QR "white" modules are bright and "black" modules use the
// dark background — ensuring scanners can read the code regardless of
// terminal color scheme.
func renderQR(text string, darkBG bool) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	size := code.Size

	// On dark backgrounds, invert: QR white → █ (bright), QR black → space (dark bg)
	// On light backgrounds: QR black → █ (dark), QR white → space (light bg)
	isSet := func(x, y int) bool {
		v := x >= 0 && x < size && y >= 0 && y < size && code.Black(x, y)
		if darkBG {
			return !v // invert for dark terminals
		}
		return v
	}

	// Quiet zone of 1 cell around the code
	for y := -1; y < size+1; y += 2 {
		b.WriteString("  ")
		for x := -1; x < size+1; x++ {
			top := isSet(x, y)
			bot := isSet(x, y+1)

			switch {
			case top && bot:
				b.WriteRune('\u2588') // █
			case top && !bot:
				b.WriteRune('\u2580') // ▀
			case !top && bot:
				b.WriteRune('\u2584') // ▄
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteRune('\n')
	}

	return b.String(), nil
}

// renderIDOverlay renders the device ID and QR code as a modal overlay.
func renderIDOverlay(s *AppState, styles Styles, darkBG bool) string {
	var b strings.Builder

	b.WriteString(styles.Title.Render("Device Identification"))
	b.WriteString("\n\n")

	// Full device ID (formatted with dashes for easy reading/copying)
	b.WriteString(fmt.Sprintf("  %s\n", styles.Label.Render("Device ID")))
	b.WriteString(fmt.Sprintf("  %s\n\n", s.MyID))

	// QR code
	qrStr, err := renderQR(s.MyID, darkBG)
	if err != nil {
		b.WriteString(styles.StateError.Render(fmt.Sprintf("  QR error: %v", err)))
	} else {
		b.WriteString(qrStr)
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  Scan this QR code with the Syncthing app on another device."))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  Press I or Esc to close."))

	return b.String()
}
