package gbytes

import (
	"errors"
	"io"
	"time"
)

// ErrTimeout is returned by TimeoutCloser, TimeoutReader, and TimeoutWriter when the underlying Closer/Reader/Writer does not return within the specified timeout
var ErrTimeout = errors.New("timeout occurred")

// TimeoutCloser returns an io.Closer that wraps the passed-in io.Closer.  If the underlying Closer fails to close within the alloted timeout ErrTimeout is returned.
func TimeoutCloser(c io.Closer, timeout time.Duration) io.Closer {
	return timeoutReaderWriterCloser{c: c, d: timeout}
}

// TimeoutReader returns an io.Reader that wraps the passed-in io.Reader.  If the underlying Reader fails to read within the alloted timeout ErrTimeout is returned.
func TimeoutReader(r io.Reader, timeout time.Duration) io.Reader {
	return timeoutReaderWriterCloser{r: r, d: timeout}
}

// TimeoutWriter returns an io.Writer that wraps the passed-in io.Writer.  If the underlying Writer fails to write within the alloted timeout ErrTimeout is returned.
func TimeoutWriter(w io.Writer, timeout time.Duration) io.Writer {
	return timeoutReaderWriterCloser{w: w, d: timeout}
}

type timeoutReaderWriterCloser struct {
	c io.Closer
	w io.Writer
	r io.Reader
	d time.Duration
}

func (t timeoutReaderWriterCloser) Close() error {
	done := make(chan struct{})
	var err error

	go func() {
		err = t.c.Close()
		close(done)
	}()

	select {
	case <-done:
		return err
	case <-time.After(t.d):
		return ErrTimeout
	}
}

func (t timeoutReaderWriterCloser) Read(p []byte) (int, error) {
	done := make(chan struct{})
	var n int
	var err error

	go func() {
		n, err = t.r.Read(p)
		close(done)
	}()

	select {
	case <-done:
		return n, err
	case <-time.After(t.d):
		return 0, ErrTimeout
	}
}

func (t timeoutReaderWriterCloser) Write(p []byte) (int, error) {
	done := make(chan struct{})
	var n int
	var err error

	go func() {
		n, err = t.w.Write(p)
		close(done)
	}()

	select {
	case <-done:
		return n, err
	case <-time.After(t.d):
		return 0, ErrTimeout
	}
}
