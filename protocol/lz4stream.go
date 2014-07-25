package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	lz4 "github.com/bkaradzic/go-lz4"
)

const lz4Magic = 0x5e63b278

type lz4Writer struct {
	wr  io.Writer
	mut sync.Mutex
	buf []byte
}

func newLZ4Writer(w io.Writer) *lz4Writer {
	return &lz4Writer{wr: w}
}

func (w *lz4Writer) Write(bs []byte) (int, error) {
	w.mut.Lock()
	defer w.mut.Unlock()

	var err error
	w.buf, err = lz4.Encode(w.buf[:cap(w.buf)], bs)
	if err != nil {
		return 0, err
	}

	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:], lz4Magic)
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(w.buf)))
	_, err = w.wr.Write(hdr[:])
	if err != nil {
		return 0, err
	}

	_, err = w.wr.Write(w.buf)
	if err != nil {
		return 0, err
	}

	if debug {
		l.Debugf("lz4 write; %d / %d bytes", len(bs), 8+len(w.buf))
	}
	return len(bs), nil
}

type lz4Reader struct {
	rd     io.Reader
	mut    sync.Mutex
	buf    []byte
	ebuf   []byte
	obuf   *bytes.Buffer
	ibytes uint64
	obytes uint64
}

func newLZ4Reader(r io.Reader) *lz4Reader {
	return &lz4Reader{rd: r}
}

func (r *lz4Reader) Read(bs []byte) (int, error) {
	r.mut.Lock()
	defer r.mut.Unlock()

	if r.obuf == nil {
		r.obuf = bytes.NewBuffer(nil)
	}

	if r.obuf.Len() == 0 {
		if err := r.moreBits(); err != nil {
			return 0, err
		}
	}

	n, err := r.obuf.Read(bs)
	if debug {
		l.Debugf("lz4 read; %d bytes", n)
	}
	return n, err
}

func (r *lz4Reader) moreBits() error {
	var hdr [8]byte
	_, err := io.ReadFull(r.rd, hdr[:])
	if binary.BigEndian.Uint32(hdr[0:]) != lz4Magic {
		return errors.New("bad magic")
	}

	ln := int(binary.BigEndian.Uint32(hdr[4:]))
	if len(r.buf) < ln {
		r.buf = make([]byte, int(ln))
	} else {
		r.buf = r.buf[:ln]
	}

	_, err = io.ReadFull(r.rd, r.buf)
	if err != nil {
		return err
	}

	r.ebuf, err = lz4.Decode(r.ebuf[:cap(r.ebuf)], r.buf)
	if err != nil {
		return err
	}

	if debug {
		l.Debugf("lz4 moreBits: %d / %d bytes", ln+8, len(r.ebuf))
	}

	_, err = r.obuf.Write(r.ebuf)
	return err
}
