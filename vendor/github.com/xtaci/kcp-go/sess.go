package kcp

import (
	"crypto/rand"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

type errTimeout struct {
	error
}

func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }
func (errTimeout) Error() string   { return "i/o timeout" }

const (
	// 16-bytes magic number for each packet
	nonceSize = 16

	// 4-bytes packet checksum
	crcSize = 4

	// overall crypto header size
	cryptHeaderSize = nonceSize + crcSize

	// maximum packet size
	mtuLimit = 2048

	// packet receiving channel limit
	rxQueueLimit = 2048

	// FEC keeps rxFECMulti* (dataShard+parityShard) ordered packets in memory
	rxFECMulti = 3
)

const (
	errBrokenPipe       = "broken pipe"
	errInvalidOperation = "invalid operation"
)

var (
	// global packet buffer
	// shared among sending/receiving/FEC
	xmitBuf sync.Pool

	// monotonic session id
	sid uint32
)

func init() {
	xmitBuf.New = func() interface{} {
		return make([]byte, mtuLimit)
	}
}

type (
	// UDPSession defines a KCP session implemented by UDP
	UDPSession struct {
		sid   uint32         // session id(monotonic)
		conn  net.PacketConn // the underlying packet connection
		kcp   *KCP           // KCP ARQ protocol
		l     *Listener      // point to the Listener if it's accepted by Listener
		block BlockCrypt     // block encryption

		// kcp receiving is based on packets
		// sockbuff turns packets into stream
		sockbuff []byte

		fec           *FEC     // forward error correction
		fecDataShards [][]byte // data shards cache
		fecShardCount int      // count the number of datashards collected
		fecMaxSize    int      // record maximum data length in datashard

		fecHeaderOffset  int // FEC header offset in packet
		fecPayloadOffset int // FEC payload offset in packet

		// settings
		remote         net.Addr  // remote peer address
		rd             time.Time // read deadline
		wd             time.Time // write deadline
		headerSize     int       // the overall header size added before KCP frame
		updateInterval int32     // interval in seconds to call kcp.flush()
		ackNoDelay     bool      // send ack immediately for each incoming packet

		// notifications
		die          chan struct{} // notify session has Closed
		chReadEvent  chan struct{} // notify Read() can be called without blocking
		chWriteEvent chan struct{} // notify Write() can be called without blocking

		isClosed bool // flag the session has Closed
		mu       sync.Mutex
	}

	setReadBuffer interface {
		SetReadBuffer(bytes int) error
	}

	setWriteBuffer interface {
		SetWriteBuffer(bytes int) error
	}
)

// newUDPSession create a new udp session for client or server
func newUDPSession(conv uint32, dataShards, parityShards int, l *Listener, conn net.PacketConn, remote net.Addr, block BlockCrypt) *UDPSession {
	sess := new(UDPSession)
	sess.sid = atomic.AddUint32(&sid, 1)
	sess.die = make(chan struct{})
	sess.chReadEvent = make(chan struct{}, 1)
	sess.chWriteEvent = make(chan struct{}, 1)
	sess.remote = remote
	sess.conn = conn
	sess.l = l
	sess.block = block

	// FEC initialization
	sess.fec = newFEC(rxFECMulti*(dataShards+parityShards), dataShards, parityShards)
	if sess.fec != nil {
		if sess.block != nil {
			sess.fecHeaderOffset = cryptHeaderSize
		}
		sess.fecPayloadOffset = sess.fecHeaderOffset + fecHeaderSize

		// fec data shards
		sess.fecDataShards = make([][]byte, sess.fec.shardSize)
		for k := range sess.fecDataShards {
			sess.fecDataShards[k] = make([]byte, mtuLimit)
		}
	}

	// calculate header size
	if sess.block != nil {
		sess.headerSize += cryptHeaderSize
	}
	if sess.fec != nil {
		sess.headerSize += fecHeaderSizePlus2
	}

	sess.kcp = NewKCP(conv, func(buf []byte, size int) {
		if size >= IKCP_OVERHEAD {
			sess.output(buf[:size])
		}
	})
	sess.kcp.SetMtu(IKCP_MTU_DEF - sess.headerSize)
	sess.kcp.setFEC(dataShards, parityShards)

	updater.addSession(sess)
	if sess.l == nil { // it's a client connection
		go sess.readLoop()
		atomic.AddUint64(&DefaultSnmp.ActiveOpens, 1)
	} else {
		atomic.AddUint64(&DefaultSnmp.PassiveOpens, 1)
	}
	currestab := atomic.AddUint64(&DefaultSnmp.CurrEstab, 1)
	maxconn := atomic.LoadUint64(&DefaultSnmp.MaxConn)
	if currestab > maxconn {
		atomic.CompareAndSwapUint64(&DefaultSnmp.MaxConn, maxconn, currestab)
	}

	return sess
}

// Read implements the Conn Read method.
func (s *UDPSession) Read(b []byte) (n int, err error) {
	for {
		s.mu.Lock()
		if len(s.sockbuff) > 0 { // copy from buffer
			n = copy(b, s.sockbuff)
			s.sockbuff = s.sockbuff[n:]
			s.mu.Unlock()
			return n, nil
		}

		if s.isClosed {
			s.mu.Unlock()
			return 0, errors.New(errBrokenPipe)
		}

		if !s.rd.IsZero() {
			if time.Now().After(s.rd) { // timeout
				s.mu.Unlock()
				return 0, errTimeout{}
			}
		}

		if n := s.kcp.PeekSize(); n > 0 { // data arrived
			if len(b) >= n {
				s.kcp.Recv(b)
			} else {
				buf := make([]byte, n)
				s.kcp.Recv(buf)
				n = copy(b, buf)
				s.sockbuff = buf[n:] // store remaining bytes into sockbuff for next read
			}
			s.mu.Unlock()
			atomic.AddUint64(&DefaultSnmp.BytesReceived, uint64(n))
			return n, nil
		}

		var timeout *time.Timer
		var c <-chan time.Time
		if !s.rd.IsZero() {
			delay := s.rd.Sub(time.Now())
			timeout = time.NewTimer(delay)
			c = timeout.C
		}
		s.mu.Unlock()

		// wait for read event or timeout
		select {
		case <-s.chReadEvent:
		case <-c:
		case <-s.die:
		}

		if timeout != nil {
			timeout.Stop()
		}
	}
}

// Write implements the Conn Write method.
func (s *UDPSession) Write(b []byte) (n int, err error) {
	for {
		s.mu.Lock()
		if s.isClosed {
			s.mu.Unlock()
			return 0, errors.New(errBrokenPipe)
		}

		if !s.wd.IsZero() {
			if time.Now().After(s.wd) { // timeout
				s.mu.Unlock()
				return 0, errTimeout{}
			}
		}

		if s.kcp.WaitSnd() < int(s.kcp.Cwnd()) {
			n = len(b)
			for {
				if len(b) <= int(s.kcp.mss) {
					s.kcp.Send(b)
					break
				} else {
					s.kcp.Send(b[:s.kcp.mss])
					b = b[s.kcp.mss:]
				}
			}

			s.kcp.flush(false)
			s.mu.Unlock()
			atomic.AddUint64(&DefaultSnmp.BytesSent, uint64(n))
			return n, nil
		}

		var timeout *time.Timer
		var c <-chan time.Time
		if !s.wd.IsZero() {
			delay := s.wd.Sub(time.Now())
			timeout = time.NewTimer(delay)
			c = timeout.C
		}
		s.mu.Unlock()

		// wait for write event or timeout
		select {
		case <-s.chWriteEvent:
		case <-c:
		case <-s.die:
		}

		if timeout != nil {
			timeout.Stop()
		}
	}
}

// Close closes the connection.
func (s *UDPSession) Close() error {
	updater.removeSession(s)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isClosed {
		return errors.New(errBrokenPipe)
	}
	close(s.die)
	s.isClosed = true
	atomic.AddUint64(&DefaultSnmp.CurrEstab, ^uint64(0))
	if s.l == nil { // client socket close
		return s.conn.Close()
	}

	return nil
}

// LocalAddr returns the local network address. The Addr returned is shared by all invocations of LocalAddr, so do not modify it.
func (s *UDPSession) LocalAddr() net.Addr { return s.conn.LocalAddr() }

// RemoteAddr returns the remote network address. The Addr returned is shared by all invocations of RemoteAddr, so do not modify it.
func (s *UDPSession) RemoteAddr() net.Addr { return s.remote }

// SetDeadline sets the deadline associated with the listener. A zero time value disables the deadline.
func (s *UDPSession) SetDeadline(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rd = t
	s.wd = t
	return nil
}

// SetReadDeadline implements the Conn SetReadDeadline method.
func (s *UDPSession) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rd = t
	return nil
}

// SetWriteDeadline implements the Conn SetWriteDeadline method.
func (s *UDPSession) SetWriteDeadline(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wd = t
	return nil
}

// SetWindowSize set maximum window size
func (s *UDPSession) SetWindowSize(sndwnd, rcvwnd int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kcp.WndSize(sndwnd, rcvwnd)
}

// SetMtu sets the maximum transmission unit(not including UDP header)
func (s *UDPSession) SetMtu(mtu int) bool {
	if mtu > mtuLimit {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.kcp.SetMtu(mtu - s.headerSize)
	return true
}

// SetStreamMode toggles the stream mode on/off
func (s *UDPSession) SetStreamMode(enable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if enable {
		s.kcp.stream = 1
	} else {
		s.kcp.stream = 0
	}
}

// SetACKNoDelay changes ack flush option, set true to flush ack immediately,
func (s *UDPSession) SetACKNoDelay(nodelay bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ackNoDelay = nodelay
}

// SetNoDelay calls nodelay() of kcp
func (s *UDPSession) SetNoDelay(nodelay, interval, resend, nc int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kcp.NoDelay(nodelay, interval, resend, nc)
	atomic.StoreInt32(&s.updateInterval, int32(interval))
}

// SetDSCP sets the 6bit DSCP field of IP header, no effect if it's accepted from Listener
func (s *UDPSession) SetDSCP(dscp int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l == nil {
		if nc, ok := s.conn.(*ConnectedUDPConn); ok {
			return ipv4.NewConn(nc.Conn).SetTOS(dscp << 2)
		} else if nc, ok := s.conn.(net.Conn); ok {
			return ipv4.NewConn(nc).SetTOS(dscp << 2)
		}
	}
	return errors.New(errInvalidOperation)
}

// SetReadBuffer sets the socket read buffer, no effect if it's accepted from Listener
func (s *UDPSession) SetReadBuffer(bytes int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l == nil {
		if nc, ok := s.conn.(setReadBuffer); ok {
			return nc.SetReadBuffer(bytes)
		}
	}
	return errors.New(errInvalidOperation)
}

// SetWriteBuffer sets the socket write buffer, no effect if it's accepted from Listener
func (s *UDPSession) SetWriteBuffer(bytes int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l == nil {
		if nc, ok := s.conn.(setWriteBuffer); ok {
			return nc.SetWriteBuffer(bytes)
		}
	}
	return errors.New(errInvalidOperation)
}

// output pipeline entry
// steps for output data processing:
// 1. FEC
// 2. CRC32
// 3. Encryption
// 4. emit to emitTask
// 5. emitTask WriteTo kernel
func (s *UDPSession) output(buf []byte) {
	var ecc [][]byte

	// extend buf's header space
	ext := xmitBuf.Get().([]byte)[:s.headerSize+len(buf)]
	copy(ext[s.headerSize:], buf)

	// FEC stage
	if s.fec != nil {
		s.fec.markData(ext[s.fecHeaderOffset:])
		binary.LittleEndian.PutUint16(ext[s.fecPayloadOffset:], uint16(len(ext[s.fecPayloadOffset:])))

		// copy data to fec datashards
		sz := len(ext)
		s.fecDataShards[s.fecShardCount] = s.fecDataShards[s.fecShardCount][:sz]
		copy(s.fecDataShards[s.fecShardCount], ext)
		s.fecShardCount++

		// record max datashard length
		if sz > s.fecMaxSize {
			s.fecMaxSize = sz
		}

		//  calculate Reed-Solomon Erasure Code
		if s.fecShardCount == s.fec.dataShards {
			// bzero each datashard's tail
			for i := 0; i < s.fec.dataShards; i++ {
				shard := s.fecDataShards[i]
				slen := len(shard)
				xorBytes(shard[slen:s.fecMaxSize], shard[slen:s.fecMaxSize], shard[slen:s.fecMaxSize])
			}

			// calculation of RS
			ecc = s.fec.Encode(s.fecDataShards, s.fecPayloadOffset, s.fecMaxSize)
			for k := range ecc {
				s.fec.markFEC(ecc[k][s.fecHeaderOffset:])
				ecc[k] = ecc[k][:s.fecMaxSize]
			}

			// reset counters to zero
			s.fecShardCount = 0
			s.fecMaxSize = 0
		}
	}

	// encryption stage
	if s.block != nil {
		io.ReadFull(rand.Reader, ext[:nonceSize])
		checksum := crc32.ChecksumIEEE(ext[cryptHeaderSize:])
		binary.LittleEndian.PutUint32(ext[nonceSize:], checksum)
		s.block.Encrypt(ext, ext)

		if ecc != nil {
			for k := range ecc {
				io.ReadFull(rand.Reader, ecc[k][:nonceSize])
				checksum := crc32.ChecksumIEEE(ecc[k][cryptHeaderSize:])
				binary.LittleEndian.PutUint32(ecc[k][nonceSize:], checksum)
				s.block.Encrypt(ecc[k], ecc[k])
			}
		}
	}

	// emit stage
	defaultEmitter.emit(emitPacket{s.conn, s.remote, ext, true})
	if ecc != nil {
		for k := range ecc {
			defaultEmitter.emit(emitPacket{s.conn, s.remote, ecc[k], false})
		}
	}
}

// kcp update, returns interval for next calling
func (s *UDPSession) update() time.Duration {
	s.mu.Lock()
	s.kcp.flush(false)
	if s.kcp.WaitSnd() < int(s.kcp.Cwnd()) {
		s.notifyWriteEvent()
	}
	s.mu.Unlock()
	return time.Duration(atomic.LoadInt32(&s.updateInterval)) * time.Millisecond
}

// GetConv gets conversation id of a session
func (s *UDPSession) GetConv() uint32 {
	return s.kcp.conv
}

func (s *UDPSession) notifyReadEvent() {
	select {
	case s.chReadEvent <- struct{}{}:
	default:
	}
}

func (s *UDPSession) notifyWriteEvent() {
	select {
	case s.chWriteEvent <- struct{}{}:
	default:
	}
}

func (s *UDPSession) kcpInput(data []byte) {
	var kcpInErrors, fecErrs, fecRecovered, fecParityShards uint64

	if s.fec != nil {
		f := s.fec.decodeBytes(data)
		s.mu.Lock()
		if f.flag == typeData {
			if ret := s.kcp.Input(data[fecHeaderSizePlus2:], true, s.ackNoDelay); ret != 0 {
				kcpInErrors++
			}
		}

		if f.flag == typeData || f.flag == typeFEC {
			if f.flag == typeFEC {
				fecParityShards++
			}

			if recovers := s.fec.Decode(f); recovers != nil {
				for _, r := range recovers {
					if len(r) >= 2 { // must be larger than 2bytes
						sz := binary.LittleEndian.Uint16(r)
						if int(sz) <= len(r) && sz >= 2 {
							if ret := s.kcp.Input(r[2:sz], false, s.ackNoDelay); ret == 0 {
								fecRecovered++
							} else {
								kcpInErrors++
							}
						} else {
							fecErrs++
						}
					} else {
						fecErrs++
					}
				}
			}
		}

		// notify reader
		if n := s.kcp.PeekSize(); n > 0 {
			s.notifyReadEvent()
		}
		s.mu.Unlock()
	} else {
		s.mu.Lock()
		if ret := s.kcp.Input(data, true, s.ackNoDelay); ret != 0 {
			kcpInErrors++
		}
		// notify reader
		if n := s.kcp.PeekSize(); n > 0 {
			s.notifyReadEvent()
		}
		s.mu.Unlock()
	}

	atomic.AddUint64(&DefaultSnmp.InPkts, 1)
	atomic.AddUint64(&DefaultSnmp.InBytes, uint64(len(data)))
	if fecParityShards > 0 {
		atomic.AddUint64(&DefaultSnmp.FECParityShards, fecParityShards)
	}
	if kcpInErrors > 0 {
		atomic.AddUint64(&DefaultSnmp.KCPInErrors, kcpInErrors)
	}
	if fecErrs > 0 {
		atomic.AddUint64(&DefaultSnmp.FECErrs, fecErrs)
	}
	if fecRecovered > 0 {
		atomic.AddUint64(&DefaultSnmp.FECRecovered, fecRecovered)
	}
}

func (s *UDPSession) receiver(ch chan []byte) {
	for {
		data := xmitBuf.Get().([]byte)[:mtuLimit]
		if n, _, err := s.conn.ReadFrom(data); err == nil && n >= s.headerSize+IKCP_OVERHEAD {
			select {
			case ch <- data[:n]:
			case <-s.die:
			}
		} else if err != nil {
			return
		} else {
			atomic.AddUint64(&DefaultSnmp.InErrs, 1)
		}
	}
}

// read loop for client session
func (s *UDPSession) readLoop() {
	chPacket := make(chan []byte, rxQueueLimit)
	go s.receiver(chPacket)

	for {
		select {
		case data := <-chPacket:
			raw := data
			dataValid := false
			if s.block != nil {
				s.block.Decrypt(data, data)
				data = data[nonceSize:]
				checksum := crc32.ChecksumIEEE(data[crcSize:])
				if checksum == binary.LittleEndian.Uint32(data) {
					data = data[crcSize:]
					dataValid = true
				} else {
					atomic.AddUint64(&DefaultSnmp.InCsumErrors, 1)
				}
			} else if s.block == nil {
				dataValid = true
			}

			if dataValid {
				s.kcpInput(data)
			}
			xmitBuf.Put(raw)
		case <-s.die:
			return
		}
	}
}

type (
	// Listener defines a server listening for connections
	Listener struct {
		block        BlockCrypt     // block encryption
		dataShards   int            // FEC data shard
		parityShards int            // FEC parity shard
		fec          *FEC           // FEC mock initialization
		conn         net.PacketConn // the underlying packet connection

		sessions    map[string]*UDPSession // all sessions accepted by this Listener
		chAccepts   chan *UDPSession       // Listen() backlog
		chDeadlinks chan net.Addr          // session close queue
		headerSize  int                    // the overall header size added before KCP frame
		die         chan struct{}          // notify the listener has closed
		rd          atomic.Value           // read deadline for Accept()
		wd          atomic.Value
	}

	// incoming packet
	inPacket struct {
		from net.Addr
		data []byte
	}
)

// monitor incoming data for all connections of server
func (l *Listener) monitor() {
	chPacket := make(chan inPacket, rxQueueLimit)
	go l.receiver(chPacket)
	for {
		select {
		case p := <-chPacket:
			raw := p.data
			data := p.data
			from := p.from
			dataValid := false
			if l.block != nil {
				l.block.Decrypt(data, data)
				data = data[nonceSize:]
				checksum := crc32.ChecksumIEEE(data[crcSize:])
				if checksum == binary.LittleEndian.Uint32(data) {
					data = data[crcSize:]
					dataValid = true
				} else {
					atomic.AddUint64(&DefaultSnmp.InCsumErrors, 1)
				}
			} else if l.block == nil {
				dataValid = true
			}

			if dataValid {
				addr := from.String()
				s, ok := l.sessions[addr]
				if !ok { // new session
					var conv uint32
					convValid := false
					if l.fec != nil {
						isfec := binary.LittleEndian.Uint16(data[4:])
						if isfec == typeData {
							conv = binary.LittleEndian.Uint32(data[fecHeaderSizePlus2:])
							convValid = true
						}
					} else {
						conv = binary.LittleEndian.Uint32(data)
						convValid = true
					}

					if convValid {
						s := newUDPSession(conv, l.dataShards, l.parityShards, l, l.conn, from, l.block)
						s.kcpInput(data)
						l.sessions[addr] = s
						l.chAccepts <- s
					}
				} else {
					s.kcpInput(data)
				}
			}

			xmitBuf.Put(raw)
		case deadlink := <-l.chDeadlinks:
			delete(l.sessions, deadlink.String())
		case <-l.die:
			return
		}
	}
}

func (l *Listener) receiver(ch chan inPacket) {
	for {
		data := xmitBuf.Get().([]byte)[:mtuLimit]
		if n, from, err := l.conn.ReadFrom(data); err == nil && n >= l.headerSize+IKCP_OVERHEAD {
			ch <- inPacket{from, data[:n]}
		} else if err != nil {
			return
		} else {
			atomic.AddUint64(&DefaultSnmp.InErrs, 1)
		}
	}
}

// SetReadBuffer sets the socket read buffer for the Listener
func (l *Listener) SetReadBuffer(bytes int) error {
	if nc, ok := l.conn.(setReadBuffer); ok {
		return nc.SetReadBuffer(bytes)
	}
	return errors.New(errInvalidOperation)
}

// SetWriteBuffer sets the socket write buffer for the Listener
func (l *Listener) SetWriteBuffer(bytes int) error {
	if nc, ok := l.conn.(setWriteBuffer); ok {
		return nc.SetWriteBuffer(bytes)
	}
	return errors.New(errInvalidOperation)
}

// SetDSCP sets the 6bit DSCP field of IP header
func (l *Listener) SetDSCP(dscp int) error {
	if nc, ok := l.conn.(net.Conn); ok {
		return ipv4.NewConn(nc).SetTOS(dscp << 2)
	}
	return errors.New(errInvalidOperation)
}

// Accept implements the Accept method in the Listener interface; it waits for the next call and returns a generic Conn.
func (l *Listener) Accept() (net.Conn, error) {
	return l.AcceptKCP()
}

// AcceptKCP accepts a KCP connection
func (l *Listener) AcceptKCP() (*UDPSession, error) {
	var timeout <-chan time.Time
	if tdeadline, ok := l.rd.Load().(time.Time); ok && !tdeadline.IsZero() {
		timeout = time.After(tdeadline.Sub(time.Now()))
	}

	select {
	case <-timeout:
		return nil, &errTimeout{}
	case c := <-l.chAccepts:
		return c, nil
	case <-l.die:
		return nil, errors.New(errBrokenPipe)
	}
}

// SetDeadline sets the deadline associated with the listener. A zero time value disables the deadline.
func (l *Listener) SetDeadline(t time.Time) error {
	l.SetReadDeadline(t)
	l.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline implements the Conn SetReadDeadline method.
func (l *Listener) SetReadDeadline(t time.Time) error {
	l.rd.Store(t)
	return nil
}

// SetWriteDeadline implements the Conn SetWriteDeadline method.
func (l *Listener) SetWriteDeadline(t time.Time) error {
	l.wd.Store(t)
	return nil
}

// Close stops listening on the UDP address. Already Accepted connections are not closed.
func (l *Listener) Close() error {
	close(l.die)
	return l.conn.Close()
}

// Addr returns the listener's network address, The Addr returned is shared by all invocations of Addr, so do not modify it.
func (l *Listener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

// Listen listens for incoming KCP packets addressed to the local address laddr on the network "udp",
func Listen(laddr string) (net.Listener, error) {
	return ListenWithOptions(laddr, nil, 0, 0)
}

// ListenWithOptions listens for incoming KCP packets addressed to the local address laddr on the network "udp" with packet encryption,
// dataShards, parityShards defines Reed-Solomon Erasure Coding parameters
func ListenWithOptions(laddr string, block BlockCrypt, dataShards, parityShards int) (*Listener, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", laddr)
	if err != nil {
		return nil, errors.Wrap(err, "net.ResolveUDPAddr")
	}
	conn, err := net.ListenUDP("udp", udpaddr)
	if err != nil {
		return nil, errors.Wrap(err, "net.ListenUDP")
	}

	return ServeConn(block, dataShards, parityShards, conn)
}

// ServeConn serves KCP protocol for a single packet connection.
func ServeConn(block BlockCrypt, dataShards, parityShards int, conn net.PacketConn) (*Listener, error) {
	l := new(Listener)
	l.conn = conn
	l.sessions = make(map[string]*UDPSession)
	l.chAccepts = make(chan *UDPSession, 1024)
	l.chDeadlinks = make(chan net.Addr, 1024)
	l.die = make(chan struct{})
	l.dataShards = dataShards
	l.parityShards = parityShards
	l.block = block
	l.fec = newFEC(rxFECMulti*(dataShards+parityShards), dataShards, parityShards)

	// calculate header size
	if l.block != nil {
		l.headerSize += cryptHeaderSize
	}
	if l.fec != nil {
		l.headerSize += fecHeaderSizePlus2
	}

	go l.monitor()
	return l, nil
}

// Dial connects to the remote address "raddr" on the network "udp"
func Dial(raddr string) (net.Conn, error) {
	return DialWithOptions(raddr, nil, 0, 0)
}

// DialWithOptions connects to the remote address "raddr" on the network "udp" with packet encryption
func DialWithOptions(raddr string, block BlockCrypt, dataShards, parityShards int) (*UDPSession, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", raddr)
	if err != nil {
		return nil, errors.Wrap(err, "net.ResolveUDPAddr")
	}

	udpconn, err := net.DialUDP("udp", nil, udpaddr)
	if err != nil {
		return nil, errors.Wrap(err, "net.DialUDP")
	}

	return NewConn(raddr, block, dataShards, parityShards, &ConnectedUDPConn{udpconn, udpconn})
}

// NewConn establishes a session and talks KCP protocol over a packet connection.
func NewConn(raddr string, block BlockCrypt, dataShards, parityShards int, conn net.PacketConn) (*UDPSession, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", raddr)
	if err != nil {
		return nil, errors.Wrap(err, "net.ResolveUDPAddr")
	}

	var convid uint32
	binary.Read(rand.Reader, binary.LittleEndian, &convid)
	return newUDPSession(convid, dataShards, parityShards, nil, conn, udpaddr, block), nil
}

func currentMs() uint32 {
	return uint32(time.Now().UnixNano() / int64(time.Millisecond))
}

// ConnectedUDPConn is a wrapper for net.UDPConn which converts WriteTo syscalls
// to Write syscalls that are 4 times faster on some OS'es. This should only be
// used for connections that were produced by a net.Dial* call.
type ConnectedUDPConn struct {
	*net.UDPConn
	Conn net.Conn // underlying connection if any
}

// WriteTo redirects all writes to the Write syscall, which is 4 times faster.
func (c *ConnectedUDPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.Write(b)
}
