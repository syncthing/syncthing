// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricHashedBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "scanner",
		Name:      "hashed_bytes_total",
		Help:      "Total amount of data hashed, per folder",
	}, []string{"folder"})

	metricScannedItems = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "scanner",
		Name:      "scanned_items_total",
		Help:      "Total number of items (files/directories) inspected, per folder",
	}, []string{"folder"})
)

func registerFolderMetrics(folderID string) {
	// Register metrics for this folder, so that counters are present even
	// when zero.
	metricHashedBytes.WithLabelValues(folderID)
	metricScannedItems.WithLabelValues(folderID)
}
