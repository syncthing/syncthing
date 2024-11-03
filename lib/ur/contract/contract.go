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
	"time"

	"github.com/syncthing/syncthing/lib/structutil"
)

type Report struct {
	// v1 fields

	UniqueID       string  `json:"uniqueID,omitempty" metric:"-" since:"1"`
	Version        string  `json:"version,omitempty" metric:"reports_total,gaugeVec:version" since:"1"`
	LongVersion    string  `json:"longVersion,omitempty" metric:"-" since:"1"`
	Platform       string  `json:"platform,omitempty" metric:"-" since:"1"`
	NumFolders     int     `json:"numFolders,omitempty" metric:"num_folders,summary" since:"1"`
	NumDevices     int     `json:"numDevices,omitempty" metric:"num_devices,summary" since:"1"`
	TotFiles       int     `json:"totFiles,omitempty" metric:"total_files,summary" since:"1"`
	FolderMaxFiles int     `json:"folderMaxFiles,omitempty" metric:"folder_max_files,summary" since:"1"`
	TotMiB         int     `json:"totMiB,omitempty" metric:"total_data_mib,summary" since:"1"`
	FolderMaxMiB   int     `json:"folderMaxMiB,omitempty" metric:"folder_max_data_mib,summary" since:"1"`
	MemoryUsageMiB int     `json:"memoryUsageMiB,omitempty" metric:"memory_usage_mib,summary" since:"1"`
	SHA256Perf     float64 `json:"sha256Perf,omitempty" metric:"sha256_perf_mibps,summary" since:"1"`
	HashPerf       float64 `json:"hashPerf,omitempty" metric:"hash_perf_mibps,summary" since:"1"`
	MemorySize     int     `json:"memorySize,omitempty" metric:"memory_size_mib,summary" since:"1"`

	// v2 fields

	URVersion int `json:"urVersion,omitempty" metric:"reports_by_urversion_total,gaugeVec:version" since:"2"`
	NumCPU    int `json:"numCPU,omitempty" metric:"num_cpu,summary" since:"2"`

	FolderUses struct {
		SendOnly            int `json:"sendonly,omitempty" metric:"folder_feature{feature=ModeSendonly},summary" since:"2"`
		SendReceive         int `json:"sendreceive,omitempty" metric:"folder_feature{feature=ModeSendReceive},summary" since:"2"`
		ReceiveOnly         int `json:"receiveonly,omitempty" metric:"folder_feature{feature=ModeReceiveOnly},summary" since:"2"`
		IgnorePerms         int `json:"ignorePerms,omitempty" metric:"folder_feature{feature=IgnorePerms},summary" since:"2"`
		IgnoreDelete        int `json:"ignoreDelete,omitempty" metric:"folder_feature{feature=IgnoreDelete},summary" since:"2"`
		AutoNormalize       int `json:"autoNormalize,omitempty" metric:"folder_feature{feature=AutoNormalize},summary" since:"2"`
		SimpleVersioning    int `json:"simpleVersioning,omitempty" metric:"folder_feature{feature=VersioningSimple},summary" since:"2"`
		ExternalVersioning  int `json:"externalVersioning,omitempty" metric:"folder_feature{feature=VersioningExternal},summary" since:"2"`
		StaggeredVersioning int `json:"staggeredVersioning,omitempty" metric:"folder_feature{feature=VersioningStaggered},summary" since:"2"`
		TrashcanVersioning  int `json:"trashcanVersioning,omitempty" metric:"folder_feature{feature=VersioningTrashcan},summary" since:"2"`
	} `json:"folderUses,omitempty" since:"2"`

	DeviceUses struct {
		Introducer       int `json:"introducer,omitempty" metric:"device_feature{feature=Introducer},summary" since:"2"`
		CustomCertName   int `json:"customCertName,omitempty" metric:"device_feature{feature=CustomCertName},summary" since:"2"`
		CompressAlways   int `json:"compressAlways,omitempty" metric:"device_feature{feature=CompressAlways},summary" since:"2"`
		CompressMetadata int `json:"compressMetadata,omitempty" metric:"device_feature{feature=CompressMetadata},summary" since:"2"`
		CompressNever    int `json:"compressNever,omitempty" metric:"device_feature{feature=CompressNever},summary" since:"2"`
		DynamicAddr      int `json:"dynamicAddr,omitempty" metric:"device_feature{feature=AddressDynamic},summary" since:"2"`
		StaticAddr       int `json:"staticAddr,omitempty" metric:"device_feature{feature=AddressStatic},summary" since:"2"`
	} `json:"deviceUses,omitempty" since:"2"`

	Announce struct {
		GlobalEnabled     bool `json:"globalEnabled,omitempty" metric:"discovery_feature_count{feature=GlobalEnabled},gauge" since:"2"`
		LocalEnabled      bool `json:"localEnabled,omitempty" metric:"discovery_feature_count{feature=LocalEnabled},gauge" since:"2"`
		DefaultServersDNS int  `json:"defaultServersDNS,omitempty" metric:"discovery_default_servers,summary" since:"2"`
		OtherServers      int  `json:"otherServers,omitempty" metric:"discovery_other_servers,summary" since:"2"`
	} `json:"announce,omitempty" since:"2"`

	Relays struct {
		Enabled        bool `json:"enabled,omitempty" metric:"relay_feature_enabled,gauge" since:"2"`
		DefaultServers int  `json:"defaultServers,omitempty" metric:"relay_feature_count{feature=DefaultServers},summary" since:"2"`
		OtherServers   int  `json:"otherServers,omitempty" metric:"relay_feature_count{feature=OtherServers},summary" since:"2"`
	} `json:"relays,omitempty" since:"2"`

	UsesRateLimit        bool `json:"usesRateLimit,omitempty" metric:"feature_count{feature=RateLimitsEnabled},gauge" since:"2"`
	UpgradeAllowedManual bool `json:"upgradeAllowedManual,omitempty" metric:"feature_count{feature=UpgradeAllowedManual},gauge" since:"2"`
	UpgradeAllowedAuto   bool `json:"upgradeAllowedAuto,omitempty" metric:"feature_count{feature=UpgradeAllowedAuto},gauge" since:"2"`

	// V2.5 fields (fields that were in v2 but never added to the database
	UpgradeAllowedPre bool  `json:"upgradeAllowedPre,omitempty" metric:"upgrade_allowed_pre,gauge" since:"2"`
	RescanIntvs       []int `json:"rescanIntvs,omitempty" metric:"folder_rescan_intervals,summary" since:"2"`

	// v3 fields

	Uptime  int    `json:"uptime,omitempty" metric:"uptime_seconds,summary" since:"3"`
	NATType string `json:"natType,omitempty" metric:"nat_detection,gaugeVec:type" since:"3"`

	AlwaysLocalNets            bool `json:"alwaysLocalNets,omitempty" metric:"feature_count{feature=AlwaysLocalNets},gauge" since:"3"`
	CacheIgnoredFiles          bool `json:"cacheIgnoredFiles,omitempty" metric:"feature_count{feature=CacheIgnoredFiles},gauge" since:"3"`
	OverwriteRemoteDeviceNames bool `json:"overwriteRemoteDeviceNames,omitempty" metric:"feature_count{feature=OverwriteRemoteDeviceNames},gauge" since:"3"`
	ProgressEmitterEnabled     bool `json:"progressEmitterEnabled,omitempty" metric:"feature_count{feature=ProgressEmitterEnabled},gauge" since:"3"`
	CustomDefaultFolderPath    bool `json:"customDefaultFolderPath,omitempty" metric:"feature_count{feature=CustomDefaultFolderPath},gauge" since:"3"`
	CustomTrafficClass         bool `json:"customTrafficClass,omitempty" metric:"feature_count{feature=CustomTrafficClass},gauge" since:"3"`
	CustomTempIndexMinBlocks   bool `json:"customTempIndexMinBlocks,omitempty" metric:"feature_count{feature=CustomTempIndexMinBlocks},gauge" since:"3"`
	TemporariesDisabled        bool `json:"temporariesDisabled,omitempty" metric:"feature_count{feature=TemporariesDisabled},gauge" since:"3"`
	TemporariesCustom          bool `json:"temporariesCustom,omitempty" metric:"feature_count{feature=TemporariesCustom},gauge" since:"3"`
	LimitBandwidthInLan        bool `json:"limitBandwidthInLan,omitempty" metric:"feature_count{feature=LimitBandwidthInLAN},gauge" since:"3"`
	CustomReleaseURL           bool `json:"customReleaseURL,omitempty" metric:"feature_count{feature=CustomReleaseURL},gauge" since:"3"`
	RestartOnWakeup            bool `json:"restartOnWakeup,omitempty" metric:"feature_count{feature=RestartOnWakeup},gauge" since:"3"`
	CustomStunServers          bool `json:"customStunServers,omitempty" metric:"feature_count{feature=CustomSTUNServers},gauge" since:"3"`

	FolderUsesV3 struct {
		ScanProgressDisabled    int            `json:"scanProgressDisabled,omitempty" metric:"folder_feature{feature=ScanProgressDisabled},summary" since:"3"`
		ConflictsDisabled       int            `json:"conflictsDisabled,omitempty" metric:"folder_feature{feature=ConflictsDisabled},summary" since:"3"`
		ConflictsUnlimited      int            `json:"conflictsUnlimited,omitempty" metric:"folder_feature{feature=ConflictsUnlimited},summary" since:"3"`
		ConflictsOther          int            `json:"conflictsOther,omitempty" metric:"folder_feature{feature=ConflictsOther},summary" since:"3"`
		DisableSparseFiles      int            `json:"disableSparseFiles,omitempty" metric:"folder_feature{feature=DisableSparseFiles},summary" since:"3"`
		DisableTempIndexes      int            `json:"disableTempIndexes,omitempty" metric:"folder_feature{feature=DisableTempIndexes},summary" since:"3"`
		AlwaysWeakHash          int            `json:"alwaysWeakHash,omitempty" metric:"folder_feature{feature=AlwaysWeakhash},summary" since:"3"`
		CustomWeakHashThreshold int            `json:"customWeakHashThreshold,omitempty" metric:"folder_feature{feature=CustomWeakhashThreshold},summary" since:"3"`
		FsWatcherEnabled        int            `json:"fsWatcherEnabled,omitempty" metric:"folder_feature{feature=FSWatcherEnabled},summary" since:"3"`
		PullOrder               map[string]int `json:"pullOrder,omitempty" metric:"folder_pull_order,summaryVec:order" since:"3"`
		FilesystemType          map[string]int `json:"filesystemType,omitempty" metric:"folder_file_system_type,summaryVec:type" since:"3"`
		FsWatcherDelays         []int          `json:"fsWatcherDelays,omitempty" metric:"folder_fswatcher_delays,summary" since:"3"`
		CustomMarkerName        int            `json:"customMarkerName,omitempty" metric:"folder_feature{feature=CustomMarkername},summary" since:"3"`
		CopyOwnershipFromParent int            `json:"copyOwnershipFromParent,omitempty" metric:"folder_feature{feature=CopyParentOwnership},summary" since:"3"`
		ModTimeWindowS          []int          `json:"modTimeWindowS,omitempty" metric:"folder_modtime_window_s,summary" since:"3"`
		MaxConcurrentWrites     []int          `json:"maxConcurrentWrites,omitempty" metric:"folder_max_concurrent_writes,summary" since:"3"`
		DisableFsync            int            `json:"disableFsync,omitempty" metric:"folder_feature{feature=DisableFsync},summary" since:"3"`
		BlockPullOrder          map[string]int `json:"blockPullOrder,omitempty" metric:"folder_block_pull_order:summaryVec:order" since:"3"`
		CopyRangeMethod         map[string]int `json:"copyRangeMethod,omitempty" metric:"folder_copy_range_method:summaryVec:method" since:"3"`
		CaseSensitiveFS         int            `json:"caseSensitiveFS,omitempty" metric:"folder_feature{feature=CaseSensitiveFS},summary" since:"3"`
		ReceiveEncrypted        int            `json:"receiveencrypted,omitempty" metric:"folder_feature{feature=ReceiveEncrypted},summary" since:"3"`
		SendXattrs              int            `json:"sendXattrs,omitempty" metric:"folder_feature{feature=SendXattrs},summary" since:"3"`
		SyncXattrs              int            `json:"syncXattrs,omitempty" metric:"folder_feature{feature=SyncXattrs},summary" since:"3"`
		SendOwnership           int            `json:"sendOwnership,omitempty" metric:"folder_feature{feature=SendOwnership},summary" since:"3"`
		SyncOwnership           int            `json:"syncOwnership,omitempty" metric:"folder_feature{feature=SyncOwnership},summary" since:"3"`
	} `json:"folderUsesV3,omitempty" since:"3"`

	DeviceUsesV3 struct {
		Untrusted           int `json:"untrusted,omitempty" metric:"device_feature{feature=Untrusted},summary" since:"3"`
		UsesRateLimit       int `json:"usesRateLimit,omitempty" metric:"device_feature{feature=RateLimitsEnabled},summary" since:"3"`
		MultipleConnections int `json:"multipleConnections,omitempty" metric:"device_feature{feature=MultipleConnections},summary" since:"3"`
	} `json:"deviceUsesV3,omitempty" since:"3"`

	GUIStats struct {
		Enabled                   int            `json:"enabled,omitempty" metric:"gui_feature_count{feature=Enabled},summary" since:"3"`
		UseTLS                    int            `json:"useTLS,omitempty" metric:"gui_feature_count{feature=TLS},summary" since:"3"`
		UseAuth                   int            `json:"useAuth,omitempty" metric:"gui_feature_count{feature=Authentication},summary" since:"3"`
		InsecureAdminAccess       int            `json:"insecureAdminAccess,omitempty" metric:"gui_feature_count{feature=InsecureAdminAccess},summary" since:"3"`
		Debugging                 int            `json:"debugging,omitempty" metric:"gui_feature_count{feature=Debugging},summary" since:"3"`
		InsecureSkipHostCheck     int            `json:"insecureSkipHostCheck,omitempty" metric:"gui_feature_count{feature=InsecureSkipHostCheck},summary" since:"3"`
		InsecureAllowFrameLoading int            `json:"insecureAllowFrameLoading,omitempty" metric:"gui_feature_count{feature=InsecureAllowFrameLoading},summary" since:"3"`
		ListenLocal               int            `json:"listenLocal,omitempty" metric:"gui_feature_count{feature=ListenLocal},summary" since:"3"`
		ListenUnspecified         int            `json:"listenUnspecified,omitempty" metric:"gui_feature_count{feature=ListenUnspecified},summary" since:"3"`
		Theme                     map[string]int `json:"theme,omitempty" metric:"gui_theme,summaryVec:theme" since:"3"`
	} `json:"guiStats,omitempty" since:"3"`

	BlockStats struct {
		Total             int `json:"total,omitempty" metric:"blocks_processed_total,gauge" since:"3"`
		Renamed           int `json:"renamed,omitempty" metric:"blocks_processed{source=renamed},gauge" since:"3"`
		Reused            int `json:"reused,omitempty" metric:"blocks_processed{source=reused},gauge" since:"3"`
		Pulled            int `json:"pulled,omitempty" metric:"blocks_processed{source=pulled},gauge" since:"3"`
		CopyOrigin        int `json:"copyOrigin,omitempty" metric:"blocks_processed{source=copy_origin},gauge" since:"3"`
		CopyOriginShifted int `json:"copyOriginShifted,omitempty" metric:"blocks_processed{source=copy_origin_shifted},gauge" since:"3"`
		CopyElsewhere     int `json:"copyElsewhere,omitempty" metric:"blocks_processed{source=copy_elsewhere},gauge" since:"3"`
	} `json:"blockStats,omitempty" since:"3"`

	TransportStats map[string]int `json:"transportStats,omitempty" since:"3"`

	IgnoreStats struct {
		Lines           int `json:"lines,omitempty" metric:"folder_ignore_lines_total,summary" since:"3"`
		Inverts         int `json:"inverts,omitempty" metric:"folder_ignore_lines{kind=inverts},summary" since:"3"`
		Folded          int `json:"folded,omitempty" metric:"folder_ignore_lines{kind=folded},summary" since:"3"`
		Deletable       int `json:"deletable,omitempty" metric:"folder_ignore_lines{kind=deletable},summary" since:"3"`
		Rooted          int `json:"rooted,omitempty" metric:"folder_ignore_lines{kind=rooted},summary" since:"3"`
		Includes        int `json:"includes,omitempty" metric:"folder_ignore_lines{kind=includes},summary" since:"3"`
		EscapedIncludes int `json:"escapedIncludes,omitempty" metric:"folder_ignore_lines{kind=escapedIncludes},summary" since:"3"`
		DoubleStars     int `json:"doubleStars,omitempty" metric:"folder_ignore_lines{kind=doubleStars},summary" since:"3"`
		Stars           int `json:"stars,omitempty" metric:"folder_ignore_lines{kind=stars},summary" since:"3"`
	} `json:"ignoreStats,omitempty" since:"3"`

	// V3 fields added late in the RC
	WeakHashEnabled bool `json:"weakHashEnabled,omitempty" metric:"-" since:"3"` // Deprecated and not provided client-side anymore

	// Added in post processing
	Received     time.Time `json:"received,omitempty"`
	Date         string    `json:"date,omitempty"`
	Address      string    `json:"address,omitempty"`
	OS           string    `json:"os" metric:"reports_total,gaugeVec:os"`
	Arch         string    `json:"arch" metric:"reports_total,gaugeVec:arch"`
	Compiler     string    `json:"compiler" metric:"builder,gaugeVec:compiler"`
	Builder      string    `json:"builder" metric:"builder,gaugeVec:builder"`
	Distribution string    `json:"distribution" metric:"builder,gaugeVec:distribution"`
	Country      string    `json:"country" metric:"location,gaugeVec:country"`
	CountryCode  string    `json:"countryCode" metric:"location,gaugeVec:countryCode"`
	MajorVersion string    `json:"majorVersion" metric:"reports_by_major_total,gaugeVec:version"`
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
