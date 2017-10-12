// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
)

type WeakHashSelectionMethod int

const (
	WeakHashAuto WeakHashSelectionMethod = iota
	WeakHashAlways
	WeakHashNever
)

func (m WeakHashSelectionMethod) MarshalString() (string, error) {
	switch m {
	case WeakHashAuto:
		return "auto", nil
	case WeakHashAlways:
		return "always", nil
	case WeakHashNever:
		return "never", nil
	default:
		return "", fmt.Errorf("unrecognized hash selection method")
	}
}

func (m WeakHashSelectionMethod) String() string {
	s, err := m.MarshalString()
	if err != nil {
		panic(err)
	}
	return s
}

func (m *WeakHashSelectionMethod) UnmarshalString(value string) error {
	switch value {
	case "auto":
		*m = WeakHashAuto
		return nil
	case "always":
		*m = WeakHashAlways
		return nil
	case "never":
		*m = WeakHashNever
		return nil
	}
	return fmt.Errorf("unrecognized hash selection method")
}

func (m WeakHashSelectionMethod) MarshalJSON() ([]byte, error) {
	val, err := m.MarshalString()
	if err != nil {
		return nil, err
	}
	return json.Marshal(val)
}

func (m *WeakHashSelectionMethod) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return m.UnmarshalString(value)
}

func (m WeakHashSelectionMethod) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	val, err := m.MarshalString()
	if err != nil {
		return err
	}
	return e.EncodeElement(val, start)
}

func (m *WeakHashSelectionMethod) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var value string
	if err := d.DecodeElement(&value, &start); err != nil {
		return err
	}
	return m.UnmarshalString(value)
}

func (WeakHashSelectionMethod) ParseDefault(value string) (interface{}, error) {
	var m WeakHashSelectionMethod
	err := m.UnmarshalString(value)
	return m, err
}

type OptionsConfiguration struct {
	ListenAddresses         []string                `xml:"listenAddress" json:"listenAddresses" default:"default"`
	GlobalAnnServers        []string                `xml:"globalAnnounceServer" json:"globalAnnounceServers" json:"globalAnnounceServer" default:"default"`
	GlobalAnnEnabled        bool                    `xml:"globalAnnounceEnabled" json:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled         bool                    `xml:"localAnnounceEnabled" json:"localAnnounceEnabled" default:"true"`
	LocalAnnPort            int                     `xml:"localAnnouncePort" json:"localAnnouncePort" default:"21027"`
	LocalAnnMCAddr          string                  `xml:"localAnnounceMCAddr" json:"localAnnounceMCAddr" default:"[ff12::8384]:21027"`
	MaxSendKbps             int                     `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps             int                     `xml:"maxRecvKbps" json:"maxRecvKbps"`
	ReconnectIntervalS      int                     `xml:"reconnectionIntervalS" json:"reconnectionIntervalS" default:"60"`
	RelaysEnabled           bool                    `xml:"relaysEnabled" json:"relaysEnabled" default:"true"`
	RelayReconnectIntervalM int                     `xml:"relayReconnectIntervalM" json:"relayReconnectIntervalM" default:"10"`
	StartBrowser            bool                    `xml:"startBrowser" json:"startBrowser" default:"true"`
	NATEnabled              bool                    `xml:"natEnabled" json:"natEnabled" default:"true"`
	NATLeaseM               int                     `xml:"natLeaseMinutes" json:"natLeaseMinutes" default:"60"`
	NATRenewalM             int                     `xml:"natRenewalMinutes" json:"natRenewalMinutes" default:"30"`
	NATTimeoutS             int                     `xml:"natTimeoutSeconds" json:"natTimeoutSeconds" default:"10"`
	URAccepted              int                     `xml:"urAccepted" json:"urAccepted"` // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)
	URSeen                  int                     `xml:"urSeen" json:"urSeen"`         // Report which the user has been prompted for.
	URUniqueID              string                  `xml:"urUniqueID" json:"urUniqueId"` // Unique ID for reporting purposes, regenerated when UR is turned on.
	URURL                   string                  `xml:"urURL" json:"urURL" default:"https://data.syncthing.net/newdata"`
	URPostInsecurely        bool                    `xml:"urPostInsecurely" json:"urPostInsecurely" default:"false"` // For testing
	URInitialDelayS         int                     `xml:"urInitialDelayS" json:"urInitialDelayS" default:"1800"`
	RestartOnWakeup         bool                    `xml:"restartOnWakeup" json:"restartOnWakeup" default:"true"`
	AutoUpgradeIntervalH    int                     `xml:"autoUpgradeIntervalH" json:"autoUpgradeIntervalH" default:"12"` // 0 for off
	UpgradeToPreReleases    bool                    `xml:"upgradeToPreReleases" json:"upgradeToPreReleases"`              // when auto upgrades are enabled
	KeepTemporariesH        int                     `xml:"keepTemporariesH" json:"keepTemporariesH" default:"24"`         // 0 for off
	CacheIgnoredFiles       bool                    `xml:"cacheIgnoredFiles" json:"cacheIgnoredFiles" default:"false"`
	ProgressUpdateIntervalS int                     `xml:"progressUpdateIntervalS" json:"progressUpdateIntervalS" default:"5"`
	LimitBandwidthInLan     bool                    `xml:"limitBandwidthInLan" json:"limitBandwidthInLan" default:"false"`
	MinHomeDiskFree         Size                    `xml:"minHomeDiskFree" json:"minHomeDiskFree" default:"1 %"`
	ReleasesURL             string                  `xml:"releasesURL" json:"releasesURL" default:"https://upgrades.syncthing.net/meta.json"`
	AlwaysLocalNets         []string                `xml:"alwaysLocalNet" json:"alwaysLocalNets"`
	OverwriteRemoteDevNames bool                    `xml:"overwriteRemoteDeviceNamesOnConnect" json:"overwriteRemoteDeviceNamesOnConnect" default:"false"`
	TempIndexMinBlocks      int                     `xml:"tempIndexMinBlocks" json:"tempIndexMinBlocks" default:"10"`
	UnackedNotificationIDs  []string                `xml:"unackedNotificationID" json:"unackedNotificationIDs"`
	TrafficClass            int                     `xml:"trafficClass" json:"trafficClass"`
	WeakHashSelectionMethod WeakHashSelectionMethod `xml:"weakHashSelectionMethod" json:"weakHashSelectionMethod"`
	StunServers             []string                `xml:"stunServer" json:"stunServers" default:"default"`
	StunKeepaliveS          int                     `xml:"stunKeepaliveSeconds" json:"stunKeepaliveSeconds" default:"24"`
	DefaultKCPEnabled       bool                    `xml:"defaultKCPEnabled" json:"defaultKCPEnabled" default:"false"`
	KCPNoDelay              bool                    `xml:"kcpNoDelay" json:"kcpNoDelay" default:"false"`
	KCPUpdateIntervalMs     int                     `xml:"kcpUpdateIntervalMs" json:"kcpUpdateIntervalMs" default:"25"`
	KCPFastResend           bool                    `xml:"kcpFastResend" json:"kcpFastResend" default:"false"`
	KCPCongestionControl    bool                    `xml:"kcpCongestionControl" json:"kcpCongestionControl" default:"true"`
	KCPSendWindowSize       int                     `xml:"kcpSendWindowSize" json:"kcpSendWindowSize" default:"128"`
	KCPReceiveWindowSize    int                     `xml:"kcpReceiveWindowSize" json:"kcpReceiveWindowSize" default:"128"`
	DefaultFolderPath       string                  `xml:"defaultFolderPath" json:"defaultFolderPath" default:"~"`

	DeprecatedUPnPEnabled        bool     `xml:"upnpEnabled,omitempty" json:"-"`
	DeprecatedUPnPLeaseM         int      `xml:"upnpLeaseMinutes,omitempty" json:"-"`
	DeprecatedUPnPRenewalM       int      `xml:"upnpRenewalMinutes,omitempty" json:"-"`
	DeprecatedUPnPTimeoutS       int      `xml:"upnpTimeoutSeconds,omitempty" json:"-"`
	DeprecatedRelayServers       []string `xml:"relayServer,omitempty" json:"-"`
	DeprecatedMinHomeDiskFreePct float64  `xml:"minHomeDiskFreePct" json:"-"`
}

func (orig OptionsConfiguration) Copy() OptionsConfiguration {
	c := orig
	c.ListenAddresses = make([]string, len(orig.ListenAddresses))
	copy(c.ListenAddresses, orig.ListenAddresses)
	c.GlobalAnnServers = make([]string, len(orig.GlobalAnnServers))
	copy(c.GlobalAnnServers, orig.GlobalAnnServers)
	c.AlwaysLocalNets = make([]string, len(orig.AlwaysLocalNets))
	copy(c.AlwaysLocalNets, orig.AlwaysLocalNets)
	c.UnackedNotificationIDs = make([]string, len(orig.UnackedNotificationIDs))
	copy(c.UnackedNotificationIDs, orig.UnackedNotificationIDs)
	return c
}
