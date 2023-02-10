// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package httpcache

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SinglePathCache struct {
	next http.Handler
	keep time.Duration

	mut  sync.RWMutex
	resp *recordedResponse
}

func SinglePath(next http.Handler, keep time.Duration) *SinglePathCache {
	return &SinglePathCache{
		next: next,
		keep: keep,
	}
}

type recordedResponse struct {
	status int
	header http.Header
	data   []byte
	gzip   []byte
	when   time.Time
	keep   time.Duration
}

func (resp *recordedResponse) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for k, v := range resp.header {
		w.Header()[k] = v
	}

	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(resp.keep.Seconds())))

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprint(len(resp.gzip)))
		w.WriteHeader(resp.status)
		_, _ = w.Write(resp.gzip)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprint(len(resp.data)))
	w.WriteHeader(resp.status)
	_, _ = w.Write(resp.data)
}

type responseRecorder struct {
	resp *recordedResponse
}

func (r *responseRecorder) WriteHeader(status int) {
	r.resp.status = status
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.resp.data = append(r.resp.data, data...)
	return len(data), nil
}

func (r *responseRecorder) Header() http.Header {
	return r.resp.header
}

func (s *SinglePathCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.next.ServeHTTP(w, r)
		return
	}

	w.Header().Set("X-Cache", "MISS")

	s.mut.RLock()
	ok := s.serveCached(w, r)
	s.mut.RUnlock()
	if ok {
		return
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	if s.serveCached(w, r) {
		return
	}

	rec := &recordedResponse{status: http.StatusOK, header: make(http.Header), when: time.Now(), keep: s.keep}
	childRec := r.Clone(context.Background())
	childRec.Header.Del("Accept-Encoding") // don't let the client dictate the encoding
	s.next.ServeHTTP(&responseRecorder{resp: rec}, childRec)

	if rec.status == http.StatusOK {
		buf := new(bytes.Buffer)
		gw := gzip.NewWriter(buf)
		_, _ = gw.Write(rec.data)
		gw.Close()
		rec.gzip = buf.Bytes()

		s.resp = rec
	}

	rec.ServeHTTP(w, r)
}

func (s *SinglePathCache) serveCached(w http.ResponseWriter, r *http.Request) bool {
	if s.resp == nil || time.Since(s.resp.when) > s.keep {
		return false
	}

	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-From", s.resp.when.Format(time.RFC3339))

	s.resp.ServeHTTP(w, r)
	return true
}
