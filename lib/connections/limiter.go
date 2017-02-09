// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"fmt"
	"io"
	"sync/atomic"

	"github.com/syncthing/syncthing/lib/config"
	"golang.org/x/net/context"
	"golang.org/x/time/rate"
)

// limiter manages a read and write rate limit, reacting to config changes
// as appropriate.
type limiter struct {
	write     *rate.Limiter
	read      *rate.Limiter
	limitsLAN atomicBool
}

const limiterBurstSize = 4 * 128 << 10

func newLimiter(cfg *config.Wrapper) *limiter {
	l := &limiter{
		write: rate.NewLimiter(rate.Inf, limiterBurstSize),
		read:  rate.NewLimiter(rate.Inf, limiterBurstSize),
	}
	cfg.Subscribe(l)
	prev := config.Configuration{Options: config.OptionsConfiguration{MaxRecvKbps: -1, MaxSendKbps: -1}}
	l.CommitConfiguration(prev, cfg.RawCopy())
	return l
}

func (lim *limiter) newReadLimiter(r io.Reader, isLAN bool) io.Reader {
	return &limitedReader{reader: r, limiter: lim, isLAN: isLAN}
}

func (lim *limiter) newWriteLimiter(w io.Writer, isLAN bool) io.Writer {
	return &limitedWriter{writer: w, limiter: lim, isLAN: isLAN}
}

func (lim *limiter) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (lim *limiter) CommitConfiguration(from, to config.Configuration) bool {
	if from.Options.MaxRecvKbps == to.Options.MaxRecvKbps &&
		from.Options.MaxSendKbps == to.Options.MaxSendKbps &&
		from.Options.LimitBandwidthInLan == to.Options.LimitBandwidthInLan {
		return true
	}

	// The rate variables are in KiB/s in the config (despite the camel casing
	// of the name). We multiply by 1024 to get bytes/s.

	if to.Options.MaxRecvKbps <= 0 {
		lim.read.SetLimit(rate.Inf)
	} else {
		lim.read.SetLimit(1024 * rate.Limit(to.Options.MaxRecvKbps))
	}

	if to.Options.MaxSendKbps <= 0 {
		lim.write.SetLimit(rate.Inf)
	} else {
		lim.write.SetLimit(1024 * rate.Limit(to.Options.MaxSendKbps))
	}

	lim.limitsLAN.set(to.Options.LimitBandwidthInLan)

	sendLimitStr := "is unlimited"
	recvLimitStr := "is unlimited"
	if to.Options.MaxSendKbps > 0 {
		sendLimitStr = fmt.Sprintf("limit is %d KiB/s", to.Options.MaxSendKbps)
	}
	if to.Options.MaxRecvKbps > 0 {
		recvLimitStr = fmt.Sprintf("limit is %d KiB/s", to.Options.MaxRecvKbps)
	}
	l.Infof("Send rate %s, receive rate %s", sendLimitStr, recvLimitStr)

	if to.Options.LimitBandwidthInLan {
		l.Infoln("Rate limits apply to LAN connections")
	} else {
		l.Infoln("Rate limits do not apply to LAN connections")
	}

	return true
}

func (lim *limiter) String() string {
	// required by config.Committer interface
	return "connections.limiter"
}

// limitedReader is a rate limited io.Reader
type limitedReader struct {
	reader  io.Reader
	limiter *limiter
	isLAN   bool
}

func (r *limitedReader) Read(buf []byte) (int, error) {
	n, err := r.reader.Read(buf)
	if !r.isLAN || r.limiter.limitsLAN.get() {
		take(r.limiter.read, n)
	}
	return n, err
}

// limitedWriter is a rate limited io.Writer
type limitedWriter struct {
	writer  io.Writer
	limiter *limiter
	isLAN   bool
}

func (w *limitedWriter) Write(buf []byte) (int, error) {
	if !w.isLAN || w.limiter.limitsLAN.get() {
		take(w.limiter.write, len(buf))
	}
	return w.writer.Write(buf)
}

// take is a utility function to consume tokens from a rate.Limiter. No call
// to WaitN can be larger than the limiter burst size so we split it up into
// several calls when necessary.
func take(l *rate.Limiter, tokens int) {
	if tokens < limiterBurstSize {
		// This is the by far more common case so we get it out of the way
		// early.
		l.WaitN(context.TODO(), tokens)
		return
	}

	for tokens > 0 {
		// Consume limiterBurstSize tokens at a time until we're done.
		if tokens > limiterBurstSize {
			l.WaitN(context.TODO(), limiterBurstSize)
			tokens -= limiterBurstSize
		} else {
			l.WaitN(context.TODO(), tokens)
			tokens = 0
		}
	}
}

type atomicBool int32

func (b *atomicBool) set(v bool) {
	if v {
		atomic.StoreInt32((*int32)(b), 1)
	} else {
		atomic.StoreInt32((*int32)(b), 0)
	}
}

func (b *atomicBool) get() bool {
	return atomic.LoadInt32((*int32)(b)) != 0
}
