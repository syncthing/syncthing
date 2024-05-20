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
		Help:      "Total amount of data sent, per device",
	}, []string{"device"})
	metricDeviceSentUncompressedBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "sent_uncompressed_bytes_total",
		Help:      "Total amount of data sent, before compression, per device",
	}, []string{"device"})
	metricDeviceSentMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "sent_messages_total",
		Help:      "Total number of messages sent, per device",
	}, []string{"device"})

	metricDeviceRecvBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "recv_bytes_total",
		Help:      "Total amount of data received, per device",
	}, []string{"device"})
	metricDeviceRecvDecompressedBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "recv_decompressed_bytes_total",
		Help:      "Total amount of data received, after decompression, per device",
	}, []string{"device"})
	metricDeviceRecvMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "protocol",
		Name:      "recv_messages_total",
		Help:      "Total number of messages received, per device",
	}, []string{"device"})
)

func registerDeviceMetrics(deviceID string) {
	// Register metrics for this device, so that counters are present even
	// when zero.
	metricDeviceSentBytes.WithLabelValues(deviceID)
	metricDeviceSentUncompressedBytes.WithLabelValues(deviceID)
	metricDeviceSentMessages.WithLabelValues(deviceID)
	metricDeviceRecvBytes.WithLabelValues(deviceID)
	metricDeviceRecvDecompressedBytes.WithLabelValues(deviceID)
	metricDeviceRecvMessages.WithLabelValues(deviceID)
}
