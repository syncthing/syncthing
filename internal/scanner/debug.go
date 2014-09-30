// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package scanner

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/internal/logger"
)

var (
	debug = strings.Contains(os.Getenv("STTRACE"), "scanner") || os.Getenv("STTRACE") == "all"
	l     = logger.DefaultLogger
)
