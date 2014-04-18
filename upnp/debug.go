package upnp

import (
	"log"
	"os"
	"strings"
)

var (
	dlog  = log.New(os.Stderr, "upnp: ", log.Lmicroseconds|log.Lshortfile)
	debug = strings.Contains(os.Getenv("STTRACE"), "upnp")
)
