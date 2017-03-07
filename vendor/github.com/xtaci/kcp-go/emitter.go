package kcp

import (
	"net"
	"sync/atomic"
)

var defaultEmitter Emitter

const emitQueue = 8192

func init() {
	defaultEmitter.init()
}

type (
	emitPacket struct {
		conn    net.PacketConn
		to      net.Addr
		data    []byte
		recycle bool
	}

	// Emitter is the global packet sender
	Emitter struct {
		ch chan emitPacket
	}
)

func (e *Emitter) init() {
	e.ch = make(chan emitPacket, emitQueue)
	go e.emitTask()
}

// keepon writing packets to kernel
func (e *Emitter) emitTask() {
	for p := range e.ch {
		if n, err := p.conn.WriteTo(p.data, p.to); err == nil {
			atomic.AddUint64(&DefaultSnmp.OutSegs, 1)
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
