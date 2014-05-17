package xdr

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "xdr") || os.Getenv("STTRACE") == "all"
	dl    = logger.DefaultLogger
)

const maxDebugBytes = 32
