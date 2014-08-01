// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package protocol

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "protocol") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
