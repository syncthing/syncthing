// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ur

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"

	"github.com/thejerf/suture/v4"
)

var (
	// When a specific failure first occurs, it is delayed by minDelay. If
	// more of the same failures occurs those are further delayed and
	// aggregated for maxDelay.
	minDelay             = 10 * time.Second
	maxDelay             = time.Minute
	sendTimeout          = time.Minute
	evChanClosed         = "failure event channel closed"
	invalidEventDataType = "failure event data is not a string"
)

type FailureReport struct {
	Description string
	Count       int
	Version     string
}

type FailureHandler interface {
	suture.Service
	config.Committer
}

func NewFailureHandler(cfg config.Wrapper, evLogger events.Logger) FailureHandler {
	return &failureHandler{
		cfg:      cfg,
		evLogger: evLogger,
		optsChan: make(chan config.OptionsConfiguration),
		buf:      make(map[string]*failureStat),
	}
}

type failureHandler struct {
	cfg      config.Wrapper
	evLogger events.Logger
	optsChan chan config.OptionsConfiguration
	buf      map[string]*failureStat
}

type failureStat struct {
	first, last time.Time
	count       int
}

func (h *failureHandler) Serve(ctx context.Context) error {
	h.cfg.Subscribe(h)
	defer h.cfg.Unsubscribe(h)

	url, sub, evChan := h.applyOpts(h.cfg.Options(), nil)

	h.handleOldFailureReports(ctx, url, sub != nil)

	var err error
	timer := time.NewTimer(minDelay)
	resetTimer := make(chan struct{})
	for err == nil {
		select {
		case opts := <-h.optsChan:
			url, sub, evChan = h.applyOpts(opts, sub)
		case e, ok := <-evChan:
			if !ok {
				// Just to be safe - shouldn't ever happen, as
				// evChan is set to nil when unsubscribing.
				h.addReport(evChanClosed, time.Now())
				evChan = nil
				continue
			}
			descr, ok := e.Data.(string)
			if !ok {
				// Same here, shouldn't ever happen.
				h.addReport(invalidEventDataType, time.Now())
				continue
			}
			h.addReport(descr, e.Time)
		case <-timer.C:
			reports := make([]FailureReport, 0, len(h.buf))
			now := time.Now()
			for descr, stat := range h.buf {
				if now.Sub(stat.last) > minDelay || now.Sub(stat.first) > maxDelay {
					reports = append(reports, newFailureReport(descr, stat.count))
					delete(h.buf, descr)
				}
			}
			if len(reports) > 0 {
				// Lets keep process events/configs while it might be timing out for a while
				go func() {
					if err := sendFailureReports(ctx, reports, url); err != nil {
						l.Infoln("Failed to send failure report:", err)
					}
					select {
					case resetTimer <- struct{}{}:
					case <-ctx.Done():
					}
				}()
			} else {
				timer.Reset(minDelay)
			}
		case <-resetTimer:
			timer.Reset(minDelay)
		case <-ctx.Done():
			err = ctx.Err()
		}
	}

	if sub != nil {
		sub.Unsubscribe()
		reports := make([]FailureReport, 0, len(h.buf))
		for descr, stat := range h.buf {
			reports = append(reports, newFailureReport(descr, stat.count))
		}
		h.buf = make(map[string]*failureStat)
		if err := writeFailures(locations.Get(locations.FailuresFile), reports); err != nil && !os.IsNotExist(err) {
			l.Warnln("Failed to write failures to be sent later:", err)
		}
	}

	return err
}

func (h *failureHandler) applyOpts(opts config.OptionsConfiguration, sub events.Subscription) (string, events.Subscription, <-chan events.Event) {
	// Sub nil checks just for safety - config updates can be racy.
	url := opts.CRURL + "/failure"
	if opts.URAccepted > 0 {
		if sub == nil {
			sub = h.evLogger.Subscribe(events.Failure)
		}
		return url, sub, sub.C()
	}
	if sub != nil {
		sub.Unsubscribe()
	}
	return url, nil, nil
}

func (h *failureHandler) addReport(descr string, evTime time.Time) {
	if stat, ok := h.buf[descr]; ok {
		stat.last = evTime
		stat.count++
		return
	}
	h.buf[descr] = &failureStat{
		first: evTime,
		last:  evTime,
		count: 1,
	}
}

func (h *failureHandler) handleOldFailureReports(ctx context.Context, url string, shouldReport bool) {
	path := locations.Get(locations.FailuresFile)
	if !shouldReport {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			l.Debugln("Failed to delete previous failures:", err)
		}
		return
	}
	reports, err := readFailures(path)
	if err != nil {
		if !os.IsNotExist(err) {
			l.Infoln("Failed to read previous failures:", err)
		}
		return
	}
	if err := sendFailureReports(ctx, reports, url); err != nil {
		// Lets pretend they're new
		now := time.Now()
		for _, r := range reports {
			h.buf[r.Description] = &failureStat{
				first: now,
				last:  now,
				count: r.Count,
			}
		}
	}
}

func readFailures(path string) ([]FailureReport, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	d := json.NewDecoder(fd)
	var reports []FailureReport
	err = d.Decode(&reports)
	return reports, err
}

func writeFailures(path string, reports []FailureReport) error {
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer fd.Close()
	e := json.NewEncoder(fd)
	return e.Encode(reports)
}

func (h *failureHandler) VerifyConfiguration(_, _ config.Configuration) error {
	return nil
}

func (h *failureHandler) CommitConfiguration(from, to config.Configuration) bool {
	if from.Options.CREnabled != to.Options.CREnabled || from.Options.CRURL != to.Options.CRURL {
		h.optsChan <- to.Options
	}
	return true
}

func (h *failureHandler) String() string {
	return "FailureHandler"
}

func sendFailureReports(ctx context.Context, reports []FailureReport, url string) error {
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(reports); err != nil {
		panic(err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			Proxy:       http.ProxyFromEnvironment,
		},
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, sendTimeout)
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func newFailureReport(descr string, count int) FailureReport {
	return FailureReport{
		Description: descr,
		Count:       count,
		Version:     build.LongVersion,
	}
}
