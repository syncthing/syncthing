package kcp

import (
	"net"
	"sync/atomic"
)

var defaultEmitter Emitter

func init() {
	defaultEmitter.init()
}

type (
	// packet emit request
	emitPacket struct {
		conn net.PacketConn
		to   net.Addr
		data []byte
		// mark this packet should recycle to global xmitBuf
		recycle bool
	}

	// Emitter is the global packet sender
	Emitter struct {
		ch chan emitPacket
	}
)

func (e *Emitter) init() {
	e.ch = make(chan emitPacket)
	go e.emitTask()
}

// keepon writing packets to kernel
func (e *Emitter) emitTask() {
	for p := range e.ch {
		if n, err := p.conn.WriteTo(p.data, p.to); err == nil {
			atomic.AddUint64(&DefaultSnmp.OutPkts, 1)
			atomic.AddUint64(&DefaultSnmp.OutBytes, uint64(n))
		}
		if p.recycle {
			xmitBuf.Put(p.data)
		}
	}
}

func (e *Emitter) emit(p emitPacket) {
	e.ch <- p
}
