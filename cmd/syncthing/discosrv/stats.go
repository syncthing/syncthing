// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discosrv

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	apiRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "api_requests_total",
			Help:      "Number of API requests.",
		}, []string{"type", "result"})
	apiRequestsSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  "syncthing",
			Subsystem:  "discovery",
			Name:       "api_requests_seconds",
			Help:       "Latency of API requests.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, []string{"type"})

	lookupRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "lookup_requests_total",
			Help:      "Number of lookup requests.",
		}, []string{"result"})
	announceRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "announcement_requests_total",
			Help:      "Number of announcement requests.",
		}, []string{"result"})

	replicationSendsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "replication_sends_total",
			Help:      "Number of replication sends.",
		}, []string{"result"})
	replicationRecvsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "replication_recvs_total",
			Help:      "Number of replication receives.",
		}, []string{"result"})

	databaseKeys = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "database_keys",
			Help:      "Number of database keys at last count.",
		}, []string{"category"})
	databaseStatisticsSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "database_statistics_seconds",
			Help:      "Time spent running the statistics routine.",
		})

	databaseOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "discovery",
			Name:      "database_operations_total",
			Help:      "Number of database operations.",
		}, []string{"operation", "result"})
	databaseOperationSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  "syncthing",
			Subsystem:  "discovery",
			Name:       "database_operation_seconds",
			Help:       "Latency of database operations.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, []string{"operation"})
)

const (
	dbOpGet             = "get"
	dbOpPut             = "put"
	dbOpMerge           = "merge"
	dbOpDelete          = "delete"
	dbResSuccess        = "success"
	dbResNotFound       = "not_found"
	dbResError          = "error"
	dbResUnmarshalError = "unmarsh_err"
)

func init() {
	prometheus.MustRegister(apiRequestsTotal, apiRequestsSeconds,
		lookupRequestsTotal, announceRequestsTotal,
		replicationSendsTotal, replicationRecvsTotal,
		databaseKeys, databaseStatisticsSeconds,
		databaseOperations, databaseOperationSeconds)

	processCollectorOpts := collectors.ProcessCollectorOpts{
		Namespace: "syncthing_discovery",
		PidFn: func() (int, error) {
			return os.Getpid(), nil
		},
	}

	prometheus.MustRegister(
		collectors.NewProcessCollector(processCollectorOpts),
	)
}
