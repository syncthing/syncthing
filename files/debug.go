package files

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "files") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
