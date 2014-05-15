package mc

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "mc") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
