// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	os.Setenv("STMONITORED", "yes")
	l.SetPrefix("[monitor] ")

	args := os.Args
	var restarts [countRestarts]time.Time

	sign := make(chan os.Signal, 1)
	sigTerm := syscall.Signal(0xf)
	signal.Notify(sign, os.Interrupt, sigTerm, os.Kill)

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

		// Let the next child process know that this is not the first time
		// it's starting up.
		os.Setenv("STRESTART", "yes")

		stdoutMut.Lock()
		stdoutFirstLines = make([]string, 0, 10)
		stdoutLastLines = make([]string, 0, 50)
		stdoutMut.Unlock()

		go copyStderr(stderr)
		go copyStdout(stdout)

		exit := make(chan error)

		go func() {
			exit <- cmd.Wait()
		}()

		select {
		case s := <-sign:
			l.Infof("Signal %d received; exiting", s)
			cmd.Process.Kill()
			<-exit
			return

		case err = <-exit:
			if err == nil {
				// Successfull exit indicates an intentional shutdown
				return
			} else if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					switch status.ExitStatus() {
					case exitUpgrading:
						// Restart the monitor process to release the .old
						// binary as part of the upgrade process.
						l.Infoln("Restarting monitor...")
						os.Setenv("STNORESTART", "")
						err := exec.Command(args[0], args[1:]...).Start()
						if err != nil {
							l.Warnln("restart:", err)
						}
						return
					}
				}
			}
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
				l.Warnln("Please create an issue at https://github.com/syncthing/syncthing/issues/ with the panic log attached")

				stdoutMut.Lock()
				for _, line := range stdoutFirstLines {
					panicFd.WriteString(line)
				}
				panicFd.WriteString("...\n")
				for _, line := range stdoutLastLines {
					panicFd.WriteString(line)
				}
				stdoutMut.Unlock()
			}

			panicFd.WriteString("Panic at " + time.Now().Format(time.RFC3339) + "\n")
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
			return
		}

		stdoutMut.Lock()
		if len(stdoutFirstLines) < cap(stdoutFirstLines) {
			stdoutFirstLines = append(stdoutFirstLines, line)
		} else {
			if l := len(stdoutLastLines); l == cap(stdoutLastLines) {
				stdoutLastLines = stdoutLastLines[:l-1]
			}
			stdoutLastLines = append(stdoutLastLines, line)
		}
		stdoutMut.Unlock()

		os.Stdout.WriteString(line)
	}
}
