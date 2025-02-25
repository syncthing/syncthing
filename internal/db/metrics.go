// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"iter"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

var (
	metricCurrentOperations = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "syncthing",
		Subsystem: "db",
		Name:      "operations_current",
	}, []string{"folder", "operation"})
	metricTotalOperationSeconds = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "db",
		Name:      "operation_seconds_total",
		Help:      "Total time spent in database operations, per folder and operation",
	}, []string{"folder", "operation"})
	metricTotalOperationsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "db",
		Name:      "operations_total",
		Help:      "Total number of database operations, per folder and operation",
	}, []string{"folder", "operation"})
	metricTotalFilesUpdatedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "db",
		Name:      "files_updated_total",
		Help:      "Total number of files updated",
	}, []string{"folder"})
)

func MetricsWrap(db DB) DB {
	return metricsDB{db}
}

type metricsDB struct {
	DB
}

func (m metricsDB) account(folder, op string) func() {
	t0 := time.Now()
	metricCurrentOperations.WithLabelValues(folder, op).Inc()
	return func() {
		if dur := time.Since(t0).Seconds(); dur > 0 {
			metricTotalOperationSeconds.WithLabelValues(folder, op).Add(dur)
		}
		metricTotalOperationsCount.WithLabelValues(folder, op).Inc()
		metricCurrentOperations.WithLabelValues(folder, op).Dec()
	}
}

func (m metricsDB) AllForBlocksHash(folder string, h []byte) iter.Seq2[protocol.FileInfo, error] {
	defer m.account(folder, "AllForBlocksHash")()
	return m.DB.AllForBlocksHash(folder, h)
}

func (m metricsDB) AllForBlocksHashAnyFolder(errptr *error, h []byte) iter.Seq2[string, protocol.FileInfo] {
	defer m.account("-", "AllForBlocksHashAnyFolder")()
	return m.DB.AllForBlocksHashAnyFolder(errptr, h)
}

func (m metricsDB) AllGlobal(folder string) iter.Seq2[protocol.FileInfo, error] {
	defer m.account(folder, "AllGlobal")()
	return m.DB.AllGlobal(folder)
}

func (m metricsDB) AllGlobalPrefix(folder string, prefix string) iter.Seq2[protocol.FileInfo, error] {
	defer m.account(folder, "AllGlobalPrefix")()
	return m.DB.AllGlobalPrefix(folder, prefix)
}

func (m metricsDB) AllLocal(folder string, device protocol.DeviceID) iter.Seq2[protocol.FileInfo, error] {
	defer m.account(folder, "AllLocal")()
	return m.DB.AllLocal(folder, device)
}

func (m metricsDB) AllLocalPrefixed(folder string, device protocol.DeviceID, prefix string) iter.Seq2[protocol.FileInfo, error] {
	defer m.account(folder, "AllLocalPrefixed")()
	return m.DB.AllLocalPrefixed(folder, device, prefix)
}

func (m metricsDB) AllLocalSequenced(folder string, device protocol.DeviceID, startSeq int64) iter.Seq2[protocol.FileInfo, error] {
	defer m.account(folder, "AllLocalSequenced")()
	return m.DB.AllLocalSequenced(folder, device, startSeq)
}

func (m metricsDB) AllNeededNames(folder string, device protocol.DeviceID, order config.PullOrder, limit int) iter.Seq2[string, error] {
	defer m.account(folder, "AllNeededNames")()
	return m.DB.AllNeededNames(folder, device, order, limit)
}

func (m metricsDB) Availability(folder, file string) ([]protocol.DeviceID, error) {
	defer m.account(folder, "Availability")()
	return m.DB.Availability(folder, file)
}

func (m metricsDB) Blocks(hash []byte) iter.Seq2[BlockMapEntry, error] {
	defer m.account("-", "Blocks")()
	return m.DB.Blocks(hash)
}

func (m metricsDB) Close() error {
	defer m.account("-", "Close")()
	return m.DB.Close()
}

func (m metricsDB) DevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	defer m.account(folder, "DevicesForFolder")()
	return m.DB.DevicesForFolder(folder)
}

func (m metricsDB) DropAllFiles(folder string, device protocol.DeviceID) error {
	defer m.account(folder, "DropAllFiles")()
	return m.DB.DropAllFiles(folder, device)
}

func (m metricsDB) DropDevice(device protocol.DeviceID) error {
	defer m.account("-", "DropDevice")()
	return m.DB.DropDevice(device)
}

func (m metricsDB) DropFilesNamed(folder string, device protocol.DeviceID, names []string) error {
	defer m.account(folder, "DropFilesNamed")()
	return m.DB.DropFilesNamed(folder, device, names)
}

func (m metricsDB) DropFolder(folder string) error {
	defer m.account(folder, "DropFolder")()
	return m.DB.DropFolder(folder)
}

func (m metricsDB) DropIndexIDs() error {
	defer m.account("-", "DropIndexIDs")()
	return m.DB.DropIndexIDs()
}

func (m metricsDB) Folders() ([]string, error) {
	defer m.account("-", "Folders")()
	return m.DB.Folders()
}

func (m metricsDB) Global(folder string, file string) (protocol.FileInfo, bool, error) {
	defer m.account(folder, "Global")()
	return m.DB.Global(folder, file)
}

func (m metricsDB) GlobalSize(folder string) Counts {
	defer m.account(folder, "GlobalSize")()
	return m.DB.GlobalSize(folder)
}

func (m metricsDB) IndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	defer m.account(folder, "IndexID")()
	return m.DB.IndexID(folder, device)
}

func (m metricsDB) KV() KV {
	defer m.account("-", "KV")()
	return m.DB.KV()
}

func (m metricsDB) Local(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	defer m.account(folder, "Local")()
	return m.DB.Local(folder, device, file)
}

func (m metricsDB) LocalSize(folder string, device protocol.DeviceID) Counts {
	defer m.account(folder, "LocalSize")()
	return m.DB.LocalSize(folder, device)
}

func (m metricsDB) NeedSize(folder string, device protocol.DeviceID) Counts {
	defer m.account(folder, "NeedSize")()
	return m.DB.NeedSize(folder, device)
}

func (m metricsDB) ReceiveOnlySize(folder string) Counts {
	defer m.account(folder, "ReceiveOnlySize")()
	return m.DB.ReceiveOnlySize(folder)
}

func (m metricsDB) Sequence(folder string, device protocol.DeviceID) (int64, error) {
	defer m.account(folder, "Sequence")()
	return m.DB.Sequence(folder, device)
}

func (m metricsDB) SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	defer m.account(folder, "SetIndexID")()
	return m.DB.SetIndexID(folder, device, id)
}

func (m metricsDB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	defer m.account(folder, "Update")()
	defer metricTotalFilesUpdatedCount.WithLabelValues(folder).Add(float64(len(fs)))
	return m.DB.Update(folder, device, fs)
}
