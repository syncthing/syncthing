// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/time/rate"
	"math/rand"
	"testing"
)

var device1, device2, device3, device4 protocol.DeviceID
var dev1Conf, dev2Conf, dev3Conf, dev4Conf config.DeviceConfiguration
var cfg *config.Wrapper

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
	device3, _ = protocol.DeviceIDFromString("LGFPDIT-7SKNNJL-VJZA4FC-7QNCRKA-CE753K7-2BW5QDK-2FOZ7FR-FEP57QJ")
	device4, _ = protocol.DeviceIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")

	cfg = config.Wrap("/dev/null", config.New(device1))
}

func TestLimiterInit(t *testing.T) {
	initConfig()
	lim := newLimiter(cfg)

	expectedR := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(dev2Conf.MaxRecvKbps*1024), limiterBurstSize),
		device3: rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}

	expectedW := map[protocol.DeviceID]*rate.Limiter{
		device2: rate.NewLimiter(rate.Limit(dev2Conf.MaxSendKbps*1024), limiterBurstSize),
		device3: rate.NewLimiter(rate.Inf, limiterBurstSize),
		device4: rate.NewLimiter(rate.Inf, limiterBurstSize),
	}

	actualR := lim.deviceReadLimiters
	actualW := lim.deviceWriteLimiters

	checkActualAndExpected(actualR, actualW, expectedR, expectedW, t)
}

func TestSetDeviceLimits(t *testing.T) {
	initConfig()
	lim := newLimiter(cfg)

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

	waiter, _ := cfg.SetDevices([]config.DeviceConfiguration{dev1Conf, dev2Conf, dev3Conf, dev4Conf})
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

	checkActualAndExpected(actualR, actualW, expectedR, expectedW, t)
}

func TestRemoveDevice(t *testing.T) {
	initConfig()
	lim := newLimiter(cfg)

	waiter, _ := cfg.RemoveDevice(device3)
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

	checkActualAndExpected(actualR, actualW, expectedR, expectedW, t)
}

func TestAddDevice(t *testing.T) {
	initConfig()
	lim := newLimiter(cfg)

	addedDevice, _ := protocol.DeviceIDFromString("XZJ4UNS-ENI7QGJ-J45DT6G-QSGML2K-6I4XVOG-NAZ7BF5-2VAOWNT-TFDOMQU")
	addDevConf := config.NewDeviceConfiguration(addedDevice, "addedDevice")
	addDevConf.MaxRecvKbps = 120
	addDevConf.MaxSendKbps = 240

	waiter, _ := cfg.SetDevice(addDevConf)
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

	checkActualAndExpected(actualR, actualW, expectedR, expectedW, t)
}

func TestAddAndRemove(t *testing.T) {
	initConfig()
	lim := newLimiter(cfg)

	addedDevice, _ := protocol.DeviceIDFromString("XZJ4UNS-ENI7QGJ-J45DT6G-QSGML2K-6I4XVOG-NAZ7BF5-2VAOWNT-TFDOMQU")
	addDevConf := config.NewDeviceConfiguration(addedDevice, "addedDevice")
	addDevConf.MaxRecvKbps = 120
	addDevConf.MaxSendKbps = 240

	waiter, _ := cfg.SetDevice(addDevConf)
	waiter.Wait()
	waiter, _ = cfg.RemoveDevice(device3)
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

	checkActualAndExpected(actualR, actualW, expectedR, expectedW, t)
}

func initConfig() {
	dev1Conf = config.NewDeviceConfiguration(device1, "device1")
	dev2Conf = config.NewDeviceConfiguration(device2, "device2")
	dev3Conf = config.NewDeviceConfiguration(device3, "device3")
	dev4Conf = config.NewDeviceConfiguration(device4, "device4")

	dev2Conf.MaxRecvKbps = rand.Int() % 100000
	dev2Conf.MaxSendKbps = rand.Int() % 100000

	waiter, _ := cfg.SetDevices([]config.DeviceConfiguration{dev1Conf, dev2Conf, dev3Conf, dev4Conf})
	waiter.Wait()
}

func checkActualAndExpected(actualR, actualW, expectedR, expectedW map[protocol.DeviceID]*rate.Limiter, t *testing.T) {
	if len(expectedW) != len(actualW) || len(expectedR) != len(actualR) {
		t.Errorf("Map lengths differ!")
	}

	for key, val := range expectedR {
		if _, ok := actualR[key]; !ok {
			t.Errorf("Device %s not found in limiter", key)
		}

		if val.Limit() != actualR[key].Limit() {
			t.Errorf("Limits for device %s differ actual: %f, expected: %f", key, actualR[key].Limit(), val.Limit())
		}
		if expectedW[key].Limit() != actualW[key].Limit() {
			t.Errorf("Limits for device %s differ actual: %f, expected: %f", key, actualW[key].Limit(), expectedW[key].Limit())
		}
	}
}
