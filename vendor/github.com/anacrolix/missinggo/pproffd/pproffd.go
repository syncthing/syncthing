package pproffd

import (
	"io"
	"net"
	"os"
	"runtime/pprof"
)

var p = pprof.NewProfile("fds")

type fd int

func (me *fd) Closed() {
	p.Remove(me)
}

func Add(skip int) (ret *fd) {
	ret = new(fd)
	p.Add(ret, skip+2)
	return
}

type closeWrapper struct {
	fd *fd
	c  io.Closer
}

func (me closeWrapper) Close() error {
	me.fd.Closed()
	return me.c.Close()
}

func newCloseWrapper(c io.Closer) closeWrapper {
	return closeWrapper{
		fd: Add(2),
		c:  c,
	}
}

type wrappedNetConn struct {
	net.Conn
	closeWrapper
}

func (me wrappedNetConn) Close() error {
	return me.closeWrapper.Close()
}

// Tracks a net.Conn until Close() is explicitly called.
func WrapNetConn(nc net.Conn) net.Conn {
	if nc == nil {
		return nil
	}
	return wrappedNetConn{
		nc,
		newCloseWrapper(nc),
	}
}

type OSFile interface {
	io.Reader
	io.Seeker
	io.Closer
	io.Writer
	Stat() (os.FileInfo, error)
	io.ReaderAt
	io.WriterAt
}

type wrappedOSFile struct {
	*os.File
	closeWrapper
}

func (me wrappedOSFile) Close() error {
	return me.closeWrapper.Close()
}

func WrapOSFile(f *os.File) OSFile {
	return &wrappedOSFile{f, newCloseWrapper(f)}
}
