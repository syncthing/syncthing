package scanner

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "scanner") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
