package versioner

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "versioner") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
