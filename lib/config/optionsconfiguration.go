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
	"github.com/syncthing/syncthing/lib/stringutil"
	"github.com/syncthing/syncthing/lib/structutil"
)

type OptionsConfiguration struct {
	RawListenAddresses          []string `json:"listenAddresses" xml:"listenAddress" default:"default"`
	RawGlobalAnnServers         []string `json:"globalAnnounceServers" xml:"globalAnnounceServer" default:"default"`
	GlobalAnnEnabled            bool     `json:"globalAnnounceEnabled" xml:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled             bool     `json:"localAnnounceEnabled" xml:"localAnnounceEnabled" default:"true"`
	LocalAnnPort                int      `json:"localAnnouncePort" xml:"localAnnouncePort" default:"21027"`
	LocalAnnMCAddr              string   `json:"localAnnounceMCAddr" xml:"localAnnounceMCAddr" default:"[ff12::8384]:21027"`
	MaxSendKbps                 int      `json:"maxSendKbps" xml:"maxSendKbps"`
	MaxRecvKbps                 int      `json:"maxRecvKbps" xml:"maxRecvKbps"`
	ReconnectIntervalS          int      `json:"reconnectionIntervalS" xml:"reconnectionIntervalS" default:"60"`
	RelaysEnabled               bool     `json:"relaysEnabled" xml:"relaysEnabled" default:"true"`
	RelayReconnectIntervalM     int      `json:"relayReconnectIntervalM" xml:"relayReconnectIntervalM" default:"10"`
	StartBrowser                bool     `json:"startBrowser" xml:"startBrowser" default:"true"`
	NATEnabled                  bool     `json:"natEnabled" xml:"natEnabled" default:"true"`
	NATLeaseM                   int      `json:"natLeaseMinutes" xml:"natLeaseMinutes" default:"60"`
	NATRenewalM                 int      `json:"natRenewalMinutes" xml:"natRenewalMinutes" default:"30"`
	NATTimeoutS                 int      `json:"natTimeoutSeconds" xml:"natTimeoutSeconds" default:"10"`
	URAccepted                  int      `json:"urAccepted" xml:"urAccepted"`
	URSeen                      int      `json:"urSeen" xml:"urSeen"`
	URUniqueID                  string   `json:"urUniqueId" xml:"urUniqueID"`
	URURL                       string   `json:"urURL" xml:"urURL" default:"https://data.syncthing.net/newdata"`
	URPostInsecurely            bool     `json:"urPostInsecurely" xml:"urPostInsecurely" default:"false"`
	URInitialDelayS             int      `json:"urInitialDelayS" xml:"urInitialDelayS" default:"1800"`
	AutoUpgradeIntervalH        int      `json:"autoUpgradeIntervalH" xml:"autoUpgradeIntervalH" default:"12"`
	UpgradeToPreReleases        bool     `json:"upgradeToPreReleases" xml:"upgradeToPreReleases"`
	KeepTemporariesH            int      `json:"keepTemporariesH" xml:"keepTemporariesH" default:"24"`
	CacheIgnoredFiles           bool     `json:"cacheIgnoredFiles" xml:"cacheIgnoredFiles" default:"false"`
	ProgressUpdateIntervalS     int      `json:"progressUpdateIntervalS" xml:"progressUpdateIntervalS" default:"5"`
	LimitBandwidthInLan         bool     `json:"limitBandwidthInLan" xml:"limitBandwidthInLan" default:"false"`
	MinHomeDiskFree             Size     `json:"minHomeDiskFree" xml:"minHomeDiskFree" default:"1 %"`
	ReleasesURL                 string   `json:"releasesURL" xml:"releasesURL" default:"https://upgrades.syncthing.net/meta.json"`
	AlwaysLocalNets             []string `json:"alwaysLocalNets" xml:"alwaysLocalNet"`
	OverwriteRemoteDevNames     bool     `json:"overwriteRemoteDeviceNamesOnConnect" xml:"overwriteRemoteDeviceNamesOnConnect" default:"false"`
	TempIndexMinBlocks          int      `json:"tempIndexMinBlocks" xml:"tempIndexMinBlocks" default:"10"`
	UnackedNotificationIDs      []string `json:"unackedNotificationIDs" xml:"unackedNotificationID"`
	TrafficClass                int      `json:"trafficClass" xml:"trafficClass"`
	DeprecatedDefaultFolderPath string   `json:"-" xml:"defaultFolderPath,omitempty"` // Deprecated: Do not use.
	SetLowPriority              bool     `json:"setLowPriority" xml:"setLowPriority" default:"true"`
	RawMaxFolderConcurrency     int      `json:"maxFolderConcurrency" xml:"maxFolderConcurrency"`
	CRURL                       string   `json:"crURL" xml:"crashReportingURL" default:"https://crash.syncthing.net/newcrash"`
	CREnabled                   bool     `json:"crashReportingEnabled" xml:"crashReportingEnabled" default:"true"`
	StunKeepaliveStartS         int      `json:"stunKeepaliveStartS" xml:"stunKeepaliveStartS" default:"180"`
	StunKeepaliveMinS           int      `json:"stunKeepaliveMinS" xml:"stunKeepaliveMinS" default:"20"`
	RawStunServers              []string `json:"stunServers" xml:"stunServer" default:"default"`
	DatabaseTuning              Tuning   `json:"databaseTuning" xml:"databaseTuning" restart:"true"`
	RawMaxCIRequestKiB          int      `json:"maxConcurrentIncomingRequestKiB" xml:"maxConcurrentIncomingRequestKiB"`
	AnnounceLANAddresses        bool     `json:"announceLANAddresses" xml:"announceLANAddresses" default:"true"`
	SendFullIndexOnUpgrade      bool     `json:"sendFullIndexOnUpgrade" xml:"sendFullIndexOnUpgrade"`
	FeatureFlags                []string `json:"featureFlags" xml:"featureFlag"`
	// The number of connections at which we stop trying to connect to more
	// devices, zero meaning no limit. Does not affect incoming connections.
	ConnectionLimitEnough int `json:"connectionLimitEnough" xml:"connectionLimitEnough"`
	// The maximum number of connections which we will allow in total, zero
	// meaning no limit. Affects incoming connections and prevents
	// attempting outgoing connections.
	ConnectionLimitMax int `json:"connectionLimitMax" xml:"connectionLimitMax"`
	// When set, this allows TLS 1.2 on sync connections, where we otherwise
	// default to TLS 1.3+ only.
	InsecureAllowOldTLSVersions        bool `json:"insecureAllowOldTLSVersions" xml:"insecureAllowOldTLSVersions"`
	ConnectionPriorityTCPLAN           int  `json:"connectionPriorityTcpLan" xml:"connectionPriorityTcpLan" default:"10"`
	ConnectionPriorityQUICLAN          int  `json:"connectionPriorityQuicLan" xml:"connectionPriorityQuicLan" default:"20"`
	ConnectionPriorityTCPWAN           int  `json:"connectionPriorityTcpWan" xml:"connectionPriorityTcpWan" default:"30"`
	ConnectionPriorityQUICWAN          int  `json:"connectionPriorityQuicWan" xml:"connectionPriorityQuicWan" default:"40"`
	ConnectionPriorityRelay            int  `json:"connectionPriorityRelay" xml:"connectionPriorityRelay" default:"50"`
	ConnectionPriorityUpgradeThreshold int  `json:"connectionPriorityUpgradeThreshold" xml:"connectionPriorityUpgradeThreshold" default:"0"`
	// Legacy deprecated
	DeprecatedUPnPEnabled        bool     `json:"-" xml:"upnpEnabled,omitempty"`        // Deprecated: Do not use.
	DeprecatedUPnPLeaseM         int      `json:"-" xml:"upnpLeaseMinutes,omitempty"`   // Deprecated: Do not use.
	DeprecatedUPnPRenewalM       int      `json:"-" xml:"upnpRenewalMinutes,omitempty"` // Deprecated: Do not use.
	DeprecatedUPnPTimeoutS       int      `json:"-" xml:"upnpTimeoutSeconds,omitempty"` // Deprecated: Do not use.
	DeprecatedRelayServers       []string `json:"-" xml:"relayServer,omitempty"`        // Deprecated: Do not use.
	DeprecatedMinHomeDiskFreePct float64  `json:"-" xml:"minHomeDiskFreePct,omitempty"` // Deprecated: Do not use.
	DeprecatedMaxConcurrentScans int      `json:"-" xml:"maxConcurrentScans,omitempty"` // Deprecated: Do not use.
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

func (opts *OptionsConfiguration) prepare(guiPWIsSet bool) {
	structutil.FillNilSlices(opts)

	opts.RawListenAddresses = stringutil.UniqueTrimmedStrings(opts.RawListenAddresses)
	opts.RawGlobalAnnServers = stringutil.UniqueTrimmedStrings(opts.RawGlobalAnnServers)

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

	if opts.ConnectionPriorityQUICWAN <= opts.ConnectionPriorityQUICLAN {
		l.Warnln("Connection priority number for QUIC over WAN must be worse (higher) than QUIC over LAN. Correcting.")
		opts.ConnectionPriorityQUICWAN = opts.ConnectionPriorityQUICLAN + 1
	}
	if opts.ConnectionPriorityTCPWAN <= opts.ConnectionPriorityTCPLAN {
		l.Warnln("Connection priority number for TCP over WAN must be worse (higher) than TCP over LAN. Correcting.")
		opts.ConnectionPriorityTCPWAN = opts.ConnectionPriorityTCPLAN + 1
	}

	// If usage reporting is enabled we must have a unique ID.
	if opts.URAccepted > 0 && opts.URUniqueID == "" {
		opts.URUniqueID = rand.String(8)
	}
}

// RequiresRestartOnly returns a copy with only the attributes that require
// restart on change.
func (opts OptionsConfiguration) RequiresRestartOnly() OptionsConfiguration {
	optsCopy := opts
	blank := OptionsConfiguration{}
	copyMatchingTag(&blank, &optsCopy, "restart", func(v string) bool {
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
	return stringutil.UniqueTrimmedStrings(addresses)
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

	addresses = stringutil.UniqueTrimmedStrings(addresses)

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
	return stringutil.UniqueTrimmedStrings(servers)
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
// them is set. It's the point where we should stop dialing.
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
