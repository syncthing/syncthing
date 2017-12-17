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
	"github.com/syncthing/syncthing/lib/protocol"
)

// limiter manages a read and write rate limit, reacting to config changes
// as appropriate.
type limiter struct {
	globalWrite *rate.Limiter
	globalRead *rate.Limiter
	write     *rate.Limiter
	read      *rate.Limiter
	limitsLAN atomicBool
	remoteID  protocol.DeviceID
	deviceID protocol.DeviceID
}
var myID protocol.DeviceID

const limiterBurstSize = 4 * 128 << 10

func newLimiter(deviceID protocol.DeviceID, cfg *config.Wrapper) *limiters {
	devices := getInitialDevicesConfiguration(cfg.RawCopy())
	myID = deviceID
	l := make(limiters)
	globalRead := rate.NewLimiter(rate.Inf, limiterBurstSize)
	globalWrite := rate.NewLimiter(rate.Inf, limiterBurstSize)
	for _, v := range devices {
		l[v.DeviceID] = &limiter{
			globalWrite: globalWrite,
			globalRead: globalRead,
			write: rate.NewLimiter(rate.Inf, limiterBurstSize),
			read: rate.NewLimiter(rate.Inf, limiterBurstSize),
			remoteID: v.DeviceID,
			deviceID: myID,
		}
	}
	//l := &limiter{
	//	write: rate.NewLimiter(rate.Inf, limiterBurstSize),
	//	read:  rate.NewLimiter(rate.Inf, limiterBurstSize),
	//	DeviceID:myID,
	//}
	cfg.Subscribe(&l)
	prev := config.Configuration{Options: config.OptionsConfiguration{MaxRecvKbps: -1, MaxSendKbps: -1}, Devices: devices}
	l.CommitConfiguration(prev, cfg.RawCopy())
	return &l
}

func getInitialDevicesConfiguration(cfgCopy config.Configuration) config.DeviceConfigurationList {
	for _, value := range cfgCopy.Devices {
		value.MaxRecvKbps = -1
		value.MaxSendKbps = -1
	}
	return cfgCopy.Devices
}

func (lim *limiters) newReadLimiter(remoteID protocol.DeviceID, r io.Reader, isLAN bool) io.Reader {
	return &limitedReader{reader: r, limiter: (*lim)[remoteID], isLAN: isLAN}
}

func (lim *limiters) newWriteLimiter(remoteID protocol.DeviceID, w io.Writer, isLAN bool) io.Writer {
	return &limitedWriter{writer: w, limiter: (*lim)[remoteID], isLAN: isLAN}
}

func (lim *limiters) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (lim *limiters) checkDeviceLimits(from, to config.Configuration) bool {
	for i, v := range from.Devices {
		if v.MaxSendKbps != to.Devices[i].MaxSendKbps || v.MaxRecvKbps != to.Devices[i].MaxRecvKbps {
			return false
		}
	}
	return true
}

func (lim *limiters) rebuildMap(to config.Configuration) {
	*lim = make(limiters)

	for _, v := range to.Devices {
		(*lim)[v.DeviceID] = &limiter{
			globalWrite: rate.NewLimiter(rate.Inf, limiterBurstSize),
			globalRead: rate.NewLimiter(rate.Inf, limiterBurstSize),
			write: rate.NewLimiter(1024 * rate.Limit(v.MaxSendKbps), limiterBurstSize),
			read: rate.NewLimiter(1024 * rate.Limit(v.MaxRecvKbps), limiterBurstSize),
			remoteID: v.DeviceID,
			deviceID: myID,
		}
	}
}

func (lim *limiters) CommitConfiguration(from, to config.Configuration) bool {
	if len(from.Devices) == len(to.Devices) &&
		from.Options.MaxRecvKbps == to.Options.MaxRecvKbps &&
		from.Options.MaxSendKbps == to.Options.MaxSendKbps &&
		from.Options.LimitBandwidthInLan == to.Options.LimitBandwidthInLan &&
		lim.checkDeviceLimits(from, to){
		return true
	}

	// New device has been added, need rebuild lim? TODO check this
	if len(from.Devices) != len(to.Devices) {
		lim.rebuildMap(to)
	}

	// The rate variables are in KiB/s in the config (despite the camel casing
	// of the name). We multiply by 1024 to get bytes/s.
	for _, v := range to.Devices {
		if v.DeviceID == myID {
			// this is limiter created for local device. Should skip this
			continue
		}
		if v.MaxRecvKbps > 0 {
			(*lim)[v.DeviceID].read.SetLimit(1024 * rate.Limit(v.MaxRecvKbps))
		} else if to.Options.MaxRecvKbps <= 0 {
			(*lim)[v.DeviceID].read.SetLimit(rate.Inf)
		} else {
			(*lim)[v.DeviceID].read.SetLimit(1024 * rate.Limit(to.Options.MaxRecvKbps))
		}

		if v.MaxSendKbps > 0 {
			(*lim)[v.DeviceID].write.SetLimit(1024 * rate.Limit(v.MaxSendKbps))
		} else if to.Options.MaxSendKbps <= 0 {
			(*lim)[v.DeviceID].write.SetLimit(rate.Inf)
		} else {
			(*lim)[v.DeviceID].write.SetLimit(1024 * rate.Limit(to.Options.MaxSendKbps))
		}

		(*lim)[v.DeviceID].limitsLAN.set(to.Options.LimitBandwidthInLan)

		sendLimitStr := "is unlimited"
		recvLimitStr := "is unlimited"
		if v.MaxSendKbps > 0 {
			sendLimitStr = fmt.Sprintf("limit is %d KiB/s", v.MaxSendKbps)
		}else if to.Options.MaxSendKbps > 0 {
			sendLimitStr = fmt.Sprintf("limit is %d KiB/s", to.Options.MaxSendKbps)
		}

		if v.MaxRecvKbps > 0 {
			recvLimitStr = fmt.Sprintf("limit is %d KiB/s", v.MaxRecvKbps)
		}else if to.Options.MaxRecvKbps > 0 {
			recvLimitStr = fmt.Sprintf("limit is %d KiB/s", to.Options.MaxRecvKbps)
		}
		l.Infof("Device %s: send rate %s, receive rate %s", v.DeviceID,sendLimitStr, recvLimitStr)
	}
	if to.Options.LimitBandwidthInLan {
		l.Infoln("Rate limits apply to LAN connections")
	} else {
		l.Infoln("Rate limits do not apply to LAN connections")
	}
	return true
}

func (lim *limiters) String() string {
	// required by config.Committer interface
	return "connections.limiter"
}

// limitedReader is a rate limited io.Reader
type  limitedReader struct {
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

type limiters map[protocol.DeviceID]*limiter // TODO mabe this should be a map [deviceID]limiter? seems like a reasonable idea