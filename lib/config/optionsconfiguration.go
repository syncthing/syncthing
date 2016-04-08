// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package config

type OptionsConfiguration struct {
	ListenAddress           []string `xml:"listenAddress" json:"listenAddress" default:"tcp://0.0.0.0:22000"`
	GlobalAnnServers        []string `xml:"globalAnnounceServer" json:"globalAnnounceServers" json:"globalAnnounceServer" default:"default"`
	GlobalAnnEnabled        bool     `xml:"globalAnnounceEnabled" json:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled         bool     `xml:"localAnnounceEnabled" json:"localAnnounceEnabled" default:"true"`
	LocalAnnPort            int      `xml:"localAnnouncePort" json:"localAnnouncePort" default:"21027"`
	LocalAnnMCAddr          string   `xml:"localAnnounceMCAddr" json:"localAnnounceMCAddr" default:"[ff12::8384]:21027"`
	RelayServers            []string `xml:"relayServer" json:"relayServers" default:"dynamic+https://relays.syncthing.net/endpoint"`
	MaxSendKbps             int      `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps             int      `xml:"maxRecvKbps" json:"maxRecvKbps"`
	ReconnectIntervalS      int      `xml:"reconnectionIntervalS" json:"reconnectionIntervalS" default:"60"`
	RelaysEnabled           bool     `xml:"relaysEnabled" json:"relaysEnabled" default:"true"`
	RelayReconnectIntervalM int      `xml:"relayReconnectIntervalM" json:"relayReconnectIntervalM" default:"10"`
	StartBrowser            bool     `xml:"startBrowser" json:"startBrowser" default:"true"`
	UPnPEnabled             bool     `xml:"upnpEnabled" json:"upnpEnabled" default:"true"`
	UPnPLeaseM              int      `xml:"upnpLeaseMinutes" json:"upnpLeaseMinutes" default:"60"`
	UPnPRenewalM            int      `xml:"upnpRenewalMinutes" json:"upnpRenewalMinutes" default:"30"`
	UPnPTimeoutS            int      `xml:"upnpTimeoutSeconds" json:"upnpTimeoutSeconds" default:"10"`
	URAccepted              int      `xml:"urAccepted" json:"urAccepted"` // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)
	URUniqueID              string   `xml:"urUniqueID" json:"urUniqueId"` // Unique ID for reporting purposes, regenerated when UR is turned on.
	URURL                   string   `xml:"urURL" json:"urURL" default:"https://data.syncthing.net/newdata"`
	URPostInsecurely        bool     `xml:"urPostInsecurely" json:"urPostInsecurely" default:"false"` // For testing
	URInitialDelayS         int      `xml:"urInitialDelayS" json:"urInitialDelayS" default:"1800"`
	RestartOnWakeup         bool     `xml:"restartOnWakeup" json:"restartOnWakeup" default:"true"`
	AutoUpgradeIntervalH    int      `xml:"autoUpgradeIntervalH" json:"autoUpgradeIntervalH" default:"12"` // 0 for off
	KeepTemporariesH        int      `xml:"keepTemporariesH" json:"keepTemporariesH" default:"24"`         // 0 for off
	CacheIgnoredFiles       bool     `xml:"cacheIgnoredFiles" json:"cacheIgnoredFiles" default:"false"`
	ProgressUpdateIntervalS int      `xml:"progressUpdateIntervalS" json:"progressUpdateIntervalS" default:"5"`
	SymlinksEnabled         bool     `xml:"symlinksEnabled" json:"symlinksEnabled" default:"true"`
	LimitBandwidthInLan     bool     `xml:"limitBandwidthInLan" json:"limitBandwidthInLan" default:"false"`
	MinHomeDiskFreePct      float64  `xml:"minHomeDiskFreePct" json:"minHomeDiskFreePct" default:"1"`
	ReleasesURL             string   `xml:"releasesURL" json:"releasesURL" default:"https://api.github.com/repos/syncthing/syncthing/releases?per_page=30"`
	AlwaysLocalNets         []string `xml:"alwaysLocalNet" json:"alwaysLocalNets"`
	OverwriteNames          bool     `xml:"overwriteNames" json:"overwriteNames" default:"false"`
}

func (orig OptionsConfiguration) Copy() OptionsConfiguration {
	c := orig
	c.ListenAddress = make([]string, len(orig.ListenAddress))
	copy(c.ListenAddress, orig.ListenAddress)
	c.GlobalAnnServers = make([]string, len(orig.GlobalAnnServers))
	copy(c.GlobalAnnServers, orig.GlobalAnnServers)
	c.RelayServers = make([]string, len(orig.RelayServers))
	copy(c.RelayServers, orig.RelayServers)
	c.AlwaysLocalNets = make([]string, len(orig.AlwaysLocalNets))
	copy(c.AlwaysLocalNets, orig.AlwaysLocalNets)
	return c
}
