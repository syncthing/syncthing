package utp

import (
	"net"
	"testing"
)

func getTCPConnectionPair() (net.Conn, net.Conn, error) {
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}

	var conn0 net.Conn
	var err0 error
	done := make(chan struct{})
	go func() {
		conn0, err0 = lst.Accept()
		close(done)
	}()

	conn1, err := net.Dial("tcp", lst.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	<-done
	if err0 != nil {
		return nil, nil, err0
	}
	return conn0, conn1, nil
}

func getUTPConnectionPair() (net.Conn, net.Conn, error) {
	lst, err := NewSocket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer lst.Close()

	var conn0 net.Conn
	var err0 error
	done := make(chan struct{})
	go func() {
		conn0, err0 = lst.Accept()
		close(done)
	}()

	conn1, err := Dial(lst.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	<-done
	if err0 != nil {
		return nil, nil, err0
	}

	return conn0, conn1, nil
}

func benchConnPair(b *testing.B, c0, c1 net.Conn) {
	b.ReportAllocs()
	b.SetBytes(128 << 10)
	b.ResetTimer()

	request := make([]byte, 52)
	response := make([]byte, (128<<10)+8)

	pair := []net.Conn{c0, c1}
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			pair[0] = c0
			pair[1] = c1
		} else {
			pair[0] = c1
			pair[1] = c0
		}

		if _, err := pair[0].Write(request); err != nil {
			b.Fatal(err)
		}

		if _, err := pair[1].Read(request[:8]); err != nil {
			b.Fatal(err)
		}
		if _, err := pair[1].Read(request[8:]); err != nil {
			b.Fatal(err)
		}
		if _, err := pair[1].Write(response); err != nil {
			b.Fatal(err)
		}
		if _, err := pair[0].Read(response[:8]); err != nil {
			b.Fatal(err)
		}
		if _, err := pair[0].Read(response[8:]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSyncthingTCP(b *testing.B) {
	conn0, conn1, err := getTCPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}

	defer conn0.Close()
	defer conn1.Close()

	benchConnPair(b, conn0, conn1)
}

func BenchmarkSyncthingUDPUTP(b *testing.B) {
	conn0, conn1, err := getUTPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}

	defer conn0.Close()
	defer conn1.Close()

	benchConnPair(b, conn0, conn1)
}

func BenchmarkSyncthingInprocUTP(b *testing.B) {
	c0, c1 := connPair()
	defer c0.Close()
	defer c1.Close()
	benchConnPair(b, c0, c1)
}
