package protocol

import (
	"errors"
	"io"
	"testing"
	"testing/quick"
	"time"
)

func TestHeaderFunctions(t *testing.T) {
	f := func(ver, id, typ int) bool {
		ver = int(uint(ver) % 16)
		id = int(uint(id) % 4096)
		typ = int(uint(typ) % 256)
		h0 := header{ver, id, typ}
		h1 := decodeHeader(encodeHeader(h0))
		return h0 == h1
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestPing(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection("c0", ar, bw, nil, nil)
	c1 := NewConnection("c1", br, aw, nil, nil)

	if ok := c0.ping(); !ok {
		t.Error("c0 ping failed")
	}
	if ok := c1.ping(); !ok {
		t.Error("c1 ping failed")
	}
}

func TestPingErr(t *testing.T) {
	e := errors.New("Something broke")

	for i := 0; i < 12; i++ {
		for j := 0; j < 12; j++ {
			m0 := &TestModel{}
			m1 := &TestModel{}

			ar, aw := io.Pipe()
			br, bw := io.Pipe()
			eaw := &ErrPipe{PipeWriter: *aw, max: i, err: e}
			ebw := &ErrPipe{PipeWriter: *bw, max: j, err: e}

			c0 := NewConnection("c0", ar, ebw, m0, nil)
			NewConnection("c1", br, eaw, m1, nil)

			res := c0.ping()
			if (i < 4 || j < 4) && res {
				t.Errorf("Unexpected ping success; i=%d, j=%d", i, j)
			} else if (i >= 8 && j >= 8) && !res {
				t.Errorf("Unexpected ping fail; i=%d, j=%d", i, j)
			}
		}
	}
}

func TestRequestResponseErr(t *testing.T) {
	e := errors.New("Something broke")

	var pass bool
	for i := 0; i < 48; i++ {
		for j := 0; j < 38; j++ {
			m0 := &TestModel{data: []byte("response data")}
			m1 := &TestModel{}

			ar, aw := io.Pipe()
			br, bw := io.Pipe()
			eaw := &ErrPipe{PipeWriter: *aw, max: i, err: e}
			ebw := &ErrPipe{PipeWriter: *bw, max: j, err: e}

			NewConnection("c0", ar, ebw, m0, nil)
			c1 := NewConnection("c1", br, eaw, m1, nil)

			d, err := c1.Request("default", "tn", 1234)
			if err == e || err == ErrClosed {
				t.Logf("Error at %d+%d bytes", i, j)
				if !m1.closed {
					t.Error("c1 not closed")
				}
				time.Sleep(1 * time.Millisecond)
				if !m0.closed {
					t.Error("c0 not closed")
				}
				continue
			}
			if err != nil {
				t.Error(err)
			}
			if string(d) != "response data" {
				t.Errorf("Incorrect response data %q", string(d))
			}
			if m0.repo != "default" {
				t.Error("Incorrect repo %q", m0.repo)
			}
			if m0.name != "tn" {
				t.Error("Incorrect name %q", m0.name)
			}
			if m0.offset != 1234*BlockSize {
				t.Error("Incorrect offset %d", m0.offset)
			}
			if m0.size != BlockSize {
				t.Error("Incorrect size %d", m0.size)
			}
			t.Logf("Pass at %d+%d bytes", i, j)
			pass = true
		}
	}
	if !pass {
		t.Error("Never passed")
	}
}

func TestVersionErr(t *testing.T) {
	m0 := &TestModel{}
	m1 := &TestModel{}

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection("c0", ar, bw, m0, nil)
	NewConnection("c1", br, aw, m1, nil)

	c0.xw.WriteUint32(encodeHeader(header{
		version: 2,
		msgID:   0,
		msgType: 0,
	}))
	c0.flush()

	if !m1.closed {
		t.Error("Connection should close due to unknown version")
	}
}

func TestTypeErr(t *testing.T) {
	m0 := &TestModel{}
	m1 := &TestModel{}

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection("c0", ar, bw, m0, nil)
	NewConnection("c1", br, aw, m1, nil)

	c0.xw.WriteUint32(encodeHeader(header{
		version: 0,
		msgID:   0,
		msgType: 42,
	}))
	c0.flush()

	if !m1.closed {
		t.Error("Connection should close due to unknown message type")
	}
}

func TestClose(t *testing.T) {
	m0 := &TestModel{}
	m1 := &TestModel{}

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection("c0", ar, bw, m0, nil)
	NewConnection("c1", br, aw, m1, nil)

	c0.close(nil)

	ok := c0.isClosed()
	if !ok {
		t.Fatal("Connection should be closed")
	}

	// None of these should panic, some should return an error

	ok = c0.ping()
	if ok {
		t.Error("Ping should not return true")
	}

	c0.Index("default", nil)
	c0.Index("default", nil)

	_, err := c0.Request("default", "foo", 0)
	if err == nil {
		t.Error("Request should return an error")
	}
}
