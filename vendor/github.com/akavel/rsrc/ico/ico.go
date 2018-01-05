// Package ico describes Windows ICO file format.
package ico

// ICO: http://msdn.microsoft.com/en-us/library/ms997538.aspx
// BMP/DIB: http://msdn.microsoft.com/en-us/library/windows/desktop/dd183562%28v=vs.85%29.aspx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"sort"
)

const (
	BI_RGB = 0
)

type ICONDIR struct {
	Reserved uint16 // must be 0
	Type     uint16 // Resource Type (1 for icons)
	Count    uint16 // How many images?
}

type IconDirEntryCommon struct {
	Width      byte   // Width, in pixels, of the image
	Height     byte   // Height, in pixels, of the image
	ColorCount byte   // Number of colors in image (0 if >=8bpp)
	Reserved   byte   // Reserved (must be 0)
	Planes     uint16 // Color Planes
	BitCount   uint16 // Bits per pixel
	BytesInRes uint32 // How many bytes in this resource?
}

type ICONDIRENTRY struct {
	IconDirEntryCommon
	ImageOffset uint32 // Where in the file is this image? [from beginning of file]
}

type BITMAPINFOHEADER struct {
	Size          uint32
	Width         int32
	Height        int32  // NOTE: "represents the combined height of the XOR and AND masks. Remember to divide this number by two before using it to perform calculations for either of the XOR or AND masks."
	Planes        uint16 // [BMP/DIB]: "is always 1"
	BitCount      uint16
	Compression   uint32 // for ico = 0
	SizeImage     uint32
	XPelsPerMeter int32  // for ico = 0
	YPelsPerMeter int32  // for ico = 0
	ClrUsed       uint32 // for ico = 0
	ClrImportant  uint32 // for ico = 0
}

type RGBQUAD struct {
	Blue     byte
	Green    byte
	Red      byte
	Reserved byte // must be 0
}

func skip(r io.Reader, n int64) error {
	_, err := io.CopyN(ioutil.Discard, r, n)
	return err
}

type icoOffset struct {
	n      int
	offset uint32
}

type rawico struct {
	icoinfo ICONDIRENTRY
	bmpinfo *BITMAPINFOHEADER
	idx     int
	data    []byte
}

type byOffsets []rawico

func (o byOffsets) Len() int           { return len(o) }
func (o byOffsets) Less(i, j int) bool { return o[i].icoinfo.ImageOffset < o[j].icoinfo.ImageOffset }
func (o byOffsets) Swap(i, j int) {
	tmp := o[i]
	o[i] = o[j]
	o[j] = tmp
}

type ICO struct {
	image.Image
}

func DecodeHeaders(r io.Reader) ([]ICONDIRENTRY, error) {
	var hdr ICONDIR
	err := binary.Read(r, binary.LittleEndian, &hdr)
	if err != nil {
		return nil, err
	}
	if hdr.Reserved != 0 || hdr.Type != 1 {
		return nil, fmt.Errorf("bad magic number")
	}

	entries := make([]ICONDIRENTRY, hdr.Count)
	for i := 0; i < len(entries); i++ {
		err = binary.Read(r, binary.LittleEndian, &entries[i])
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// NOTE: won't succeed on files with overlapping offsets
func unused_decodeAll(r io.Reader) ([]*ICO, error) {
	var hdr ICONDIR
	err := binary.Read(r, binary.LittleEndian, &hdr)
	if err != nil {
		return nil, err
	}
	if hdr.Reserved != 0 || hdr.Type != 1 {
		return nil, fmt.Errorf("bad magic number")
	}

	raws := make([]rawico, hdr.Count)
	for i := 0; i < len(raws); i++ {
		err = binary.Read(r, binary.LittleEndian, &raws[i].icoinfo)
		if err != nil {
			return nil, err
		}
		raws[i].idx = i
	}

	sort.Sort(byOffsets(raws))

	offset := uint32(binary.Size(&hdr) + len(raws)*binary.Size(ICONDIRENTRY{}))
	for i := 0; i < len(raws); i++ {
		err = skip(r, int64(raws[i].icoinfo.ImageOffset-offset))
		if err != nil {
			return nil, err
		}
		offset = raws[i].icoinfo.ImageOffset

		raws[i].bmpinfo = &BITMAPINFOHEADER{}
		err = binary.Read(r, binary.LittleEndian, raws[i].bmpinfo)
		if err != nil {
			return nil, err
		}

		err = skip(r, int64(raws[i].bmpinfo.Size-uint32(binary.Size(BITMAPINFOHEADER{}))))
		if err != nil {
			return nil, err
		}
		raws[i].data = make([]byte, raws[i].icoinfo.BytesInRes-raws[i].bmpinfo.Size)
		_, err = io.ReadFull(r, raws[i].data)
		if err != nil {
			return nil, err
		}
	}

	icos := make([]*ICO, len(raws))
	for i := 0; i < len(raws); i++ {
		fmt.Println(i)
		icos[raws[i].idx], err = decode(raws[i].bmpinfo, &raws[i].icoinfo, raws[i].data)
		if err != nil {
			return nil, err
		}
	}
	return icos, nil
}

func decode(info *BITMAPINFOHEADER, icoinfo *ICONDIRENTRY, data []byte) (*ICO, error) {
	if info.Compression != BI_RGB {
		return nil, fmt.Errorf("ICO compression not supported (got %d)", info.Compression)
	}

	//if info.ClrUsed!=0 {
	//	panic(info.ClrUsed)
	//}

	r := bytes.NewBuffer(data)

	bottomup := info.Height > 0
	if !bottomup {
		info.Height = -info.Height
	}

	switch info.BitCount {
	case 8:
		ncol := int(icoinfo.ColorCount)
		if ncol == 0 {
			ncol = 256
		}

		pal := make(color.Palette, ncol)
		for i := 0; i < ncol; i++ {
			var rgb RGBQUAD
			err := binary.Read(r, binary.LittleEndian, &rgb)
			if err != nil {
				return nil, err
			}
			pal[i] = color.NRGBA{R: rgb.Red, G: rgb.Green, B: rgb.Blue, A: 0xff} //FIXME: is Alpha ok 0xff?
		}
		fmt.Println(pal)

		fmt.Println(info.SizeImage, len(data)-binary.Size(RGBQUAD{})*len(pal), info.Width, info.Height)

	default:
		return nil, fmt.Errorf("unsupported ICO bit depth (BitCount) %d", info.BitCount)
	}

	return nil, nil
}
