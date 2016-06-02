package utp

import (
	"fmt"
	"io"
)

func WriteStatus(w io.Writer) {
	mu.RLock()
	defer mu.RUnlock()
	for s := range sockets {
		writeSocketStatus(w, s)
		fmt.Fprintf(w, "\n")
	}
}

func writeSocketStatus(w io.Writer, s *Socket) {
	fmt.Fprintf(w, "%s\n", s.pc.LocalAddr())
	fmt.Fprintf(w, "%d attached conns\n", len(s.conns))
	fmt.Fprintf(w, "backlog: %d\n", len(s.backlog))
	fmt.Fprintf(w, "closed: %v\n", s.closed.IsSet())
	fmt.Fprintf(w, "unused reads: %d\n", len(s.unusedReads))
	fmt.Fprintf(w, "readerr: %v\n", s.ReadErr)
}
