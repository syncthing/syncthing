package protocol

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "protocol") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
