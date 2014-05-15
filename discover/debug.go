package discover

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "discover") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
