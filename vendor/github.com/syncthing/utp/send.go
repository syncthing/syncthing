package utp

import (
	"log"
	"time"

	"github.com/anacrolix/missinggo"
)

type send struct {
	acked       bool // Closed with Conn lock.
	payloadSize uint32
	started     missinggo.MonotonicTime
	_type       st
	connID      uint16
	payload     []byte
	seqNr       uint16
	conn        *Conn

	acksSkipped int
	resendTimer *time.Timer
	numResends  int
}

// first is true if this is the first time the send is acked. latency is
// calculated for the first ack.
func (s *send) Ack() (latency time.Duration, first bool) {
	first = !s.acked
	if first {
		latency = missinggo.MonotonicSince(s.started)
	}
	s.acked = true
	cond.Broadcast()
	if s.resendTimer != nil {
		s.resendTimer.Stop()
		s.resendTimer = nil
	}
	return
}

func (s *send) timedOut() {
	s.conn.destroy(errAckTimeout)
}

func (s *send) timeoutResend() {
	mu.Lock()
	defer mu.Unlock()
	if missinggo.MonotonicSince(s.started) >= writeTimeout {
		s.timedOut()
		return
	}
	if s.acked || s.conn.destroyed.Get() {
		return
	}
	rt := s.conn.resendTimeout()
	s.resend()
	s.numResends++
	s.resendTimer.Reset(rt * time.Duration(s.numResends))
}

func (s *send) resend() {
	if s.acked {
		return
	}
	err := s.conn.send(s._type, s.connID, s.payload, s.seqNr)
	if err != nil {
		log.Printf("error resending packet: %s", err)
	}
}
