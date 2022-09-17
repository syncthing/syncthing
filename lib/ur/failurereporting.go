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
	"runtime/pprof"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/svcutil"

	"github.com/thejerf/suture/v4"
)

var (
	// When a specific failure first occurs, it is delayed by minDelay. If
	// more of the same failures occurs those are further delayed and
	// aggregated for maxDelay.
	minDelay             = 10 * time.Second
	maxDelay             = time.Minute
	sendTimeout          = time.Minute
	finalSendTimeout     = svcutil.ServiceTimeout / 2
	evChanClosed         = "failure event channel closed"
	invalidEventDataType = "failure event data is not a string"
)

type FailureReport struct {
	FailureData
	Count   int
	Version string
}

type FailureData struct {
	Description string
	Goroutines  string
	Extra       map[string]string
}

func FailureDataWithGoroutines(description string) FailureData {
	var buf strings.Builder
	pprof.Lookup("goroutine").WriteTo(&buf, 1)
	return FailureData{
		Description: description,
		Goroutines:  buf.String(),
		Extra:       make(map[string]string),
	}
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
	data        FailureData
}

func (h *failureHandler) Serve(ctx context.Context) error {
	cfg := h.cfg.Subscribe(h)
	defer h.cfg.Unsubscribe(h)
	url, sub, evChan := h.applyOpts(cfg.Options, nil)

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
				h.addReport(FailureData{Description: evChanClosed}, time.Now())
				evChan = nil
				continue
			}
			var data FailureData
			switch d := e.Data.(type) {
			case string:
				data.Description = d
			case FailureData:
				data = d
			default:
				// Same here, shouldn't ever happen.
				h.addReport(FailureData{Description: invalidEventDataType}, time.Now())
				continue
			}
			h.addReport(data, e.Time)
		case <-timer.C:
			reports := make([]FailureReport, 0, len(h.buf))
			now := time.Now()
			for descr, stat := range h.buf {
				if now.Sub(stat.last) > minDelay || now.Sub(stat.first) > maxDelay {
					reports = append(reports, newFailureReport(stat))
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
			err = ctx.Err()
		}
	}

	if sub != nil {
		sub.Unsubscribe()
		if len(h.buf) > 0 {
			reports := make([]FailureReport, 0, len(h.buf))
			for _, stat := range h.buf {
				reports = append(reports, newFailureReport(stat))
			}
			timeout, cancel := context.WithTimeout(context.Background(), finalSendTimeout)
			defer cancel()
			sendFailureReports(timeout, reports, url)
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

func (h *failureHandler) addReport(data FailureData, evTime time.Time) {
	if stat, ok := h.buf[data.Description]; ok {
		stat.last = evTime
		stat.count++
		return
	}
	h.buf[data.Description] = &failureStat{
		first: evTime,
		last:  evTime,
		count: 1,
		data:  data,
	}
}

func (h *failureHandler) CommitConfiguration(from, to config.Configuration) bool {
	if from.Options.CREnabled != to.Options.CREnabled || from.Options.CRURL != to.Options.CRURL {
		h.optsChan <- to.Options
	}
	return true
}

func (*failureHandler) String() string {
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
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, &b)
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
}

func newFailureReport(stat *failureStat) FailureReport {
	return FailureReport{
		FailureData: stat.data,
		Count:       stat.count,
		Version:     build.LongVersion,
	}
}
