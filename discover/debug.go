package discover

import (
	"log"
	"os"
	"strings"
)

var (
	dlog  = log.New(os.Stderr, "discover: ", log.Lmicroseconds|log.Lshortfile)
	debug = strings.Contains(os.Getenv("STTRACE"), "discover")
)
