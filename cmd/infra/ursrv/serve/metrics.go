// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricReportsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "ursrv_v2",
		Name:      "incoming_reports_total",
	}, []string{"result"})
	metricsCollectsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "ursrv_v2",
		Name:      "collects_total",
	})
	metricsCollectSecondsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "ursrv_v2",
		Name:      "collect_seconds_total",
	})
	metricsCollectSecondsLast = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "ursrv_v2",
		Name:      "collect_seconds_last",
	})
	metricsWriteSecondsLast = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "ursrv_v2",
		Name:      "write_seconds_last",
	})
)

func init() {
	metricReportsTotal.WithLabelValues("fail")
	metricReportsTotal.WithLabelValues("replace")
	metricReportsTotal.WithLabelValues("accept")
}
