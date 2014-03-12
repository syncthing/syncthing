// Package buffers manages a set of reusable byte buffers.
package buffers

const (
	largeMin = 1024
)

var (
	smallBuffers = make(chan []byte, 32)
	largeBuffers = make(chan []byte, 32)
)

func Get(size int) []byte {
	var ch = largeBuffers
	if size < largeMin {
		ch = smallBuffers
	}

	var buf []byte
	select {
	case buf = <-ch:
	default:
	}

	if len(buf) < size {
		return make([]byte, size)
	}
	return buf[:size]
}

func Put(buf []byte) {
	buf = buf[:cap(buf)]
	if len(buf) == 0 {
		return
	}

	var ch = largeBuffers
	if len(buf) < largeMin {
		ch = smallBuffers
	}

	select {
	case ch <- buf:
	default:
	}
}
