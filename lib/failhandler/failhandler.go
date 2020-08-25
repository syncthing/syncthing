// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package failhandler

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
	"github.com/syncthing/syncthing/lib/util"

	"github.com/thejerf/suture"
)

var (
	// When a specific failure first occurs, it is delayed by minDelay. If
	// more of the same failures occurs those are further delayed and
	// aggregated for maxDelay.
	minDelay    = 10 * time.Second
	maxDelay    = time.Minute
	sendTimeout = time.Minute
)

type Report struct {
	Description string
	Count       int
	Version     string
}

type Handler interface {
	suture.Service
	config.Committer
}

func New(cfg config.Wrapper, evLogger events.Logger) Handler {
	h := &handler{
		cfg:      cfg,
		evLogger: evLogger,
		optsChan: make(chan config.OptionsConfiguration),
	}
	h.Service = util.AsServiceWithError(h.serve, h.String())
	return h
}

type handler struct {
	suture.Service
	cfg      config.Wrapper
	evLogger events.Logger
	optsChan chan config.OptionsConfiguration
	evChan   <-chan events.Event
	buf      map[string]*stat
}

type stat struct {
	first, last time.Time
	count       int
}

func (h *handler) serve(ctx context.Context) error {
	go func() {
		h.optsChan <- h.cfg.Options()
	}()
	h.cfg.Subscribe(h)
	defer h.cfg.Unsubscribe(h)

	var url string
	var err error
	var sub events.Subscription
	timer := time.NewTimer(minDelay)
outer:
	for {
		select {
		case opts := <-h.optsChan:
			// Sub nil checks just for safety - config updates can be racy.
			if opts.CREnabled {
				if sub == nil {
					sub = h.evLogger.Subscribe(events.Failure)
					h.evChan = sub.C()
				}
			} else if sub != nil {
				sub.Unsubscribe()
				sub = nil
			}
			url = opts.CRURL + "/failure"
		case e := <-h.evChan:
			descr := e.Data.(string)
			if stat, ok := h.buf[descr]; ok {
				stat.last = e.Time
				stat.count++
			}
			h.buf[descr] = &stat{
				first: e.Time,
				last:  e.Time,
				count: 1,
			}
		case <-timer.C:
			reports := make([]Report, 0, len(h.buf))
			now := time.Now()
			for descr, stat := range h.buf {
				if now.Sub(stat.last) > minDelay || now.Sub(stat.first) > maxDelay {
					reports = append(reports, Report{
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
					sendReports(ctx, reports, url)
					timer.Reset(minDelay)
				}()
			}
		case <-ctx.Done():
			break outer
		}
	}

	if sub != nil {
		sub.Unsubscribe()
	}
	return err
}

func (h *handler) VerifyConfiguration(_, _ config.Configuration) error {
	return nil
}

func (h *handler) CommitConfiguration(from, to config.Configuration) bool {
	if from.Options.CREnabled != to.Options.CREnabled || from.Options.CRURL != to.Options.CRURL {
		h.optsChan <- to.Options
	}
	return true
}

func (h *handler) String() string {
	return "FailHandler"
}

func sendReports(ctx context.Context, reports []Report, url string) {
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
