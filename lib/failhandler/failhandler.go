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
	Descr   string
	Count   int
	Version string
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
	ticker := time.NewTicker(minDelay)
	for err == nil {
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
			url = opts.CRURL
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
		case now := <-ticker.C:
			reports := make([]Report, 0, len(h.buf))
			for descr, stat := range h.buf {
				if now.Sub(stat.last) > minDelay || now.Sub(stat.first) > maxDelay {
					reports = append(reports, Report{
						Descr:   descr,
						Count:   stat.count,
						Version: build.LongVersion,
					})
					delete(h.buf, descr)
				}
			}
			if len(reports) > 0 {
				err = sendReports(ctx, reports, url)
			}
		case <-ctx.Done():
			err = ctx.Err()
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

func sendReports(ctx context.Context, reports []Report, url string) error {
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(reports); err != nil {
		return err
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, sendTimeout)
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
