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
	metricDHTRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "dht",
		Name:      "requests_total",
		Help:      "Total number of incoming DHT requests",
	}, []string{"type"})

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
	t := req.GetType()
	metricDHTRequestsTotal.WithLabelValues(t.String()).Inc()
	switch t {
	case pb.Message_PUT_VALUE, pb.Message_GET_VALUE:
		slog.Debug("Incoming DHT request", "type", t, "key", string(req.GetKey()), "peer", s.Conn().RemotePeer())
	}
}
