// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.bsd file.

// Package coding implements low-level QR coding details.
package coding

import (
	"fmt"
	"strconv"
	"strings"
	"github.com/vitrun/qart/gf256"
)

// Field is the field for QR error correction.
var Field = gf256.NewField(0x11d, 2)

// A Version represents a QR version.
// The version specifies the size of the QR code:
// a QR code with version v has 4v+17 pixels on a side.
// Versions number from 1 to 40: the larger the version,
// the more information the code can store.
type Version int

const MinVersion = 1
const MaxVersion = 40

func (v Version) String() string {
	return strconv.Itoa(int(v))
}

func (v Version) sizeClass() int {
	if v <= 9 {
		return 0
	}
	if v <= 26 {
		return 1
	}
	return 2
}

// DataBytes returns the number of data bytes that can be
// stored in a QR code with the given version and level.
func (v Version) DataBytes(l Level) int {
	vt := &vtab[v]
	lev := &vt.level[l]
	return vt.bytes - lev.nblock*lev.check
}

// Encoding implements a QR data encoding scheme.
// The implementations--Numeric, Alphanumeric, and String--specify
// the character set and the mapping from UTF-8 to code bits.
// The more restrictive the mode, the fewer code bits are needed.
type Encoding interface {
	Check() error
	Bits(v Version) int
	Encode(b *Bits, v Version)
}

type Bits struct {
	b    []byte
	nbit int
}

func (b *Bits) Reset() {
	b.b = b.b[:0]
	b.nbit = 0
}

func (b *Bits) Bits() int {
	return b.nbit
}

func (b *Bits) Bytes() []byte {
	if b.nbit%8 != 0 {
		panic("fractional byte")
	}
	return b.b
}

func (b *Bits) Append(p []byte) {
	if b.nbit%8 != 0 {
		panic("fractional byte")
	}
	b.b = append(b.b, p...)
	b.nbit += 8 * len(p)
}

func (b *Bits) Write(v uint, nbit int) {
	for nbit > 0 {
		n := nbit
		if n > 8 {
			n = 8
		}
		if b.nbit%8 == 0 {
			b.b = append(b.b, 0)
		} else {
			m := -b.nbit & 7
			if n > m {
				n = m
			}
		}
		b.nbit += n
		sh := uint(nbit - n)
		b.b[len(b.b)-1] |= uint8(v >> sh << uint(-b.nbit&7))
		v -= v >> sh << sh
		nbit -= n
	}
}

// Num is the encoding for numeric data.
// The only valid characters are the decimal digits 0 through 9.
type Num string

func (s Num) String() string {
	return fmt.Sprintf("Num(%#q)", string(s))
}

func (s Num) Check() error {
	for _, c := range s {
		if c < '0' || '9' < c {
			return fmt.Errorf("non-numeric string %#q", string(s))
		}
	}
	return nil
}

var numLen = [3]int{10, 12, 14}

func (s Num) Bits(v Version) int {
	return 4 + numLen[v.sizeClass()] + (10*len(s)+2)/3
}

func (s Num) Encode(b *Bits, v Version) {
	b.Write((uint)(1), 4)
	b.Write(uint(len(s)), numLen[v.sizeClass()])
	var i int
	for i = 0; i+3 <= len(s); i += 3 {
		w := uint(s[i]-'0')*100 + uint(s[i+1]-'0')*10 + uint(s[i+2]-'0')
		b.Write(w, 10)
	}
	switch len(s) - i {
	case 1:
		w := uint(s[i] - '0')
		b.Write(w, 4)
	case 2:
		w := uint(s[i]-'0')*10 + uint(s[i+1]-'0')
		b.Write(w, 7)
	}
}

// Alpha is the encoding for alphanumeric data.
// The valid characters are 0-9A-Z$%*+-./: and space.
type Alpha string

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

func (s Alpha) String() string {
	return fmt.Sprintf("Alpha(%#q)", string(s))
}

func (s Alpha) Check() error {
	for _, c := range s {
		if strings.IndexRune(alphabet, c) < 0 {
			return fmt.Errorf("non-alphanumeric string %#q", string(s))
		}
	}
	return nil
}

var alphaLen = [3]int{9, 11, 13}

func (s Alpha) Bits(v Version) int {
	return 4 + alphaLen[v.sizeClass()] + (11*len(s)+1)/2
}

func (s Alpha) Encode(b *Bits, v Version) {
	b.Write((uint)(2), 4)
	b.Write(uint(len(s)), alphaLen[v.sizeClass()])
	var i int
	for i = 0; i+2 <= len(s); i += 2 {
		w := uint(strings.IndexRune(alphabet, rune(s[i])))*45 +
				uint(strings.IndexRune(alphabet, rune(s[i+1])))
		b.Write(w, 11)
	}

	if i < len(s) {
		w := uint(strings.IndexRune(alphabet, rune(s[i])))
		b.Write(w, 6)
	}
}

// String is the encoding for 8-bit data.  All bytes are valid.
type String string

func (s String) String() string {
	return fmt.Sprintf("String(%#q)", string(s))
}

func (s String) Check() error {
	return nil
}

var stringLen = [3]int{8, 16, 16}

func (s String) Bits(v Version) int {
	return 4 + stringLen[v.sizeClass()] + 8*len(s)
}

func (s String) Encode(b *Bits, v Version) {
	b.Write((uint)(4), 4)
	b.Write(uint(len(s)), stringLen[v.sizeClass()])
	for i := 0; i < len(s); i++ {
		b.Write(uint(s[i]), 8)
	}
}

// A Pixel describes a single pixel in a QR code.
type Pixel uint32

const (
	Black Pixel = 1 << iota
	Invert
)

func (p Pixel) Offset() uint {
	return uint(p >> 6)
}

func OffsetPixel(o uint) Pixel {
	return Pixel(o << 6)
}

func (r PixelRole) Pixel() Pixel {
	return Pixel(r << 2)
}

func (p Pixel) Role() PixelRole {
	return PixelRole(p>>2) & 15
}

func (p Pixel) String() string {
	s := p.Role().String()
	if p&Black != 0 {
		s += "+black"
	}
	if p&Invert != 0 {
		s += "+invert"
	}
	s += "+" + strconv.FormatUint(uint64(p.Offset()), 10)
	return s
}

// A PixelRole describes the role of a QR pixel.
type PixelRole uint32

const (
	_         PixelRole = iota
	Position            // position squares (large)
	Alignment           // alignment squares (small)
	Timing              // timing strip between position squares
	Format              // format metadata
	PVersion            // version pattern
	Unused              // unused pixel
	Data                // data bit
	Check               // error correction check bit
	Extra
)

var roles = []string{
	"",
	"position",
	"alignment",
	"timing",
	"format",
	"pversion",
	"unused",
	"data",
	"check",
	"extra",
}

func (r PixelRole) String() string {
	if Position <= r && r <= Check {
		return roles[r]
	}
	return strconv.Itoa(int(r))
}

// A Level represents a QR error correction level.
// From least to most tolerant of errors, they are L, M, Q, H.
type Level int

const (
	L Level = iota
	M
	Q
	H
)

func (l Level) String() string {
	if L <= l && l <= H {
		return "LMQH"[l : l+1]
	}
	return strconv.Itoa(int(l))
}

// A Code is a square pixel grid.
type Code struct {
	Bitmap []byte // 1 is black, 0 is white
	Size   int    // number of pixels on a side
	Stride int    // number of bytes per row
}

func (c *Code) Black(x, y int) bool {
	return 0 <= x && x < c.Size && 0 <= y && y < c.Size &&
			c.Bitmap[y*c.Stride+x/8]&(1<<uint(7-x&7)) != 0
}

// A Mask describes a mask that is applied to the QR
// code to avoid QR artifacts being interpreted as
// alignment and timing patterns (such as the squares
// in the corners).  Valid masks are integers from 0 to 7.
type Mask int

// http://www.swetake.com/qr/qr5_en.html
var mfunc = []func(int, int) bool{
	func(i, j int) bool { return (i+j)%2 == 0 },
	func(i, _ int) bool { return i%2 == 0 },
	func(_, j int) bool { return j%3 == 0 },
	func(i, j int) bool { return (i+j)%3 == 0 },
	func(i, j int) bool { return (i/2+j/3)%2 == 0 },
	func(i, j int) bool { return i*j%2+i*j%3 == 0 },
	func(i, j int) bool { return (i*j%2+i*j%3)%2 == 0 },
	func(i, j int) bool { return (i*j%3+(i+j)%2)%2 == 0 },
}

func (m Mask) Invert(y, x int) bool {
	if m < 0 {
		return false
	}
	return mfunc[m](y, x)
}

// A Plan describes how to construct a QR code
// with a specific version, level, and mask.
type Plan struct {
	Version Version
	Level   Level
	Mask    Mask

	DataBytes  int // number of data bytes
	CheckBytes int // number of error correcting (checksum) bytes
	Blocks     int // number of data blocks

	Pixel [][]Pixel // pixel map
}

// NewPlan returns a Plan for a QR code with the given
// version, level, and mask.
func NewPlan(version Version, level Level, mask Mask) (*Plan, error) {
	p, err := vplan(version)
	if err != nil {
		return nil, err
	}
	if err := fplan(level, mask, p); err != nil {
		return nil, err
	}
	if err := lplan(version, level, p); err != nil {
		return nil, err
	}
	if err := mplan(mask, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (b *Bits) Pad(n int) {
	if n < 0 {
		panic("qr: invalid pad size")
	}
	if n <= 4 {
		b.Write((uint)(0), n)
	} else {
		b.Write((uint)(0), 4)
		n -= 4
		n -= -b.Bits() & 7
		b.Write((uint)(0), -b.Bits()&7)
		pad := n / 8
		for i := 0; i < pad; i += 2 {
			b.Write((uint)(0xec), 8)
			if i+1 >= pad {
				break
			}
			b.Write((uint)(0x11), 8)
		}
	}
}

func (b *Bits) AddCheckBytes(v Version, l Level) {
	nd := v.DataBytes(l)
	if b.nbit < nd*8 {
		b.Pad(nd*8 - b.nbit)
	}
	if b.nbit != nd*8 {
		panic("qr: too much data")
	}

	dat := b.Bytes()
	vt := &vtab[v]
	lev := &vt.level[l]
	db := nd / lev.nblock
	extra := nd % lev.nblock
	chk := make([]byte, lev.check)
	rs := gf256.NewRSEncoder(Field, lev.check)
	for i := 0; i < lev.nblock; i++ {
		if i == lev.nblock-extra {
			db++
		}
		rs.ECC(dat[:db], chk)
		b.Append(chk)
		dat = dat[db:]
	}

	if len(b.Bytes()) != vt.bytes {
		panic("qr: internal error")
	}
}

func (p *Plan) Encode(text ...Encoding) (*Code, error) {
	var b Bits
	for _, t := range text {
		if err := t.Check(); err != nil {
			return nil, err
		}
		t.Encode(&b, p.Version)
	}
	if b.Bits() > p.DataBytes*8 {
		return nil, fmt.Errorf("cannot encode %d bits into %d-bit code", b.Bits(), p.DataBytes*8)
	}
	b.AddCheckBytes(p.Version, p.Level)
	bytes := b.Bytes()

	// Now we have the checksum bytes and the data bytes.
	// Construct the actual code.
	c := &Code{Size: len(p.Pixel), Stride: (len(p.Pixel) + 7) &^ 7}
	c.Bitmap = make([]byte, c.Stride*c.Size)
	crow := c.Bitmap
	for _, row := range p.Pixel {
		for x, pix := range row {
			switch pix.Role() {
			case Data, Check:
				o := pix.Offset()
				if bytes[o/8]&(1<<uint(7-o&7)) != 0 {
					pix ^= Black
				}
			}
			if pix&Black != 0 {
				crow[x/8] |= 1 << uint(7-x&7)
			}
		}
		crow = crow[c.Stride:]
	}
	return c, nil
}

// A version describes metadata associated with a version.
type version struct {
	apos    int
	astride int
	bytes   int
	pattern int
	level   [4]level
}

type level struct {
	nblock int
	check  int
}

var vtab = []version{
	{},
	{100, 100, 26, 0x0, [4]level{{1, 7}, {1, 10}, {1, 13}, {1, 17}}},          // 1
	{16, 100, 44, 0x0, [4]level{{1, 10}, {1, 16}, {1, 22}, {1, 28}}},          // 2
	{20, 100, 70, 0x0, [4]level{{1, 15}, {1, 26}, {2, 18}, {2, 22}}},          // 3
	{24, 100, 100, 0x0, [4]level{{1, 20}, {2, 18}, {2, 26}, {4, 16}}},         // 4
	{28, 100, 134, 0x0, [4]level{{1, 26}, {2, 24}, {4, 18}, {4, 22}}},         // 5
	{32, 100, 172, 0x0, [4]level{{2, 18}, {4, 16}, {4, 24}, {4, 28}}},         // 6
	{20, 16, 196, 0x7c94, [4]level{{2, 20}, {4, 18}, {6, 18}, {5, 26}}},       // 7
	{22, 18, 242, 0x85bc, [4]level{{2, 24}, {4, 22}, {6, 22}, {6, 26}}},       // 8
	{24, 20, 292, 0x9a99, [4]level{{2, 30}, {5, 22}, {8, 20}, {8, 24}}},       // 9
	{26, 22, 346, 0xa4d3, [4]level{{4, 18}, {5, 26}, {8, 24}, {8, 28}}},       // 10
	{28, 24, 404, 0xbbf6, [4]level{{4, 20}, {5, 30}, {8, 28}, {11, 24}}},      // 11
	{30, 26, 466, 0xc762, [4]level{{4, 24}, {8, 22}, {10, 26}, {11, 28}}},     // 12
	{32, 28, 532, 0xd847, [4]level{{4, 26}, {9, 22}, {12, 24}, {16, 22}}},     // 13
	{24, 20, 581, 0xe60d, [4]level{{4, 30}, {9, 24}, {16, 20}, {16, 24}}},     // 14
	{24, 22, 655, 0xf928, [4]level{{6, 22}, {10, 24}, {12, 30}, {18, 24}}},    // 15
	{24, 24, 733, 0x10b78, [4]level{{6, 24}, {10, 28}, {17, 24}, {16, 30}}},   // 16
	{28, 24, 815, 0x1145d, [4]level{{6, 28}, {11, 28}, {16, 28}, {19, 28}}},   // 17
	{28, 26, 901, 0x12a17, [4]level{{6, 30}, {13, 26}, {18, 28}, {21, 28}}},   // 18
	{28, 28, 991, 0x13532, [4]level{{7, 28}, {14, 26}, {21, 26}, {25, 26}}},   // 19
	{32, 28, 1085, 0x149a6, [4]level{{8, 28}, {16, 26}, {20, 30}, {25, 28}}},  // 20
	{26, 22, 1156, 0x15683, [4]level{{8, 28}, {17, 26}, {23, 28}, {25, 30}}},  // 21
	{24, 24, 1258, 0x168c9, [4]level{{9, 28}, {17, 28}, {23, 30}, {34, 24}}},  // 22
	{28, 24, 1364, 0x177ec, [4]level{{9, 30}, {18, 28}, {25, 30}, {30, 30}}},  // 23
	{26, 26, 1474, 0x18ec4, [4]level{{10, 30}, {20, 28}, {27, 30}, {32, 30}}}, // 24
	{30, 26, 1588, 0x191e1, [4]level{{12, 26}, {21, 28}, {29, 30}, {35, 30}}}, // 25
	{28, 28, 1706, 0x1afab, [4]level{{12, 28}, {23, 28}, {34, 28}, {37, 30}}}, // 26
	{32, 28, 1828, 0x1b08e, [4]level{{12, 30}, {25, 28}, {34, 30}, {40, 30}}}, // 27
	{24, 24, 1921, 0x1cc1a, [4]level{{13, 30}, {26, 28}, {35, 30}, {42, 30}}}, // 28
	{28, 24, 2051, 0x1d33f, [4]level{{14, 30}, {28, 28}, {38, 30}, {45, 30}}}, // 29
	{24, 26, 2185, 0x1ed75, [4]level{{15, 30}, {29, 28}, {40, 30}, {48, 30}}}, // 30
	{28, 26, 2323, 0x1f250, [4]level{{16, 30}, {31, 28}, {43, 30}, {51, 30}}}, // 31
	{32, 26, 2465, 0x209d5, [4]level{{17, 30}, {33, 28}, {45, 30}, {54, 30}}}, // 32
	{28, 28, 2611, 0x216f0, [4]level{{18, 30}, {35, 28}, {48, 30}, {57, 30}}}, // 33
	{32, 28, 2761, 0x228ba, [4]level{{19, 30}, {37, 28}, {51, 30}, {60, 30}}}, // 34
	{28, 24, 2876, 0x2379f, [4]level{{19, 30}, {38, 28}, {53, 30}, {63, 30}}}, // 35
	{22, 26, 3034, 0x24b0b, [4]level{{20, 30}, {40, 28}, {56, 30}, {66, 30}}}, // 36
	{26, 26, 3196, 0x2542e, [4]level{{21, 30}, {43, 28}, {59, 30}, {70, 30}}}, // 37
	{30, 26, 3362, 0x26a64, [4]level{{22, 30}, {45, 28}, {62, 30}, {74, 30}}}, // 38
	{24, 28, 3532, 0x27541, [4]level{{24, 30}, {47, 28}, {65, 30}, {77, 30}}}, // 39
	{28, 28, 3706, 0x28c69, [4]level{{25, 30}, {49, 28}, {68, 30}, {81, 30}}}, // 40
}

func grid(siz int) [][]Pixel {
	m := make([][]Pixel, siz)
	pix := make([]Pixel, siz*siz)
	for i := range m {
		m[i], pix = pix[:siz], pix[siz:]
	}
	return m
}

// vplan creates a Plan for the given version.
func vplan(v Version) (*Plan, error) {
	p := &Plan{Version: v}
	if v < 1 || v > 40 {
		return nil, fmt.Errorf("invalid QR version %d", int(v))
	}
	siz := 17 + int(v)*4
	m := grid(siz)
	p.Pixel = m

	// Timing markers (overwritten by boxes).
	const ti = 6 // timing is in row/column 6 (counting from 0)
	for i := range m {
		p := Timing.Pixel()
		if i&1 == 0 {
			p |= Black
		}
		m[i][ti] = p
		m[ti][i] = p
	}

	// Position boxes.
	posBox(m, 0, 0)
	posBox(m, siz-7, 0)
	posBox(m, 0, siz-7)

	// Alignment boxes.
	info := &vtab[v]
	for x := 4; x+5 < siz; {
		for y := 4; y+5 < siz; {
			// don't overwrite timing markers
			if (x < 7 && y < 7) || (x < 7 && y+5 >= siz-7) || (x+5 >= siz-7 && y < 7) {
			} else {
				alignBox(m, x, y)
			}
			if y == 4 {
				y = info.apos
			} else {
				y += info.astride
			}
		}
		if x == 4 {
			x = info.apos
		} else {
			x += info.astride
		}
	}

	// Version pattern.
	pat := vtab[v].pattern
	if pat != 0 {
		v := pat
		for x := 0; x < 6; x++ {
			for y := 0; y < 3; y++ {
				p := PVersion.Pixel()
				if v&1 != 0 {
					p |= Black
				}
				m[siz-11+y][x] = p
				m[x][siz-11+y] = p
				v >>= 1
			}
		}
	}

	// One lonely black pixel
	m[siz-8][8] = Unused.Pixel() | Black

	return p, nil
}

// fplan adds the format pixels
func fplan(l Level, m Mask, p *Plan) error {
	// Format pixels.
	fb := uint32(l^1) << 13 // level: L=01, M=00, Q=11, H=10
	fb |= uint32(m) << 10   // mask
	const formatPoly = 0x537
	rem := fb
	for i := 14; i >= 10; i-- {
		if rem&(1<<uint(i)) != 0 {
			rem ^= formatPoly << uint(i-10)
		}
	}
	fb |= rem
	invert := uint32(0x5412)
	siz := len(p.Pixel)
	for i := uint(0); i < 15; i++ {
		pix := Format.Pixel() + OffsetPixel(i)
		if (fb>>i)&1 == 1 {
			pix |= Black
		}
		if (invert>>i)&1 == 1 {
			pix ^= Invert | Black
		}
		// top left
		switch {
		case i < 6:
			p.Pixel[i][8] = pix
		case i < 8:
			p.Pixel[i+1][8] = pix
		case i < 9:
			p.Pixel[8][7] = pix
		default:
			p.Pixel[8][14-i] = pix
		}
		// bottom right
		switch {
		case i < 8:
			p.Pixel[8][siz-1-int(i)] = pix
		default:
			p.Pixel[siz-1-int(14-i)][8] = pix
		}
	}
	return nil
}

// lplan edits a version-only Plan to add information
// about the error correction levels.
func lplan(v Version, l Level, p *Plan) error {
	p.Level = l

	nblock := vtab[v].level[l].nblock
	ne := vtab[v].level[l].check
	nde := (vtab[v].bytes - ne*nblock) / nblock
	extra := (vtab[v].bytes - ne*nblock) % nblock
	dataBits := (nde*nblock + extra) * 8
	checkBits := ne * nblock * 8

	p.DataBytes = vtab[v].bytes - ne*nblock
	p.CheckBytes = ne * nblock
	p.Blocks = nblock

	// Make data + checksum pixels.
	data := make([]Pixel, dataBits)
	for i := range data {
		data[i] = Data.Pixel() | OffsetPixel(uint(i))
	}
	check := make([]Pixel, checkBits)
	for i := range check {
		check[i] = Check.Pixel() | OffsetPixel(uint(i+dataBits))
	}

	// Split into blocks.
	dataList := make([][]Pixel, nblock)
	checkList := make([][]Pixel, nblock)
	for i := 0; i < nblock; i++ {
		// The last few blocks have an extra data byte (8 pixels).
		nd := nde
		if i >= nblock-extra {
			nd++
		}
		dataList[i], data = data[0:nd*8], data[nd*8:]
		checkList[i], check = check[0:ne*8], check[ne*8:]
	}
	if len(data) != 0 || len(check) != 0 {
		panic("data/check math")
	}

	// Build up bit sequence, taking first byte of each block,
	// then second byte, and so on.  Then checksums.
	bits := make([]Pixel, dataBits+checkBits)
	dst := bits
	for i := 0; i < nde+1; i++ {
		for _, b := range dataList {
			if i*8 < len(b) {
				copy(dst, b[i*8:(i+1)*8])
				dst = dst[8:]
			}
		}
	}
	for i := 0; i < ne; i++ {
		for _, b := range checkList {
			if i*8 < len(b) {
				copy(dst, b[i*8:(i+1)*8])
				dst = dst[8:]
			}
		}
	}
	if len(dst) != 0 {
		panic("dst math")
	}

	// Sweep up pair of columns,
	// then down, assigning to right then left pixel.
	// Repeat.
	// See Figure 2 of http://www.pclviewer.com/rs2/qrtopology.htm
	siz := len(p.Pixel)
	rem := make([]Pixel, 7)
	for i := range rem {
		rem[i] = Extra.Pixel()
	}
	src := append(bits, rem...)
	for x := siz; x > 0; {
		for y := siz - 1; y >= 0; y-- {
			if p.Pixel[y][x-1].Role() == 0 {
				p.Pixel[y][x-1], src = src[0], src[1:]
			}
			if p.Pixel[y][x-2].Role() == 0 {
				p.Pixel[y][x-2], src = src[0], src[1:]
			}
		}
		x -= 2
		if x == 7 { // vertical timing strip
			x--
		}
		for y := 0; y < siz; y++ {
			if p.Pixel[y][x-1].Role() == 0 {
				p.Pixel[y][x-1], src = src[0], src[1:]
			}
			if p.Pixel[y][x-2].Role() == 0 {
				p.Pixel[y][x-2], src = src[0], src[1:]
			}
		}
		x -= 2
	}
	return nil
}

// mplan edits a version+level-only Plan to add the mask.
func mplan(m Mask, p *Plan) error {
	p.Mask = m
	for y, row := range p.Pixel {
		for x, pix := range row {
			if r := pix.Role(); (r == Data || r == Check || r == Extra) && p.Mask.Invert(y, x) {
				row[x] ^= Black | Invert
			}
		}
	}
	return nil
}

// posBox draws a position (large) box at upper left x, y.
func posBox(m [][]Pixel, x, y int) {
	pos := Position.Pixel()
	// box
	for dy := 0; dy < 7; dy++ {
		for dx := 0; dx < 7; dx++ {
			p := pos
			if dx == 0 || dx == 6 || dy == 0 || dy == 6 || 2 <= dx && dx <= 4 && 2 <= dy && dy <= 4 {
				p |= Black
			}
			m[y+dy][x+dx] = p
		}
	}
	// white border
	for dy := -1; dy < 8; dy++ {
		if 0 <= y+dy && y+dy < len(m) {
			if x > 0 {
				m[y+dy][x-1] = pos
			}
			if x+7 < len(m) {
				m[y+dy][x+7] = pos
			}
		}
	}
	for dx := -1; dx < 8; dx++ {
		if 0 <= x+dx && x+dx < len(m) {
			if y > 0 {
				m[y-1][x+dx] = pos
			}
			if y+7 < len(m) {
				m[y+7][x+dx] = pos
			}
		}
	}
}

// alignBox draw an alignment (small) box at upper left x, y.
func alignBox(m [][]Pixel, x, y int) {
	// box
	align := Alignment.Pixel()
	for dy := 0; dy < 5; dy++ {
		for dx := 0; dx < 5; dx++ {
			p := align
			if dx == 0 || dx == 4 || dy == 0 || dy == 4 || dx == 2 && dy == 2 {
				p |= Black
			}
			m[y+dy][x+dx] = p
		}
	}
}

