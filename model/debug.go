package model

import (
	"os"
	"strings"

	"github.com/calmh/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "model") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
