/*
 * Copyright 2011-2012 Branimir Karadzic. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without modification,
 * are permitted provided that the following conditions are met:
 *
 *    1. Redistributions of source code must retain the above copyright notice, this
 *       list of conditions and the following disclaimer.
 *
 *    2. Redistributions in binary form must reproduce the above copyright notice,
 *       this list of conditions and the following disclaimer in the documentation
 *       and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY COPYRIGHT HOLDER ``AS IS'' AND ANY EXPRESS OR
 * IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT
 * SHALL COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT,
 * INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
 * LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
 * PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
 * WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE
 * OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF
 * THE POSSIBILITY OF SUCH DAMAGE.
 */

package lz4

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	// ErrCorrupt indicates the input was corrupt
	ErrCorrupt = errors.New("corrupt input")
)

const (
	mlBits  = 4
	mlMask  = (1 << mlBits) - 1
	runBits = 8 - mlBits
	runMask = (1 << runBits) - 1
)

type decoder struct {
	src  []byte
	dst  []byte
	spos uint32
	dpos uint32
	ref  uint32
}

func (d *decoder) readByte() (uint8, error) {
	if int(d.spos) == len(d.src) {
		return 0, io.EOF
	}
	b := d.src[d.spos]
	d.spos++
	return b, nil
}

func (d *decoder) getLen() (uint32, error) {

	length := uint32(0)
	ln, err := d.readByte()
	if err != nil {
		return 0, ErrCorrupt
	}
	for ln == 255 {
		length += 255
		ln, err = d.readByte()
		if err != nil {
			return 0, ErrCorrupt
		}
	}
	length += uint32(ln)

	return length, nil
}

func (d *decoder) cp(length, decr uint32) {

	if int(d.ref+length) < int(d.dpos) {
		copy(d.dst[d.dpos:], d.dst[d.ref:d.ref+length])
	} else {
		for ii := uint32(0); ii < length; ii++ {
			d.dst[d.dpos+ii] = d.dst[d.ref+ii]
		}
	}
	d.dpos += length
	d.ref += length - decr
}

func (d *decoder) finish(err error) error {
	if err == io.EOF {
		return nil
	}

	return err
}

// Decode returns the decoded form of src.  The returned slice may be a
// subslice of dst if it was large enough to hold the entire decoded block.
func Decode(dst, src []byte) ([]byte, error) {

	if len(src) < 4 {
		return nil, ErrCorrupt
	}

	uncompressedLen := binary.LittleEndian.Uint32(src)

	if uncompressedLen == 0 {
		return nil, nil
	}

	if uncompressedLen > MaxInputSize {
		return nil, ErrTooLarge
	}

	if dst == nil || len(dst) < int(uncompressedLen) {
		dst = make([]byte, uncompressedLen)
	}

	d := decoder{src: src, dst: dst[:uncompressedLen], spos: 4}

	decr := []uint32{0, 3, 2, 3}

	for {
		code, err := d.readByte()
		if err != nil {
			return d.dst, d.finish(err)
		}

		length := uint32(code >> mlBits)
		if length == runMask {
			ln, err := d.getLen()
			if err != nil {
				return nil, ErrCorrupt
			}
			length += ln
		}

		if int(d.spos+length) > len(d.src) || int(d.dpos+length) > len(d.dst) {
			return nil, ErrCorrupt
		}

		for ii := uint32(0); ii < length; ii++ {
			d.dst[d.dpos+ii] = d.src[d.spos+ii]
		}

		d.spos += length
		d.dpos += length

		if int(d.spos) == len(d.src) {
			return d.dst, nil
		}

		if int(d.spos+2) >= len(d.src) {
			return nil, ErrCorrupt
		}

		back := uint32(d.src[d.spos]) | uint32(d.src[d.spos+1])<<8

		if back > d.dpos {
			return nil, ErrCorrupt
		}

		d.spos += 2
		d.ref = d.dpos - back

		length = uint32(code & mlMask)
		if length == mlMask {
			ln, err := d.getLen()
			if err != nil {
				return nil, ErrCorrupt
			}
			length += ln
		}

		literal := d.dpos - d.ref

		if literal < 4 {
			if int(d.dpos+4) > len(d.dst) {
				return nil, ErrCorrupt
			}

			d.cp(4, decr[literal])
		} else {
			length += 4
		}

		if d.dpos+length > uncompressedLen {
			return nil, ErrCorrupt
		}

		d.cp(length, 0)
	}
}
