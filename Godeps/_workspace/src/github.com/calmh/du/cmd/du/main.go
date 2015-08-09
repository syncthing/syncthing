package main

import (
	"fmt"
	"log"
	"os"

	"github.com/calmh/du"
)

var KB = int64(1024)

func main() {
	usage, err := du.Get(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Free:", usage.FreeBytes/(KB*KB), "MiB")
	fmt.Println("Available:", usage.AvailBytes/(KB*KB), "MiB")
	fmt.Println("Size:", usage.TotalBytes/(KB*KB), "MiB")
}
