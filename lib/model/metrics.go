// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricFolderState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_state",
		Help:      "Current folder state",
	}, []string{"folder"})
	metricFolderSummary = promauto.NewGaugeVec(prometheus.GaugeOpts{ //nolint:promlinter
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_summary",
		Help:      "Current folder summary data (counts for global/local/need files/directories/symlinks/deleted/bytes)",
	}, []string{"folder", "scope", "type"})

	metricFolderPulls = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_pulls_total",
		Help:      "Total number of folder pull iterations, per folder ID",
	}, []string{"folder"})
	metricFolderPullSeconds = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_pull_seconds_total",
		Help:      "Total time spent in folder pull iterations, per folder ID",
	}, []string{"folder"})

	metricFolderScans = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_scans_total",
		Help:      "Total number of folder scan iterations, per folder ID",
	}, []string{"folder"})
	metricFolderScanSeconds = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_scan_seconds_total",
		Help:      "Total time spent in folder scan iterations, per folder ID",
	}, []string{"folder"})

	metricFolderProcessedBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_processed_bytes_total",
		Help:      "Total amount of data processed during folder syncing, per folder ID and data source (network/local_origin/local_other/skipped)",
	}, []string{"folder", "source"})

	metricFolderConflictsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_conflicts_total",
		Help:      "Total number of conflicts",
	}, []string{"folder"})
)

const (
	metricSourceNetwork     = "network"      // from the network
	metricSourceLocalOrigin = "local_origin" // from the existing version of the local file
	metricSourceLocalOther  = "local_other"  // from a different local file
	metricSourceSkipped     = "skipped"      // block of all zeroes, invented out of thin air

	metricScopeGlobal = "global"
	metricScopeLocal  = "local"
	metricScopeNeed   = "need"

	metricTypeFiles       = "files"
	metricTypeDirectories = "directories"
	metricTypeSymlinks    = "symlinks"
	metricTypeDeleted     = "deleted"
	metricTypeBytes       = "bytes"
)

func registerFolderMetrics(folderID string) {
	// Register metrics for this folder, so that counters are present even
	// when zero.
	metricFolderState.WithLabelValues(folderID)
	metricFolderPulls.WithLabelValues(folderID)
	metricFolderPullSeconds.WithLabelValues(folderID)
	metricFolderScans.WithLabelValues(folderID)
	metricFolderScanSeconds.WithLabelValues(folderID)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceNetwork)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceLocalOrigin)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceLocalOther)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceSkipped)
	metricFolderConflictsTotal.WithLabelValues(folderID)
}
