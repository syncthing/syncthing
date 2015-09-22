// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"os"
	"strings"

	"github.com/calmh/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "protocol") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
