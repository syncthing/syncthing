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
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/net/context"
	"golang.org/x/time/rate"
)

// limiter manages a read and write rate limit, reacting to config changes
// as appropriate.
type limiter struct {
	write               *rate.Limiter
	read                *rate.Limiter
	limitsLAN           atomicBool
	deviceReadLimiters  map[protocol.DeviceID]*rate.Limiter
	deviceWriteLimiters map[protocol.DeviceID]*rate.Limiter
	mu                  sync.Mutex
}

const limiterBurstSize = 4 * 128 << 10

func newLimiter(cfg *config.Wrapper) *limiter {
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

			l.Infof("Device %s send rate %s, receive rate %s", dev.DeviceID, readLimitStr, writeLimitStr)
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
	l.Infof("Overall send rate %s, receive rate %s", sendLimitStr, recvLimitStr)

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

func (lim *limiter) getLimiters(remoteID protocol.DeviceID, c internalConn, isLAN bool) (io.Reader, io.Writer) {
	lim.mu.Lock()
	wr := lim.newLimitedWriterLocked(remoteID, c, isLAN)
	rd := lim.newLimitedReaderLocked(remoteID, c, isLAN)
	lim.mu.Unlock()
	return rd, wr
}

func (lim *limiter) newLimitedReaderLocked(remoteID protocol.DeviceID, r io.Reader, isLAN bool) io.Reader {
	return &limitedReader{reader: r, limiter: lim, deviceLimiter: lim.getReadLimiterLocked(remoteID), isLAN: isLAN}
}

func (lim *limiter) newLimitedWriterLocked(remoteID protocol.DeviceID, w io.Writer, isLAN bool) io.Writer {
	return &limitedWriter{writer: w, limiter: lim, deviceLimiter: lim.getWriteLimiterLocked(remoteID), isLAN: isLAN}
}

// limitedReader is a rate limited io.Reader
type limitedReader struct {
	reader        io.Reader
	limiter       *limiter
	deviceLimiter *rate.Limiter
	isLAN         bool
}

func (r *limitedReader) Read(buf []byte) (int, error) {
	n, err := r.reader.Read(buf)
	if !r.isLAN || r.limiter.limitsLAN.get() {
		take(r.limiter.read, r.deviceLimiter, n)
	}
	return n, err
}

// limitedWriter is a rate limited io.Writer
type limitedWriter struct {
	writer        io.Writer
	limiter       *limiter
	deviceLimiter *rate.Limiter
	isLAN         bool
}

func (w *limitedWriter) Write(buf []byte) (int, error) {
	if !w.isLAN || w.limiter.limitsLAN.get() {
		take(w.limiter.write, w.deviceLimiter, len(buf))
	}
	return w.writer.Write(buf)
}

// take is a utility function to consume tokens from a overall rate.Limiter and deviceLimiter.
// No call to WaitN can be larger than the limiter burst size so we split it up into
// several calls when necessary.
func take(overallLimiter, deviceLimiter *rate.Limiter, tokens int) {
	if tokens < limiterBurstSize {
		// This is the by far more common case so we get it out of the way
		// early.
		deviceLimiter.WaitN(context.TODO(), tokens)
		overallLimiter.WaitN(context.TODO(), tokens)
		return
	}

	for tokens > 0 {
		// Consume limiterBurstSize tokens at a time until we're done.
		if tokens > limiterBurstSize {
			deviceLimiter.WaitN(context.TODO(), limiterBurstSize)
			overallLimiter.WaitN(context.TODO(), limiterBurstSize)
			tokens -= limiterBurstSize
		} else {
			deviceLimiter.WaitN(context.TODO(), tokens)
			overallLimiter.WaitN(context.TODO(), tokens)
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

// Utility functions for atomic operations on device limiters map
func (lim *limiter) getWriteLimiterLocked(deviceID protocol.DeviceID) *rate.Limiter {
	limiter, ok := lim.deviceWriteLimiters[deviceID]

	if !ok {
		limiter = rate.NewLimiter(rate.Inf, limiterBurstSize)
		lim.deviceWriteLimiters[deviceID] = limiter
	}

	return limiter
}

func (lim *limiter) getReadLimiterLocked(deviceID protocol.DeviceID) *rate.Limiter {
	limiter, ok := lim.deviceReadLimiters[deviceID]

	if !ok {
		limiter = rate.NewLimiter(rate.Inf, limiterBurstSize)
		lim.deviceReadLimiters[deviceID] = limiter
	}

	return limiter
}
