// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ur

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/sha256"
)

const (
	// How long to sleep the panic uploader routine after an upload
	panicUploadSleepBase = 10 * time.Second
	// Multiplier for increasing sleep time
	panicUploadSleepMult = 2
	// Maximum sleep time
	panicUploadSleepMax = 10 * time.Minute
)

// uploadPanicLogs attempts to upload all the panic logs in the named
// directory to the crash reporting server as urlBase. After each upload or
// attempted upload a pause is made, to avoid swamping the reporting server.
// Uploads are attempted with the newest log first.
//
// This can can block for a long time.
func uploadPanicLogs(urlBase, dir string) {
	files, err := filepath.Glob(filepath.Join(dir, "panic-*.log"))
	if err != nil {
		l.Warnln("Failed to list panic logs:", err)
		return
	}

	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	sleep := panicUploadSleepBase
	for _, file := range files {
		uploadPanicLog(urlBase, file, sleep)
		sleep *= panicUploadSleepMult
		if sleep > panicUploadSleepMax {
			sleep = panicUploadSleepMax
		}
	}
}

// uploadPanicLog attempts to upload the named panic log to the crash
// reporting server at urlBase. The panic ID is constructed as the sha256 of
// the log contents. A HEAD request is made to see if the log has already
// been reported. If not, a PUT is made with the log contents.
func uploadPanicLog(urlBase, file string, wait time.Duration) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		l.Warnln("Failed to read existing panic file:", err)
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	url := fmt.Sprintf("%s/%s", urlBase, hash)

	headReq, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		l.Warnln("Failed to construct crash reporting request (check):", err)
		return
	}

	resp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		l.Warnln("Failed to contact crash reporting server (check):", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		// It's known, we're done
		return
	}

	l.Infoln("Reporting crash found in", filepath.Base(file), "...")
	defer time.Sleep(wait)

	putReq, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		l.Warnln("Failed to construct crash reporting request (upload):", err)
		return
	}
	resp, err = http.DefaultClient.Do(putReq)
	if err != nil {
		l.Warnln("Failed to contact crash reporting server (upload):", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		l.Warnln("Failed to upload crash report:", resp.Status)
	}
}
