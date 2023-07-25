// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package events

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var metricEvents = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "syncthing",
	Subsystem: "events",
	Name:      "total",
	Help:      "Total number of created/forwarded/dropped events",
}, []string{"event", "state"})

const (
	metricEventStateCreated   = "created"
	metricEventStateDelivered = "delivered"
	metricEventStateDropped   = "dropped"
)
