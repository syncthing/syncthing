package beacon

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "beacon") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
