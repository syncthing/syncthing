package pfilter

import (
	"net"
	"sync"
)

var (
	maxPacketSize = 1500
	bufPool       = sync.Pool{
		New: func() interface{} {
			return make([]byte, maxPacketSize)
		},
	}
	errTimeout = &netError{
		msg:       "i/o timeout",
		timeout:   true,
		temporary: true,
	}
	errClosed = &netError{
		msg:       "use of closed network connection",
		timeout:   false,
		temporary: false,
	}

	// Compile time interface assertion.
	_ net.Error = (*netError)(nil)
)

type netError struct {
	msg       string
	timeout   bool
	temporary bool
}

func (e *netError) Error() string   { return e.msg }
func (e *netError) Timeout() bool   { return e.timeout }
func (e *netError) Temporary() bool { return e.temporary }

type filteredConnList []*FilteredConn

func (r filteredConnList) Len() int           { return len(r) }
func (r filteredConnList) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r filteredConnList) Less(i, j int) bool { return r[i].priority < r[j].priority }

type packet struct {
	n    int
	addr net.Addr
	err  error
	buf  []byte
}
