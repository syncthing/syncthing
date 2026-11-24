// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	"log/slog"

	pb "github.com/libp2p/go-libp2p-kad-dht/pb"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricAnnouncementUpdatesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "dht",
		Name:      "announcement_updates_total",
		Help:      "Total number of incoming DHT PUT_VALUE requests",
	})

	metricAnnouncementQueriesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "dht",
		Name:      "announcement_queries_total",
		Help:      "Total number of incoming DHT GET_VALUE requests",
	})

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

func countIncomingRecordRequest(_ context.Context, s network.Stream, req *pb.Message) {
	switch req.GetType() {
	case pb.Message_PUT_VALUE:
		metricAnnouncementUpdatesTotal.Inc()
		slog.Debug("Incoming PUT_VALUE request", "key", string(req.Key), "peer", s.Conn().RemotePeer())
	case pb.Message_GET_VALUE:
		metricAnnouncementQueriesTotal.Inc()
		slog.Debug("Incoming GET_VALUE request", "key", string(req.Key), "peer", s.Conn().RemotePeer())
	}
}
