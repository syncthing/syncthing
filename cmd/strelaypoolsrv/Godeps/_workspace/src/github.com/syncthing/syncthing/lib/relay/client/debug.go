// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("relay", "")
)

func init() {
	l.SetDebug("relay", strings.Contains(os.Getenv("STTRACE"), "relay") || os.Getenv("STTRACE") == "all")
}
