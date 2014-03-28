package files

import (
	"log"
	"os"
	"strings"
)

var (
	dlog  = log.New(os.Stderr, "files: ", log.Lmicroseconds|log.Lshortfile)
	debug = strings.Contains(os.Getenv("STTRACE"), "files")
)
