// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricDeviceSentBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "sent_bytes_total",
		Help:      "Total amount of data sent",
	}, []string{"device"})
	metricDeviceSentUncompressedBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "sent_uncompressed_bytes_total",
		Help:      "Total amount of data sent, before compression",
	}, []string{"device"})
	metricDeviceSentMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "sent_messages_total",
		Help:      "Total number of messages sent",
	}, []string{"device"})

	metricDeviceRecvBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "recv_bytes_total",
		Help:      "Total amount of data received",
	}, []string{"device"})
	metricDeviceRecvMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "recv_messages_total",
		Help:      "Total number of messages received",
	}, []string{"device"})
)

func registerDeviceMetrics(deviceID string) {
	// Register metrics for this device, so that counters are present even
	// when zero.
	metricDeviceSentBytes.WithLabelValues(deviceID)
	metricDeviceSentUncompressedBytes.WithLabelValues(deviceID)
	metricDeviceSentMessages.WithLabelValues(deviceID)
	metricDeviceRecvBytes.WithLabelValues(deviceID)
	metricDeviceRecvMessages.WithLabelValues(deviceID)
}
