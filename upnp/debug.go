package upnp

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "upnp") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
