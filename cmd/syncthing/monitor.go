// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/svcutil"
)

var (
	stdoutFirstLines []string // The first 10 lines of stdout
	stdoutLastLines  []string // The last 50 lines of stdout
	stdoutMut        sync.Mutex
)

const (
	restartCounts         = 4
	restartPause          = 1 * time.Second
	restartLoopThreshold  = 60 * time.Second
	logFileAutoCloseDelay = 5 * time.Second
	logFileMaxOpenTime    = time.Minute
	panicUploadMaxWait    = 30 * time.Second
	panicUploadNoticeWait = 10 * time.Second
)

func (c *serveCmd) monitorMain() {
	var dst io.Writer = os.Stdout

	logFile := locations.Get(locations.LogFile)
	if logFile != "-" {
		if expanded, err := fs.ExpandTilde(logFile); err == nil {
			logFile = expanded
		}
		var fileDst io.Writer
		var err error
		open := func(name string) (io.WriteCloser, error) {
			return newAutoclosedFile(name, logFileAutoCloseDelay, logFileMaxOpenTime)
		}
		if c.LogMaxSize > 0 {
			fileDst, err = newRotatedFile(logFile, open, int64(c.LogMaxSize), c.LogMaxFiles)
		} else {
			fileDst, err = open(logFile)
		}
		if err != nil {
			slog.Error("Failed to set up logging to file, proceeding with logging to stdout only", slogutil.Error(err))
		} else {
			if build.IsWindows {
				// Translate line breaks to Windows standard
				fileDst = osutil.ReplacingWriter{
					Writer: fileDst,
					From:   '\n',
					To:     []byte{'\r', '\n'},
				}
			}

			// Log to both stdout and file.
			dst = io.MultiWriter(dst, fileDst)

			slog.Info("Saved log output", slogutil.FilePath(logFile))
		}
	}

	args := os.Args
	binary, err := getBinary(args[0])
	if err != nil {
		slog.Error("Failed to start the main Syncthing process", slogutil.Error(err))
		panic("Error starting the main Syncthing process")
	}
	var restarts [restartCounts]time.Time

	stopSign := make(chan os.Signal, 1)
	signal.Notify(stopSign, os.Interrupt, sigTerm)
	restartSign := make(chan os.Signal, 1)
	sigHup := syscall.Signal(1)
	signal.Notify(restartSign, sigHup)

	childEnv := childEnv()
	first := true
	for {
		maybeReportPanics()

		if t := time.Since(restarts[0]); t < restartLoopThreshold {
			slog.Error("Too many restarts; not retrying further", slog.Int("count", restartCounts), slog.Any("interval", t))
			os.Exit(svcutil.ExitError.AsInt())
		}

		copy(restarts[0:], restarts[1:])
		restarts[len(restarts)-1] = time.Now()

		cmd := exec.Command(binary, args[1:]...)
		cmd.Env = childEnv

		stderr, err := cmd.StderrPipe()
		if err != nil {
			panic(err)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}

		slog.Debug("Starting syncthing")
		err = cmd.Start()
		if err != nil {
			slog.Error("Failed to start the main Syncthing process", slogutil.Error(err))
			panic("Error starting the main Syncthing process")
		}

		stdoutMut.Lock()
		stdoutFirstLines = make([]string, 0, 10)
		stdoutLastLines = make([]string, 0, 50)
		stdoutMut.Unlock()

		var wg sync.WaitGroup

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

		stopped := false
		select {
		case s := <-stopSign:
			slog.Info("Received signal; exiting", "signal", s)
			cmd.Process.Signal(sigTerm)
			err = <-exit
			stopped = true

		case s := <-restartSign:
			slog.Info("Received signal; restarting", "signal", s)
			cmd.Process.Signal(sigHup)
			err = <-exit

		case err = <-exit:
		}

		if err == nil {
			// Successful exit indicates an intentional shutdown
			os.Exit(svcutil.ExitSuccess.AsInt())
		}

		exiterr := &exec.ExitError{}
		if errors.As(err, &exiterr) {
			exitCode := exiterr.ExitCode()
			if stopped || c.NoRestart {
				os.Exit(exitCode)
			}
			if exitCode == svcutil.ExitUpgrade.AsInt() {
				// Restart the monitor process to release the .old
				// binary as part of the upgrade process.
				slog.Info("Restarting monitor...")
				if err = restartMonitor(binary, args); err != nil {
					slog.Error("Failed to restart monitor", slogutil.Error(err))
				}
				os.Exit(exitCode)
			}
		}

		if c.NoRestart {
			os.Exit(svcutil.ExitError.AsInt())
		}

		slog.Info("Syncthing exited", slogutil.Error(err))
		time.Sleep(restartPause)

		if first {
			// Let the next child process know that this is not the first time
			// it's starting up.
			childEnv = append(childEnv, "STRESTART=yes")
			first = false
		}
	}
}

func getBinary(args0 string) (string, error) {
	e, err := os.Executable()
	if err == nil {
		return e, nil
	}
	// Check if args0 cuts it
	e, lerr := exec.LookPath(args0)
	if lerr == nil {
		return e, nil
	}
	return "", err
}

func copyStderr(stderr io.Reader, dst io.Writer) {
	br := bufio.NewReader(stderr)

	var panicFd *os.File
	defer func() {
		if panicFd != nil {
			_ = panicFd.Close()
			maybeReportPanics()
		}
	}()

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}

		dst.Write([]byte(line))

		if panicFd == nil && (strings.HasPrefix(line, "panic:") || strings.HasPrefix(line, "fatal error:")) {
			panicFd, err = os.Create(locations.GetTimestamped(locations.PanicLog))
			if err != nil {
				slog.Error("Failed to create panic log", slogutil.Error(err))
				continue
			}

			slog.Error("Panic detected, writing to file", slogutil.FilePath(panicFd.Name()))
			slog.Info("Please check for existing issues with similar panic message at https://github.com/syncthing/syncthing/issues/")
			slog.Info("If no issue with similar panic message exists, please create a new issue with the panic log attached")

			stdoutMut.Lock()
			for _, line := range stdoutFirstLines {
				panicFd.WriteString(line)
			}
			panicFd.WriteString("...\n")
			for _, line := range stdoutLastLines {
				panicFd.WriteString(line)
			}
			stdoutMut.Unlock()

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

func restartMonitor(binary string, args []string) error {
	// Set the STRESTART environment variable to indicate to the next
	// process that this is a restart and not initial start. This prevents
	// opening the browser on startup.
	os.Setenv("STRESTART", "yes")

	if !build.IsWindows {
		// syscall.Exec is the cleanest way to restart on Unixes as it
		// replaces the current process with the new one, keeping the pid and
		// controlling terminal and so on
		return restartMonitorUnix(binary, args)
	}

	// but it isn't supported on Windows, so there we start a normal
	// exec.Command and return.
	return restartMonitorWindows(binary, args)
}

func restartMonitorUnix(binary string, args []string) error {
	return syscall.Exec(binary, args, os.Environ())
}

func restartMonitorWindows(binary string, args []string) error {
	cmd := exec.Command(binary, args[1:]...)
	// Retain the standard streams
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	return cmd.Start()
}

// rotatedFile keeps a set of rotating logs. There will be the base file plus up
// to maxFiles rotated ones, each ~ maxSize bytes large.
type rotatedFile struct {
	name        string
	create      createFn
	maxSize     int64 // bytes
	maxFiles    int
	currentFile io.WriteCloser
	currentSize int64
}

type createFn func(name string) (io.WriteCloser, error)

func newRotatedFile(name string, create createFn, maxSize int64, maxFiles int) (*rotatedFile, error) {
	var size int64
	if info, err := os.Lstat(name); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		size = 0
	} else {
		size = info.Size()
	}
	writer, err := create(name)
	if err != nil {
		return nil, err
	}
	return &rotatedFile{
		name:        name,
		create:      create,
		maxSize:     maxSize,
		maxFiles:    maxFiles,
		currentFile: writer,
		currentSize: size,
	}, nil
}

func (r *rotatedFile) Write(bs []byte) (int, error) {
	// Check if we're about to exceed the max size, and if so close this
	// file so we'll start on a new one.
	if r.currentSize+int64(len(bs)) > r.maxSize {
		r.currentFile.Close()
		r.currentSize = 0
		r.rotate()
		f, err := r.create(r.name)
		if err != nil {
			return 0, err
		}
		r.currentFile = f
	}

	n, err := r.currentFile.Write(bs)
	r.currentSize += int64(n)
	return n, err
}

func (r *rotatedFile) rotate() {
	// The files are named "name", "name.0", "name.1", ...
	// "name.(r.maxFiles-1)". Increase the numbers on the
	// suffixed ones.
	for i := r.maxFiles - 1; i > 0; i-- {
		from := numberedFile(r.name, i-1)
		to := numberedFile(r.name, i)
		err := os.Rename(from, to)
		if err != nil && !os.IsNotExist(err) {
			fmt.Println("LOG: Rotating logs:", err)
		}
	}

	// Rename the base to base.0
	err := os.Rename(r.name, numberedFile(r.name, 0))
	if err != nil && !os.IsNotExist(err) {
		fmt.Println("LOG: Rotating logs:", err)
	}
}

// numberedFile adds the number between the file name and the extension.
func numberedFile(name string, num int) string {
	ext := filepath.Ext(name) // contains the dot
	withoutExt := name[:len(name)-len(ext)]
	return fmt.Sprintf("%s.%d%s", withoutExt, num, ext)
}

// An autoclosedFile is an io.WriteCloser that opens itself for appending on
// Write() and closes itself after an interval of no writes (closeDelay) or
// when the file has been open for too long (maxOpenTime). A call to Write()
// will return any error that happens on the resulting Open() call too. Errors
// on automatic Close() calls are silently swallowed...
type autoclosedFile struct {
	name        string        // path to write to
	closeDelay  time.Duration // close after this long inactivity
	maxOpenTime time.Duration // or this long after opening

	fd         io.WriteCloser // underlying WriteCloser
	opened     time.Time      // timestamp when the file was last opened
	closed     chan struct{}  // closed on Close(), stops the closerLoop
	closeTimer *time.Timer    // fires closeDelay after a write

	mut sync.Mutex
}

func newAutoclosedFile(name string, closeDelay, maxOpenTime time.Duration) (*autoclosedFile, error) {
	f := &autoclosedFile{
		name:        name,
		closeDelay:  closeDelay,
		maxOpenTime: maxOpenTime,
		closed:      make(chan struct{}),
		closeTimer:  time.NewTimer(time.Minute),
	}
	f.mut.Lock()
	defer f.mut.Unlock()
	if err := f.ensureOpenLocked(); err != nil {
		return nil, err
	}
	go f.closerLoop()
	return f, nil
}

func (f *autoclosedFile) Write(bs []byte) (int, error) {
	f.mut.Lock()
	defer f.mut.Unlock()

	// Make sure the file is open for appending
	if err := f.ensureOpenLocked(); err != nil {
		return 0, err
	}

	// If we haven't run into the maxOpenTime, postpone close for another
	// closeDelay
	if time.Since(f.opened) < f.maxOpenTime {
		f.closeTimer.Reset(f.closeDelay)
	}

	return f.fd.Write(bs)
}

func (f *autoclosedFile) Close() error {
	f.mut.Lock()
	defer f.mut.Unlock()

	// Stop the timer and closerLoop() routine
	f.closeTimer.Stop()
	close(f.closed)

	// Close the file, if it's open
	if f.fd != nil {
		return f.fd.Close()
	}

	return nil
}

// Must be called with f.mut held!
func (f *autoclosedFile) ensureOpenLocked() error {
	if f.fd != nil {
		// File is already open
		return nil
	}

	// We open the file for write only, and create it if it doesn't exist.
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND

	fd, err := os.OpenFile(f.name, flags, 0o644)
	if err != nil {
		return err
	}

	f.fd = fd
	f.opened = time.Now()
	return nil
}

func (f *autoclosedFile) closerLoop() {
	for {
		select {
		case <-f.closeTimer.C:
			// Close the file when the timer expires.
			f.mut.Lock()
			if f.fd != nil {
				f.fd.Close() // errors, schmerrors
				f.fd = nil
			}
			f.mut.Unlock()

		case <-f.closed:
			return
		}
	}
}

// Returns the desired child environment, properly filtered and added to.
func childEnv() []string {
	var env []string
	for _, str := range os.Environ() {
		if strings.HasPrefix(str, "STNORESTART=") {
			continue
		}
		if strings.HasPrefix(str, "STMONITORED=") {
			continue
		}
		env = append(env, str)
	}
	env = append(env, "STMONITORED=yes")
	return env
}

// maybeReportPanics tries to figure out if crash reporting is on or off,
// and reports any panics it can find if it's enabled. We spend at most
// panicUploadMaxWait uploading panics...
func maybeReportPanics() {
	// Try to get a config to see if/where panics should be reported.
	cfg, err := loadOrDefaultConfig()
	if err != nil {
		slog.Error("Couldn't load config; not reporting crash")
		return
	}

	// Bail if we're not supposed to report panics.
	opts := cfg.Options()
	if !opts.CREnabled {
		return
	}

	// Set up a timeout on the whole operation.
	ctx, cancel := context.WithTimeout(context.Background(), panicUploadMaxWait)
	defer cancel()

	// Print a notice if the upload takes a long time.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(panicUploadNoticeWait):
			slog.Warn("Uploading crash reports is taking a while, please wait")
		}
	}()

	// Report the panics.
	dir := locations.GetBaseDir(locations.ConfigBaseDir)
	uploadPanicLogs(ctx, opts.CRURL, dir)
}
