package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

func main() {
	buf := make([]byte, 4096)
	var err error
	for err == nil {
		n, err := io.ReadFull(os.Stdin, buf)
		if n > 0 {
			buf = buf[:n]
			repl := bytes.Replace(buf, []byte("\n"), []byte("\r\n"), -1)
			_, err = os.Stdout.Write(repl)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		if err == io.EOF {
			return
		}
		buf = buf[:cap(buf)]
	}
	fmt.Println(err)
	os.Exit(1)
}
