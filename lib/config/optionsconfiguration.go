// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"runtime"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/util"
)

func (opts OptionsConfiguration) Copy() OptionsConfiguration {
	optsCopy := opts
	optsCopy.RawListenAddresses = make([]string, len(opts.RawListenAddresses))
	copy(optsCopy.RawListenAddresses, opts.RawListenAddresses)
	optsCopy.RawGlobalAnnServers = make([]string, len(opts.RawGlobalAnnServers))
	copy(optsCopy.RawGlobalAnnServers, opts.RawGlobalAnnServers)
	optsCopy.AlwaysLocalNets = make([]string, len(opts.AlwaysLocalNets))
	copy(optsCopy.AlwaysLocalNets, opts.AlwaysLocalNets)
	optsCopy.UnackedNotificationIDs = make([]string, len(opts.UnackedNotificationIDs))
	copy(optsCopy.UnackedNotificationIDs, opts.UnackedNotificationIDs)
	return optsCopy
}

func (opts *OptionsConfiguration) prepare(guiPWIsSet bool) {
	util.FillNilSlices(opts)

	opts.RawListenAddresses = util.UniqueTrimmedStrings(opts.RawListenAddresses)
	opts.RawGlobalAnnServers = util.UniqueTrimmedStrings(opts.RawGlobalAnnServers)

	// Very short reconnection intervals are annoying
	if opts.ReconnectIntervalS < 5 {
		opts.ReconnectIntervalS = 5
	}

	if guiPWIsSet && len(opts.UnackedNotificationIDs) > 0 {
		for i, key := range opts.UnackedNotificationIDs {
			if key == "authenticationUserAndPassword" {
				opts.UnackedNotificationIDs = append(opts.UnackedNotificationIDs[:i], opts.UnackedNotificationIDs[i+1:]...)
				break
			}
		}
	}

	// Negative limits are meaningless, zero means unlimited.
	if opts.ConnectionLimitEnough < 0 {
		opts.ConnectionLimitEnough = 0
	}
	if opts.ConnectionLimitMax < 0 {
		opts.ConnectionLimitMax = 0
	}
}

// RequiresRestartOnly returns a copy with only the attributes that require
// restart on change.
func (opts OptionsConfiguration) RequiresRestartOnly() OptionsConfiguration {
	optsCopy := opts
	blank := OptionsConfiguration{}
	util.CopyMatchingTag(&blank, &optsCopy, "restart", func(v string) bool {
		if len(v) > 0 && v != "true" {
			panic(fmt.Sprintf(`unexpected tag value: %s. Expected untagged or "true"`, v))
		}
		return v != "true"
	})
	return optsCopy
}

func (opts OptionsConfiguration) IsStunDisabled() bool {
	return opts.StunKeepaliveMinS < 1 || opts.StunKeepaliveStartS < 1 || !opts.NATEnabled
}

func (opts OptionsConfiguration) ListenAddresses() []string {
	var addresses []string
	for _, addr := range opts.RawListenAddresses {
		switch addr {
		case "default":
			addresses = append(addresses, DefaultListenAddresses...)
		default:
			addresses = append(addresses, addr)
		}
	}
	return util.UniqueTrimmedStrings(addresses)
}

func (opts OptionsConfiguration) StunServers() []string {
	var addresses []string
	for _, addr := range opts.RawStunServers {
		switch addr {
		case "default":
			defaultPrimaryAddresses := make([]string, len(DefaultPrimaryStunServers))
			copy(defaultPrimaryAddresses, DefaultPrimaryStunServers)
			rand.Shuffle(defaultPrimaryAddresses)
			addresses = append(addresses, defaultPrimaryAddresses...)

			defaultSecondaryAddresses := make([]string, len(DefaultSecondaryStunServers))
			copy(defaultSecondaryAddresses, DefaultSecondaryStunServers)
			rand.Shuffle(defaultSecondaryAddresses)
			addresses = append(addresses, defaultSecondaryAddresses...)
		default:
			addresses = append(addresses, addr)
		}
	}

	addresses = util.UniqueTrimmedStrings(addresses)

	return addresses
}

func (opts OptionsConfiguration) GlobalDiscoveryServers() []string {
	var servers []string
	for _, srv := range opts.RawGlobalAnnServers {
		switch srv {
		case "default":
			servers = append(servers, DefaultDiscoveryServers...)
		case "default-v4":
			servers = append(servers, DefaultDiscoveryServersV4...)
		case "default-v6":
			servers = append(servers, DefaultDiscoveryServersV6...)
		default:
			servers = append(servers, srv)
		}
	}
	return util.UniqueTrimmedStrings(servers)
}

func (opts OptionsConfiguration) MaxFolderConcurrency() int {
	// If a value is set, trust that.
	if opts.RawMaxFolderConcurrency > 0 {
		return opts.RawMaxFolderConcurrency
	}
	if opts.RawMaxFolderConcurrency < 0 {
		// -1 etc means unlimited, which in the implementation means zero
		return 0
	}
	// Otherwise default to the number of CPU cores in the system as a rough
	// approximation of system powerfullness.
	if n := runtime.GOMAXPROCS(-1); n > 0 {
		return n
	}
	// We should never get here to begin with, but since we're here let's
	// use some sort of reasonable compromise between the old "no limit" and
	// getting nothing done... (Median number of folders out there at time
	// of writing is two, 95-percentile at 12 folders.)
	return 4 // https://xkcd.com/221/
}

func (opts OptionsConfiguration) MaxConcurrentIncomingRequestKiB() int {
	// Negative is disabled, which in limiter land is spelled zero
	if opts.RawMaxCIRequestKiB < 0 {
		return 0
	}

	if opts.RawMaxCIRequestKiB == 0 {
		// The default is 256 MiB
		return 256 * 1024 // KiB
	}

	// We can't really do less than a couple of concurrent blocks or we'll
	// pretty much stall completely. Check that an explicit value is large
	// enough.
	const minAllowed = 2 * protocol.MaxBlockSize / 1024
	if opts.RawMaxCIRequestKiB < minAllowed {
		return minAllowed
	}

	// Roll with it.
	return opts.RawMaxCIRequestKiB
}

func (opts OptionsConfiguration) AutoUpgradeEnabled() bool {
	return opts.AutoUpgradeIntervalH > 0
}

func (opts OptionsConfiguration) FeatureFlag(name string) bool {
	for _, flag := range opts.FeatureFlags {
		if flag == name {
			return true
		}
	}

	return false
}

// LowestConnectionLimit is the lower of ConnectionLimitEnough or
// ConnectionLimitMax, or whichever of them is actually set if only one of
// them is set. It's the point where we should stop dialling.
func (opts OptionsConfiguration) LowestConnectionLimit() int {
	limit := opts.ConnectionLimitEnough
	if limit == 0 || (opts.ConnectionLimitMax != 0 && opts.ConnectionLimitMax < limit) {
		// It doesn't really make sense to set Max lower than Enough but
		// someone might do it while experimenting and it's easy for us to
		// do the right thing.
		limit = opts.ConnectionLimitMax
	}
	return limit
}
