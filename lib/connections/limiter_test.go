// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"io"
	"math/rand"
	"sync/atomic"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/netutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/time/rate"
)

var (
	device1, device2, device3, device4     protocol.DeviceID
	dev1Conf, dev2Conf, dev3Conf, dev4Conf config.DeviceConfiguration
)

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
	device3, _ = protocol.DeviceIDFromString("LGFPDIT-7SKNNJL-VJZA4FC-7QNCRKA-CE753K7-2BW5QDK-2FOZ7FR-FEP57QJ")
	device4, _ = protocol.DeviceIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")
}

func newDeviceConfiguration(w config.Wrapper, id protocol.DeviceID, name string) config.DeviceConfiguration {
	cfg := w.DefaultDevice()
	cfg.DeviceID = id
	cfg.Name = name
	return cfg
}

func initConfig() (config.Wrapper, context.CancelFunc) {
	wrapper := config.Wrap("/dev/null", config.New(device1), device1, events.NoopLogger)
	dev1Conf = newDeviceConfiguration(wrapper, device1, "device1")
	dev2Conf = newDeviceConfiguration(wrapper, device2, "device2")
	dev3Conf = newDeviceConfiguration(wrapper, device3, "device3")
	dev4Conf = newDeviceConfiguration(wrapper, device4, "device4")

	ctx, cancel := context.WithCancel(context.Background())
	go wrapper.Serve(ctx)

	dev2Conf.MaxRecvKbps = rand.Int() % 100000
	dev2Conf.MaxSendKbps = rand.Int() % 100000

	waiter, _ := wrapper.Modify(func(cfg *config.Configuration) {
		cfg.SetDevices([]config.DeviceConfiguration{dev1Conf, dev2Conf, dev3Conf, dev4Conf})
	})
	waiter.Wait()
	return wrapper, cancel
}

func TestLimiterInit(t *testing.T) {
	wrapper, wrapperCancel := initConfig()
	defer wrapperCancel()
	lim := newLimiter(device1, wrapper)

	device2ReadLimit := dev2Conf.MaxRecvKbps
	device2WriteLimit := dev2Conf.MaxSendKbps

	expectedR := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(device2ReadLimit*1024), limiterBurstSize),
		device3: rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}

	expectedW := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(device2WriteLimit*1024), limiterBurstSize),
		device3: rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}

	actualR := lim.deviceReadLimiters
	actualW := lim.deviceWriteLimiters

	checkActualAndExpected(t, actualR, actualW, expectedR, expectedW)
}

func TestSetDeviceLimits(t *testing.T) {
	wrapper, wrapperCancel := initConfig()
	defer wrapperCancel()
	lim := newLimiter(device1, wrapper)

	// should still be inf/inf because this is local device
	dev1ReadLimit := rand.Int() % 100000
	dev1WriteLimit := rand.Int() % 100000
	dev1Conf.MaxRecvKbps = dev1ReadLimit
	dev1Conf.MaxSendKbps = dev1WriteLimit

	dev2ReadLimit := rand.Int() % 100000
	dev2WriteLimit := rand.Int() % 100000
	dev2Conf.MaxRecvKbps = dev2ReadLimit
	dev2Conf.MaxSendKbps = dev2WriteLimit

	dev3ReadLimit := rand.Int() % 10000
	dev3Conf.MaxRecvKbps = dev3ReadLimit

	waiter, _ := wrapper.Modify(func(cfg *config.Configuration) {
		cfg.SetDevices([]config.DeviceConfiguration{dev1Conf, dev2Conf, dev3Conf, dev4Conf})
	})
	waiter.Wait()

	expectedR := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(dev2ReadLimit*1024), limiterBurstSize),
		device3: rate.NewLimiter(rate.Limit(dev3ReadLimit*1024), limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}
	expectedW := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(dev2WriteLimit*1024), limiterBurstSize),
		device3: rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}

	actualR := lim.deviceReadLimiters
	actualW := lim.deviceWriteLimiters

	checkActualAndExpected(t, actualR, actualW, expectedR, expectedW)
}

func TestRemoveDevice(t *testing.T) {
	wrapper, wrapperCancel := initConfig()
	defer wrapperCancel()
	lim := newLimiter(device1, wrapper)

	waiter, _ := wrapper.RemoveDevice(device3)
	waiter.Wait()
	expectedR := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(dev2Conf.MaxRecvKbps*1024), limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}
	expectedW := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(dev2Conf.MaxSendKbps*1024), limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}
	actualR := lim.deviceReadLimiters
	actualW := lim.deviceWriteLimiters

	checkActualAndExpected(t, actualR, actualW, expectedR, expectedW)
}

func TestAddDevice(t *testing.T) {
	wrapper, wrapperCancel := initConfig()
	defer wrapperCancel()
	lim := newLimiter(device1, wrapper)

	addedDevice, _ := protocol.DeviceIDFromString("XZJ4UNS-ENI7QGJ-J45DT6G-QSGML2K-6I4XVOG-NAZ7BF5-2VAOWNT-TFDOMQU")
	addDevConf := newDeviceConfiguration(wrapper, addedDevice, "addedDevice")
	addDevConf.MaxRecvKbps = 120
	addDevConf.MaxSendKbps = 240

	waiter, _ := wrapper.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(addDevConf)
	})
	waiter.Wait()

	expectedR := map[protocol.DeviceID]*rate.Limiter{
		device2:     rate.NewLimiter(rate.Limit(dev2Conf.MaxRecvKbps*1024), limiterBurstSize),
		device3:     rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4:     rate.NewLimiter(rate.Inf, limiterBurstSize),
		addedDevice: rate.NewLimiter(rate.Limit(addDevConf.MaxRecvKbps*1024), limiterBurstSize),
	}

	expectedW := map[protocol.DeviceID]*rate.Limiter{
		device2:     rate.NewLimiter(rate.Limit(dev2Conf.MaxSendKbps*1024), limiterBurstSize),
		device3:     rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4:     rate.NewLimiter(rate.Inf, limiterBurstSize),
		addedDevice: rate.NewLimiter(rate.Limit(addDevConf.MaxSendKbps*1024), limiterBurstSize),
	}
	actualR := lim.deviceReadLimiters
	actualW := lim.deviceWriteLimiters

	checkActualAndExpected(t, actualR, actualW, expectedR, expectedW)
}

func TestAddAndRemove(t *testing.T) {
	wrapper, wrapperCancel := initConfig()
	defer wrapperCancel()
	lim := newLimiter(device1, wrapper)

	addedDevice, _ := protocol.DeviceIDFromString("XZJ4UNS-ENI7QGJ-J45DT6G-QSGML2K-6I4XVOG-NAZ7BF5-2VAOWNT-TFDOMQU")
	addDevConf := newDeviceConfiguration(wrapper, addedDevice, "addedDevice")
	addDevConf.MaxRecvKbps = 120
	addDevConf.MaxSendKbps = 240

	waiter, _ := wrapper.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(addDevConf)
	})
	waiter.Wait()
	waiter, _ = wrapper.RemoveDevice(device3)
	waiter.Wait()

	expectedR := map[protocol.DeviceID]*rate.Limiter{
		device2:     rate.NewLimiter(rate.Limit(dev2Conf.MaxRecvKbps*1024), limiterBurstSize),
		device4:     rate.NewLimiter(rate.Inf, limiterBurstSize),
		addedDevice: rate.NewLimiter(rate.Limit(addDevConf.MaxRecvKbps*1024), limiterBurstSize),
	}

	expectedW := map[protocol.DeviceID]*rate.Limiter{
		device2:     rate.NewLimiter(rate.Limit(dev2Conf.MaxSendKbps*1024), limiterBurstSize),
		device4:     rate.NewLimiter(rate.Inf, limiterBurstSize),
		addedDevice: rate.NewLimiter(rate.Limit(addDevConf.MaxSendKbps*1024), limiterBurstSize),
	}
	actualR := lim.deviceReadLimiters
	actualW := lim.deviceWriteLimiters

	checkActualAndExpected(t, actualR, actualW, expectedR, expectedW)
}

func TestLimitedWriterWrite(t *testing.T) {
	// Check that the limited writer writes the correct data in the correct manner.

	// A buffer with random data that is larger than the write size and not
	// a precise multiple either.
	src := make([]byte, int(12.5*8192))
	if _, err := crand.Reader.Read(src); err != nil {
		t.Fatal(err)
	}

	// Write it to the destination using a limited writer, with a wrapper to
	// count the write calls. The defaults on the limited writer should mean
	// it is used (and doesn't take the fast path). In practice the limiter
	// won't delay the test as the burst size is large enough to accommodate
	// regardless of the rate.
	dst := new(bytes.Buffer)
	cw := &countingWriter{w: dst}
	lw := netutil.NewLimitedWriter(cw, waiterHolder{
		waiter:    rate.NewLimiter(rate.Limit(42), limiterBurstSize),
		limitsLAN: new(atomic.Bool),
		isLAN:     false, // enables limiting
	})
	if _, err := io.Copy(lw, bytes.NewReader(src)); err != nil {
		t.Fatal(err)
	}

	// Verify there were lots of writes (we expect one kilobyte write size
	// for the very low rate in this test) and that the end result is
	// identical.
	if cw.writeCount < 10*8 {
		t.Error("expected lots of smaller writes")
	}
	if cw.writeCount > 15*8 {
		t.Error("expected fewer larger writes")
	}
	if !bytes.Equal(src, dst.Bytes()) {
		t.Error("results should be equal")
	}

	// Write it to the destination using a limited writer, with a wrapper to
	// count the write calls. Now we make sure the fast path is used.
	dst = new(bytes.Buffer)
	cw = &countingWriter{w: dst}
	lw = netutil.NewLimitedWriter(cw, waiterHolder{
		waiter:    rate.NewLimiter(rate.Limit(42), limiterBurstSize),
		limitsLAN: new(atomic.Bool),
		isLAN:     true, // disables limiting
	})
	if _, err := io.Copy(lw, bytes.NewReader(src)); err != nil {
		t.Fatal(err)
	}

	// Verify there were a single write and that the end result is identical.
	if cw.writeCount != 1 {
		t.Error("expected just the one write")
	}
	if !bytes.Equal(src, dst.Bytes()) {
		t.Error("results should be equal")
	}

	// Once more, but making sure the fast path is used for an unlimited
	// rate, with multiple unlimited raters even (global and per-device).
	dst = new(bytes.Buffer)
	cw = &countingWriter{w: dst}
	lw = netutil.NewLimitedWriter(cw, waiterHolder{
		waiter:    totalWaiter{rate.NewLimiter(rate.Inf, limiterBurstSize), rate.NewLimiter(rate.Inf, limiterBurstSize)},
		limitsLAN: new(atomic.Bool),
		isLAN:     false, // enables limiting
	})
	if _, err := io.Copy(lw, bytes.NewReader(src)); err != nil {
		t.Fatal(err)
	}

	// Verify there were a single write and that the end result is identical.
	if cw.writeCount != 1 {
		t.Error("expected just the one write")
	}
	if !bytes.Equal(src, dst.Bytes()) {
		t.Error("results should be equal")
	}

	// Once more, but making sure we *don't* take the fast path when there
	// is a combo of limited and unlimited writers.
	dst = new(bytes.Buffer)
	cw = &countingWriter{w: dst}
	lw = netutil.NewLimitedWriter(cw, waiterHolder{
		waiter: totalWaiter{
			rate.NewLimiter(rate.Inf, limiterBurstSize),
			rate.NewLimiter(rate.Limit(42), limiterBurstSize),
			rate.NewLimiter(rate.Inf, limiterBurstSize),
		},
		limitsLAN: new(atomic.Bool),
		isLAN:     false, // enables limiting
	})
	if _, err := io.Copy(lw, bytes.NewReader(src)); err != nil {
		t.Fatal(err)
	}

	// Verify there were lots of writes and that the end result is identical.
	if cw.writeCount < 10*8 {
		t.Error("expected lots of smaller writes")
	}
	if cw.writeCount > 15*8 {
		t.Error("expected fewer larger writes")
	}
	if !bytes.Equal(src, dst.Bytes()) {
		t.Error("results should be equal")
	}
}

func TestTotalWaiterLimit(t *testing.T) {
	cases := []struct {
		w waiter
		r rate.Limit
	}{
		{
			totalWaiter{},
			rate.Inf,
		},
		{
			totalWaiter{rate.NewLimiter(rate.Inf, 42)},
			rate.Inf,
		},
		{
			totalWaiter{rate.NewLimiter(rate.Inf, 42), rate.NewLimiter(rate.Inf, 42)},
			rate.Inf,
		},
		{
			totalWaiter{rate.NewLimiter(rate.Inf, 42), rate.NewLimiter(rate.Limit(12), 42), rate.NewLimiter(rate.Limit(15), 42)},
			rate.Limit(12),
		},
	}

	for _, tc := range cases {
		l := tc.w.Limit()
		if l != tc.r {
			t.Error("incorrect limit returned")
		}
	}
}

func checkActualAndExpected(t *testing.T, actualR, actualW, expectedR, expectedW map[protocol.DeviceID]*rate.Limiter) {
	t.Helper()
	if len(expectedW) != len(actualW) || len(expectedR) != len(actualR) {
		t.Errorf("Map lengths differ!")
	}

	for key, val := range expectedR {
		if _, ok := actualR[key]; !ok {
			t.Errorf("Device %s not found in limiter", key)
		}

		if val.Limit() != actualR[key].Limit() {
			t.Errorf("Read limits for device %s differ actual: %f, expected: %f", key, actualR[key].Limit(), val.Limit())
		}
		if expectedW[key].Limit() != actualW[key].Limit() {
			t.Errorf("Write limits for device %s differ actual: %f, expected: %f", key, actualW[key].Limit(), expectedW[key].Limit())
		}
	}
}

type countingWriter struct {
	w          io.Writer
	writeCount int
}

func (w *countingWriter) Write(data []byte) (int, error) {
	w.writeCount++
	return w.w.Write(data)
}
