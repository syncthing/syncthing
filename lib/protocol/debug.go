// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("protocol", "The BEP protocol")
)

func init() {
	l.SetDebug("protocol", strings.Contains(os.Getenv("STTRACE"), "protocol") || os.Getenv("STTRACE") == "all")
}
