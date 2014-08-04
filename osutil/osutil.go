// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package osutil implements utilities for native OS support.
package osutil

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"time"
)

func Rename(from, to string) error {
	if runtime.GOOS == "windows" {
		os.Chmod(to, 0666) // Make sure the file is user writeable
		err := os.Remove(to)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	defer os.Remove(from) // Don't leave a dangling temp file in case of rename error
	return os.Rename(from, to)
}

func GetLockPort() (*net.TCPListener, int, error) {
	lockConn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IP{127, 0, 0, 1}})
	if err != nil {
		return nil, 0, err
	}
	addr := lockConn.Addr().(*net.TCPAddr)
	return lockConn, addr.Port, nil
}

func WaitForParentExit() error {
	lockPortStr := os.Getenv("STRESTART")
	lockPort, err := strconv.Atoi(lockPortStr)
	if err != nil {
		return fmt.Errorf("Invalid lock port %q: %v", lockPortStr, err)
	}
	// Wait for the listen address to become free, indicating that the parent has exited.
	for {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", lockPort))
		if err == nil {
			ln.Close()
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}
