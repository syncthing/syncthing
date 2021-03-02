// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ur

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"math/rand"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

// Current version number of the usage report, for acceptance purposes. If
// fields are added or changed this integer must be incremented so that users
// are prompted for acceptance of the new report.
const Version = 3

var StartTime = time.Now()

type Service struct {
	cfg                config.Wrapper
	model              model.Model
	connectionsService connections.Service
	noUpgrade          bool
	forceRun           chan struct{}
}

func New(cfg config.Wrapper, m model.Model, connectionsService connections.Service, noUpgrade bool) *Service {
	return &Service{
		cfg:                cfg,
		model:              m,
		connectionsService: connectionsService,
		noUpgrade:          noUpgrade,
		forceRun:           make(chan struct{}, 1), // Buffered to prevent locking
	}
}

// ReportData returns the data to be sent in a usage report with the currently
// configured usage reporting version.
func (s *Service) ReportData(ctx context.Context) (*contract.Report, error) {
	urVersion := s.cfg.Options().URAccepted
	return s.reportData(ctx, urVersion, false)
}

// ReportDataPreview returns a preview of the data to be sent in a usage report
// with the given version.
func (s *Service) ReportDataPreview(ctx context.Context, urVersion int) (*contract.Report, error) {
	return s.reportData(ctx, urVersion, true)
}

func (s *Service) reportData(ctx context.Context, urVersion int, preview bool) (*contract.Report, error) {
	opts := s.cfg.Options()
	defaultFolder := s.cfg.DefaultFolder()

	var totFiles, maxFiles int
	var totBytes, maxBytes int64
	for folderID := range s.cfg.Folders() {
		snap, err := s.model.DBSnapshot(folderID)
		if err != nil {
			continue
		}
		global := snap.GlobalSize()
		snap.Release()
		totFiles += int(global.Files)
		totBytes += global.Bytes
		if int(global.Files) > maxFiles {
			maxFiles = int(global.Files)
		}
		if global.Bytes > maxBytes {
			maxBytes = global.Bytes
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	report := contract.New()

	report.URVersion = urVersion
	report.UniqueID = opts.URUniqueID
	report.Version = build.Version
	report.LongVersion = build.LongVersion
	report.Platform = runtime.GOOS + "-" + runtime.GOARCH
	report.NumFolders = len(s.cfg.Folders())
	report.NumDevices = len(s.cfg.Devices())
	report.TotFiles = totFiles
	report.FolderMaxFiles = maxFiles
	report.TotMiB = int(totBytes / 1024 / 1024)
	report.FolderMaxMiB = int(maxBytes / 1024 / 1024)
	report.MemoryUsageMiB = int((mem.Sys - mem.HeapReleased) / 1024 / 1024)
	report.SHA256Perf = CpuBench(ctx, 5, 125*time.Millisecond, false)
	report.HashPerf = CpuBench(ctx, 5, 125*time.Millisecond, true)
	report.MemorySize = int(memorySize() / 1024 / 1024)
	report.NumCPU = runtime.NumCPU()

	for _, cfg := range s.cfg.Folders() {
		report.RescanIntvs = append(report.RescanIntvs, cfg.RescanIntervalS)

		switch cfg.Type {
		case config.FolderTypeSendOnly:
			report.FolderUses.SendOnly++
		case config.FolderTypeSendReceive:
			report.FolderUses.SendReceive++
		case config.FolderTypeReceiveOnly:
			report.FolderUses.ReceiveOnly++
		}
		if cfg.IgnorePerms {
			report.FolderUses.IgnorePerms++
		}
		if cfg.IgnoreDelete {
			report.FolderUses.IgnoreDelete++
		}
		if cfg.AutoNormalize {
			report.FolderUses.AutoNormalize++
		}
		switch cfg.Versioning.Type {
		case "":
			// None
		case "simple":
			report.FolderUses.SimpleVersioning++
		case "staggered":
			report.FolderUses.StaggeredVersioning++
		case "external":
			report.FolderUses.ExternalVersioning++
		case "trashcan":
			report.FolderUses.TrashcanVersioning++
		default:
			l.Warnf("Unhandled versioning type for usage reports: %s", cfg.Versioning.Type)
		}
	}
	sort.Ints(report.RescanIntvs)

	for _, cfg := range s.cfg.Devices() {
		if cfg.Introducer {
			report.DeviceUses.Introducer++
		}
		if cfg.CertName != "" && cfg.CertName != "syncthing" {
			report.DeviceUses.CustomCertName++
		}
		switch cfg.Compression {
		case protocol.CompressionAlways:
			report.DeviceUses.CompressAlways++
		case protocol.CompressionMetadata:
			report.DeviceUses.CompressMetadata++
		case protocol.CompressionNever:
			report.DeviceUses.CompressNever++
		default:
			l.Warnf("Unhandled versioning type for usage reports: %s", cfg.Compression)
		}

		for _, addr := range cfg.Addresses {
			if addr == "dynamic" {
				report.DeviceUses.DynamicAddr++
			} else {
				report.DeviceUses.StaticAddr++
			}
		}
	}

	report.Announce.GlobalEnabled = opts.GlobalAnnEnabled
	report.Announce.LocalEnabled = opts.LocalAnnEnabled
	for _, addr := range opts.RawGlobalAnnServers {
		if addr == "default" || addr == "default-v4" || addr == "default-v6" {
			report.Announce.DefaultServersDNS++
		} else {
			report.Announce.OtherServers++
		}
	}

	report.Relays.Enabled = opts.RelaysEnabled
	for _, addr := range s.cfg.Options().ListenAddresses() {
		switch {
		case addr == "dynamic+https://relays.syncthing.net/endpoint":
			report.Relays.DefaultServers++
		case strings.HasPrefix(addr, "relay://") || strings.HasPrefix(addr, "dynamic+http"):
			report.Relays.OtherServers++

		}
	}

	report.UsesRateLimit = opts.MaxRecvKbps > 0 || opts.MaxSendKbps > 0
	report.UpgradeAllowedManual = !(upgrade.DisabledByCompilation || s.noUpgrade)
	report.UpgradeAllowedAuto = !(upgrade.DisabledByCompilation || s.noUpgrade) && opts.AutoUpgradeEnabled()
	report.UpgradeAllowedPre = !(upgrade.DisabledByCompilation || s.noUpgrade) && opts.AutoUpgradeEnabled() && opts.UpgradeToPreReleases

	// V3

	if urVersion >= 3 {
		report.Uptime = s.UptimeS()
		report.NATType = s.connectionsService.NATType()
		report.AlwaysLocalNets = len(opts.AlwaysLocalNets) > 0
		report.CacheIgnoredFiles = opts.CacheIgnoredFiles
		report.OverwriteRemoteDeviceNames = opts.OverwriteRemoteDevNames
		report.ProgressEmitterEnabled = opts.ProgressUpdateIntervalS > -1
		report.CustomDefaultFolderPath = defaultFolder.Path != "~"
		report.CustomTrafficClass = opts.TrafficClass != 0
		report.CustomTempIndexMinBlocks = opts.TempIndexMinBlocks != 10
		report.TemporariesDisabled = opts.KeepTemporariesH == 0
		report.TemporariesCustom = opts.KeepTemporariesH != 24
		report.LimitBandwidthInLan = opts.LimitBandwidthInLan
		report.CustomReleaseURL = opts.ReleasesURL != "https=//upgrades.syncthing.net/meta.json"
		report.RestartOnWakeup = opts.RestartOnWakeup
		report.CustomStunServers = len(opts.RawStunServers) != 1 || opts.RawStunServers[0] != "default"

		for _, cfg := range s.cfg.Folders() {
			if cfg.ScanProgressIntervalS < 0 {
				report.FolderUsesV3.ScanProgressDisabled++
			}
			if cfg.MaxConflicts == 0 {
				report.FolderUsesV3.ConflictsDisabled++
			} else if cfg.MaxConflicts < 0 {
				report.FolderUsesV3.ConflictsUnlimited++
			} else {
				report.FolderUsesV3.ConflictsOther++
			}
			if cfg.DisableSparseFiles {
				report.FolderUsesV3.DisableSparseFiles++
			}
			if cfg.DisableTempIndexes {
				report.FolderUsesV3.DisableTempIndexes++
			}
			if cfg.WeakHashThresholdPct < 0 {
				report.FolderUsesV3.AlwaysWeakHash++
			} else if cfg.WeakHashThresholdPct != 25 {
				report.FolderUsesV3.CustomWeakHashThreshold++
			}
			if cfg.FSWatcherEnabled {
				report.FolderUsesV3.FsWatcherEnabled++
			}
			report.FolderUsesV3.PullOrder[cfg.Order.String()]++
			report.FolderUsesV3.FilesystemType[cfg.FilesystemType.String()]++
			report.FolderUsesV3.FsWatcherDelays = append(report.FolderUsesV3.FsWatcherDelays, cfg.FSWatcherDelayS)
			if cfg.MarkerName != config.DefaultMarkerName {
				report.FolderUsesV3.CustomMarkerName++
			}
			if cfg.CopyOwnershipFromParent {
				report.FolderUsesV3.CopyOwnershipFromParent++
			}
			report.FolderUsesV3.ModTimeWindowS = append(report.FolderUsesV3.ModTimeWindowS, int(cfg.ModTimeWindow().Seconds()))
			report.FolderUsesV3.MaxConcurrentWrites = append(report.FolderUsesV3.MaxConcurrentWrites, cfg.MaxConcurrentWrites)
			if cfg.DisableFsync {
				report.FolderUsesV3.DisableFsync++
			}
			report.FolderUsesV3.BlockPullOrder[cfg.BlockPullOrder.String()]++
			report.FolderUsesV3.CopyRangeMethod[cfg.CopyRangeMethod.String()]++
			if cfg.CaseSensitiveFS {
				report.FolderUsesV3.CaseSensitiveFS++
			}
		}
		sort.Ints(report.FolderUsesV3.FsWatcherDelays)

		for _, cfg := range s.cfg.Devices() {
			if cfg.Untrusted {
				report.DeviceUsesV3.Untrusted++
			}
		}

		guiCfg := s.cfg.GUI()
		// Anticipate multiple GUI configs in the future, hence store counts.
		if guiCfg.Enabled {
			report.GUIStats.Enabled++
			if guiCfg.UseTLS() {
				report.GUIStats.UseTLS++
			}
			if len(guiCfg.User) > 0 && len(guiCfg.Password) > 0 {
				report.GUIStats.UseAuth++
			}
			if guiCfg.InsecureAdminAccess {
				report.GUIStats.InsecureAdminAccess++
			}
			if guiCfg.Debugging {
				report.GUIStats.Debugging++
			}
			if guiCfg.InsecureSkipHostCheck {
				report.GUIStats.InsecureSkipHostCheck++
			}
			if guiCfg.InsecureAllowFrameLoading {
				report.GUIStats.InsecureAllowFrameLoading++
			}

			addr, err := net.ResolveTCPAddr("tcp", guiCfg.Address())
			if err == nil {
				if addr.IP.IsLoopback() {
					report.GUIStats.ListenLocal++

				} else if addr.IP.IsUnspecified() {
					report.GUIStats.ListenUnspecified++
				}
			}
			report.GUIStats.Theme[guiCfg.Theme]++
		}
	}

	s.model.UsageReportingStats(report, urVersion, preview)

	if err := report.ClearForVersion(urVersion); err != nil {
		return nil, err
	}

	return report, nil
}

func (s *Service) UptimeS() int {
	return int(time.Since(StartTime).Seconds())
}

func (s *Service) sendUsageReport(ctx context.Context) error {
	d, err := s.ReportData(ctx)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(d); err != nil {
		return err
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			Proxy:       http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: s.cfg.Options().URPostInsecurely,
			},
		},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.Options().URURL, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *Service) Serve(ctx context.Context) error {
	s.cfg.Subscribe(s)
	defer s.cfg.Unsubscribe(s)

	t := time.NewTimer(time.Duration(s.cfg.Options().URInitialDelayS) * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.forceRun:
			t.Reset(0)
		case <-t.C:
			if s.cfg.Options().URAccepted >= 2 {
				err := s.sendUsageReport(ctx)
				if err != nil {
					l.Infoln("Usage report:", err)
				} else {
					l.Infof("Sent usage report (version %d)", s.cfg.Options().URAccepted)
				}
			}
			t.Reset(24 * time.Hour) // next report tomorrow
		}
	}
}

func (s *Service) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (s *Service) CommitConfiguration(from, to config.Configuration) bool {
	if from.Options.URAccepted != to.Options.URAccepted || from.Options.URUniqueID != to.Options.URUniqueID || from.Options.URURL != to.Options.URURL {
		select {
		case s.forceRun <- struct{}{}:
		default:
			// s.forceRun is one buffered, so even though nothing
			// was sent, a run will still happen after this point.
		}
	}
	return true
}

func (*Service) String() string {
	return "ur.Service"
}

var (
	blocksResult    []protocol.BlockInfo // so the result is not optimized away
	blocksResultMut sync.Mutex
)

// CpuBench returns CPU performance as a measure of single threaded SHA-256 MiB/s
func CpuBench(ctx context.Context, iterations int, duration time.Duration, useWeakHash bool) float64 {
	blocksResultMut.Lock()
	defer blocksResultMut.Unlock()

	dataSize := 16 * protocol.MinBlockSize
	bs := make([]byte, dataSize)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Read(bs)

	var perf float64
	for i := 0; i < iterations; i++ {
		if v := cpuBenchOnce(ctx, duration, useWeakHash, bs); v > perf {
			perf = v
		}
	}
	blocksResult = nil
	return perf
}

func cpuBenchOnce(ctx context.Context, duration time.Duration, useWeakHash bool, bs []byte) float64 {
	t0 := time.Now()
	b := 0
	var err error
	for time.Since(t0) < duration {
		r := bytes.NewReader(bs)
		blocksResult, err = scanner.Blocks(ctx, r, protocol.MinBlockSize, int64(len(bs)), nil, useWeakHash)
		if err != nil {
			return 0 // Context done
		}
		b += len(bs)
	}
	d := time.Since(t0)
	return float64(int(float64(b)/d.Seconds()/(1<<20)*100)) / 100
}
