// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Registry tracks connections/addresses on which we are listening on, to allow us to pick a connection/address that
// has a NAT port mapping. This also makes our outgoing port stable and same as incoming port which should allow
// better probability of punching through.
package registry

import (
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/sync"
)

var (
	Default = New()
)

type Registry struct {
	mut       sync.Mutex
	available map[string][]interface{}
}

func New() *Registry {
	return &Registry{
		mut:       sync.NewMutex(),
		available: make(map[string][]interface{}),
	}
}

func (r *Registry) Register(scheme string, item interface{}) {
	r.mut.Lock()
	defer r.mut.Unlock()

	r.available[scheme] = append(r.available[scheme], item)
}

func (r *Registry) Unregister(scheme string, item interface{}) {
	r.mut.Lock()
	defer r.mut.Unlock()

	candidates := r.available[scheme]
	for i, existingItem := range candidates {
		if existingItem == item {
			copy(candidates[i:], candidates[i+1:])
			candidates[len(candidates)-1] = nil
			r.available[scheme] = candidates[:len(candidates)-1]
			break
		}
	}
}

func (r *Registry) Get(scheme string, less func(i, j interface{}) bool) interface{} {
	r.mut.Lock()
	defer r.mut.Unlock()

	candidates := make([]interface{}, 0)
	for availableScheme, items := range r.available {
		// quic:// should be considered ok for both quic4:// and quic6://
		if strings.HasPrefix(scheme, availableScheme) {
			candidates = append(candidates, items...)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return less(candidates[i], candidates[j])
	})
	return candidates[0]
}

func Register(scheme string, item interface{}) {
	Default.Register(scheme, item)
}

func Unregister(scheme string, item interface{}) {
	Default.Unregister(scheme, item)
}

func Get(scheme string, less func(i, j interface{}) bool) interface{} {
	return Default.Get(scheme, less)
}
