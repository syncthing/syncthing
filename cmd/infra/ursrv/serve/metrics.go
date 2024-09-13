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

var metricReportsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "syncthing",
	Subsystem: "ursrv",
	Name:      "reports_total",
}, []string{"version"})

func init() {
	metricReportsTotal.WithLabelValues("fail")
	metricReportsTotal.WithLabelValues("duplicate")
	metricReportsTotal.WithLabelValues("v1")
	metricReportsTotal.WithLabelValues("v2")
	metricReportsTotal.WithLabelValues("v3")
}
