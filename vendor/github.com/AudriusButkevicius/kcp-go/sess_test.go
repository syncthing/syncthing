package kcp

import (
	"crypto/sha1"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const port = "127.0.0.1:9999"
const salt = "kcptest"

var key = []byte("testkey")
var fec = 4

func DialTest() (*UDPSession, error) {
	pass := pbkdf2.Key(key, []byte(salt), 4096, 32, sha1.New)
	block, _ := NewNoneBlockCrypt(pass)
	//block, _ := NewSimpleXORBlockCrypt(pass)
	//block, _ := NewTEABlockCrypt(pass[:16])
	//block, _ := NewAESBlockCrypt(pass)
	return Dial(port, block, NewFEC(10, 3))
}

func ListenTest() (*Listener, error) {
	pass := pbkdf2.Key(key, []byte(salt), 4096, 32, sha1.New)
	block, _ := NewNoneBlockCrypt(pass)
	//block, _ := NewSimpleXORBlockCrypt(pass)
	//block, _ := NewTEABlockCrypt(pass[:16])
	//block, _ := NewAESBlockCrypt(pass)
	return ListenWithOptions(port, block, NewFEC(10, 3))
}

func server() {
	l, err := ListenTest()
	if err != nil {
		panic(err)
	}

	l.SetReadBuffer(16 * 1024 * 1024)
	l.SetWriteBuffer(16 * 1024 * 1024)
	l.SetDSCP(46)
	log.Println("listening on:", l.Addr())
	for {
		s, err := l.Accept()
		if err != nil {
			panic(err)
		}

		// coverage test
		s.SetReadBuffer(16 * 1024 * 1024)
		s.SetWriteBuffer(16 * 1024 * 1024)
		s.SetKeepAlive(1)
		go handleClient(s)
	}
}

func init() {
	go server()
}

func handleClient(conn *UDPSession) {
	conn.SetStreamMode(true)
	conn.SetWindowSize(1024, 1024)
	conn.SetNoDelay(1, 20, 2, 1)
	conn.SetDSCP(46)
	conn.SetMtu(1450)
	conn.SetACKNoDelay(false)
	conn.SetReadDeadline(time.Now().Add(time.Hour))
	conn.SetWriteDeadline(time.Now().Add(time.Hour))
	fmt.Println("new client", conn.RemoteAddr())
	buf := make([]byte, 65536)
	count := 0
	for {
		n, err := conn.Read(buf)
		if err != nil {
			panic(err)
		}
		count++
		conn.Write(buf[:n])
	}
}

func TestTimeout(t *testing.T) {
	cli, err := DialTest()
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 10)

	//timeout
	cli.SetDeadline(time.Now().Add(time.Second))
	<-time.After(2 * time.Second)
	n, err := cli.Read(buf)
	if n != 0 || err == nil {
		t.Fail()
	}
	n, err = cli.Write(buf)
	if n != 0 || err == nil {
		t.Fail()
	}
}

func TestClose(t *testing.T) {
	cli, err := DialTest()
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 10)

	cli.Close()
	if cli.Close() == nil {
		t.Fail()
	}
	n, err := cli.Write(buf)
	if n != 0 || err == nil {
		t.Fail()
	}
	n, err = cli.Read(buf)
	if n != 0 || err == nil {
		t.Fail()
	}
}

func TestSendRecv(t *testing.T) {
	var wg sync.WaitGroup
	const par = 1
	wg.Add(par)
	for i := 0; i < par; i++ {
		go client(&wg)
	}
	wg.Wait()
}

func client(wg *sync.WaitGroup) {
	cli, err := DialTest()
	if err != nil {
		panic(err)
	}
	cli.SetReadBuffer(16 * 1024 * 1024)
	cli.SetWriteBuffer(16 * 1024 * 1024)
	cli.SetStreamMode(true)
	cli.SetNoDelay(1, 20, 2, 1)
	cli.SetACKNoDelay(true)
	cli.SetDeadline(time.Now().Add(time.Minute))
	const N = 100
	buf := make([]byte, 10)
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		fmt.Println("sent:", msg)
		cli.Write([]byte(msg))
		if n, err := cli.Read(buf); err == nil {
			fmt.Println("recv:", string(buf[:n]))
		} else {
			panic(err)
		}
	}
	cli.Close()
	wg.Done()
}

func TestBigPacket(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go client2(&wg)
	wg.Wait()
}

func client2(wg *sync.WaitGroup) {
	cli, err := DialTest()
	if err != nil {
		panic(err)
	}
	cli.SetNoDelay(1, 20, 2, 1)
	const N = 10
	buf := make([]byte, 1024*512)
	msg := make([]byte, 1024*512)
	for i := 0; i < N; i++ {
		cli.Write(msg)
	}
	println("total written:", len(msg)*N)

	nrecv := 0
	cli.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		n, err := cli.Read(buf)
		if err != nil {
			break
		} else {
			nrecv += n
			if nrecv == len(msg)*N {
				break
			}
		}
	}

	println("total recv:", nrecv)
	cli.Close()
	wg.Done()
}

func TestSpeed(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go client3(&wg)
	wg.Wait()
}

func client3(wg *sync.WaitGroup) {
	cli, err := DialTest()
	if err != nil {
		panic(err)
	}
	log.Println("remote:", cli.RemoteAddr(), "local:", cli.LocalAddr())
	log.Println("conv:", cli.GetConv())
	cli.SetNoDelay(1, 20, 2, 1)
	start := time.Now()

	go func() {
		buf := make([]byte, 1024*1024)
		nrecv := 0
		for {
			n, err := cli.Read(buf)
			if err != nil {
				fmt.Println(err)
				break
			} else {
				nrecv += n
				if nrecv == 4096*4096 {
					break
				}
			}
		}
		println("total recv:", nrecv)
		cli.Close()
		fmt.Println("time for 16MB rtt with encryption", time.Now().Sub(start))
		fmt.Printf("%+v\n", DefaultSnmp.Copy())
		wg.Done()
	}()
	msg := make([]byte, 4096)
	cli.SetWindowSize(1024, 1024)
	for i := 0; i < 4096; i++ {
		cli.Write(msg)
	}
}

func TestParallel(t *testing.T) {
	par := 200
	var wg sync.WaitGroup
	wg.Add(par)
	fmt.Println("testing parallel", par, "connections")
	for i := 0; i < par; i++ {
		go client4(&wg)
	}
	wg.Wait()
}

func client4(wg *sync.WaitGroup) {
	cli, err := DialTest()
	if err != nil {
		panic(err)
	}
	const N = 100
	cli.SetNoDelay(1, 20, 2, 1)
	buf := make([]byte, 10)
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		cli.Write([]byte(msg))
		if _, err := cli.Read(buf); err != nil {
			break
		}
		<-time.After(10 * time.Millisecond)
	}
	cli.Close()
	wg.Done()
}
