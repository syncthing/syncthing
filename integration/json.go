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
