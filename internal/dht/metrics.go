// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricDatastoreItems = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "dht",
		Name:      "datastore_items",
		Help:      "Number of items in the datastore",
	}, []string{"datastore"})

	metricDatastoreOperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "dht",
		Name:      "datastore_operations_total",
		Help:      "Total number of datastore operations",
	}, []string{"datastore", "operation", "result"})
)
