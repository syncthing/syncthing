// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	headRequestTimeout = 10 * time.Second
	putRequestTimeout  = time.Minute
)

// uploadPanicLogs attempts to upload all the panic logs in the named
// directory to the crash reporting server as urlBase. Uploads are attempted
// with the newest log first.
//
// This can can block for a long time. The context can set a final deadline
// for this.
func uploadPanicLogs(ctx context.Context, urlBase, dir string) {
	files, err := filepath.Glob(filepath.Join(dir, "panic-*.log"))
	if err != nil {
		l.Warnln("Failed to list panic logs:", err)
		return
	}

	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, file := range files {
		if strings.Contains(file, ".reported.") {
			// We've already sent this file. It'll be cleaned out at some
			// point.
			continue
		}

		if err := uploadPanicLog(ctx, urlBase, file); err != nil {
			l.Warnln("Reporting crash:", err)
		} else {
			// Rename the log so we don't have to try to report it again. This
			// succeeds, or it does not. There is no point complaining about it.
			_ = os.Rename(file, strings.Replace(file, ".log", ".reported.log", 1))
		}
	}
}

// uploadPanicLog attempts to upload the named panic log to the crash
// reporting server at urlBase. The panic ID is constructed as the sha256 of
// the log contents. A HEAD request is made to see if the log has already
// been reported. If not, a PUT is made with the log contents.
func uploadPanicLog(ctx context.Context, urlBase, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	// Remove log lines, for privacy.
	data = filterLogLines(data)

	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	l.Infof("Reporting crash found in %s (report ID %s) ...\n", filepath.Base(file), hash[:8])

	url := fmt.Sprintf("%s/%s", urlBase, hash)
	headReq, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return err
	}

	// Set a reasonable timeout on the HEAD request
	headCtx, headCancel := context.WithTimeout(ctx, headRequestTimeout)
	defer headCancel()
	headReq = headReq.WithContext(headCtx)

	resp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		// It's known, we're done
		return nil
	}

	putReq, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	// Set a reasonable timeout on the PUT request
	putCtx, putCancel := context.WithTimeout(ctx, putRequestTimeout)
	defer putCancel()
	putReq = putReq.WithContext(putCtx)

	resp, err = http.DefaultClient.Do(putReq)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload: %s", resp.Status)
	}

	return nil
}

// filterLogLines returns the data without any log lines between the first
// line and the panic trace. This is done in-place: the original data slice
// is destroyed.
func filterLogLines(data []byte) []byte {
	filtered := data[:0]
	matched := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		switch {
		case !matched && bytes.HasPrefix(line, []byte("Panic ")):
			// This begins the panic trace, set the matched flag and append.
			matched = true
			fallthrough
		case len(filtered) == 0 || matched:
			// This is the first line or inside the panic trace.
			if len(filtered) > 0 {
				// We add the newline before rather than after because
				// bytes.Split sees the \n as *separator* and not line
				// ender, so ir will generate a last empty line that we
				// don't really want. (We want to keep blank lines in the
				// middle of the trace though.)
				filtered = append(filtered, '\n')
			}
			// Remove the device ID prefix. The "plus two" stuff is because
			// the line will look like "[foo] whatever" and the end variable
			// will end up pointing at the ] and we want to step over that
			// and the following space.
			if end := bytes.Index(line, []byte("]")); end > 1 && end < len(line)-2 && bytes.HasPrefix(line, []byte("[")) {
				line = line[end+2:]
			}
			filtered = append(filtered, line...)
		}
	}
	return filtered
}
