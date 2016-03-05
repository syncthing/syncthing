package qr

import (
	"image"
	"image/color"
)

// average convert the sums to averages and returns the result.
func average(sum []uint64, w, h int, n uint64) *image.RGBA {
	ret := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			index := 4 * (y*w + x)
			pix := ret.Pix[y*ret.Stride+x*4:]
			pix[0] = uint8(sum[index+0] / n)
			pix[1] = uint8(sum[index+1] / n)
			pix[2] = uint8(sum[index+2] / n)
			pix[3] = uint8(sum[index+3] / n)
		}
	}
	return ret
}

// ResizeRGBA returns a scaled copy of the RGBA image slice r of m.
// The returned image has width w and height h.
func ResizeRGBA(m *image.RGBA, r image.Rectangle, w, h int) *image.RGBA {
	ww, hh := uint64(w), uint64(h)
	dx, dy := uint64(r.Dx()), uint64(r.Dy())
	// See comment in Resize.
	n, sum := dx*dy, make([]uint64, 4*w*h)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		pix := m.Pix[(y-r.Min.Y)*m.Stride:]
		for x := r.Min.X; x < r.Max.X; x++ {
			// Get the source pixel.
			p := pix[(x-r.Min.X)*4:]
			r64 := uint64(p[0])
			g64 := uint64(p[1])
			b64 := uint64(p[2])
			a64 := uint64(p[3])
			// Spread the source pixel over 1 or more destination rows.
			py := uint64(y) * hh
			for remy := hh; remy > 0; {
				qy := dy - (py % dy)
				if qy > remy {
					qy = remy
				}
				// Spread the source pixel over 1 or more destination columns.
				px := uint64(x) * ww
				index := 4 * ((py/dy)*ww + (px / dx))
				for remx := ww; remx > 0; {
					qx := dx - (px % dx)
					if qx > remx {
						qx = remx
					}
					qxy := qx * qy
					sum[index+0] += r64 * qxy
					sum[index+1] += g64 * qxy
					sum[index+2] += b64 * qxy
					sum[index+3] += a64 * qxy
					index += 4
					px += qx
					remx -= qx
				}
				py += qy
				remy -= qy
			}
		}
	}
	return average(sum, w, h, (uint64)(n))
}

// ResizeNRGBA returns a scaled copy of the RGBA image slice r of m.
// The returned image has width w and height h.
func ResizeNRGBA(m *image.NRGBA, r image.Rectangle, w, h int) *image.RGBA {
	ww, hh := uint64(w), uint64(h)
	dx, dy := uint64(r.Dx()), uint64(r.Dy())
	// See comment in Resize.
	n, sum := dx*dy, make([]uint64, 4*w*h)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		pix := m.Pix[(y-r.Min.Y)*m.Stride:]
		for x := r.Min.X; x < r.Max.X; x++ {
			// Get the source pixel.
			p := pix[(x-r.Min.X)*4:]
			r64 := uint64(p[0])
			g64 := uint64(p[1])
			b64 := uint64(p[2])
			a64 := uint64(p[3])
			r64 = (r64 * a64) / 255
			g64 = (g64 * a64) / 255
			b64 = (b64 * a64) / 255
			// Spread the source pixel over 1 or more destination rows.
			py := uint64(y) * hh
			for remy := hh; remy > 0; {
				qy := dy - (py % dy)
				if qy > remy {
					qy = remy
				}
				// Spread the source pixel over 1 or more destination columns.
				px := uint64(x) * ww
				index := 4 * ((py/dy)*ww + (px / dx))
				for remx := ww; remx > 0; {
					qx := dx - (px % dx)
					if qx > remx {
						qx = remx
					}
					qxy := qx * qy
					sum[index+0] += r64 * qxy
					sum[index+1] += g64 * qxy
					sum[index+2] += b64 * qxy
					sum[index+3] += a64 * qxy
					index += 4
					px += qx
					remx -= qx
				}
				py += qy
				remy -= qy
			}
		}
	}
	return average(sum, w, h, (uint64)(n))
}

// Resample returns a resampled copy of the image slice r of m.
// The returned image has width w and height h.
func Resample(m image.Image, r image.Rectangle, w, h int) *image.RGBA {
	if w < 0 || h < 0 {
		return nil
	}
	if w == 0 || h == 0 || r.Dx() <= 0 || r.Dy() <= 0 {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	curw, curh := r.Dx(), r.Dy()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Get a source pixel.
			subx := x * curw / w
			suby := y * curh / h
			r32, g32, b32, a32 := m.At(subx, suby).RGBA()
			r := uint8(r32 >> 8)
			g := uint8(g32 >> 8)
			b := uint8(b32 >> 8)
			a := uint8(a32 >> 8)
			img.SetRGBA(x, y, color.RGBA{r, g, b, a})
		}
	}
	return img
}
