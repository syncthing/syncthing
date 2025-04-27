// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/config"

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
	metricFolderSummary = promauto.NewGaugeVec(prometheus.GaugeOpts{
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
		Help:      "Total amount of data processed during folder syncing, per folder ID and data source (network/local_origin/local_other/local_shifted/skipped)",
	}, []string{"folder", "source"})

	metricFolderConflictsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "model",
		Name:      "folder_conflicts_total",
		Help:      "Total number of conflicts",
	}, []string{"folder"})
)

const (
	metricSourceNetwork      = "network"       // from the network
	metricSourceLocalOrigin  = "local_origin"  // from the existing version of the local file
	metricSourceLocalOther   = "local_other"   // from a different local file
	metricSourceLocalShifted = "local_shifted" // from the existing version of the local file, rolling hash shifted
	metricSourceSkipped      = "skipped"       // block of all zeroes, invented out of thin air

	metricScopeGlobal = "global"
	metricScopeLocal  = "local"
	metricScopeNeed   = "need"

	metricTypeFiles       = "files"
	metricTypeDirectories = "directories"
	metricTypeSymlinks    = "symlinks"
	metricTypeDeleted     = "deleted"
	metricTypeBytes       = "bytes"
)

func registerFolderMetrics(fc config.FolderConfiguration) {
	registerInfoGauge(fc)
	// Register metrics for this folder, so that
	// counters are present even when zero.
	folderID := fc.ID
	metricFolderState.WithLabelValues(folderID)
	metricFolderPulls.WithLabelValues(folderID)
	metricFolderPullSeconds.WithLabelValues(folderID)
	metricFolderScans.WithLabelValues(folderID)
	metricFolderScanSeconds.WithLabelValues(folderID)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceNetwork)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceLocalOrigin)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceLocalOther)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceLocalShifted)
	metricFolderProcessedBytesTotal.WithLabelValues(folderID, metricSourceSkipped)
	metricFolderConflictsTotal.WithLabelValues(folderID)
}

func registerInfoGauge(fc config.FolderConfiguration) {
	// Create a dynamic "info" gauge to help users
	// map IDs to humane strings.
	// It produces a constant `1`
	info_gauge := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "syncthing",
			Name:      "folder_info_" + fc.ID,
			Help:      "Folder metadata info",
			ConstLabels: prometheus.Labels{
				"id":    fc.ID,
				"label": fc.Label,
				"path":  fc.Path,
			},
		},
		func() float64 { return 1 },
	)
	// This can error, but we have no way to recover or fallback.
	// The tests, for example, register ID `ro` many times.
	prometheus.DefaultRegisterer.Register(info_gauge)
}
