// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/time/rate"
)

// limiter manages a read and write rate limit, reacting to config changes
// as appropriate.
type limiter struct {
	mu                  sync.Mutex
	write               *rate.Limiter
	read                *rate.Limiter
	limitsLAN           atomicBool
	deviceReadLimiters  map[protocol.DeviceID]*rate.Limiter
	deviceWriteLimiters map[protocol.DeviceID]*rate.Limiter
}

type waiter interface {
	// This is the rate limiting operation
	WaitN(ctx context.Context, n int) error
}

const limiterBurstSize = 4 * 128 << 10

func newLimiter(cfg config.Wrapper) *limiter {
	l := &limiter{
		write:               rate.NewLimiter(rate.Inf, limiterBurstSize),
		read:                rate.NewLimiter(rate.Inf, limiterBurstSize),
		mu:                  sync.NewMutex(),
		deviceReadLimiters:  make(map[protocol.DeviceID]*rate.Limiter),
		deviceWriteLimiters: make(map[protocol.DeviceID]*rate.Limiter),
	}

	cfg.Subscribe(l)
	prev := config.Configuration{Options: config.OptionsConfiguration{MaxRecvKbps: -1, MaxSendKbps: -1}}

	l.CommitConfiguration(prev, cfg.RawCopy())
	return l
}

// This function sets limiters according to corresponding DeviceConfiguration
func (lim *limiter) setLimitsLocked(device config.DeviceConfiguration) bool {
	readLimiter := lim.getReadLimiterLocked(device.DeviceID)
	writeLimiter := lim.getWriteLimiterLocked(device.DeviceID)

	// limiters for this device are created so we can store previous rates for logging
	previousReadLimit := readLimiter.Limit()
	previousWriteLimit := writeLimiter.Limit()
	currentReadLimit := rate.Limit(device.MaxRecvKbps) * 1024
	currentWriteLimit := rate.Limit(device.MaxSendKbps) * 1024
	if device.MaxSendKbps <= 0 {
		currentWriteLimit = rate.Inf
	}
	if device.MaxRecvKbps <= 0 {
		currentReadLimit = rate.Inf
	}
	// Nothing about this device has changed. Start processing next device
	if previousWriteLimit == currentWriteLimit && previousReadLimit == currentReadLimit {
		return false
	}

	readLimiter.SetLimit(currentReadLimit)
	writeLimiter.SetLimit(currentWriteLimit)

	return true
}

// This function handles removing, adding and updating of device limiters.
func (lim *limiter) processDevicesConfigurationLocked(from, to config.Configuration) {
	seen := make(map[protocol.DeviceID]struct{})

	// Mark devices which should not be removed, create new limiters if needed and assign new limiter rate
	for _, dev := range to.Devices {
		if dev.DeviceID == to.MyID {
			// This limiter was created for local device. Should skip this device
			continue
		}
		seen[dev.DeviceID] = struct{}{}

		if lim.setLimitsLocked(dev) {
			readLimitStr := "is unlimited"
			if dev.MaxRecvKbps > 0 {
				readLimitStr = fmt.Sprintf("limit is %d KiB/s", dev.MaxRecvKbps)
			}
			writeLimitStr := "is unlimited"
			if dev.MaxSendKbps > 0 {
				writeLimitStr = fmt.Sprintf("limit is %d KiB/s", dev.MaxSendKbps)
			}

			l.Infof("Device %s send rate %s, receive rate %s", dev.DeviceID, writeLimitStr, readLimitStr)
		}
	}

	// Delete remote devices which were removed in new configuration
	for _, dev := range from.Devices {
		if _, ok := seen[dev.DeviceID]; !ok {
			l.Debugf("deviceID: %s should be removed", dev.DeviceID)

			delete(lim.deviceWriteLimiters, dev.DeviceID)
			delete(lim.deviceReadLimiters, dev.DeviceID)
		}
	}
}

func (lim *limiter) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (lim *limiter) CommitConfiguration(from, to config.Configuration) bool {
	// to ensure atomic update of configuration
	lim.mu.Lock()
	defer lim.mu.Unlock()

	// Delete, add or update limiters for devices
	lim.processDevicesConfigurationLocked(from, to)

	if from.Options.MaxRecvKbps == to.Options.MaxRecvKbps &&
		from.Options.MaxSendKbps == to.Options.MaxSendKbps &&
		from.Options.LimitBandwidthInLan == to.Options.LimitBandwidthInLan {
		return true
	}

	limited := false
	sendLimitStr := "is unlimited"
	recvLimitStr := "is unlimited"

	// The rate variables are in KiB/s in the config (despite the camel casing
	// of the name). We multiply by 1024 to get bytes/s.
	if to.Options.MaxRecvKbps <= 0 {
		lim.read.SetLimit(rate.Inf)
	} else {
		lim.read.SetLimit(1024 * rate.Limit(to.Options.MaxRecvKbps))
		recvLimitStr = fmt.Sprintf("limit is %d KiB/s", to.Options.MaxRecvKbps)
		limited = true
	}

	if to.Options.MaxSendKbps <= 0 {
		lim.write.SetLimit(rate.Inf)
	} else {
		lim.write.SetLimit(1024 * rate.Limit(to.Options.MaxSendKbps))
		sendLimitStr = fmt.Sprintf("limit is %d KiB/s", to.Options.MaxSendKbps)
		limited = true
	}

	lim.limitsLAN.set(to.Options.LimitBandwidthInLan)

	l.Infof("Overall send rate %s, receive rate %s", sendLimitStr, recvLimitStr)

	if limited {
		if to.Options.LimitBandwidthInLan {
			l.Infoln("Rate limits apply to LAN connections")
		} else {
			l.Infoln("Rate limits do not apply to LAN connections")
		}
	}

	return true
}

func (lim *limiter) String() string {
	// required by config.Committer interface
	return "connections.limiter"
}

func (lim *limiter) getLimiters(remoteID protocol.DeviceID, rw io.ReadWriter, isLAN bool) (io.Reader, io.Writer) {
	lim.mu.Lock()
	wr := lim.newLimitedWriterLocked(remoteID, rw, isLAN)
	rd := lim.newLimitedReaderLocked(remoteID, rw, isLAN)
	lim.mu.Unlock()
	return rd, wr
}

func (lim *limiter) newLimitedReaderLocked(remoteID protocol.DeviceID, r io.Reader, isLAN bool) io.Reader {
	return &limitedReader{
		reader:    r,
		limitsLAN: &lim.limitsLAN,
		waiter:    totalWaiter{lim.getReadLimiterLocked(remoteID), lim.read},
		isLAN:     isLAN,
	}
}

func (lim *limiter) newLimitedWriterLocked(remoteID protocol.DeviceID, w io.Writer, isLAN bool) io.Writer {
	return &limitedWriter{
		writer:    w,
		limitsLAN: &lim.limitsLAN,
		waiter:    totalWaiter{lim.getWriteLimiterLocked(remoteID), lim.write},
		isLAN:     isLAN,
	}
}

func (lim *limiter) getReadLimiterLocked(deviceID protocol.DeviceID) *rate.Limiter {
	return getRateLimiter(lim.deviceReadLimiters, deviceID)
}

func (lim *limiter) getWriteLimiterLocked(deviceID protocol.DeviceID) *rate.Limiter {
	return getRateLimiter(lim.deviceWriteLimiters, deviceID)
}

func getRateLimiter(m map[protocol.DeviceID]*rate.Limiter, deviceID protocol.DeviceID) *rate.Limiter {
	limiter, ok := m[deviceID]
	if !ok {
		limiter = rate.NewLimiter(rate.Inf, limiterBurstSize)
		m[deviceID] = limiter
	}
	return limiter
}

// limitedReader is a rate limited io.Reader
type limitedReader struct {
	reader    io.Reader
	limitsLAN *atomicBool
	waiter    waiter
	isLAN     bool
}

func (r *limitedReader) Read(buf []byte) (int, error) {
	n, err := r.reader.Read(buf)
	if !r.isLAN || r.limitsLAN.get() {
		take(r.waiter, n)
	}
	return n, err
}

// limitedWriter is a rate limited io.Writer
type limitedWriter struct {
	writer    io.Writer
	limitsLAN *atomicBool
	waiter    waiter
	isLAN     bool
}

func (w *limitedWriter) Write(buf []byte) (int, error) {
	if !w.isLAN || w.limitsLAN.get() {
		take(w.waiter, len(buf))
	}
	return w.writer.Write(buf)
}

// take is a utility function to consume tokens from a overall rate.Limiter and deviceLimiter.
// No call to WaitN can be larger than the limiter burst size so we split it up into
// several calls when necessary.
func take(waiter waiter, tokens int) {
	if tokens < limiterBurstSize {
		// This is the by far more common case so we get it out of the way
		// early.
		waiter.WaitN(context.TODO(), tokens)
		return
	}

	for tokens > 0 {
		// Consume limiterBurstSize tokens at a time until we're done.
		if tokens > limiterBurstSize {
			waiter.WaitN(context.TODO(), limiterBurstSize)
			tokens -= limiterBurstSize
		} else {
			waiter.WaitN(context.TODO(), tokens)
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

// totalWaiter waits for all of the waiters
type totalWaiter []waiter

func (tw totalWaiter) WaitN(ctx context.Context, n int) error {
	for _, w := range tw {
		if err := w.WaitN(ctx, n); err != nil {
			// error here is context cancellation, most likely, so we abort
			// early
			return err
		}
	}
	return nil
}
