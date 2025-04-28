// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"github.com/syncthing/syncthing/lib/config"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var metricDeviceActiveConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "syncthing",
	Subsystem: "connections",
	Name:      "active",
	Help:      "Number of currently active connections, per device. If value is 0, the device is disconnected.",
}, []string{"device"})

func registerDeviceMetrics(dc config.DeviceConfiguration) {
	registerDeviceInfoGauge(dc)
	// Register metrics for this device, so that counters & gauges are present even
	// when zero.
	deviceID := dc.DeviceID.String()
	metricDeviceActiveConnections.WithLabelValues(deviceID)
}

func registerDeviceInfoGauge(dc config.DeviceConfiguration) {
	// Create a dynamic "info" gauge to help users
	// map IDs to human-readable strings.
	// It produces a constant `1`
	did := dc.DeviceID.String()
	info_gauge := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "syncthing",
			Name:      "device_info_" + did,
			Help:      "Device metadata info",
			ConstLabels: prometheus.Labels{
				"id":   did,
				"name": dc.Name,
			},
		},
		func() float64 { return 1 },
	)
	prometheus.DefaultRegisterer.Register(info_gauge)
}
