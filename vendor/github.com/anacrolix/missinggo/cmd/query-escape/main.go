package main

import (
	"fmt"
	"net/url"
	"os"
)

func main() {
	fmt.Println(url.QueryEscape(os.Args[1]))
}
