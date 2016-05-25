package main

import (
	"fmt"
	"os"
)

func main() {
	for _, v := range os.Environ() {
		fmt.Printf("%s\n", v)
	}
}
