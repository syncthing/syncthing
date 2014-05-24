package protocol

import (
	"io"
	"sync/atomic"
)

type countingReader struct {
	io.Reader
	tot uint64
}

var (
	totalIncoming uint64
	totalOutgoing uint64
)

func (c *countingReader) Read(bs []byte) (int, error) {
	n, err := c.Reader.Read(bs)
	atomic.AddUint64(&c.tot, uint64(n))
	atomic.AddUint64(&totalIncoming, uint64(n))
	return n, err
}

func (c *countingReader) Tot() uint64 {
	return atomic.LoadUint64(&c.tot)
}

type countingWriter struct {
	io.Writer
	tot uint64
}

func (c *countingWriter) Write(bs []byte) (int, error) {
	n, err := c.Writer.Write(bs)
	atomic.AddUint64(&c.tot, uint64(n))
	atomic.AddUint64(&totalOutgoing, uint64(n))
	return n, err
}

func (c *countingWriter) Tot() uint64 {
	return atomic.LoadUint64(&c.tot)
}

func TotalInOut() (uint64, uint64) {
	return atomic.LoadUint64(&totalIncoming), atomic.LoadUint64(&totalOutgoing)
}
