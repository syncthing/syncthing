// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package contract

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/structutil"
)

type Report struct {
	// Generated
	Received time.Time `json:"-"` // Only from DB
	Date     string    `json:"date,omitempty"`
	Address  string    `json:"address,omitempty"`

	// v1 fields

	UniqueID       string  `json:"uniqueID,omitempty" since:"1"`
	Version        string  `json:"version,omitempty" since:"1"`
	LongVersion    string  `json:"longVersion,omitempty" since:"1"`
	Platform       string  `json:"platform,omitempty" since:"1"`
	NumFolders     int     `json:"numFolders,omitempty" since:"1"`
	NumDevices     int     `json:"numDevices,omitempty" since:"1"`
	TotFiles       int     `json:"totFiles,omitempty" since:"1"`
	FolderMaxFiles int     `json:"folderMaxFiles,omitempty" since:"1"`
	TotMiB         int     `json:"totMiB,omitempty" since:"1"`
	FolderMaxMiB   int     `json:"folderMaxMiB,omitempty" since:"1"`
	MemoryUsageMiB int     `json:"memoryUsageMiB,omitempty" since:"1"`
	SHA256Perf     float64 `json:"sha256Perf,omitempty" since:"1"`
	HashPerf       float64 `json:"hashPerf,omitempty" since:"1"` // Was previously not stored server-side
	MemorySize     int     `json:"memorySize,omitempty" since:"1"`

	// v2 fields

	URVersion  int `json:"urVersion,omitempty" since:"2"`
	NumCPU     int `json:"numCPU,omitempty" since:"2"`
	FolderUses struct {
		SendOnly            int `json:"sendonly,omitempty" since:"2"`
		SendReceive         int `json:"sendreceive,omitempty" since:"2"` // Was previously not stored server-side
		ReceiveOnly         int `json:"receiveonly,omitempty" since:"2"`
		IgnorePerms         int `json:"ignorePerms,omitempty" since:"2"`
		IgnoreDelete        int `json:"ignoreDelete,omitempty" since:"2"`
		AutoNormalize       int `json:"autoNormalize,omitempty" since:"2"`
		SimpleVersioning    int `json:"simpleVersioning,omitempty" since:"2"`
		ExternalVersioning  int `json:"externalVersioning,omitempty" since:"2"`
		StaggeredVersioning int `json:"staggeredVersioning,omitempty" since:"2"`
		TrashcanVersioning  int `json:"trashcanVersioning,omitempty" since:"2"`
	} `json:"folderUses,omitempty" since:"2"`

	DeviceUses struct {
		Introducer       int `json:"introducer,omitempty" since:"2"`
		CustomCertName   int `json:"customCertName,omitempty" since:"2"`
		CompressAlways   int `json:"compressAlways,omitempty" since:"2"`
		CompressMetadata int `json:"compressMetadata,omitempty" since:"2"`
		CompressNever    int `json:"compressNever,omitempty" since:"2"`
		DynamicAddr      int `json:"dynamicAddr,omitempty" since:"2"`
		StaticAddr       int `json:"staticAddr,omitempty" since:"2"`
	} `json:"deviceUses,omitempty" since:"2"`

	Announce struct {
		GlobalEnabled     bool `json:"globalEnabled,omitempty" since:"2"`
		LocalEnabled      bool `json:"localEnabled,omitempty" since:"2"`
		DefaultServersDNS int  `json:"defaultServersDNS,omitempty" since:"2"`
		DefaultServersIP  int  `json:"defaultServersIP,omitempty" since:"2"` // Deprecated and not provided client-side anymore
		OtherServers      int  `json:"otherServers,omitempty" since:"2"`
	} `json:"announce,omitempty" since:"2"`

	Relays struct {
		Enabled        bool `json:"enabled,omitempty" since:"2"`
		DefaultServers int  `json:"defaultServers,omitempty" since:"2"`
		OtherServers   int  `json:"otherServers,omitempty" since:"2"`
	} `json:"relays,omitempty" since:"2"`

	UsesRateLimit        bool `json:"usesRateLimit,omitempty" since:"2"`
	UpgradeAllowedManual bool `json:"upgradeAllowedManual,omitempty" since:"2"`
	UpgradeAllowedAuto   bool `json:"upgradeAllowedAuto,omitempty" since:"2"`

	// V2.5 fields (fields that were in v2 but never added to the database
	UpgradeAllowedPre bool  `json:"upgradeAllowedPre,omitempty" since:"2"`
	RescanIntvs       []int `json:"rescanIntvs,omitempty" since:"2"`

	// v3 fields

	Uptime                     int    `json:"uptime,omitempty" since:"3"`
	NATType                    string `json:"natType,omitempty" since:"3"`
	AlwaysLocalNets            bool   `json:"alwaysLocalNets,omitempty" since:"3"`
	CacheIgnoredFiles          bool   `json:"cacheIgnoredFiles,omitempty" since:"3"`
	OverwriteRemoteDeviceNames bool   `json:"overwriteRemoteDeviceNames,omitempty" since:"3"`
	ProgressEmitterEnabled     bool   `json:"progressEmitterEnabled,omitempty" since:"3"`
	CustomDefaultFolderPath    bool   `json:"customDefaultFolderPath,omitempty" since:"3"`
	WeakHashSelection          string `json:"weakHashSelection,omitempty" since:"3"` // Deprecated and not provided client-side anymore
	CustomTrafficClass         bool   `json:"customTrafficClass,omitempty" since:"3"`
	CustomTempIndexMinBlocks   bool   `json:"customTempIndexMinBlocks,omitempty" since:"3"`
	TemporariesDisabled        bool   `json:"temporariesDisabled,omitempty" since:"3"`
	TemporariesCustom          bool   `json:"temporariesCustom,omitempty" since:"3"`
	LimitBandwidthInLan        bool   `json:"limitBandwidthInLan,omitempty" since:"3"`
	CustomReleaseURL           bool   `json:"customReleaseURL,omitempty" since:"3"`
	RestartOnWakeup            bool   `json:"restartOnWakeup,omitempty" since:"3"`
	CustomStunServers          bool   `json:"customStunServers,omitempty" since:"3"`

	FolderUsesV3 struct {
		ScanProgressDisabled    int            `json:"scanProgressDisabled,omitempty" since:"3"`
		ConflictsDisabled       int            `json:"conflictsDisabled,omitempty" since:"3"`
		ConflictsUnlimited      int            `json:"conflictsUnlimited,omitempty" since:"3"`
		ConflictsOther          int            `json:"conflictsOther,omitempty" since:"3"`
		DisableSparseFiles      int            `json:"disableSparseFiles,omitempty" since:"3"`
		DisableTempIndexes      int            `json:"disableTempIndexes,omitempty" since:"3"`
		AlwaysWeakHash          int            `json:"alwaysWeakHash,omitempty" since:"3"`
		CustomWeakHashThreshold int            `json:"customWeakHashThreshold,omitempty" since:"3"`
		FsWatcherEnabled        int            `json:"fsWatcherEnabled,omitempty" since:"3"`
		PullOrder               map[string]int `json:"pullOrder,omitempty" since:"3"`
		FilesystemType          map[string]int `json:"filesystemType,omitempty" since:"3"`
		FsWatcherDelays         []int          `json:"fsWatcherDelays,omitempty" since:"3"`
		CustomMarkerName        int            `json:"customMarkerName,omitempty" since:"3"`
		CopyOwnershipFromParent int            `json:"copyOwnershipFromParent,omitempty" since:"3"`
		ModTimeWindowS          []int          `json:"modTimeWindowS,omitempty" since:"3"`
		MaxConcurrentWrites     []int          `json:"maxConcurrentWrites,omitempty" since:"3"`
		DisableFsync            int            `json:"disableFsync,omitempty" since:"3"`
		BlockPullOrder          map[string]int `json:"blockPullOrder,omitempty" since:"3"`
		CopyRangeMethod         map[string]int `json:"copyRangeMethod,omitempty" since:"3"`
		CaseSensitiveFS         int            `json:"caseSensitiveFS,omitempty" since:"3"`
		ReceiveEncrypted        int            `json:"receiveencrypted,omitempty" since:"3"`
	} `json:"folderUsesV3,omitempty" since:"3"`

	DeviceUsesV3 struct {
		Untrusted int `json:"untrusted,omitempty" since:"3"`
	} `json:"deviceUsesV3,omitempty" since:"3"`

	GUIStats struct {
		Enabled                   int            `json:"enabled,omitempty" since:"3"`
		UseTLS                    int            `json:"useTLS,omitempty" since:"3"`
		UseAuth                   int            `json:"useAuth,omitempty" since:"3"`
		InsecureAdminAccess       int            `json:"insecureAdminAccess,omitempty" since:"3"`
		Debugging                 int            `json:"debugging,omitempty" since:"3"`
		InsecureSkipHostCheck     int            `json:"insecureSkipHostCheck,omitempty" since:"3"`
		InsecureAllowFrameLoading int            `json:"insecureAllowFrameLoading,omitempty" since:"3"`
		ListenLocal               int            `json:"listenLocal,omitempty" since:"3"`
		ListenUnspecified         int            `json:"listenUnspecified,omitempty" since:"3"`
		Theme                     map[string]int `json:"theme,omitempty" since:"3"`
	} `json:"guiStats,omitempty" since:"3"`

	BlockStats struct {
		Total             int `json:"total,omitempty" since:"3"`
		Renamed           int `json:"renamed,omitempty" since:"3"`
		Reused            int `json:"reused,omitempty" since:"3"`
		Pulled            int `json:"pulled,omitempty" since:"3"`
		CopyOrigin        int `json:"copyOrigin,omitempty" since:"3"`
		CopyOriginShifted int `json:"copyOriginShifted,omitempty" since:"3"`
		CopyElsewhere     int `json:"copyElsewhere,omitempty" since:"3"`
	} `json:"blockStats,omitempty" since:"3"`

	TransportStats map[string]int `json:"transportStats,omitempty" since:"3"`

	IgnoreStats struct {
		Lines           int `json:"lines,omitempty" since:"3"`
		Inverts         int `json:"inverts,omitempty" since:"3"`
		Folded          int `json:"folded,omitempty" since:"3"`
		Deletable       int `json:"deletable,omitempty" since:"3"`
		Rooted          int `json:"rooted,omitempty" since:"3"`
		Includes        int `json:"includes,omitempty" since:"3"`
		EscapedIncludes int `json:"escapedIncludes,omitempty" since:"3"`
		DoubleStars     int `json:"doubleStars,omitempty" since:"3"`
		Stars           int `json:"stars,omitempty" since:"3"`
	} `json:"ignoreStats,omitempty" since:"3"`

	// V3 fields added late in the RC
	WeakHashEnabled bool `json:"weakHashEnabled,omitempty" since:"3"` // Deprecated and not provided client-side anymore
}

func New() *Report {
	r := &Report{}
	structutil.FillNil(r)
	return r
}

func (r *Report) Validate() error {
	if r.UniqueID == "" || r.Version == "" || r.Platform == "" {
		return errors.New("missing required field")
	}
	if len(r.Date) != 8 {
		return errors.New("date not initialized")
	}

	// Early versions are no longer relevant to handle.
	if strings.HasPrefix(r.Version, "v0.") {
		return errors.New("unsupported Syncthing version")
	}

	// Only allow valid URVersions.
	if r.URVersion < 1 || r.URVersion > 3 {
		return errors.New("unsupported URVersion")
	}

	// Reports with known unrealistic values are skipped.
	if r.MemorySize >= 1073741824 {
		return errors.New("unrealistic value")
	}

	// Some fields may not be null.
	if r.RescanIntvs == nil {
		r.RescanIntvs = []int{}
	}
	if r.FolderUsesV3.FsWatcherDelays == nil {
		r.FolderUsesV3.FsWatcherDelays = []int{}
	}

	return nil
}

func (r *Report) ClearForVersion(version int) error {
	return clear(r, version)
}

func (r *Report) FieldPointers() []interface{} {
	// All the fields of the Report, in the same order as the database fields.
	return []interface{}{
		&r.Received, &r.UniqueID, &r.Version, &r.LongVersion, &r.Platform,
		&r.NumFolders, &r.NumDevices, &r.TotFiles, &r.FolderMaxFiles,
		&r.TotMiB, &r.FolderMaxMiB, &r.MemoryUsageMiB, &r.SHA256Perf,
		&r.MemorySize, &r.Date,
		// V2
		&r.URVersion, &r.NumCPU, &r.FolderUses.SendOnly, &r.FolderUses.IgnorePerms,
		&r.FolderUses.IgnoreDelete, &r.FolderUses.AutoNormalize, &r.DeviceUses.Introducer,
		&r.DeviceUses.CustomCertName, &r.DeviceUses.CompressAlways,
		&r.DeviceUses.CompressMetadata, &r.DeviceUses.CompressNever,
		&r.DeviceUses.DynamicAddr, &r.DeviceUses.StaticAddr,
		&r.Announce.GlobalEnabled, &r.Announce.LocalEnabled,
		&r.Announce.DefaultServersDNS, &r.Announce.DefaultServersIP,
		&r.Announce.OtherServers, &r.Relays.Enabled, &r.Relays.DefaultServers,
		&r.Relays.OtherServers, &r.UsesRateLimit, &r.UpgradeAllowedManual,
		&r.UpgradeAllowedAuto, &r.FolderUses.SimpleVersioning,
		&r.FolderUses.ExternalVersioning, &r.FolderUses.StaggeredVersioning,
		&r.FolderUses.TrashcanVersioning,

		// V2.5
		&r.UpgradeAllowedPre,

		// V3
		&r.Uptime, &r.NATType, &r.AlwaysLocalNets, &r.CacheIgnoredFiles,
		&r.OverwriteRemoteDeviceNames, &r.ProgressEmitterEnabled, &r.CustomDefaultFolderPath,
		&r.WeakHashSelection, &r.CustomTrafficClass, &r.CustomTempIndexMinBlocks,
		&r.TemporariesDisabled, &r.TemporariesCustom, &r.LimitBandwidthInLan,
		&r.CustomReleaseURL, &r.RestartOnWakeup, &r.CustomStunServers,

		&r.FolderUsesV3.ScanProgressDisabled, &r.FolderUsesV3.ConflictsDisabled,
		&r.FolderUsesV3.ConflictsUnlimited, &r.FolderUsesV3.ConflictsOther,
		&r.FolderUsesV3.DisableSparseFiles, &r.FolderUsesV3.DisableTempIndexes,
		&r.FolderUsesV3.AlwaysWeakHash, &r.FolderUsesV3.CustomWeakHashThreshold,
		&r.FolderUsesV3.FsWatcherEnabled,

		&r.GUIStats.Enabled, &r.GUIStats.UseTLS, &r.GUIStats.UseAuth,
		&r.GUIStats.InsecureAdminAccess,
		&r.GUIStats.Debugging, &r.GUIStats.InsecureSkipHostCheck,
		&r.GUIStats.InsecureAllowFrameLoading, &r.GUIStats.ListenLocal,
		&r.GUIStats.ListenUnspecified,

		&r.BlockStats.Total, &r.BlockStats.Renamed,
		&r.BlockStats.Reused, &r.BlockStats.Pulled, &r.BlockStats.CopyOrigin,
		&r.BlockStats.CopyOriginShifted, &r.BlockStats.CopyElsewhere,

		&r.IgnoreStats.Lines, &r.IgnoreStats.Inverts, &r.IgnoreStats.Folded,
		&r.IgnoreStats.Deletable, &r.IgnoreStats.Rooted, &r.IgnoreStats.Includes,
		&r.IgnoreStats.EscapedIncludes, &r.IgnoreStats.DoubleStars, &r.IgnoreStats.Stars,

		// V3 added late in the RC
		&r.WeakHashEnabled,
		&r.Address,

		// Receive only folders
		&r.FolderUses.ReceiveOnly,
	}
}

func (*Report) FieldNames() []string {
	// The database fields that back this struct in PostgreSQL
	return []string{
		// V1
		"Received",
		"UniqueID",
		"Version",
		"LongVersion",
		"Platform",
		"NumFolders",
		"NumDevices",
		"TotFiles",
		"FolderMaxFiles",
		"TotMiB",
		"FolderMaxMiB",
		"MemoryUsageMiB",
		"SHA256Perf",
		"MemorySize",
		"Date",
		// V2
		"ReportVersion",
		"NumCPU",
		"FolderRO",
		"FolderIgnorePerms",
		"FolderIgnoreDelete",
		"FolderAutoNormalize",
		"DeviceIntroducer",
		"DeviceCustomCertName",
		"DeviceCompressionAlways",
		"DeviceCompressionMetadata",
		"DeviceCompressionNever",
		"DeviceDynamicAddr",
		"DeviceStaticAddr",
		"AnnounceGlobalEnabled",
		"AnnounceLocalEnabled",
		"AnnounceDefaultServersDNS",
		"AnnounceDefaultServersIP",
		"AnnounceOtherServers",
		"RelayEnabled",
		"RelayDefaultServers",
		"RelayOtherServers",
		"RateLimitEnabled",
		"UpgradeAllowedManual",
		"UpgradeAllowedAuto",
		// v0.12.19+
		"FolderSimpleVersioning",
		"FolderExternalVersioning",
		"FolderStaggeredVersioning",
		"FolderTrashcanVersioning",
		// V2.5
		"UpgradeAllowedPre",
		// V3
		"Uptime",
		"NATType",
		"AlwaysLocalNets",
		"CacheIgnoredFiles",
		"OverwriteRemoteDeviceNames",
		"ProgressEmitterEnabled",
		"CustomDefaultFolderPath",
		"WeakHashSelection",
		"CustomTrafficClass",
		"CustomTempIndexMinBlocks",
		"TemporariesDisabled",
		"TemporariesCustom",
		"LimitBandwidthInLan",
		"CustomReleaseURL",
		"RestartOnWakeup",
		"CustomStunServers",

		"FolderScanProgressDisabled",
		"FolderConflictsDisabled",
		"FolderConflictsUnlimited",
		"FolderConflictsOther",
		"FolderDisableSparseFiles",
		"FolderDisableTempIndexes",
		"FolderAlwaysWeakHash",
		"FolderCustomWeakHashThreshold",
		"FolderFsWatcherEnabled",

		"GUIEnabled",
		"GUIUseTLS",
		"GUIUseAuth",
		"GUIInsecureAdminAccess",
		"GUIDebugging",
		"GUIInsecureSkipHostCheck",
		"GUIInsecureAllowFrameLoading",
		"GUIListenLocal",
		"GUIListenUnspecified",

		"BlocksTotal",
		"BlocksRenamed",
		"BlocksReused",
		"BlocksPulled",
		"BlocksCopyOrigin",
		"BlocksCopyOriginShifted",
		"BlocksCopyElsewhere",

		"IgnoreLines",
		"IgnoreInverts",
		"IgnoreFolded",
		"IgnoreDeletable",
		"IgnoreRooted",
		"IgnoreIncludes",
		"IgnoreEscapedIncludes",
		"IgnoreDoubleStars",
		"IgnoreStars",

		// V3 added late in the RC
		"WeakHashEnabled",
		"Address",

		// Receive only folders
		"FolderRecvOnly",
	}
}

func (r Report) Value() (driver.Value, error) {
	// This needs to be string, yet we read back bytes..
	bs, err := json.Marshal(r)
	return string(bs), err
}

func (r *Report) Scan(value interface{}) error {
	// Zero out the previous value
	// JSON un-marshaller does not touch fields that are not in the payload, so we carry over values from a previous
	// scan.
	*r = Report{}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &r)
}

func clear(v interface{}, since int) error {
	s := reflect.ValueOf(v).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		v := tag.Get("since")
		if v == "" {
			f.Set(reflect.Zero(f.Type()))
			continue
		}

		vn, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		if vn > since {
			f.Set(reflect.Zero(f.Type()))
			continue
		}

		// Dive deeper
		if f.Kind() == reflect.Ptr {
			f = f.Elem()
		}

		if f.Kind() == reflect.Struct {
			if err := clear(f.Addr().Interface(), since); err != nil {
				return err
			}
		}
	}
	return nil
}
