package qr

import (
	"errors"
	"image"
	"image/color"
	"github.com/vitrun/qart/coding"
)

// A Level denotes a QR error correction level.
// From least to most tolerant of errors, they are L, M, Q, H.
type Level int

const (
	L Level = iota // 20% redundant
	M              // 38% redundant
	Q              // 55% redundant
	H              // 65% redundant
)

// Encode returns an encoding of text at the given error correction level.
func Encode(text string, level Level) (*Code, error) {
	// Pick data encoding, smallest first.
	// We could split the string and use different encodings
	// but that seems like overkill for now.
	var enc coding.Encoding
	switch {
	case coding.Num(text).Check() == nil:
		enc = coding.Num(text)
	case coding.Alpha(text).Check() == nil:
		enc = coding.Alpha(text)
	default:
		enc = coding.String(text)
	}

	// Pick size.
	l := coding.Level(level)
	var v coding.Version
	for v = coding.MinVersion; ; v++ {
		if v > coding.MaxVersion {
			return nil, errors.New("text too long to encode as QR")
		}
		if enc.Bits(v) <= v.DataBytes(l)*8 {
			break
		}
	}

	// Build and execute plan.
	p, err := coding.NewPlan(v, l, 0)
	if err != nil {
		return nil, err
	}
	cc, err := p.Encode(enc)
	if err != nil {
		return nil, err
	}

	// TODO: Pick appropriate mask.

	return &Code{cc.Bitmap, cc.Size, cc.Stride, 8}, nil
}

// A Code is a square pixel grid.
// It implements image.Image and direct PNG encoding.
type Code struct {
	Bitmap []byte // 1 is black, 0 is white
	Size   int    // number of pixels on a side
	Stride int    // number of bytes per row
	Scale  int    // number of image pixels per QR pixel
}

// Black returns true if the pixel at (x,y) is black.
func (c *Code) Black(x, y int) bool {
	return 0 <= x && x < c.Size && 0 <= y && y < c.Size &&
			c.Bitmap[y*c.Stride+x/8]&(1<<uint(7-x&7)) != 0
}

// Image returns an Image displaying the code.
func (c *Code) Image() image.Image {
	return &codeImage{c}

}

// codeImage implements image.Image
type codeImage struct {
	*Code
}

var (
	whiteColor color.Color = color.Gray{0xFF}
	blackColor color.Color = color.Gray{0x00}
)

func (c *codeImage) Bounds() image.Rectangle {
	d := (c.Size + 8) * c.Scale
	return image.Rect(0, 0, d, d)
}

func (c *codeImage) At(x, y int) color.Color {
	if c.Black(x, y) {
		return blackColor
	}
	return whiteColor
}

func (c *codeImage) ColorModel() color.Model {
	return color.GrayModel
}

