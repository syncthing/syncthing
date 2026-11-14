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

var (
	metricPeers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "peers", Help: "DHT peers by population (sampled every 30 seconds)",
	}, []string{"population"})
	metricEmptyDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "routing_table_empty_seconds", Help: "Elapsed time since the routing table was first observed empty",
	})
	metricLastRoutingSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "routing_table_last_nonempty_timestamp_seconds", Help: "Last time the routing table was observed nonempty",
	})
	metricRoutingTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "routing_table_transitions_total", Help: "Observed routing table empty and recovery transitions",
	}, []string{"state"})
	metricOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "operations_total", Help: "DHT discovery operations by result",
	}, []string{"operation", "result"})
	metricOperationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "operation_duration_seconds", Help: "DHT discovery operation duration", Buckets: prometheus.ExponentialBuckets(0.01, 4, 8),
	}, []string{"operation"})
	metricLastSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "syncthing", Subsystem: "dht", Name: "operation_last_success_timestamp_seconds", Help: "Last successful DHT discovery operation",
	}, []string{"operation"})
)
