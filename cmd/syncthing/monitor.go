// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"
)

var (
	stdoutFirstLines []string // The first 10 lines of stdout
	stdoutLastLines  []string // The last 50 lines of stdout
	stdoutMut        = sync.NewMutex()
)

const (
	countRestarts = 4
	loopThreshold = 60 * time.Second
)

func monitorMain() {
	os.Setenv("STNORESTART", "yes")
	os.Setenv("STMONITORED", "yes")
	l.SetPrefix("[monitor] ")

	var err error
	var dst io.Writer = os.Stdout

	if logFile != "-" {
		var fileDst io.Writer

		fileDst, err = os.Create(logFile)
		if err != nil {
			l.Fatalln("log file:", err)
		}

		if runtime.GOOS == "windows" {
			// Translate line breaks to Windows standard
			fileDst = osutil.ReplacingWriter{
				Writer: fileDst,
				From:   '\n',
				To:     []byte{'\r', '\n'},
			}
		}

		// Log to both stdout and file.
		dst = io.MultiWriter(dst, fileDst)

		l.Infof(`Log output saved to file "%s"`, logFile)
	}

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
			l.Fatalln("stderr:", err)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			l.Fatalln("stdout:", err)
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

		wg := sync.NewWaitGroup()

		wg.Add(1)
		go func() {
			copyStderr(stderr, dst)
			wg.Done()
		}()

		wg.Add(1)
		go func() {
			copyStdout(stdout, dst)
			wg.Done()
		}()

		exit := make(chan error)

		go func() {
			wg.Wait()
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
				// Successful exit indicates an intentional shutdown
				return
			} else if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					switch status.ExitStatus() {
					case exitUpgrading:
						// Restart the monitor process to release the .old
						// binary as part of the upgrade process.
						l.Infoln("Restarting monitor...")
						os.Setenv("STNORESTART", "")
						if err = restartMonitor(args); err != nil {
							l.Warnln("Restart:", err)
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

func copyStderr(stderr io.Reader, dst io.Writer) {
	br := bufio.NewReader(stderr)

	var panicFd *os.File
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}

		if panicFd == nil {
			dst.Write([]byte(line))

			if strings.HasPrefix(line, "panic:") || strings.HasPrefix(line, "fatal error:") {
				panicFd, err = os.Create(timestampedLoc(locPanicLog))
				if err != nil {
					l.Warnln("Create panic log:", err)
					continue
				}

				l.Warnf("Panic detected, writing to \"%s\"", panicFd.Name())
				l.Warnln("Please check for existing issues with similar panic message at https://github.com/syncthing/syncthing/issues/")
				l.Warnln("If no issue with similar panic message exists, please create a new issue with the panic log attached")

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

func copyStdout(stdout io.Reader, dst io.Writer) {
	br := bufio.NewReader(stdout)
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

		dst.Write([]byte(line))
	}
}

func restartMonitor(args []string) error {
	if runtime.GOOS != "windows" {
		// syscall.Exec is the cleanest way to restart on Unixes as it
		// replaces the current process with the new one, keeping the pid and
		// controlling terminal and so on
		return restartMonitorUnix(args)
	}

	// but it isn't supported on Windows, so there we start a normal
	// exec.Command and return.
	return restartMonitorWindows(args)
}

func restartMonitorUnix(args []string) error {
	if !strings.ContainsRune(args[0], os.PathSeparator) {
		// The path to the binary doesn't contain a slash, so it should be
		// found in $PATH.
		binary, err := exec.LookPath(args[0])
		if err != nil {
			return err
		}
		args[0] = binary
	}

	return syscall.Exec(args[0], args, os.Environ())
}

func restartMonitorWindows(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	// Retain the standard streams
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	return cmd.Start()
}
