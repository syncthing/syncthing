// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

var errCorruptManifest = errors.New("leveldb: corrupt manifest")

type byteReader interface {
	io.Reader
	io.ByteReader
}

// These numbers are written to disk and should not be changed.
const (
	recComparer          = 1
	recJournalNum        = 2
	recNextNum           = 3
	recSeq               = 4
	recCompactionPointer = 5
	recDeletedTable      = 6
	recNewTable          = 7
	// 8 was used for large value refs
	recPrevJournalNum = 9
)

type cpRecord struct {
	level int
	ikey  iKey
}

type ntRecord struct {
	level int
	num   uint64
	size  uint64
	imin  iKey
	imax  iKey
}

func (r ntRecord) makeFile(s *session) *tFile {
	return newTableFile(s.getTableFile(r.num), r.size, r.imin, r.imax)
}

type dtRecord struct {
	level int
	num   uint64
}

type sessionRecord struct {
	hasRec             int
	comparer           string
	journalNum         uint64
	prevJournalNum     uint64
	nextNum            uint64
	seq                uint64
	compactionPointers []cpRecord
	addedTables        []ntRecord
	deletedTables      []dtRecord
	scratch            [binary.MaxVarintLen64]byte
	err                error
}

func (p *sessionRecord) has(rec int) bool {
	return p.hasRec&(1<<uint(rec)) != 0
}

func (p *sessionRecord) setComparer(name string) {
	p.hasRec |= 1 << recComparer
	p.comparer = name
}

func (p *sessionRecord) setJournalNum(num uint64) {
	p.hasRec |= 1 << recJournalNum
	p.journalNum = num
}

func (p *sessionRecord) setPrevJournalNum(num uint64) {
	p.hasRec |= 1 << recPrevJournalNum
	p.prevJournalNum = num
}

func (p *sessionRecord) setNextNum(num uint64) {
	p.hasRec |= 1 << recNextNum
	p.nextNum = num
}

func (p *sessionRecord) setSeq(seq uint64) {
	p.hasRec |= 1 << recSeq
	p.seq = seq
}

func (p *sessionRecord) addCompactionPointer(level int, ikey iKey) {
	p.hasRec |= 1 << recCompactionPointer
	p.compactionPointers = append(p.compactionPointers, cpRecord{level, ikey})
}

func (p *sessionRecord) resetCompactionPointers() {
	p.hasRec &= ^(1 << recCompactionPointer)
	p.compactionPointers = p.compactionPointers[:0]
}

func (p *sessionRecord) addTable(level int, num, size uint64, imin, imax iKey) {
	p.hasRec |= 1 << recNewTable
	p.addedTables = append(p.addedTables, ntRecord{level, num, size, imin, imax})
}

func (p *sessionRecord) addTableFile(level int, t *tFile) {
	p.addTable(level, t.file.Num(), t.size, t.imin, t.imax)
}

func (p *sessionRecord) resetAddedTables() {
	p.hasRec &= ^(1 << recNewTable)
	p.addedTables = p.addedTables[:0]
}

func (p *sessionRecord) deleteTable(level int, num uint64) {
	p.hasRec |= 1 << recDeletedTable
	p.deletedTables = append(p.deletedTables, dtRecord{level, num})
}

func (p *sessionRecord) resetDeletedTables() {
	p.hasRec &= ^(1 << recDeletedTable)
	p.deletedTables = p.deletedTables[:0]
}

func (p *sessionRecord) putUvarint(w io.Writer, x uint64) {
	if p.err != nil {
		return
	}
	n := binary.PutUvarint(p.scratch[:], x)
	_, p.err = w.Write(p.scratch[:n])
}

func (p *sessionRecord) putBytes(w io.Writer, x []byte) {
	if p.err != nil {
		return
	}
	p.putUvarint(w, uint64(len(x)))
	if p.err != nil {
		return
	}
	_, p.err = w.Write(x)
}

func (p *sessionRecord) encode(w io.Writer) error {
	p.err = nil
	if p.has(recComparer) {
		p.putUvarint(w, recComparer)
		p.putBytes(w, []byte(p.comparer))
	}
	if p.has(recJournalNum) {
		p.putUvarint(w, recJournalNum)
		p.putUvarint(w, p.journalNum)
	}
	if p.has(recNextNum) {
		p.putUvarint(w, recNextNum)
		p.putUvarint(w, p.nextNum)
	}
	if p.has(recSeq) {
		p.putUvarint(w, recSeq)
		p.putUvarint(w, p.seq)
	}
	for _, r := range p.compactionPointers {
		p.putUvarint(w, recCompactionPointer)
		p.putUvarint(w, uint64(r.level))
		p.putBytes(w, r.ikey)
	}
	for _, r := range p.deletedTables {
		p.putUvarint(w, recDeletedTable)
		p.putUvarint(w, uint64(r.level))
		p.putUvarint(w, r.num)
	}
	for _, r := range p.addedTables {
		p.putUvarint(w, recNewTable)
		p.putUvarint(w, uint64(r.level))
		p.putUvarint(w, r.num)
		p.putUvarint(w, r.size)
		p.putBytes(w, r.imin)
		p.putBytes(w, r.imax)
	}
	return p.err
}

func (p *sessionRecord) readUvarint(r io.ByteReader) uint64 {
	if p.err != nil {
		return 0
	}
	x, err := binary.ReadUvarint(r)
	if err != nil {
		if err == io.EOF {
			p.err = errCorruptManifest
		} else {
			p.err = err
		}
		return 0
	}
	return x
}

func (p *sessionRecord) readBytes(r byteReader) []byte {
	if p.err != nil {
		return nil
	}
	n := p.readUvarint(r)
	if p.err != nil {
		return nil
	}
	x := make([]byte, n)
	_, p.err = io.ReadFull(r, x)
	if p.err != nil {
		if p.err == io.EOF {
			p.err = errCorruptManifest
		}
		return nil
	}
	return x
}

func (p *sessionRecord) readLevel(r io.ByteReader) int {
	if p.err != nil {
		return 0
	}
	x := p.readUvarint(r)
	if p.err != nil {
		return 0
	}
	if x >= kNumLevels {
		p.err = errCorruptManifest
		return 0
	}
	return int(x)
}

func (p *sessionRecord) decode(r io.Reader) error {
	br, ok := r.(byteReader)
	if !ok {
		br = bufio.NewReader(r)
	}
	p.err = nil
	for p.err == nil {
		rec, err := binary.ReadUvarint(br)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
		switch rec {
		case recComparer:
			x := p.readBytes(br)
			if p.err == nil {
				p.setComparer(string(x))
			}
		case recJournalNum:
			x := p.readUvarint(br)
			if p.err == nil {
				p.setJournalNum(x)
			}
		case recPrevJournalNum:
			x := p.readUvarint(br)
			if p.err == nil {
				p.setPrevJournalNum(x)
			}
		case recNextNum:
			x := p.readUvarint(br)
			if p.err == nil {
				p.setNextNum(x)
			}
		case recSeq:
			x := p.readUvarint(br)
			if p.err == nil {
				p.setSeq(x)
			}
		case recCompactionPointer:
			level := p.readLevel(br)
			ikey := p.readBytes(br)
			if p.err == nil {
				p.addCompactionPointer(level, iKey(ikey))
			}
		case recNewTable:
			level := p.readLevel(br)
			num := p.readUvarint(br)
			size := p.readUvarint(br)
			imin := p.readBytes(br)
			imax := p.readBytes(br)
			if p.err == nil {
				p.addTable(level, num, size, imin, imax)
			}
		case recDeletedTable:
			level := p.readLevel(br)
			num := p.readUvarint(br)
			if p.err == nil {
				p.deleteTable(level, num)
			}
		}
	}

	return p.err
}
