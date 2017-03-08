package pfilter

import (
	"fmt"
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
	errClosed = fmt.Errorf("use of closed network connection")
)

type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

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
