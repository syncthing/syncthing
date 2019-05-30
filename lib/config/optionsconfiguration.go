// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/util"
)

type OptionsConfiguration struct {
	ListenAddresses         []string `xml:"listenAddress" json:"listenAddresses" default:"default"`
	GlobalAnnServers        []string `xml:"globalAnnounceServer" json:"globalAnnounceServers" default:"default" restart:"true"`
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
	URAccepted              int      `xml:"urAccepted" json:"urAccepted"` // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)
	URSeen                  int      `xml:"urSeen" json:"urSeen"`         // Report which the user has been prompted for.
	URUniqueID              string   `xml:"urUniqueID" json:"urUniqueId"` // Unique ID for reporting purposes, regenerated when UR is turned on.
	URURL                   string   `xml:"urURL" json:"urURL" default:"https://data.syncthing.net/newdata"`
	URPostInsecurely        bool     `xml:"urPostInsecurely" json:"urPostInsecurely" default:"false"` // For testing
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
	MaxConcurrentScans      int      `xml:"maxConcurrentScans" json:"maxConcurrentScans"`
	StunKeepaliveStartS     int      `xml:"stunKeepaliveStartS" json:"stunKeepaliveStartS" default:"180"` // 0 for off
	StunKeepaliveMinS       int      `xml:"stunKeepaliveMinS" json:"stunKeepaliveMinS" default:"20"`      // 0 for off
	StunServers             []string `xml:"stunServer" json:"stunServers" default:"default"`

	DeprecatedUPnPEnabled        bool     `xml:"upnpEnabled,omitempty" json:"-"`
	DeprecatedUPnPLeaseM         int      `xml:"upnpLeaseMinutes,omitempty" json:"-"`
	DeprecatedUPnPRenewalM       int      `xml:"upnpRenewalMinutes,omitempty" json:"-"`
	DeprecatedUPnPTimeoutS       int      `xml:"upnpTimeoutSeconds,omitempty" json:"-"`
	DeprecatedRelayServers       []string `xml:"relayServer,omitempty" json:"-"`
	DeprecatedMinHomeDiskFreePct float64  `xml:"minHomeDiskFreePct,omitempty" json:"-"`
}

func (opts OptionsConfiguration) Copy() OptionsConfiguration {
	optsCopy := opts
	optsCopy.ListenAddresses = make([]string, len(opts.ListenAddresses))
	copy(optsCopy.ListenAddresses, opts.ListenAddresses)
	optsCopy.GlobalAnnServers = make([]string, len(opts.GlobalAnnServers))
	copy(optsCopy.GlobalAnnServers, opts.GlobalAnnServers)
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
