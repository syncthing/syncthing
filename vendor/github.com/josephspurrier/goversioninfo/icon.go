package goversioninfo

import (
	"encoding/binary"
	"io"
	"os"

	"github.com/akavel/rsrc/coff"
	"github.com/akavel/rsrc/ico"
)

// *****************************************************************************
/*
Code from https://github.com/akavel/rsrc

The MIT License (MIT)

Copyright (c) 2013-2014 The rsrc Authors.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
// *****************************************************************************

const (
	rtIcon      = coff.RT_ICON
	rtGroupIcon = coff.RT_GROUP_ICON
	rtManifest  = coff.RT_MANIFEST
)

// on storing icons, see: http://blogs.msdn.com/b/oldnewthing/archive/2012/07/20/10331787.aspx
type gRPICONDIR struct {
	ico.ICONDIR
	Entries []gRPICONDIRENTRY
}

func (group gRPICONDIR) Size() int64 {
	return int64(binary.Size(group.ICONDIR) + len(group.Entries)*binary.Size(group.Entries[0]))
}

type gRPICONDIRENTRY struct {
	ico.IconDirEntryCommon
	ID uint16
}

func addIcon(coff *coff.Coff, fname string, newID <-chan uint16) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	//defer f.Close() don't defer, files will be closed by OS when app closes

	icons, err := ico.DecodeHeaders(f)
	if err != nil {
		return err
	}

	if len(icons) > 0 {
		// RT_ICONs
		group := gRPICONDIR{ICONDIR: ico.ICONDIR{
			Reserved: 0, // magic num.
			Type:     1, // magic num.
			Count:    uint16(len(icons)),
		}}
		gid := <-newID
		for _, icon := range icons {
			id := <-newID
			r := io.NewSectionReader(f, int64(icon.ImageOffset), int64(icon.BytesInRes))
			coff.AddResource(rtIcon, id, r)
			group.Entries = append(group.Entries, gRPICONDIRENTRY{IconDirEntryCommon: icon.IconDirEntryCommon, ID: id})
		}
		coff.AddResource(rtGroupIcon, gid, group)
	}

	return nil
}
