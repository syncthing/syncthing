package utp

import (
	"errors"
	"io"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/inproc"
	"github.com/anacrolix/missinggo/pproffd"
)

var (
	_ net.Listener   = &Socket{}
	_ net.PacketConn = &Socket{}
)

// Uniquely identifies any uTP connection on top of the underlying packet
// stream.
type connKey struct {
	remoteAddr resolvedAddrStr
	connID     uint16
}

// A Socket wraps a net.PacketConn, diverting uTP packets to its child uTP
// Conns.
type Socket struct {
	pc    net.PacketConn
	conns map[connKey]*Conn

	backlogNotEmpty missinggo.Event
	backlog         map[syn]struct{}

	closed    missinggo.Event
	destroyed missinggo.Event

	unusedReads chan read
	connDeadlines
	// If a read error occurs on the underlying net.PacketConn, it is put
	// here. This is because reading is done in its own goroutine to dispatch
	// to uTP Conns.
	ReadErr error
}

func listenPacket(network, addr string) (pc net.PacketConn, err error) {
	if network == "inproc" {
		return inproc.ListenPacket(network, addr)
	}
	return net.ListenPacket(network, addr)
}

// addr is used to create a listening UDP conn which becomes the underlying
// net.PacketConn for the Socket.
func NewSocket(network, addr string) (s *Socket, err error) {
	if network == "" {
		network = "udp"
	}
	pc, err := listenPacket(network, addr)
	if err != nil {
		return
	}
	return NewSocketFromPacketConn(pc)
}

// Create a Socket, using the provided net.PacketConn. If you want to retain
// use of the net.PacketConn after the Socket closes it, override your
// net.PacketConn's Close method.
func NewSocketFromPacketConn(pc net.PacketConn) (s *Socket, err error) {
	s = &Socket{
		backlog: make(map[syn]struct{}, backlog),
		pc:      pc,

		unusedReads: make(chan read, 100),
	}
	mu.Lock()
	sockets[s] = struct{}{}
	mu.Unlock()
	go s.reader()
	return
}

func (s *Socket) unusedRead(read read) {
	unusedReads.Add(1)
	select {
	case s.unusedReads <- read:
	default:
		// Drop the packet.
		unusedReadsDropped.Add(1)
	}
}

func (s *Socket) strNetAddr(str string) (a net.Addr) {
	var err error
	switch n := s.network(); n {
	case "udp":
		a, err = net.ResolveUDPAddr(n, str)
	case "inproc":
		a, err = inproc.ResolveAddr(n, str)
	default:
		panic(n)
	}
	if err != nil {
		panic(err)
	}
	return
}

func (s *Socket) pushBacklog(syn syn) {
	if _, ok := s.backlog[syn]; ok {
		return
	}
	// Pop a pseudo-random syn to make room. TODO: Use missinggo/orderedmap,
	// coz that's what is wanted here.
	for k := range s.backlog {
		if len(s.backlog) < backlog {
			break
		}
		delete(s.backlog, k)
		// A syn is sent on the remote's recv_id, so this is where we can send
		// the reset.
		s.reset(s.strNetAddr(k.addr), k.seq_nr, k.conn_id)
	}
	s.backlog[syn] = struct{}{}
	s.backlogChanged()
}

func (s *Socket) reader() {
	mu.Lock()
	defer mu.Unlock()
	defer s.destroy()
	var b [maxRecvSize]byte
	for {
		mu.Unlock()
		n, addr, err := s.pc.ReadFrom(b[:])
		mu.Lock()
		if err != nil {
			s.ReadErr = err
			return
		}
		s.dispatch(read{
			append([]byte(nil), b[:n]...),
			addr,
		})
	}
}

func (s *Socket) dispatch(read read) {
	b := read.data
	addr := read.from
	if len(b) < 20 {
		s.unusedRead(read)
		return
	}
	var h header
	hEnd, err := h.Unmarshal(b)
	if logLevel >= 1 {
		log.Printf("recvd utp msg: %s", packetDebugString(&h, b[hEnd:]))
	}
	if err != nil || h.Type > stMax || h.Version != 1 {
		s.unusedRead(read)
		return
	}
	c, ok := s.conns[connKey{resolvedAddrStr(addr.String()), func() (recvID uint16) {
		recvID = h.ConnID
		// If a SYN is resent, its connection ID field will be one lower
		// than we expect.
		if h.Type == stSyn {
			recvID++
		}
		return
	}()}]
	if ok {
		if h.Type == stSyn {
			if h.ConnID == c.send_id-2 {
				// This is a SYN for connection that cannot exist locally. The
				// connection the remote wants to establish here with the proposed
				// recv_id, already has an existing connection that was dialled
				// *out* from this socket, which is why the send_id is 1 higher,
				// rather than 1 lower than the recv_id.
				log.Print("resetting conflicting syn")
				s.reset(addr, h.SeqNr, h.ConnID)
				return
			} else if h.ConnID != c.send_id {
				panic("bad assumption")
			}
		}
		c.receivePacket(h, b[hEnd:])
		return
	}
	if h.Type == stSyn {
		if logLevel >= 1 {
			log.Printf("adding SYN to backlog")
		}
		syn := syn{
			seq_nr:  h.SeqNr,
			conn_id: h.ConnID,
			addr:    addr.String(),
		}
		s.pushBacklog(syn)
		return
	} else if h.Type != stReset {
		// This is an unexpected packet. We'll send a reset, but also pass
		// it on.
		// log.Print("resetting unexpected packet")
		// I don't think you can reset on the received packets ConnID if it isn't a SYN, as the send_id will differ in this case.
		s.reset(addr, h.SeqNr, h.ConnID)
		s.reset(addr, h.SeqNr, h.ConnID-1)
		s.reset(addr, h.SeqNr, h.ConnID+1)
	}
	s.unusedRead(read)
}

// Send a reset in response to a packet with the given header.
func (s *Socket) reset(addr net.Addr, ackNr, connId uint16) {
	b := make([]byte, 0, maxHeaderSize)
	h := header{
		Type:    stReset,
		Version: 1,
		ConnID:  connId,
		AckNr:   ackNr,
	}
	b = b[:h.Marshal(b)]
	go s.writeTo(b, addr)
}

// Return a recv_id that should be free. Handling the case where it isn't is
// deferred to a more appropriate function.
func (s *Socket) newConnID(remoteAddr resolvedAddrStr) (id uint16) {
	// Rather than use math.Rand, which requires generating all the IDs up
	// front and allocating a slice, we do it on the stack, generating the IDs
	// only as required. To do this, we use the fact that the array is
	// default-initialized. IDs that are 0, are actually their index in the
	// array. IDs that are non-zero, are +1 from their intended ID.
	var idsBack [0x10000]int
	ids := idsBack[:]
	for len(ids) != 0 {
		// Pick the next ID from the untried ids.
		i := rand.Intn(len(ids))
		id = uint16(ids[i])
		// If it's zero, then treat it as though the index i was the ID.
		// Otherwise the value we get is the ID+1.
		if id == 0 {
			id = uint16(i)
		} else {
			id--
		}
		// Check there's no connection using this ID for its recv_id...
		_, ok1 := s.conns[connKey{remoteAddr, id}]
		// and if we're connecting to our own Socket, that there isn't a Conn
		// already receiving on what will correspond to our send_id. Note that
		// we just assume that we could be connecting to our own Socket. This
		// will halve the available connection IDs to each distinct remote
		// address. Presumably that's ~0x8000, down from ~0x10000.
		_, ok2 := s.conns[connKey{remoteAddr, id + 1}]
		_, ok4 := s.conns[connKey{remoteAddr, id - 1}]
		if !ok1 && !ok2 && !ok4 {
			return
		}
		// The set of possible IDs is shrinking. The highest one will be lost, so
		// it's moved to the location of the one we just tried.
		ids[i] = len(ids) // Conveniently already +1.
		// And shrink.
		ids = ids[:len(ids)-1]
	}
	return
}

func (s *Socket) newConn(addr net.Addr) (c *Conn) {
	c = &Conn{
		socket:           s,
		remoteSocketAddr: addr,
		created:          time.Now(),
	}
	c.readCond.L = &mu
	c.sendPendingSendSendStateTimer = missinggo.StoppedFuncTimer(c.sendPendingSendStateTimerCallback)
	c.packetReadTimeoutTimer = time.AfterFunc(packetReadTimeout, c.receivePacketTimeoutCallback)
	missinggo.AddCondToFlags(
		&c.readCond,
		&c.destroyed,
		&c.gotFin,
		&c.closed,
		&c.connDeadlines.read.passed)
	return
}

func (s *Socket) Dial(addr string) (net.Conn, error) {
	return s.DialTimeout(addr, 0)
}

func (s *Socket) resolveAddr(addr string) (net.Addr, error) {
	n := s.network()
	if n == "inproc" {
		return inproc.ResolveAddr(n, addr)
	}
	return net.ResolveUDPAddr(n, addr)
}

func (s *Socket) network() string {
	return s.pc.LocalAddr().Network()
}

// A zero timeout is no timeout. This will fallback onto the write ack
// timeout.
func (s *Socket) DialTimeout(addr string, timeout time.Duration) (nc net.Conn, err error) {
	netAddr, err := s.resolveAddr(addr)
	if err != nil {
		return
	}

	mu.Lock()
	c := s.newConn(netAddr)
	c.recv_id = s.newConnID(resolvedAddrStr(netAddr.String()))
	c.send_id = c.recv_id + 1
	if logLevel >= 1 {
		log.Printf("dial registering addr: %s", netAddr.String())
	}
	if !s.registerConn(c.recv_id, resolvedAddrStr(netAddr.String()), c) {
		err = errors.New("couldn't register new connection")
		log.Println(c.recv_id, netAddr.String())
		for k, c := range s.conns {
			log.Println(k, c, c.age())
		}
		log.Printf("that's %d connections", len(s.conns))
	}
	mu.Unlock()
	if err != nil {
		return
	}

	connErr := make(chan error, 1)
	go func() {
		connErr <- c.connect()
	}()
	var timeoutCh <-chan time.Time
	if timeout != 0 {
		timeoutCh = time.After(timeout)
	}
	select {
	case err = <-connErr:
	case <-timeoutCh:
		err = errTimeout
	}
	if err != nil {
		mu.Lock()
		c.destroy(errors.New("dial timeout"))
		mu.Unlock()
		return
	}
	nc = pproffd.WrapNetConn(c)
	return
}

func (me *Socket) writeTo(b []byte, addr net.Addr) (n int, err error) {
	apdc := artificialPacketDropChance
	if apdc != 0 {
		if rand.Float64() < apdc {
			n = len(b)
			return
		}
	}
	n, err = me.pc.WriteTo(b, addr)
	return
}

// Returns true if the connection was newly registered, false otherwise.
func (s *Socket) registerConn(recvID uint16, remoteAddr resolvedAddrStr, c *Conn) bool {
	if s.conns == nil {
		s.conns = make(map[connKey]*Conn)
	}
	key := connKey{remoteAddr, recvID}
	if _, ok := s.conns[key]; ok {
		return false
	}
	c.connKey = key
	s.conns[key] = c
	return true
}

func (s *Socket) backlogChanged() {
	if len(s.backlog) != 0 {
		s.backlogNotEmpty.Set()
	} else {
		s.backlogNotEmpty.Clear()
	}
}

func (s *Socket) nextSyn() (syn syn, err error) {
	for {
		mu.Unlock()
		missinggo.WaitEvents(&mu, &s.closed, &s.backlogNotEmpty, &s.destroyed)
		mu.Lock()
		if s.closed.IsSet() {
			err = errClosed
			return
		}
		if s.destroyed.IsSet() {
			err = s.ReadErr
			return
		}
		for k := range s.backlog {
			syn = k
			delete(s.backlog, k)
			s.backlogChanged()
			return
		}
	}
}

// ACK a SYN, and return a new Conn for it. ok is false if the SYN is bad, and
// the Conn invalid.
func (s *Socket) ackSyn(syn syn) (c *Conn, ok bool) {
	c = s.newConn(s.strNetAddr(syn.addr))
	c.send_id = syn.conn_id
	c.recv_id = c.send_id + 1
	c.seq_nr = uint16(rand.Int())
	c.lastAck = c.seq_nr - 1
	c.ack_nr = syn.seq_nr
	c.sentSyn = true
	c.synAcked = true
	if !s.registerConn(c.recv_id, resolvedAddrStr(syn.addr), c) {
		// SYN that triggered this accept duplicates existing connection.
		// Ack again in case the SYN was a resend.
		c = s.conns[connKey{resolvedAddrStr(syn.addr), c.recv_id}]
		if c.send_id != syn.conn_id {
			panic(":|")
		}
		c.sendState()
		return
	}
	c.sendState()
	ok = true
	return
}

// Accept and return a new uTP connection.
func (s *Socket) Accept() (net.Conn, error) {
	mu.Lock()
	defer mu.Unlock()
	for {
		syn, err := s.nextSyn()
		if err != nil {
			return nil, err
		}
		c, ok := s.ackSyn(syn)
		if ok {
			return c, nil
		}
	}
}

// The address we're listening on for new uTP connections.
func (s *Socket) Addr() net.Addr {
	return s.pc.LocalAddr()
}

func (s *Socket) Close() error {
	mu.Lock()
	defer mu.Unlock()
	s.closed.Set()
	s.lazyDestroy()
	return nil
}

func (s *Socket) lazyDestroy() {
	if len(s.conns) != 0 {
		return
	}
	if !s.closed.IsSet() {
		return
	}
	s.destroy()
}

func (s *Socket) destroy() {
	delete(sockets, s)
	s.destroyed.Set()
	// TODO: Perhaps we should only Close the PacketConn if we created it
	// ourselves.
	s.pc.Close()
	for _, c := range s.conns {
		c.destroy(errors.New("Socket destroyed"))
	}
}

func (s *Socket) LocalAddr() net.Addr {
	return s.pc.LocalAddr()
}

func (s *Socket) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	read, ok := <-s.unusedReads
	if !ok {
		err = io.EOF
	}
	n = copy(p, read.data)
	addr = read.from
	return
}

func (s *Socket) WriteTo(b []byte, addr net.Addr) (int, error) {
	return s.pc.WriteTo(b, addr)
}
