// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

type dialQueueEntry struct {
	id         protocol.DeviceID
	lastSeen   time.Time
	shortLived bool
	targets    []dialTarget
}

type dialQueue []dialQueueEntry

func (queue dialQueue) Sort() {
	// Sort the queue with the most recently seen device at the head,
	// increasing the likelihood of connecting to a device that we're
	// already almost up to date with, index wise.
	sort.Slice(queue, func(a, b int) bool {
		qa, qb := queue[a], queue[b]
		if qa.shortLived != qb.shortLived {
			return qb.shortLived
		}
		return qa.lastSeen.After(qb.lastSeen)
	})

	// Shuffle the part of the connection queue that are devices we haven't
	// connected to recently, so that if we only try a limited set of
	// devices (or they in turn have limits and we're trying to load balance
	// over several) and the usual ones are down it won't be the same ones
	// in the same order every time.
	idx := 0
	cutoff := time.Now().Add(-recentlySeenCutoff)
	for idx < len(queue) {
		if queue[idx].lastSeen.Before(cutoff) {
			break
		}
		idx++
	}
	if idx < len(queue)-1 {
		rand.Shuffle(queue[idx:])
	}
}
