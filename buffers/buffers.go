package buffers

var buffers = make(chan []byte, 32)

func Get(size int) []byte {
	var buf []byte
	select {
	case buf = <-buffers:
	default:
	}
	if len(buf) < size {
		return make([]byte, size)
	}
	return buf[:size]
}

func Put(buf []byte) {
	if cap(buf) == 0 {
		return
	}
	buf = buf[:cap(buf)]
	select {
	case buffers <- buf:
	default:
	}
}
