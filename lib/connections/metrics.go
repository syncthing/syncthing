// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricDeviceActiveConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "connections",
		Name:      "active",
		Help:      "Number of currently active connections, per device. If value is 0, the device is disconnected.",
	}, []string{"device"})
)

func registerDeviceMetrics(deviceID string) {
	// Register metrics for this device, so that counters & gauges are present even
	// when zero.
	metricDeviceActiveConnections.WithLabelValues(deviceID)
}
