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
	"golang.org/x/net/context"
	"golang.org/x/time/rate"
	"sync"
)

// limiter manages a read and write rate limit, reacting to config changes
// as appropriate.
type limiter struct {
	write               *rate.Limiter
	read                *rate.Limiter
	limitsLAN           atomicBool
	deviceReadLimiters  *sync.Map
	deviceWriteLimiters *sync.Map
	myID                protocol.DeviceID
	mu                  *sync.Mutex
}

const limiterBurstSize = 4 * 128 << 10

func newLimiter(deviceID protocol.DeviceID, cfg *config.Wrapper) *limiter {

	l := &limiter{
		write:               rate.NewLimiter(rate.Inf, limiterBurstSize),
		read:                rate.NewLimiter(rate.Inf, limiterBurstSize),
		myID:                deviceID,
		mu:                  &sync.Mutex{},
		deviceReadLimiters:  new(sync.Map),
		deviceWriteLimiters: new(sync.Map),
	}

	// Get initial device configuration
	devices := cfg.RawCopy().Devices
	for _, value := range devices {
		value.MaxRecvKbps = -1
		value.MaxSendKbps = -1
	}
	prev := config.Configuration{Options: config.OptionsConfiguration{MaxRecvKbps: -1, MaxSendKbps: -1}, Devices: devices}

	// Keep read/write limiters for every connected device
	l.rebuildMap(prev)

	cfg.Subscribe(l)
	l.CommitConfiguration(prev, cfg.RawCopy())
	return l
}

// Create new maps with new limiters
func (lim *limiter) rebuildMap(to config.Configuration) {
	deviceWriteLimiters := new(sync.Map)
	deviceReadLimiters := new(sync.Map)

	// copy *limiter in case remote device is still connected, when remote device is added we create new read/write limiters
	for _, v := range to.Devices {
		if readLimiter, ok := lim.deviceReadLimiters.Load(v.DeviceID); ok {
			deviceReadLimiters.Store(v.DeviceID, readLimiter)
		} else {
			deviceReadLimiters.Store(v.DeviceID, rate.NewLimiter(rate.Inf, limiterBurstSize))
		}

		if writeLimiter, ok := lim.deviceWriteLimiters.Load(v.DeviceID); ok {
			deviceWriteLimiters.Store(v.DeviceID, writeLimiter)
		} else {
			deviceWriteLimiters.Store(v.DeviceID, rate.NewLimiter(rate.Inf, limiterBurstSize))
		}
	}

	// assign new maps
	lim.mu.Lock()
	defer lim.mu.Unlock()
	lim.deviceWriteLimiters = deviceWriteLimiters
	lim.deviceReadLimiters = deviceReadLimiters

	l.Debugln("Rebuild of device limiters map finished")
}

// Compare read/write limits in configurations
func (lim *limiter) checkDeviceLimits(from, to config.Configuration) bool {
	for i := range from.Devices {
		if from.Devices[i].DeviceID != to.Devices[i].DeviceID {
			// Something has changed in device configuration
			lim.rebuildMap(to)
			return false
		}
		// Read/write limits were changed for this device
		if from.Devices[i].MaxSendKbps != to.Devices[i].MaxSendKbps || from.Devices[i].MaxRecvKbps != to.Devices[i].MaxRecvKbps {
			return false
		}
	}
	return true
}

func (lim *limiter) newReadLimiter(remoteID protocol.DeviceID, r io.Reader, isLAN bool) io.Reader {
	return &limitedReader{reader: r, limiter: lim, isLAN: isLAN, remoteID: remoteID}
}

func (lim *limiter) newWriteLimiter(remoteID protocol.DeviceID, w io.Writer, isLAN bool) io.Writer {
	return &limitedWriter{writer: w, limiter: lim, isLAN: isLAN, remoteID: remoteID}
}

func (lim *limiter) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (lim *limiter) CommitConfiguration(from, to config.Configuration) bool {
	if len(from.Devices) == len(to.Devices) &&
		from.Options.MaxRecvKbps == to.Options.MaxRecvKbps &&
		from.Options.MaxSendKbps == to.Options.MaxSendKbps &&
		from.Options.LimitBandwidthInLan == to.Options.LimitBandwidthInLan &&
		lim.checkDeviceLimits(from, to) {
		return true
	}

	// A device has been added or removed
	if len(from.Devices) != len(to.Devices) {
		lim.rebuildMap(to)
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

	// Set limits for devices
	for _, v := range to.Devices {
		if v.DeviceID == lim.myID {
			// This limiter was created for local device. Should skip this device
			continue
		}

		readLimiter, _ := lim.deviceReadLimiters.Load(v.DeviceID)
		if v.MaxRecvKbps <= 0 {
			readLimiter.(*rate.Limiter).SetLimit(rate.Inf)
		} else {
			readLimiter.(*rate.Limiter).SetLimit(1024 * rate.Limit(v.MaxRecvKbps))
		}

		writeLimiter, _ := lim.deviceWriteLimiters.Load(v.DeviceID)
		if v.MaxSendKbps <= 0 {
			writeLimiter.(*rate.Limiter).SetLimit(rate.Inf)
		} else {
			writeLimiter.(*rate.Limiter).SetLimit(1024 * rate.Limit(v.MaxSendKbps))
		}

		sendLimitStr := "is unlimited"
		recvLimitStr := "is unlimited"
		if v.MaxSendKbps > 0 {
			sendLimitStr = fmt.Sprintf("limit is %d KiB/s", v.MaxSendKbps)
		}

		if v.MaxRecvKbps > 0 {
			recvLimitStr = fmt.Sprintf("limit is %d KiB/s", v.MaxRecvKbps)
		}
		l.Infof("Device %s: send rate %s, receive rate %s", v.DeviceID, sendLimitStr, recvLimitStr)
	}

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

// limitedReader is a rate limited io.Reader
type limitedReader struct {
	reader   io.Reader
	limiter  *limiter
	isLAN    bool
	remoteID protocol.DeviceID
}

func (r *limitedReader) Read(buf []byte) (int, error) {
	n, err := r.reader.Read(buf)
	if !r.isLAN || r.limiter.limitsLAN.get() {

		// in case rebuildMap was called
		r.limiter.mu.Lock()
		deviceLimiter, ok := r.limiter.deviceReadLimiters.Load(r.remoteID)
		r.limiter.mu.Unlock()
		if !ok {
			l.Debugln("deviceReadLimiter was not in the map")
			deviceLimiter = rate.NewLimiter(rate.Inf, limiterBurstSize)
		}
		take(r.limiter.read, deviceLimiter.(*rate.Limiter), n)
	}
	return n, err
}

// limitedWriter is a rate limited io.Writer
type limitedWriter struct {
	writer   io.Writer
	limiter  *limiter
	isLAN    bool
	remoteID protocol.DeviceID
}

func (w *limitedWriter) Write(buf []byte) (int, error) {
	if !w.isLAN || w.limiter.limitsLAN.get() {

		// in case rebuildMap was called
		w.limiter.mu.Lock()
		deviceLimiter, ok := w.limiter.deviceWriteLimiters.Load(w.remoteID)
		w.limiter.mu.Unlock()
		if !ok {
			l.Debugln("deviceWriteLimiter was not in the map")
			deviceLimiter = rate.NewLimiter(rate.Inf, limiterBurstSize)
		}
		take(w.limiter.write, deviceLimiter.(*rate.Limiter), len(buf))
	}
	return w.writer.Write(buf)
}

// take is a utility function to consume tokens from a rate.Limiter. No call
// to WaitN can be larger than the limiter burst size so we split it up into
// several calls when necessary.
func take(l, deviceLimiter *rate.Limiter, tokens int) {

	if tokens < limiterBurstSize {
		// This is the by far more common case so we get it out of the way
		// early.
		deviceLimiter.WaitN(context.TODO(), tokens)
		l.WaitN(context.TODO(), tokens)
		return
	}

	for tokens > 0 {
		// Consume limiterBurstSize tokens at a time until we're done.
		if tokens > limiterBurstSize {
			deviceLimiter.WaitN(context.TODO(), limiterBurstSize)
			l.WaitN(context.TODO(), limiterBurstSize)
			tokens -= limiterBurstSize
		} else {
			deviceLimiter.WaitN(context.TODO(), tokens)
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
