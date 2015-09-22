// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr

import (
	"log"
	"os"
)

var (
	debug = len(os.Getenv("XDRTRACE")) > 0
	dl    = log.New(os.Stdout, "xdr: ", log.Lshortfile|log.Ltime|log.Lmicroseconds)
)

const maxDebugBytes = 32
