// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	raven "github.com/getsentry/raven-go"
	"github.com/maruel/panicparse/stack"
)

const reportServer = "https://crash.syncthing.net/report/"

var loader = newGithubSourceCodeLoader()

func init() {
	raven.SetSourceCodeLoader(loader)
}

var (
	clients    = make(map[string]*raven.Client)
	clientsMut sync.Mutex
)

func sendReport(dsn, path string, report []byte, userID string) error {
	pkt, err := parseReport(path, report)
	if err != nil {
		return err
	}

	pkt.Interfaces = append(pkt.Interfaces, &raven.User{ID: userID})

	clientsMut.Lock()
	defer clientsMut.Unlock()

	cli, ok := clients[dsn]
	if !ok {
		cli, err = raven.New(dsn)
		if err != nil {
			return err
		}
		clients[dsn] = cli
	}

	// The client sets release and such on the packet before sending, in the
	// misguided idea that it knows this better than than the packet we give
	// it. So we copy the values from the packet to the client first...
	cli.SetRelease(pkt.Release)
	cli.SetEnvironment(pkt.Environment)

	defer cli.Wait()
	_, errC := cli.Capture(pkt, nil)
	return <-errC
}

func parseReport(path string, report []byte) (*raven.Packet, error) {
	parts := bytes.SplitN(report, []byte("\n"), 2)
	if len(parts) != 2 {
		return nil, errors.New("no first line")
	}

	version, err := parseVersion(string(parts[0]))
	if err != nil {
		return nil, err
	}
	report = parts[1]

	foundPanic := false
	var subjectLine []byte
	for {
		parts = bytes.SplitN(report, []byte("\n"), 2)
		if len(parts) != 2 {
			return nil, errors.New("no panic line found")
		}

		line := parts[0]
		report = parts[1]

		if foundPanic {
			// The previous line was our "Panic at ..." header. We are now
			// at the beginning of the real panic trace and this is our
			// subject line.
			subjectLine = line
			break
		} else if bytes.HasPrefix(line, []byte("Panic at")) {
			foundPanic = true
		}
	}

	r := bytes.NewReader(report)
	ctx, err := stack.ParseDump(r, ioutil.Discard, false)
	if err != nil {
		return nil, err
	}

	// Lock the source code loader to the version we are processing here.
	if version.commit != "" {
		// We have a commit hash, so we know exactly which source to use
		loader.LockWithVersion(version.commit)
	} else if strings.HasPrefix(version.tag, "v") {
		// Lets hope the tag is close enough
		loader.LockWithVersion(version.tag)
	} else {
		// Last resort
		loader.LockWithVersion("main")
	}
	defer loader.Unlock()

	var trace raven.Stacktrace
	for _, gr := range ctx.Goroutines {
		if gr.First {
			trace.Frames = make([]*raven.StacktraceFrame, len(gr.Stack.Calls))
			for i, sc := range gr.Stack.Calls {
				trace.Frames[len(trace.Frames)-1-i] = raven.NewStacktraceFrame(0, sc.Func.Name(), sc.SrcPath, sc.Line, 3, nil)
			}
			break
		}
	}

	pkt := &raven.Packet{
		Message:     string(subjectLine),
		Platform:    "go",
		Release:     version.tag,
		Environment: version.environment(),
		Tags: raven.Tags{
			raven.Tag{Key: "version", Value: version.version},
			raven.Tag{Key: "tag", Value: version.tag},
			raven.Tag{Key: "codename", Value: version.codename},
			raven.Tag{Key: "runtime", Value: version.runtime},
			raven.Tag{Key: "goos", Value: version.goos},
			raven.Tag{Key: "goarch", Value: version.goarch},
			raven.Tag{Key: "builder", Value: version.builder},
		},
		Extra: raven.Extra{
			"url": reportServer + path,
		},
		Interfaces: []raven.Interface{&trace},
	}
	if version.commit != "" {
		pkt.Tags = append(pkt.Tags, raven.Tag{Key: "commit", Value: version.commit})
	}
	for _, tag := range version.extra {
		pkt.Tags = append(pkt.Tags, raven.Tag{Key: tag, Value: "1"})
	}

	return pkt, nil
}

// syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC [foo, bar]
var longVersionRE = regexp.MustCompile(`syncthing\s+(v[^\s]+)\s+"([^"]+)"\s\(([^\s]+)\s+([^-]+)-([^)]+)\)\s+([^\s]+)[^\[]*(?:\[(.+)\])?$`)

type version struct {
	version  string   // "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep"
	tag      string   // "v1.1.4-rc.1"
	commit   string   // "6aaae618", blank when absent
	codename string   // "Erbium Earthworm"
	runtime  string   // "go1.12.5"
	goos     string   // "darwin"
	goarch   string   // "amd64"
	builder  string   // "jb@kvin.kastelo.net"
	extra    []string // "foo", "bar"
}

func (v version) environment() string {
	if v.commit != "" {
		return "Development"
	}
	if strings.Contains(v.tag, "-rc.") {
		return "Candidate"
	}
	if strings.Contains(v.tag, "-") {
		return "Beta"
	}
	return "Stable"
}

func parseVersion(line string) (version, error) {
	m := longVersionRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return version{}, errors.New("unintelligeble version string")
	}

	v := version{
		version:  m[1],
		codename: m[2],
		runtime:  m[3],
		goos:     m[4],
		goarch:   m[5],
		builder:  m[6],
	}

	parts := strings.Split(v.version, "+")
	v.tag = parts[0]
	if len(parts) > 1 {
		fields := strings.Split(parts[1], "-")
		if len(fields) >= 2 && strings.HasPrefix(fields[1], "g") {
			v.commit = fields[1][1:]
		}
	}

	if len(m) >= 8 && m[7] != "" {
		tags := strings.Split(m[7], ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		v.extra = tags
	}

	return v, nil
}
