// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/getsentry/raven-go"
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
		"buildTags":        strings.Join(build.Tags, ","),
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

		stacktrace := raven.GetOrNewStacktrace(cause, 3, 3, raven.IncludePaths())
		exception := raven.NewException(cause, stacktrace)

		packet := raven.NewPacketWithExtra(message, extra, exception)
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
