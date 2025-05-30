// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

// RegisterInfoMetrics registers Prometheus metrics for the given config
// wrapper.
func RegisterInfoMetrics(cfg Wrapper) {
	prometheus.DefaultRegisterer.MustRegister(prometheus.CollectorFunc((&folderInfoMetric{cfg}).Collect))
	prometheus.DefaultRegisterer.MustRegister(prometheus.CollectorFunc((&folderDeviceMetric{cfg}).Collect))
}

type folderInfoMetric struct {
	cfg Wrapper
}

var folderInfoMetricDesc = prometheus.NewDesc(
	"syncthing_config_folder_info",
	"Provides additional information labels on folders",
	[]string{"folder", "label", "type", "path", "paused"},
	nil,
)

func (m *folderInfoMetric) Collect(ch chan<- prometheus.Metric) {
	for _, folder := range m.cfg.FolderList() {
		ch <- prometheus.MustNewConstMetric(
			folderInfoMetricDesc,
			prometheus.GaugeValue, 1,
			folder.ID, folder.Label, folder.Type.String(), folder.Path, strconv.FormatBool(folder.Paused),
		)
	}
}

type folderDeviceMetric struct {
	cfg Wrapper
}

var folderDeviceMetricDesc = prometheus.NewDesc(
	"syncthing_config_device_info",
	"Provides additional information labels on devices",
	[]string{"device", "name", "introducer", "paused", "untrusted"},
	nil,
)

func (m *folderDeviceMetric) Collect(ch chan<- prometheus.Metric) {
	for _, device := range m.cfg.DeviceList() {
		ch <- prometheus.MustNewConstMetric(
			folderDeviceMetricDesc,
			prometheus.GaugeValue, 1,
			device.DeviceID.String(), device.Name, strconv.FormatBool(device.Introducer), strconv.FormatBool(device.Paused), strconv.FormatBool(device.Untrusted),
		)
	}
}
