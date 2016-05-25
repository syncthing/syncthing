package main

import (
	"fmt"
	"net/url"
	"os"
)

func main() {
	fmt.Println(url.QueryUnescape(os.Args[1]))
}
