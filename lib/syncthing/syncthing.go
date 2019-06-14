// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"io"
	"sync"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"

	"github.com/thejerf/suture"
)

const (
	bepProtocolName      = "bep/1.0"
	tlsDefaultCommonName = "syncthing"
	maxSystemErrors      = 5
	initialSystemLog     = 10
	maxSystemLog         = 250
)

type ExitStatus int

const (
	ExitSuccess ExitStatus = 0
	ExitError   ExitStatus = 1
	ExitRestart ExitStatus = 3
)

type Options struct {
	AssetDir         string
	AuditWriter      io.Writer
	DeadlockTimeoutS int
	NoUpgrade        bool
	ProfilerURL      string
	ResetDeltaIdxs   bool
	Verbose          bool
}

type App struct {
	myID        protocol.DeviceID
	mainService *suture.Supervisor
	ll          *db.Lowlevel
	opts        Options
	cfg         config.Wrapper
	exitStatus  ExitStatus
	startOnce   sync.Once
	stop        chan struct{}
	stopped     chan struct{}
}

func New(cfg config.Wrapper, opts Options) *App {
	return &App{
		opts:    opts,
		cfg:     cfg,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (a *App) Run() ExitStatus {
	a.Start()
	return a.Wait()
}

func (a *App) Start() {
	a.startOnce.Do(func() {
		if err := a.startup(); err != nil {
			close(a.stop)
			a.exitStatus = ExitError
			close(a.stopped)
			return
		}
		go a.run()
	})
}

func syncthingMain(runtimeOptions RuntimeOptions) {
	setupSignalHandling()

	// Create a main service manager. We'll add things to this as we go along.
	// We want any logging it does to go through our log system.
	mainService := suture.New("main", suture.Spec{
		Log: func(line string) {
			l.Debugln(line)
		},
		PassThroughPanics: true,
	})
	mainService.ServeBackground()

	// Set a log prefix similar to the ID we will have later on, or early log
	// lines look ugly.
	l.SetPrefix("[start] ")

	if runtimeOptions.auditEnabled {
		startAuditing(mainService, runtimeOptions.auditFile)
	}

	if runtimeOptions.verbose {
		mainService.Add(newVerboseService())
	}

	errors := logger.NewRecorder(l, logger.LevelWarn, maxSystemErrors, 0)
	systemLog := logger.NewRecorder(l, logger.LevelDebug, maxSystemLog, initialSystemLog)

	// Event subscription for the API; must start early to catch the early
	// events. The LocalChangeDetected event might overwhelm the event
	// receiver in some situations so we will not subscribe to it here.
	defaultSub := events.NewBufferedSubscription(events.Default.Subscribe(api.DefaultEventMask), api.EventSubBufferSize)
	diskSub := events.NewBufferedSubscription(events.Default.Subscribe(api.DiskEventMask), api.EventSubBufferSize)

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Attempt to increase the limit on number of open files to the maximum
	// allowed, in case we have many peers. We don't really care enough to
	// report the error if there is one.
	osutil.MaximizeOpenFileLimit()

	// Ensure that we have a certificate and key.
	cert, err := tls.LoadX509KeyPair(
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	)
	if err != nil {
		l.Infof("Generating ECDSA key and certificate for %s...", tlsDefaultCommonName)
		cert, err = tlsutil.NewCertificate(
			locations.Get(locations.CertFile),
			locations.Get(locations.KeyFile),
			tlsDefaultCommonName,
		)
		if err != nil {
			l.Infoln("Failed to generate certificate:", err)
			os.Exit(exitError)
		}
	}

	myID = protocol.NewDeviceID(cert.Certificate[0])
	l.SetPrefix(fmt.Sprintf("[%s] ", myID.String()[:5]))

	l.Infoln(build.LongVersion)
	l.Infoln("My ID:", myID)

	// Select SHA256 implementation and report. Affected by the
	// STHASHING environment variable.
	sha256.SelectAlgo()
	sha256.Report()

	// Emit the Starting event, now that we know who we are.

	events.Default.Log(events.Starting, map[string]string{
		"home": locations.GetBaseDir(locations.ConfigBaseDir),
		"myID": myID.String(),
	})

	cfg, err := loadConfigAtStartup(runtimeOptions.allowNewerConfig)
	if err != nil {
		l.Warnln("Failed to initialize config:", err)
		os.Exit(exitError)
	}

	if err := checkShortIDs(cfg); err != nil {
		l.Warnln("Short device IDs are in conflict. Unlucky!\n  Regenerate the device ID of one of the following:\n  ", err)
		os.Exit(exitError)
	}

	if len(runtimeOptions.profiler) > 0 {
		go func() {
			l.Debugln("Starting profiler on", runtimeOptions.profiler)
			runtime.SetBlockProfileRate(1)
			err := http.ListenAndServe(runtimeOptions.profiler, nil)
			if err != nil {
				l.Warnln(err)
				os.Exit(exitError)
			}
		}()
	}

	perf := ur.CpuBench(3, 150*time.Millisecond, true)
	l.Infof("Hashing performance is %.02f MB/s", perf)

	dbFile := locations.Get(locations.Database)
	ldb, err := db.Open(dbFile)
	if err != nil {
		l.Warnln("Error opening database:", err)
		os.Exit(exitError)
	}
	if err := db.UpdateSchema(ldb); err != nil {
		l.Warnln("Database schema:", err)
		os.Exit(exitError)
	}

	if runtimeOptions.resetDeltaIdxs {
		l.Infoln("Reinitializing delta index IDs")
		db.DropDeltaIndexIDs(ldb)
	}

	protectedFiles := []string{
		locations.Get(locations.Database),
		locations.Get(locations.ConfigFile),
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	}

	// Remove database entries for folders that no longer exist in the config
	folders := cfg.Folders()
	for _, folder := range ldb.ListFolders() {
		if _, ok := folders[folder]; !ok {
			l.Infof("Cleaning data for dropped folder %q", folder)
			db.DropFolder(ldb, folder)
		}
	}

	// Grab the previously running version string from the database.

	miscDB := db.NewMiscDataNamespace(ldb)
	prevVersion, _ := miscDB.String("prevVersion")

	// Strip away prerelease/beta stuff and just compare the release
	// numbers. 0.14.44 to 0.14.45-banana is an upgrade, 0.14.45-banana to
	// 0.14.45-pineapple is not.

	prevParts := strings.Split(prevVersion, "-")
	curParts := strings.Split(build.Version, "-")
	if prevParts[0] != curParts[0] {
		if prevVersion != "" {
			l.Infoln("Detected upgrade from", prevVersion, "to", build.Version)
		}

		// Drop delta indexes in case we've changed random stuff we
		// shouldn't have. We will resend our index on next connect.
		db.DropDeltaIndexIDs(ldb)

		// Remember the new version.
		miscDB.PutString("prevVersion", build.Version)
	}

	m := model.NewModel(cfg, myID, "syncthing", build.Version, ldb, protectedFiles)

	if t := os.Getenv("STDEADLOCKTIMEOUT"); t != "" {
		if secs, _ := strconv.Atoi(t); secs > 0 {
			m.StartDeadlockDetector(time.Duration(secs) * time.Second)
		}
	} else if !build.IsRelease || build.IsBeta {
		m.StartDeadlockDetector(20 * time.Minute)
	}

	if runtimeOptions.unpaused {
		setPauseState(cfg, false)
	} else if runtimeOptions.paused {
		setPauseState(cfg, true)
	}

	// Add and start folders
	for _, folderCfg := range cfg.Folders() {
		if folderCfg.Paused {
			folderCfg.CreateRoot()
			continue
		}
		m.AddFolder(folderCfg)
		m.StartFolder(folderCfg.ID)
	}

	mainService.Add(m)

	// Start discovery

	cachedDiscovery := discover.NewCachingMux()
	mainService.Add(cachedDiscovery)

	// The TLS configuration is used for both the listening socket and outgoing
	// connections.

	tlsCfg := tlsutil.SecureDefault()
	tlsCfg.Certificates = []tls.Certificate{cert}
	tlsCfg.NextProtos = []string{bepProtocolName}
	tlsCfg.ClientAuth = tls.RequestClientCert
	tlsCfg.SessionTicketsDisabled = true
	tlsCfg.InsecureSkipVerify = true

	// Start connection management

	connectionsService := connections.NewService(cfg, myID, m, tlsCfg, cachedDiscovery, bepProtocolName, tlsDefaultCommonName)
	mainService.Add(connectionsService)

	if cfg.Options().GlobalAnnEnabled {
		for _, srv := range cfg.GlobalDiscoveryServers() {
			l.Infoln("Using discovery server", srv)
			gd, err := discover.NewGlobal(srv, cert, connectionsService)
			if err != nil {
				l.Warnln("Global discovery:", err)
				continue
			}

			// Each global discovery server gets its results cached for five
			// minutes, and is not asked again for a minute when it's returned
			// unsuccessfully.
			cachedDiscovery.Add(gd, 5*time.Minute, time.Minute)
		}
	}

	if cfg.Options().LocalAnnEnabled {
		// v4 broadcasts
		bcd, err := discover.NewLocal(myID, fmt.Sprintf(":%d", cfg.Options().LocalAnnPort), connectionsService)
		if err != nil {
			l.Warnln("IPv4 local discovery:", err)
		} else {
			cachedDiscovery.Add(bcd, 0, 0)
		}
		// v6 multicasts
		mcd, err := discover.NewLocal(myID, cfg.Options().LocalAnnMCAddr, connectionsService)
		if err != nil {
			l.Warnln("IPv6 local discovery:", err)
		} else {
			cachedDiscovery.Add(mcd, 0, 0)
		}
	}

	if runtimeOptions.cpuProfile {
		f, err := os.Create(fmt.Sprintf("cpu-%d.pprof", os.Getpid()))
		if err != nil {
			l.Warnln("Creating profile:", err)
			os.Exit(exitError)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			l.Warnln("Starting profile:", err)
			os.Exit(exitError)
		}
	}

	// Candidate builds always run with usage reporting.

	if opts := cfg.Options(); build.IsCandidate {
		l.Infoln("Anonymous usage reporting is always enabled for candidate releases.")
		if opts.URAccepted != ur.Version {
			opts.URAccepted = ur.Version
			cfg.SetOptions(opts)
			cfg.Save()
			// Unique ID will be set and config saved below if necessary.
		}
	}

	// If we are going to do usage reporting, ensure we have a valid unique ID.
	if opts := cfg.Options(); opts.URAccepted > 0 && opts.URUniqueID == "" {
		opts.URUniqueID = rand.String(8)
		cfg.SetOptions(opts)
		cfg.Save()
	}

	usageReportingSvc := ur.New(cfg, m, connectionsService, noUpgradeFromEnv)
	mainService.Add(usageReportingSvc)

	// GUI

	setupGUI(mainService, cfg, m, defaultSub, diskSub, cachedDiscovery, connectionsService, usageReportingSvc, errors, systemLog, runtimeOptions)

	myDev, _ := cfg.Device(myID)
	l.Infof(`My name is "%v"`, myDev.Name)
	for _, device := range cfg.Devices() {
		if device.DeviceID != myID {
			l.Infof(`Device %s is "%v" at %v`, device.DeviceID, device.Name, device.Addresses)
		}
	}

	if opts := cfg.Options(); opts.RestartOnWakeup {
		go standbyMonitor()
	}

	// Candidate builds should auto upgrade. Make sure the option is set,
	// unless we are in a build where it's disabled or the STNOUPGRADE
	// environment variable is set.

	if build.IsCandidate && !upgrade.DisabledByCompilation && !noUpgradeFromEnv {
		l.Infoln("Automatic upgrade is always enabled for candidate releases.")
		if opts := cfg.Options(); opts.AutoUpgradeIntervalH == 0 || opts.AutoUpgradeIntervalH > 24 {
			opts.AutoUpgradeIntervalH = 12
			// Set the option into the config as well, as the auto upgrade
			// loop expects to read a valid interval from there.
			cfg.SetOptions(opts)
			cfg.Save()
		}
		// We don't tweak the user's choice of upgrading to pre-releases or
		// not, as otherwise they cannot step off the candidate channel.
	}

	if opts := cfg.Options(); opts.AutoUpgradeIntervalH > 0 {
		if noUpgradeFromEnv {
			l.Infof("No automatic upgrades; STNOUPGRADE environment variable defined.")
		} else {
			go autoUpgrade(cfg)
		}
	}

	if isSuperUser() {
		l.Warnln("Syncthing should not run as a privileged or system user. Please consider using a normal user account.")
	}

	events.Default.Log(events.StartupComplete, map[string]string{
		"myID": myID.String(),
	})

	cleanConfigDirectory()

	if cfg.Options().SetLowPriority {
		if err := osutil.SetLowPriority(); err != nil {
			l.Warnln("Failed to lower process priority:", err)
		}
	}

	code := exit.waitForExit()

	mainService.Stop()

	done := make(chan struct{})
	go func() {
		ldb.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		l.Warnln("Database failed to stop within 10s")
	}

	l.Infoln("Exiting")

	if runtimeOptions.cpuProfile {
		pprof.StopCPUProfile()
	}

	os.Exit(code)
}

func (a *App) Wait() ExitStatus {
	<-a.stopped
	return a.exitStatus
}

func (a *App) Stop(stopReason ExitStatus) ExitStatus {
	select {
	case <-a.stopped:
	case <-a.stop:
	default:
		close(a.stop)
	}
	<-a.stopped
	// If there was already an exit status set internally, ignore the reason
	// given to Stop.
	if a.exitStatus == ExitSuccess {
		a.exitStatus = stopReason
	}
	return a.exitStatus
}
