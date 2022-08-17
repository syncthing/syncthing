// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"strconv"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// unixPlatformData is used on all platforms, because apart from being the
// implementation for BasicFilesystem on Unixes it's also the implementation
// in fakeFS.
func unixPlatformData(fs Filesystem, name string, userCache *userCache, groupCache *groupCache, xattrFilter XattrFilter) (protocol.PlatformData, error) {
	stat, err := fs.Lstat(name)
	if err != nil {
		return protocol.PlatformData{}, err
	}

	ownerUID := stat.Owner()
	ownerName := ""
	if user := userCache.lookup(strconv.Itoa(ownerUID)); user != nil {
		ownerName = user.Username
	} else if ownerUID == 0 {
		// We couldn't look up a name, but UID zero should be "root". This
		// fixup works around the (unlikely) situation where the ownership
		// is 0:0 but we can't look up a name for either uid zero or gid
		// zero. If that were the case we'd return a zero PlatformData which
		// wouldn't get serialized over the wire and the other side would
		// assume a lack of ownership info...
		ownerName = "root"
	}

	groupID := stat.Group()
	groupName := ""
	if group := groupCache.lookup(strconv.Itoa(ownerUID)); group != nil {
		groupName = group.Name
	} else if groupID == 0 {
		groupName = "root"
	}

	pd := protocol.PlatformData{
		Unix: &protocol.UnixData{
			OwnerName: ownerName,
			GroupName: groupName,
			UID:       ownerUID,
			GID:       groupID,
		},
	}

	xattrs, err := fs.GetXattr(name, xattrFilter)
	if err != nil {
		return protocol.PlatformData{}, err
	}
	pd.SetXattrs(xattrs)

	return pd, nil
}

type valueCache[K comparable, V any] struct {
	validity time.Duration
	fill     func(K) (V, error)

	mut   sync.Mutex
	cache map[K]cacheEntry[V]
}

type cacheEntry[V any] struct {
	value V
	when  time.Time
}

func newValueCache[K comparable, V any](validity time.Duration, fill func(K) (V, error)) *valueCache[K, V] {
	return &valueCache[K, V]{
		validity: validity,
		fill:     fill,
		cache:    make(map[K]cacheEntry[V]),
	}
}

func (c *valueCache[K, V]) lookup(key K) V {
	c.mut.Lock()
	defer c.mut.Unlock()
	if e, ok := c.cache[key]; ok && time.Since(e.when) < c.validity {
		return e.value
	}
	var e cacheEntry[V]
	if val, err := c.fill(key); err == nil {
		e.value = val
	}
	e.when = time.Now()
	c.cache[key] = e
	return e.value
}
