// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/getsentry/raven-go"
	"github.com/maruel/panicparse/stack"
	"github.com/pkg/errors"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/logger"
)

func init() {
	// Fudge the transport first
	raven.DefaultClient.Transport = &managerAwareTransportWrapper{
		underlying: raven.DefaultClient.Transport,
	}

	// Set metadata
	buildType := ""
	if build.Version == build.UnknownVersion {
		buildType = "dev"
	} else if build.IsRelease {
		buildType = "release"
	} else if build.IsCandidate {
		buildType = "rc"
	} else if build.IsBeta {
		buildType = "beta"
	}

	raven.SetTagsContext(map[string]string{
		"os":               runtime.GOOS,
		"arch":             runtime.GOARCH,
		"goVersion":        runtime.Version(),
		"buildUser":        build.User,
		"buildHost":        build.Host,
		"buildStamp":       build.Date.String(),
		"buildCodename":    build.Codename,
		"buildTags":        fmt.Sprintf("%s", build.Tags),
		"buildIsRelease":   fmt.Sprintf("%t", build.IsRelease),
		"buildIsBeta":      fmt.Sprintf("%t", build.IsBeta),
		"buildIsCandidate": fmt.Sprintf("%t", build.IsCandidate),
		"version":          build.Version,
		"longVersion":      build.LongVersion,
		"buildType":        buildType,
	})
	raven.SetRelease(build.Version)
	raven.SetEnvironment(buildType)
	raven.SetIncludePaths([]string{"github.com/syncthing/syncthing"})

	// If release or candidate, means we have a tag on github, use a combined source code loader that gets source
	// of github.
	// Otherwise leave the loader as the default filesystem loader, as we're probably on a local dev build.
	if build.IsRelease || build.IsCandidate {
		raven.SetSourceCodeLoader(&chainedSourceCodeLoader{
			loaders: []raven.SourceCodeLoader{
				&fsLoader{
					cache: make(map[string][][]byte),
				},
				&githubSourceCodeLoader{
					cache: make(map[string][][]byte),
				},
			},
		})
	}
}

func Go(f func()) {
	go func() {
		defer ReportPanic()
		f()
	}()
}

func ReportPanic() {
	if err := recover(); err != nil {
		// Custom handling to repanic (it looks weird but what can you do)
		// Also, allows us to skip more stack frames.
		extra := raven.Extra{}
		setExtraDefaults(extra)

		var cause error
		var message string

		switch rval := err.(type) {
		case error:
			enrichExtra(extra, rval)
			cause = errors.Cause(rval)
			message = rval.Error()
		default:
			message = fmt.Sprint(rval)
			cause = errors.New(message)
		}

		currentGid := getGID()
		expectedThreadId := fmt.Sprintf("%d [%s]", currentGid, "running")

		// Construct an exception
		stacktrace := raven.GetOrNewStacktrace(cause, 3, 3, raven.IncludePaths())
		exception := &ExceptionWithThreadId{
			Exception: raven.Exception{
				Value:      cause.Error(),
				Type:       reflect.TypeOf(cause).String(),
				Module:     reflect.TypeOf(cause).PkgPath(),
				Stacktrace: stacktrace,
			},
			ThreadId: expectedThreadId,
		}

		// Get all stacktraces
		var threads []Thread
		buf := make([]byte, 4<<20)
		n := runtime.Stack(buf, true)
		buf = buf[:n]
		reader := bytes.NewReader(buf)
		ctx, err := stack.ParseDump(reader, ioutil.Discard, false)
		if err == nil {
			threads = make([]Thread, 0, len(ctx.Goroutines))
			for _, routine := range ctx.Goroutines {
				isCurrentRoutine := routine.ID == currentGid
				calls := routine.Stack.Calls
				if isCurrentRoutine {
					calls = calls[2:]
				}
				currentFrame := calls[0]
				thread := Thread{
					ID:      fmt.Sprintf("%d [%s]", routine.ID, routine.State),
					Name:    currentFrame.FullSrcLine(),
					Crashed: isCurrentRoutine,
					Current: isCurrentRoutine,
				}

				// No need to capture stack for current routine, as it uses the stack from the exception.
				if !isCurrentRoutine {
					stackSize := len(routine.Stack.Calls)
					frames := make([]*raven.StacktraceFrame, stackSize)
					for i, call := range routine.Stack.Calls {
						frame := raven.NewStacktraceFrame(0, call.Func.Name(), call.SrcPath, call.Line, 3, raven.IncludePaths())
						frames[stackSize-1-i] = frame
					}
					thread.Stacktrace = &raven.Stacktrace{
						Frames: frames,
					}
				}

				threads = append(threads, thread)
			}
		}

		// Capture log lines
		lines := logger.DefaultRecorder.Since(time.Time{})
		breadcrumbs := make([]Breadcrumb, 0, len(lines))
		for _, line := range lines {
			breadcrumbs = append(breadcrumbs, Breadcrumb{
				Timestamp: line.When,
				Type:      "default",
				Message:   line.Message,
				Level:     line.Level.String(),
				Category:  line.Facility,
			})
		}

		// Capture package information
		var packages []Package
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			packages = make([]Package, 0, len(buildInfo.Deps))
			for _, mod := range buildInfo.Deps {
				version := mod.Version
				if len(version) > 0 && version[0] == 'v' {
					version = version[1:]
				}
				packages = append(packages, Package{
					Name:    "git:https://" + mod.Path,
					Version: version,
				})
			}
		}

		packet := raven.NewPacketWithExtra(message, extra, exception, &Threads{threads}, &Breadcrumbs{breadcrumbs}, &SDK{
			Name:     "syncthing-raven-go",
			Version:  "1.2.0",
			Packages: packages,
		})
		eventID, ch := raven.Capture(packet, nil)
		if eventID != "" {
			if sendErr, ok := <-ch; ok && sendErr == nil {
				logger.DefaultLogger.Warnln("Logged panic. Event ID: " + eventID)
			}
		}

		panic(err)
	}
}

// This is lobotomised from the raven package.
func enrichExtra(extra raven.Extra, err error) {
	currentErr := err
	for currentErr != nil {
		if errWithExtra, ok := currentErr.(interface {
			error
			ExtraInfo() raven.Extra
		}); ok {
			for k, v := range errWithExtra.ExtraInfo() {
				extra[k] = v
			}
		}

		if errWithCause, ok := currentErr.(interface{ Cause() error }); ok {
			currentErr = errWithCause.Cause()
		} else {
			currentErr = nil
		}
	}
}

func setExtraDefaults(extra raven.Extra) {
	extra["runtime.Version"] = runtime.Version()
	extra["runtime.NumCPU"] = runtime.NumCPU()
	extra["runtime.GOMAXPROCS"] = runtime.GOMAXPROCS(0) // 0 just returns the current value
	extra["runtime.NumGoroutine"] = runtime.NumGoroutine()
	extra["runtime.GOOS"] = runtime.GOOS
	extra["runtime.GOARCH"] = runtime.GOARCH
}

func getGID() int {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return int(n)
}
