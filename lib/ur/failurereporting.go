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
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/events"

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
	go func() {
		select {
		case h.optsChan <- h.cfg.Options():
		case <-ctx.Done():
		}
	}()
	h.cfg.Subscribe(h)
	defer h.cfg.Unsubscribe(h)

	var url string
	var err error
	var sub events.Subscription
	var evChan <-chan events.Event
	timer := time.NewTimer(minDelay)
	resetTimer := make(chan struct{})
outer:
	for {
		select {
		case opts := <-h.optsChan:
			// Sub nil checks just for safety - config updates can be racy.
			if opts.URAccepted > 0 {
				if sub == nil {
					sub = h.evLogger.Subscribe(events.Failure)
					evChan = sub.C()
				}
			} else if sub != nil {
				sub.Unsubscribe()
				sub = nil
				evChan = nil
			}
			url = opts.CRURL + "/failure"
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
					reports = append(reports, FailureReport{
						Description: descr,
						Count:       stat.count,
						Version:     build.LongVersion,
					})
					delete(h.buf, descr)
				}
			}
			if len(reports) > 0 {
				// Lets keep process events/configs while it might be timing out for a while
				go func() {
					sendFailureReports(ctx, reports, url)
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
			break outer
		}
	}

	if sub != nil {
		sub.Unsubscribe()
	}
	return err
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

func sendFailureReports(ctx context.Context, reports []FailureReport, url string) {
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
		l.Infoln("Failed to send failure report:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		l.Infoln("Failed to send failure report:", err)
		return
	}
	resp.Body.Close()
	return
}
