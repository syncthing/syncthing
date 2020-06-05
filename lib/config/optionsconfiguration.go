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

type OptionsConfiguration struct {
	RawListenAddresses      []string `xml:"listenAddress" json:"listenAddresses" default:"default"`
	RawGlobalAnnServers     []string `xml:"globalAnnounceServer" json:"globalAnnounceServers" default:"default" restart:"true"`
	GlobalAnnEnabled        bool     `xml:"globalAnnounceEnabled" json:"globalAnnounceEnabled" default:"true" restart:"true"`
	LocalAnnEnabled         bool     `xml:"localAnnounceEnabled" json:"localAnnounceEnabled" default:"true" restart:"true"`
	LocalAnnPort            int      `xml:"localAnnouncePort" json:"localAnnouncePort" default:"21027" restart:"true"`
	LocalAnnMCAddr          string   `xml:"localAnnounceMCAddr" json:"localAnnounceMCAddr" default:"[ff12::8384]:21027" restart:"true"`
	MaxSendKbps             int      `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps             int      `xml:"maxRecvKbps" json:"maxRecvKbps"`
	ReconnectIntervalS      int      `xml:"reconnectionIntervalS" json:"reconnectionIntervalS" default:"60"`
	RelaysEnabled           bool     `xml:"relaysEnabled" json:"relaysEnabled" default:"true"`
	RelayReconnectIntervalM int      `xml:"relayReconnectIntervalM" json:"relayReconnectIntervalM" default:"10"`
	StartBrowser            bool     `xml:"startBrowser" json:"startBrowser" default:"true"`
	NATEnabled              bool     `xml:"natEnabled" json:"natEnabled" default:"true"`
	NATLeaseM               int      `xml:"natLeaseMinutes" json:"natLeaseMinutes" default:"60"`
	NATRenewalM             int      `xml:"natRenewalMinutes" json:"natRenewalMinutes" default:"30"`
	NATTimeoutS             int      `xml:"natTimeoutSeconds" json:"natTimeoutSeconds" default:"10"`
	URAccepted              int      `xml:"urAccepted" json:"urAccepted"`                                    // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)
	URSeen                  int      `xml:"urSeen" json:"urSeen"`                                            // Report which the user has been prompted for.
	URUniqueID              string   `xml:"urUniqueID" json:"urUniqueId"`                                    // Unique ID for reporting purposes, regenerated when UR is turned on.
	URURL                   string   `xml:"urURL" json:"urURL" default:"https://data.syncthing.net/newdata"` // usage reporting URL
	URPostInsecurely        bool     `xml:"urPostInsecurely" json:"urPostInsecurely" default:"false"`        // For testing
	URInitialDelayS         int      `xml:"urInitialDelayS" json:"urInitialDelayS" default:"1800"`
	RestartOnWakeup         bool     `xml:"restartOnWakeup" json:"restartOnWakeup" default:"true" restart:"true"`
	AutoUpgradeIntervalH    int      `xml:"autoUpgradeIntervalH" json:"autoUpgradeIntervalH" default:"12" restart:"true"` // 0 for off
	UpgradeToPreReleases    bool     `xml:"upgradeToPreReleases" json:"upgradeToPreReleases" restart:"true"`              // when auto upgrades are enabled
	KeepTemporariesH        int      `xml:"keepTemporariesH" json:"keepTemporariesH" default:"24"`                        // 0 for off
	CacheIgnoredFiles       bool     `xml:"cacheIgnoredFiles" json:"cacheIgnoredFiles" default:"false" restart:"true"`
	ProgressUpdateIntervalS int      `xml:"progressUpdateIntervalS" json:"progressUpdateIntervalS" default:"5"`
	LimitBandwidthInLan     bool     `xml:"limitBandwidthInLan" json:"limitBandwidthInLan" default:"false"`
	MinHomeDiskFree         Size     `xml:"minHomeDiskFree" json:"minHomeDiskFree" default:"1 %"`
	ReleasesURL             string   `xml:"releasesURL" json:"releasesURL" default:"https://upgrades.syncthing.net/meta.json" restart:"true"`
	AlwaysLocalNets         []string `xml:"alwaysLocalNet" json:"alwaysLocalNets"`
	OverwriteRemoteDevNames bool     `xml:"overwriteRemoteDeviceNamesOnConnect" json:"overwriteRemoteDeviceNamesOnConnect" default:"false"`
	TempIndexMinBlocks      int      `xml:"tempIndexMinBlocks" json:"tempIndexMinBlocks" default:"10"`
	UnackedNotificationIDs  []string `xml:"unackedNotificationID" json:"unackedNotificationIDs"`
	TrafficClass            int      `xml:"trafficClass" json:"trafficClass"`
	DefaultFolderPath       string   `xml:"defaultFolderPath" json:"defaultFolderPath" default:"~"`
	SetLowPriority          bool     `xml:"setLowPriority" json:"setLowPriority" default:"true"`
	RawMaxFolderConcurrency int      `xml:"maxFolderConcurrency" json:"maxFolderConcurrency"`
	CRURL                   string   `xml:"crashReportingURL" json:"crURL" default:"https://crash.syncthing.net/newcrash"` // crash reporting URL
	CREnabled               bool     `xml:"crashReportingEnabled" json:"crashReportingEnabled" default:"true" restart:"true"`
	StunKeepaliveStartS     int      `xml:"stunKeepaliveStartS" json:"stunKeepaliveStartS" default:"180"` // 0 for off
	StunKeepaliveMinS       int      `xml:"stunKeepaliveMinS" json:"stunKeepaliveMinS" default:"20"`      // 0 for off
	RawStunServers          []string `xml:"stunServer" json:"stunServers" default:"default"`
	DatabaseTuning          Tuning   `xml:"databaseTuning" json:"databaseTuning" restart:"true"`
	RawMaxCIRequestKiB      int      `xml:"maxConcurrentIncomingRequestKiB" json:"maxConcurrentIncomingRequestKiB"`

	DeprecatedUPnPEnabled        bool     `xml:"upnpEnabled,omitempty" json:"-"`
	DeprecatedUPnPLeaseM         int      `xml:"upnpLeaseMinutes,omitempty" json:"-"`
	DeprecatedUPnPRenewalM       int      `xml:"upnpRenewalMinutes,omitempty" json:"-"`
	DeprecatedUPnPTimeoutS       int      `xml:"upnpTimeoutSeconds,omitempty" json:"-"`
	DeprecatedRelayServers       []string `xml:"relayServer,omitempty" json:"-"`
	DeprecatedMinHomeDiskFreePct float64  `xml:"minHomeDiskFreePct,omitempty" json:"-"`
	DeprecatedMaxConcurrentScans int      `xml:"maxConcurrentScans,omitempty" json:"-"`
}

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

	if opts.RawMaxFolderConcurrency == 0 {
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

func (opts OptionsConfiguration) ShouldAutoUpgrade() bool {
	return opts.AutoUpgradeIntervalH > 0
}
