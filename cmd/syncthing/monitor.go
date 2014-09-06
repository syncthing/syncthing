// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	stdoutFirstLines []string // The first 10 lines of stdout
	stdoutLastLines  []string // The last 50 lines of stdout
	stdoutMut        sync.Mutex
)

const (
	countRestarts = 5
	loopThreshold = 15 * time.Second
)

func monitorMain() {
	os.Setenv("STNORESTART", "yes")
	l.SetPrefix("[monitor] ")

	args := os.Args
	var restarts [countRestarts]time.Time

	for {
		if t := time.Since(restarts[0]); t < loopThreshold {
			l.Warnf("%d restarts in %v; not retrying further", countRestarts, t)
			os.Exit(exitError)
		}

		copy(restarts[0:], restarts[1:])
		restarts[len(restarts)-1] = time.Now()

		cmd := exec.Command(args[0], args[1:]...)

		stderr, err := cmd.StderrPipe()
		if err != nil {
			l.Fatalln(err)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			l.Fatalln(err)
		}

		l.Infoln("Starting syncthing")
		err = cmd.Start()
		if err != nil {
			l.Fatalln(err)
		}

		stdoutMut.Lock()
		stdoutFirstLines = make([]string, 0, 10)
		stdoutLastLines = make([]string, 0, 50)
		stdoutMut.Unlock()

		go copyStderr(stderr)
		go copyStdout(stdout)

		err = cmd.Wait()
		if err == nil {
			// Successfull exit indicates an intentional shutdown
			return
		}

		l.Infoln("Syncthing exited:", err)
		time.Sleep(1 * time.Second)
	}
}

func copyStderr(stderr io.ReadCloser) {
	br := bufio.NewReader(stderr)

	var panicFd *os.File
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				l.Warnln("stderr:", err)
			}
			return
		}

		if panicFd == nil {
			os.Stderr.WriteString(line)

			if strings.HasPrefix(line, "panic:") || strings.HasPrefix(line, "fatal error:") {
				panicFd, err = os.Create(filepath.Join(confDir, time.Now().Format("panic-20060102-150405.log")))
				if err != nil {
					l.Warnln("Create panic log:", err)
					continue
				}

				l.Warnf("Panic detected, writing to \"%s\"", panicFd.Name())
				l.Warnln("Please create an issue at https://github.com/syncting/syncthing/issues/ with the panic log attached")

				stdoutMut.Lock()
				for _, line := range stdoutFirstLines {
					panicFd.WriteString(line)
				}
				panicFd.WriteString("...\n")
				for _, line := range stdoutLastLines {
					panicFd.WriteString(line)
				}
			}
		}

		if panicFd != nil {
			panicFd.WriteString(line)
		}
	}
}

func copyStdout(stderr io.ReadCloser) {
	br := bufio.NewReader(stderr)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				l.Warnln("stdout:", err)
			}
			return
		}

		stdoutMut.Lock()
		if len(stdoutFirstLines) < cap(stdoutFirstLines) {
			stdoutFirstLines = append(stdoutFirstLines, line)
		}
		if l := len(stdoutLastLines); l == cap(stdoutLastLines) {
			stdoutLastLines = stdoutLastLines[:l-1]
		}
		stdoutLastLines = append(stdoutLastLines, line)
		stdoutMut.Unlock()

		os.Stdout.WriteString(line)
	}
}
