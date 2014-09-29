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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	log.SetFlags(0)
	flag.Parse()
	path := strings.Split(flag.Arg(0), "/")

	var obj map[string]interface{}
	dec := json.NewDecoder(os.Stdin)
	dec.UseNumber()
	dec.Decode(&obj)

	var v interface{} = obj
	for _, p := range path {
		switch tv := v.(type) {
		case map[string]interface{}:
			v = tv[p]
		case []interface{}:
			i, err := strconv.Atoi(p)
			if err != nil {
				log.Fatal(err)
			}
			v = tv[i]
		default:
			return // Silence is golden
		}
	}
	fmt.Println(v)
}
