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
		Help:      "Number of database operations currently ongoing, per folder and operation",
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

func (m metricsDB) AllLocalFilesWithBlocksHash(folder string, h []byte) (iter.Seq[FileMetadata], func() error) {
	defer m.account(folder, "AllLocalFilesWithBlocksHash")()
	return m.DB.AllLocalFilesWithBlocksHash(folder, h)
}

func (m metricsDB) AllGlobalFiles(folder string) (iter.Seq[FileMetadata], func() error) {
	defer m.account(folder, "AllGlobalFiles")()
	return m.DB.AllGlobalFiles(folder)
}

func (m metricsDB) AllGlobalFilesPrefix(folder string, prefix string) (iter.Seq[FileMetadata], func() error) {
	defer m.account(folder, "AllGlobalFilesPrefix")()
	return m.DB.AllGlobalFilesPrefix(folder, prefix)
}

func (m metricsDB) AllLocalFiles(folder string, device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error) {
	defer m.account(folder, "AllLocalFiles")()
	return m.DB.AllLocalFiles(folder, device)
}

func (m metricsDB) AllLocalFilesWithPrefix(folder string, device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error) {
	defer m.account(folder, "AllLocalFilesPrefix")()
	return m.DB.AllLocalFilesWithPrefix(folder, device, prefix)
}

func (m metricsDB) AllLocalFilesBySequence(folder string, device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error) {
	defer m.account(folder, "AllLocalFilesBySequence")()
	return m.DB.AllLocalFilesBySequence(folder, device, startSeq, limit)
}

func (m metricsDB) AllNeededGlobalFiles(folder string, device protocol.DeviceID, order config.PullOrder, limit, offset int) (iter.Seq[protocol.FileInfo], func() error) {
	defer m.account(folder, "AllNeededGlobalFiles")()
	return m.DB.AllNeededGlobalFiles(folder, device, order, limit, offset)
}

func (m metricsDB) GetGlobalAvailability(folder, file string) ([]protocol.DeviceID, error) {
	defer m.account(folder, "GetGlobalAvailability")()
	return m.DB.GetGlobalAvailability(folder, file)
}

func (m metricsDB) AllLocalBlocksWithHash(folder string, hash []byte) (iter.Seq[BlockMapEntry], func() error) {
	defer m.account("-", "AllLocalBlocksWithHash")()
	return m.DB.AllLocalBlocksWithHash(folder, hash)
}

func (m metricsDB) Close() error {
	defer m.account("-", "Close")()
	return m.DB.Close()
}

func (m metricsDB) ListDevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	defer m.account(folder, "ListDevicesForFolder")()
	return m.DB.ListDevicesForFolder(folder)
}

func (m metricsDB) RemoteSequences(folder string) (map[protocol.DeviceID]int64, error) {
	defer m.account(folder, "RemoteSequences")()
	return m.DB.RemoteSequences(folder)
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

func (m metricsDB) DropAllIndexIDs() error {
	defer m.account("-", "IndexIDDropAll")()
	return m.DB.DropAllIndexIDs()
}

func (m metricsDB) ListFolders() ([]string, error) {
	defer m.account("-", "ListFolders")()
	return m.DB.ListFolders()
}

func (m metricsDB) GetGlobalFile(folder string, file string) (protocol.FileInfo, bool, error) {
	defer m.account(folder, "GetGlobalFile")()
	return m.DB.GetGlobalFile(folder, file)
}

func (m metricsDB) CountGlobal(folder string) (Counts, error) {
	defer m.account(folder, "CountGlobal")()
	return m.DB.CountGlobal(folder)
}

func (m metricsDB) GetIndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	defer m.account(folder, "IndexIDGet")()
	return m.DB.GetIndexID(folder, device)
}

func (m metricsDB) GetDeviceFile(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	defer m.account(folder, "GetDeviceFile")()
	return m.DB.GetDeviceFile(folder, device, file)
}

func (m metricsDB) CountLocal(folder string, device protocol.DeviceID) (Counts, error) {
	defer m.account(folder, "CountLocal")()
	return m.DB.CountLocal(folder, device)
}

func (m metricsDB) CountNeed(folder string, device protocol.DeviceID) (Counts, error) {
	defer m.account(folder, "CountNeed")()
	return m.DB.CountNeed(folder, device)
}

func (m metricsDB) CountReceiveOnlyChanged(folder string) (Counts, error) {
	defer m.account(folder, "CountReceiveOnlyChanged")()
	return m.DB.CountReceiveOnlyChanged(folder)
}

func (m metricsDB) GetDeviceSequence(folder string, device protocol.DeviceID) (int64, error) {
	defer m.account(folder, "GetDeviceSequence")()
	return m.DB.GetDeviceSequence(folder, device)
}

func (m metricsDB) SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	defer m.account(folder, "IndexIDSet")()
	return m.DB.SetIndexID(folder, device, id)
}

func (m metricsDB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	defer m.account(folder, "Update")()
	defer metricTotalFilesUpdatedCount.WithLabelValues(folder).Add(float64(len(fs)))
	return m.DB.Update(folder, device, fs)
}

func (m metricsDB) GetKV(key string) ([]byte, error) {
	defer m.account("-", "GetKV")()
	return m.DB.GetKV(key)
}

func (m metricsDB) PutKV(key string, val []byte) error {
	defer m.account("-", "PutKV")()
	return m.DB.PutKV(key, val)
}

func (m metricsDB) DeleteKV(key string) error {
	defer m.account("-", "DeleteKV")()
	return m.DB.DeleteKV(key)
}

func (m metricsDB) PrefixKV(prefix string) (iter.Seq[KeyValue], func() error) {
	defer m.account("-", "PrefixKV")()
	return m.DB.PrefixKV(prefix)
}
