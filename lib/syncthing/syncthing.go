// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/api"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/ur"
)

const (
	bepProtocolName        = "bep/1.0"
	tlsDefaultCommonName   = "syncthing"
	maxSystemErrors        = 5
	initialSystemLog       = 10
	maxSystemLog           = 250
	deviceCertLifetimeDays = 20 * 365
)

type Options struct {
	AuditWriter           io.Writer
	NoUpgrade             bool
	ProfilerAddr          string
	ResetDeltaIdxs        bool
	DBMaintenanceInterval time.Duration
}

type App struct {
	myID              protocol.DeviceID
	mainService       *suture.Supervisor
	cfg               config.Wrapper
	sdb               db.DB
	evLogger          events.Logger
	cert              tls.Certificate
	opts              Options
	exitStatus        svcutil.ExitStatus
	err               error
	stopOnce          sync.Once
	mainServiceCancel context.CancelFunc
	stopped           chan struct{}

	// Access to internals for direct users of this package. Note that the interface in Internals is unstable!
	Internals *Internals
}

func New(cfg config.Wrapper, sdb db.DB, evLogger events.Logger, cert tls.Certificate, opts Options) (*App, error) {
	a := &App{
		cfg:      cfg,
		sdb:      sdb,
		evLogger: evLogger,
		opts:     opts,
		cert:     cert,
		stopped:  make(chan struct{}),
	}
	close(a.stopped) // Hasn't been started, so shouldn't block on Wait.
	return a, nil
}

// Start executes the app and returns once all the startup operations are done,
// e.g. the API is ready for use.
// Must be called once only.
func (a *App) Start() error {
	// Create a main service manager. We'll add things to this as we go along.
	// We want any logging it does to go through our log system.
	spec := svcutil.SpecWithDebugLogger()
	a.mainService = suture.New("main", spec)

	// Start the supervisor and wait for it to stop to handle cleanup.
	a.stopped = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	a.mainServiceCancel = cancel
	errChan := a.mainService.ServeBackground(ctx)
	go a.wait(errChan)

	if err := a.startup(); err != nil {
		a.stopWithErr(svcutil.ExitError, err)
		return err
	}

	return nil
}

func (a *App) startup() error {
	a.mainService.Add(ur.NewFailureHandler(a.cfg, a.evLogger))

	a.mainService.Add(a.sdb.Service(a.opts.DBMaintenanceInterval))

	if a.opts.AuditWriter != nil {
		a.mainService.Add(newAuditService(a.opts.AuditWriter, a.evLogger))
	}

	// Event subscription for the API; must start early to catch the early
	// events. The LocalChangeDetected event might overwhelm the event
	// receiver in some situations so we will not subscribe to it here.
	defaultSub := events.NewBufferedSubscription(a.evLogger.Subscribe(api.DefaultEventMask), api.EventSubBufferSize)
	diskSub := events.NewBufferedSubscription(a.evLogger.Subscribe(api.DiskEventMask), api.EventSubBufferSize)

	// Attempt to increase the limit on number of open files to the maximum
	// allowed, in case we have many peers. We don't really care enough to
	// report the error if there is one.
	osutil.MaximizeOpenFileLimit()

	// Figure out our device ID and log it.
	a.myID = protocol.NewDeviceID(a.cert.Certificate[0])
	slog.Info("Calculated our device ID", slog.String("device", a.myID.String()))

	// Emit the Starting event, now that we know who we are.

	a.evLogger.Log(events.Starting, map[string]string{
		"home": locations.GetBaseDir(locations.ConfigBaseDir),
		"myID": a.myID.String(),
	})

	if err := checkShortIDs(a.cfg); err != nil {
		slog.Error("Short device IDs are in conflict; regenerate the device ID of one of the conflicting devices", slogutil.Error(err))
		return err
	}

	if len(a.opts.ProfilerAddr) > 0 {
		go func() {
			l.Debugln("Starting profiler on", a.opts.ProfilerAddr)
			runtime.SetBlockProfileRate(1)
			err := http.ListenAndServe(a.opts.ProfilerAddr, nil)
			if err != nil {
				slog.Warn("Failed to listen and serve for profiles", slogutil.Error(err))
				return
			}
		}()
	}

	if slog.Default().Enabled(context.Background(), slog.LevelInfo) {
		go func() {
			perf := ur.CpuBench(context.Background(), 3, 150*time.Millisecond)
			slog.Info("Measured hashing performance", "perf", fmt.Sprintf("%.02f MB/s", perf))
		}()
	}

	if a.opts.ResetDeltaIdxs {
		slog.Info("Reinitializing delta index IDs")
		if err := a.sdb.DropAllIndexIDs(); err != nil {
			slog.Error("Failed to drop index IDs", slogutil.Error(err))
			return err
		}
	}

	protectedFiles := []string{
		locations.Get(locations.Database),
		locations.Get(locations.ConfigFile),
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	}

	// Remove database entries for folders that no longer exist in the config
	cfgFolders := a.cfg.Folders()
	dbFolders, err := a.sdb.ListFolders()
	if err != nil {
		slog.Warn("Failed to list folders", slogutil.Error(err))
		return err
	}
	for _, folder := range dbFolders {
		if _, ok := cfgFolders[folder]; !ok {
			slog.Info("Cleaning metadata for dropped folder", "folder", folder)
			a.sdb.DropFolder(folder)
		}
	}

	// Grab the previously running version string from the database.

	miscDB := db.NewMiscDB(a.sdb)
	prevVersion, _, err := miscDB.String("prevVersion")
	if err != nil {
		slog.Error("Database error when getting previous version", slogutil.Error(err))
		return err
	}

	// Strip away prerelease/beta stuff and just compare the release
	// numbers. 0.14.44 to 0.14.45-banana is an upgrade, 0.14.45-banana to
	// 0.14.45-pineapple is not.

	prevParts := strings.Split(prevVersion, "-")
	curParts := strings.Split(build.Version, "-")
	if rel := upgrade.CompareVersions(prevParts[0], curParts[0]); rel != upgrade.Equal {
		if prevVersion != "" {
			slog.Info("Detected upgrade", "from", prevVersion, "to", build.Version)
		}

		if a.cfg.Options().SendFullIndexOnUpgrade {
			// Drop delta indexes in case we've changed random stuff we
			// shouldn't have. We will resend our index on next connect.
			if err := a.sdb.DropAllIndexIDs(); err != nil {
				slog.Warn("Failed to drop index IDs", slogutil.Error(err))
				return err
			}
		}
	}

	if build.Version != prevVersion {
		// Remember the new version.
		miscDB.PutString("prevVersion", build.Version)
	}

	if err := globalMigration(a.sdb, a.cfg); err != nil {
		slog.Warn("Failed to perform global migration", slogutil.Error(err))
		return err
	}

	keyGen := protocol.NewKeyGenerator()
	m := model.NewModel(a.cfg, a.myID, a.sdb, protectedFiles, a.evLogger, keyGen)
	a.Internals = newInternals(m)

	a.mainService.Add(m)

	// The TLS configuration is used for both the listening socket and outgoing
	// connections.

	tlsCfg := tlsutil.SecureDefaultTLS13()
	tlsCfg.Certificates = []tls.Certificate{a.cert}
	tlsCfg.NextProtos = []string{bepProtocolName}
	tlsCfg.ClientAuth = tls.RequestClientCert
	tlsCfg.SessionTicketsDisabled = true
	tlsCfg.InsecureSkipVerify = true

	// Start discovery and connection management

	// Chicken and egg, discovery manager depends on connection service to tell it what addresses it's listening on
	// Connection service depends on discovery manager to get addresses to connect to.
	// Create a wrapper that is then wired after they are both set up.
	addrLister := &lateAddressLister{}

	connRegistry := registry.New()
	discoveryManager := discover.NewManager(a.myID, a.cfg, a.cert, a.evLogger, addrLister, connRegistry)
	connectionsService := connections.NewService(a.cfg, a.myID, m, tlsCfg, discoveryManager, bepProtocolName, tlsDefaultCommonName, a.evLogger, connRegistry, keyGen)

	addrLister.AddressLister = connectionsService

	a.mainService.Add(discoveryManager)
	a.mainService.Add(connectionsService)

	a.cfg.Modify(func(cfg *config.Configuration) {
		// Candidate builds always run with usage reporting.
		if build.IsCandidate {
			slog.Info("Anonymous usage reporting is always enabled for candidate releases")
			if cfg.Options.URAccepted != ur.Version {
				cfg.Options.URAccepted = ur.Version
				// Unique ID will be set and config saved below if necessary.
			}
		}
	})

	usageReportingSvc := ur.New(a.cfg, m, connectionsService, a.opts.NoUpgrade)
	a.mainService.Add(usageReportingSvc)

	// GUI

	if err := a.setupGUI(m, defaultSub, diskSub, discoveryManager, connectionsService, usageReportingSvc, slogutil.ErrorRecorder, slogutil.GlobalRecorder, miscDB); err != nil {
		slog.Error("Failed to start API", slogutil.Error(err))
		return err
	}

	myDev, _ := a.cfg.Device(a.myID)
	slog.Info("Loaded configuration", "name", myDev.Name)
	for _, device := range a.cfg.Devices() {
		if device.DeviceID != a.myID {
			slog.Info("Loaded peer device configuration", device.DeviceID.LogAttr(), slog.String("name", device.Name), slogutil.Address(device.Addresses))
		}
	}

	if isSuperUser() {
		slog.Warn("Syncthing should not run as a privileged or system user; please consider using a normal user account")
	}

	a.evLogger.Log(events.StartupComplete, map[string]string{
		"myID": a.myID.String(),
	})

	if a.cfg.Options().SetLowPriority {
		if err := osutil.SetLowPriority(); err != nil {
			slog.Warn("Failed to lower process priority", slogutil.Error(err))
		}
	}

	return nil
}

func (a *App) wait(errChan <-chan error) {
	err := <-errChan
	a.handleMainServiceError(err)

	done := make(chan struct{})
	go func() {
		a.sdb.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		slog.Warn("Database failed to stop within 10s")
	}

	slog.Info("Exiting")

	close(a.stopped)
}

func (a *App) handleMainServiceError(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	var fatalErr *svcutil.FatalErr
	if errors.As(err, &fatalErr) {
		a.exitStatus = fatalErr.Status
		a.err = fatalErr.Err
		return
	}
	a.err = err
	a.exitStatus = svcutil.ExitError
}

// Wait blocks until the app stops running. Also returns if the app hasn't been
// started yet.
func (a *App) Wait() svcutil.ExitStatus {
	<-a.stopped
	return a.exitStatus
}

// Error returns an error if one occurred while running the app. It does not wait
// for the app to stop before returning.
func (a *App) Error() error {
	select {
	case <-a.stopped:
		return a.err
	default:
	}
	return nil
}

// Stop stops the app and sets its exit status to given reason, unless the app
// was already stopped before. In any case it returns the effective exit status.
func (a *App) Stop(stopReason svcutil.ExitStatus) svcutil.ExitStatus {
	return a.stopWithErr(stopReason, nil)
}

func (a *App) stopWithErr(stopReason svcutil.ExitStatus, err error) svcutil.ExitStatus {
	a.stopOnce.Do(func() {
		a.exitStatus = stopReason
		a.err = err
		if shouldDebug() {
			slog.Debug("Services before stop:")
			printServiceTree(os.Stdout, a.mainService, 0)
		}
		a.mainServiceCancel()
	})
	<-a.stopped
	return a.exitStatus
}

func (a *App) setupGUI(m model.Model, defaultSub, diskSub events.BufferedSubscription, discoverer discover.Manager, connectionsService connections.Service, urService *ur.Service, errors, systemLog slogutil.Recorder, miscDB *db.Typed) error {
	guiCfg := a.cfg.GUI()

	if !guiCfg.Enabled {
		return nil
	}

	if guiCfg.InsecureAdminAccess {
		slog.Warn("Insecure admin access is enabled")
	}

	summaryService := model.NewFolderSummaryService(a.cfg, m, a.myID, a.evLogger)
	a.mainService.Add(summaryService)

	apiSvc := api.New(a.myID, a.cfg, locations.Get(locations.GUIAssets), tlsDefaultCommonName, m, defaultSub, diskSub, a.evLogger, discoverer, connectionsService, urService, summaryService, errors, systemLog, a.opts.NoUpgrade, miscDB)
	a.mainService.Add(apiSvc)

	if err := apiSvc.WaitForStart(); err != nil {
		return err
	}
	return nil
}

// checkShortIDs verifies that the configuration won't result in duplicate
// short ID:s; that is, that the devices in the cluster all have unique
// initial 64 bits.
func checkShortIDs(cfg config.Wrapper) error {
	exists := make(map[protocol.ShortID]protocol.DeviceID)
	for deviceID := range cfg.Devices() {
		shortID := deviceID.Short()
		if otherID, ok := exists[shortID]; ok {
			return fmt.Errorf("%v in conflict with %v", deviceID, otherID)
		}
		exists[shortID] = deviceID
	}
	return nil
}

type supervisor interface{ Services() []suture.Service }

func printServiceTree(w io.Writer, sup supervisor, level int) {
	printService(w, sup, level)

	svcs := sup.Services()
	slices.SortFunc(svcs, func(a, b suture.Service) int {
		return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
	})

	for _, svc := range svcs {
		if sub, ok := svc.(supervisor); ok {
			printServiceTree(w, sub, level+1)
		} else {
			printService(w, svc, level+1)
		}
	}
}

func printService(w io.Writer, svc interface{}, level int) {
	type errorer interface{ Error() error }

	t := "-"
	if _, ok := svc.(supervisor); ok {
		t = "+"
	}
	fmt.Fprintln(w, strings.Repeat("  ", level), t, svc)
	if es, ok := svc.(errorer); ok {
		if err := es.Error(); err != nil {
			fmt.Fprintln(w, strings.Repeat("  ", level), "  ->", err)
		}
	}
}

type lateAddressLister struct {
	discover.AddressLister
}
