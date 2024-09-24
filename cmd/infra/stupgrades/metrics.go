// Copyright (C) 2024 The Syncthing Authors.
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
	metricUpgradeChecks = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "upgrade",
		Name:      "metadata_requests",
	})
	metricFilterCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "upgrade",
		Name:      "filter_calls",
	}, []string{"result"})
	metricHTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "upgrade",
		Name:      "http_requests",
	}, []string{"target", "result"})
)
