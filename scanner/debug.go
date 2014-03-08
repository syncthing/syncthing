package scanner

import (
	"log"
	"os"
	"strings"
)

var (
	dlog  = log.New(os.Stderr, "scanner: ", log.Lmicroseconds|log.Lshortfile)
	debug = strings.Contains(os.Getenv("STTRACE"), "scanner")
)
