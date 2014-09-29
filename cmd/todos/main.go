// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build ignore

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
