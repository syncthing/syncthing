package protocol

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

var toWrite = [][]byte{
	[]byte("this is a short byte string that should pass through somewhat compressed this is a short byte string that should pass through somewhat compressed this is a short byte string that should pass through somewhat compressed this is a short byte string that should pass through somewhat compressed this is a short byte string that should pass through somewhat compressed this is a short byte string that should pass through somewhat compressed"),
	[]byte("this is another short byte string that should pass through uncompressed"),
	[]byte{0, 1, 2, 3, 4, 5},
}

func TestLZ4Stream(t *testing.T) {
	tb := make([]byte, 128*1024)
	rand.Reader.Read(tb)
	toWrite = append(toWrite, tb)
	tb = make([]byte, 512*1024)
	rand.Reader.Read(tb)
	toWrite = append(toWrite, tb)
	toWrite = append(toWrite, toWrite[0])
	toWrite = append(toWrite, toWrite[1])

	rd, wr := io.Pipe()
	lz4r := newLZ4Reader(rd)
	lz4w := newLZ4Writer(wr)

	go func() {
		for i := 0; i < 5; i++ {
			for _, bs := range toWrite {
				n, err := lz4w.Write(bs)
				if err != nil {
					t.Error(err)
				}
				if n != len(bs) {
					t.Errorf("weird write length; %d != %d", n, len(bs))
				}
			}
		}
	}()

	buf := make([]byte, 512*1024)

	for i := 0; i < 5; i++ {
		for _, bs := range toWrite {
			n, err := lz4r.Read(buf)
			if err != nil {
				t.Fatal(err)
			}
			if n != len(bs) {
				t.Errorf("Unexpected len %d != %d", n, len(bs))
			}
			if bytes.Compare(bs, buf[:n]) != 0 {
				t.Error("Unexpected data")
			}
		}
	}
}
