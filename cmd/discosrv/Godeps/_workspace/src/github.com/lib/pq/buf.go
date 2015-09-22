package pq

import (
	"bytes"
	"encoding/binary"

	"github.com/lib/pq/oid"
)

type readBuf []byte

func (b *readBuf) int32() (n int) {
	n = int(int32(binary.BigEndian.Uint32(*b)))
	*b = (*b)[4:]
	return
}

func (b *readBuf) oid() (n oid.Oid) {
	n = oid.Oid(binary.BigEndian.Uint32(*b))
	*b = (*b)[4:]
	return
}

func (b *readBuf) int16() (n int) {
	n = int(binary.BigEndian.Uint16(*b))
	*b = (*b)[2:]
	return
}

func (b *readBuf) string() string {
	i := bytes.IndexByte(*b, 0)
	if i < 0 {
		errorf("invalid message format; expected string terminator")
	}
	s := (*b)[:i]
	*b = (*b)[i+1:]
	return string(s)
}

func (b *readBuf) next(n int) (v []byte) {
	v = (*b)[:n]
	*b = (*b)[n:]
	return
}

func (b *readBuf) byte() byte {
	return b.next(1)[0]
}

type writeBuf []byte

func (b *writeBuf) int32(n int) {
	x := make([]byte, 4)
	binary.BigEndian.PutUint32(x, uint32(n))
	*b = append(*b, x...)
}

func (b *writeBuf) int16(n int) {
	x := make([]byte, 2)
	binary.BigEndian.PutUint16(x, uint16(n))
	*b = append(*b, x...)
}

func (b *writeBuf) string(s string) {
	*b = append(*b, (s + "\000")...)
}

func (b *writeBuf) byte(c byte) {
	*b = append(*b, c)
}

func (b *writeBuf) bytes(v []byte) {
	*b = append(*b, v...)
}
