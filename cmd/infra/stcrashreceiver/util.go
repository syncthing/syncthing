// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// userIDFor returns a string we can use as the user ID for the purpose of
// counting affected users. It's the truncated hash of a salt, the user
// remote IP, and the current month.
func userIDFor(req *http.Request) string {
	addr := req.RemoteAddr
	if fwd := req.Header.Get("x-forwarded-for"); fwd != "" {
		addr = fwd
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	now := time.Now().Format("200601")
	salt := "stcrashreporter"
	hash := sha256.Sum256([]byte(salt + addr + now))
	return fmt.Sprintf("%x", hash[:8])
}

// 01234567890abcdef... => 01/23
func dirFor(base string) string {
	return filepath.Join(base[0:2], base[2:4])
}

func fullPathCompressed(root, reportID string) string {
	return filepath.Join(root, dirFor(reportID), reportID) + ".gz"
}

func compressAndWrite(bs []byte, fullPath string) error {
	// Compress the report for storage
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	_, _ = gw.Write(bs) // can't fail
	gw.Close()

	// Create an output file with the compressed report
	return os.WriteFile(fullPath, buf.Bytes(), 0o644)
}
