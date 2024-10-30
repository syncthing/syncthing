// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricCrashReportsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "crashreceiver",
		Name:      "crash_reports_total",
	}, []string{"result"})
	metricFailureReportsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "crashreceiver",
		Name:      "failure_reports_total",
	}, []string{"result"})
	metricDiskstoreFilesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "crashreceiver",
		Name:      "diskstore_files_total",
	})
	metricDiskstoreBytesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "crashreceiver",
		Name:      "diskstore_bytes_total",
	})
	metricDiskstoreOldestAgeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "crashreceiver",
		Name:      "diskstore_oldest_age_seconds",
	})
)
