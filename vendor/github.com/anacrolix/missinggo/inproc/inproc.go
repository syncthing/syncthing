package inproc

import (
	"errors"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
)

var (
	mu       sync.Mutex
	cond         = sync.Cond{L: &mu}
	nextPort int = 1
	conns        = map[int]*packetConn{}
)

type addr struct {
	Port int
}

func (addr) Network() string {
	return "inproc"
}

func (me addr) String() string {
	return ":" + strconv.FormatInt(int64(me.Port), 10)
}

func getPort() (port int) {
	mu.Lock()
	defer mu.Unlock()
	port = nextPort
	nextPort++
	return
}

func ResolveAddr(network, str string) (net.Addr, error) {
	return ResolveInprocAddr(network, str)
}

func ResolveInprocAddr(network, str string) (addr addr, err error) {
	if str == "" {
		addr.Port = getPort()
		return
	}
	_, p, err := net.SplitHostPort(str)
	if err != nil {
		return
	}
	i64, err := strconv.ParseInt(p, 10, 0)
	if err != nil {
		return
	}
	addr.Port = int(i64)
	if addr.Port == 0 {
		addr.Port = getPort()
	}
	return
}

func ListenPacket(network, addrStr string) (nc net.PacketConn, err error) {
	addr, err := ResolveInprocAddr(network, addrStr)
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := conns[addr.Port]; ok {
		err = errors.New("address in use")
		return
	}
	pc := &packetConn{
		addr:          addr,
		readDeadline:  newCondDeadline(&cond),
		writeDeadline: newCondDeadline(&cond),
	}
	conns[addr.Port] = pc
	nc = pc
	return
}

type packet struct {
	data []byte
	addr addr
}

type packetConn struct {
	closed        bool
	addr          addr
	reads         []packet
	readDeadline  *condDeadline
	writeDeadline *condDeadline
}

func (me *packetConn) Close() error {
	mu.Lock()
	defer mu.Unlock()
	me.closed = true
	delete(conns, me.addr.Port)
	cond.Broadcast()
	return nil
}

func (me *packetConn) LocalAddr() net.Addr {
	return me.addr
}

var errTimeout = errors.New("i/o timeout")

func (me *packetConn) WriteTo(b []byte, na net.Addr) (n int, err error) {
	mu.Lock()
	defer mu.Unlock()
	if me.closed {
		err = errors.New("closed")
		return
	}
	if me.writeDeadline.exceeded() {
		err = errTimeout
		return
	}
	n = len(b)
	port := missinggo.AddrPort(na)
	c, ok := conns[port]
	if !ok {
		// log.Printf("no conn for port %d", port)
		return
	}
	c.reads = append(c.reads, packet{append([]byte(nil), b...), me.addr})
	cond.Broadcast()
	return
}

func (me *packetConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	mu.Lock()
	defer mu.Unlock()
	for {
		if len(me.reads) != 0 {
			r := me.reads[0]
			me.reads = me.reads[1:]
			n = copy(b, r.data)
			addr = r.addr
			// log.Println(addr)
			return
		}
		if me.closed {
			err = io.EOF
			return
		}
		if me.readDeadline.exceeded() {
			err = errTimeout
			return
		}
		cond.Wait()
	}
}

func (me *packetConn) SetDeadline(t time.Time) error {
	me.writeDeadline.setDeadline(t)
	me.readDeadline.setDeadline(t)
	return nil
}

func (me *packetConn) SetReadDeadline(t time.Time) error {
	me.readDeadline.setDeadline(t)
	return nil
}

func (me *packetConn) SetWriteDeadline(t time.Time) error {
	me.writeDeadline.setDeadline(t)
	return nil
}

func newCondDeadline(cond *sync.Cond) (ret *condDeadline) {
	ret = &condDeadline{
		timer: time.AfterFunc(math.MaxInt64, func() {
			mu.Lock()
			ret._exceeded = true
			mu.Unlock()
			cond.Broadcast()
		}),
	}
	ret.setDeadline(time.Time{})
	return
}

type condDeadline struct {
	mu        sync.Mutex
	_exceeded bool
	timer     *time.Timer
}

func (me *condDeadline) setDeadline(t time.Time) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me._exceeded = false
	if t.IsZero() {
		me.timer.Stop()
		return
	}
	me.timer.Reset(t.Sub(time.Now()))
}

func (me *condDeadline) exceeded() bool {
	me.mu.Lock()
	defer me.mu.Unlock()
	return me._exceeded
}
