package utp

import (
	"fmt"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/inproc"
	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcceptOnDestroyedSocket(t *testing.T) {
	pc, err := net.ListenPacket("udp", "localhost:0")
	require.NoError(t, err)
	s, err := NewSocketFromPacketConn(pc)
	require.NoError(t, err)
	go pc.Close()
	_, err = s.Accept()
	assert.Contains(t, err.Error(), "use of closed network connection")
}

func TestSocketDeadlines(t *testing.T) {
	s, err := NewSocket("udp", "localhost:0")
	require.NoError(t, err)
	defer s.Close()
	assert.NoError(t, s.SetReadDeadline(time.Now()))
	_, _, err = s.ReadFrom(nil)
	assert.Equal(t, errTimeout, err)
	assert.NoError(t, s.SetWriteDeadline(time.Now()))
	_, err = s.WriteTo(nil, nil)
	assert.Equal(t, errTimeout, err)
	assert.NoError(t, s.SetDeadline(time.Time{}))
	assert.NoError(t, s.Close())
}

func TestSaturateSocketConnIDs(t *testing.T) {
	s, err := NewSocket("inproc", "")
	require.NoError(t, err)
	defer s.Close()
	var acceptedConns, dialedConns []net.Conn
	for range iter.N(500) {
		accepted := make(chan struct{})
		go func() {
			c, err := s.Accept()
			if err != nil {
				t.Log(err)
				return
			}
			acceptedConns = append(acceptedConns, c)
			close(accepted)
		}()
		c, err := s.Dial(s.Addr().String())
		require.NoError(t, err)
		dialedConns = append(dialedConns, c)
		<-accepted
	}
	t.Logf("%d dialed conns, %d accepted", len(dialedConns), len(acceptedConns))
	for i := range iter.N(len(dialedConns)) {
		data := []byte(fmt.Sprintf("%7d", i))
		dc := dialedConns[i]
		n, err := dc.Write(data)
		require.NoError(t, err)
		require.EqualValues(t, 7, n)
		require.NoError(t, dc.Close())
		var b [8]byte
		ac := acceptedConns[i]
		n, err = ac.Read(b[:])
		require.NoError(t, err)
		require.EqualValues(t, 7, n)
		require.EqualValues(t, data, b[:n])
		n, err = ac.Read(b[:])
		require.EqualValues(t, 0, n)
		require.EqualValues(t, io.EOF, err)
		ac.Close()
	}
}

func TestUTPRawConn(t *testing.T) {
	l, err := NewSocket("inproc", "")
	require.NoError(t, err)
	defer l.Close()
	go func() {
		for {
			_, err := l.Accept()
			if err != nil {
				break
			}
		}
	}()
	// Connect a UTP peer to see if the RawConn will still work.
	log.Print("dialing")
	utpPeer := func() net.Conn {
		s, _ := NewSocket("inproc", "")
		defer s.Close()
		ret, err := s.Dial(fmt.Sprintf("localhost:%d", missinggo.AddrPort(l.Addr())))
		require.NoError(t, err)
		return ret
	}()
	log.Print("dial returned")
	if err != nil {
		t.Fatalf("error dialing utp listener: %s", err)
	}
	defer utpPeer.Close()
	peer, err := inproc.ListenPacket("inproc", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	msgsReceived := 0
	const N = 500 // How many messages to send.
	readerStopped := make(chan struct{})
	// The reader goroutine.
	go func() {
		defer close(readerStopped)
		b := make([]byte, 500)
		for i := 0; i < N; i++ {
			n, _, err := l.ReadFrom(b)
			if err != nil {
				t.Fatalf("error reading from raw conn: %s", err)
			}
			msgsReceived++
			var d int
			fmt.Sscan(string(b[:n]), &d)
			if d != i {
				log.Printf("got wrong number: expected %d, got %d", i, d)
			}
		}
	}()
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("localhost:%d", missinggo.AddrPort(l.Addr())))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < N; i++ {
		_, err := peer.WriteTo([]byte(fmt.Sprintf("%d", i)), udpAddr)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Microsecond)
	}
	select {
	case <-readerStopped:
	case <-time.After(time.Second):
		t.Fatal("reader timed out")
	}
	if msgsReceived != N {
		t.Fatalf("messages received: %d", msgsReceived)
	}
}

func TestAcceptGone(t *testing.T) {
	s, err := NewSocket("udp", "localhost:0")
	require.NoError(t, err)
	defer s.Close()
	_, err = DialTimeout(s.Addr().String(), time.Millisecond)
	require.Error(t, err)
	// Will succeed because we don't signal that we give up dialing, or check
	// that the handshake is completed before returning the new Conn.
	c, err := s.Accept()
	require.NoError(t, err)
	defer c.Close()
	err = c.SetReadDeadline(time.Now().Add(time.Millisecond))
	require.NoError(t, err)
	_, err = c.Read(nil)
	require.EqualError(t, err, "i/o timeout")
}
