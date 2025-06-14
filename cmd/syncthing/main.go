// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof" // Need to import this to support STPROFILER.
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"slices"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gofrs/flock"
	"github.com/thejerf/suture/v4"
	"github.com/willabides/kongplete"

	"github.com/syncthing/syncthing/cmd/syncthing/cli"
	"github.com/syncthing/syncthing/cmd/syncthing/decrypt"
	"github.com/syncthing/syncthing/cmd/syncthing/generate"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/sqlite"
	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/syncthing"
	"github.com/syncthing/syncthing/lib/upgrade"
)

const (
	sigTerm = syscall.Signal(15)
)

const (
	extraUsage = `
The --logflags value is a sum of the following:

   1  Date
   2  Time
   4  Microsecond time
   8  Long filename
  16  Short filename

I.e. to prefix each log line with time and filename, set --logflags=18 (2 + 16
from above). The value 0 is used to disable all of the above. The default is
to show date and time (3).

Logging always happens to the command line (stdout) and optionally to the
file at the path specified by --logfile=path. In addition to an path, the special
values "default" and "-" may be used. The former logs to DATADIR/syncthing.log
(see --data), which is the default on Windows, and the latter only to stdout,
no file, which is the default anywhere else.


Development Settings
--------------------

The following environment variables modify Syncthing's behavior in ways that
are mostly useful for developers. Use with care. See also the --debug-* options
above.

 STTRACE           A comma separated string of facilities to trace. The valid
                   facility strings are listed below.

 STLOCKTHRESHOLD   Used for debugging internal deadlocks; sets debug
                   sensitivity.  Use only under direction of a developer.

 STVERSIONEXTRA    Add extra information to the version string in logs and the
                   version line in the GUI. Can be set to the name of a wrapper
                   or tool controlling syncthing to communicate this to the end
                   user.

 GOMAXPROCS        Set the maximum number of CPU cores to use. Defaults to all
                   available CPU cores.

 GOGC              Percentage of heap growth at which to trigger GC. Default is
                   100. Lower numbers keep peak memory usage down, at the price
                   of CPU usage (i.e. performance).


Debugging Facilities
--------------------

The following are valid values for the STTRACE variable:

%s
`
)

var (
	upgradeCheckInterval = 5 * time.Minute
	upgradeRetryInterval = time.Hour
	upgradeCheckKey      = "lastUpgradeCheck"
	upgradeTimeKey       = "lastUpgradeTime"
	upgradeVersionKey    = "lastUpgradeVersion"

	errTooEarlyUpgradeCheck = fmt.Errorf("last upgrade check happened less than %v ago, skipping", upgradeCheckInterval)
	errTooEarlyUpgrade      = fmt.Errorf("last upgrade happened less than %v ago, skipping", upgradeRetryInterval)
)

// The entrypoint struct is the main entry point for the command line parser. The
// commands and options here are top level commands to syncthing.
// Cli is just a placeholder for the help text (see main).
type CLI struct {
	// The directory options are defined at top level and available for all
	// subcommands. Their settings take effect on the `locations` package by
	// way of the command line parser, so anything using `locations.Get` etc
	// will be doing the right thing.
	ConfDir string `name:"config" short:"C" placeholder:"PATH" env:"STCONFDIR" help:"Set configuration directory (config and keys)"`
	DataDir string `name:"data" short:"D" placeholder:"PATH" env:"STDATADIR" help:"Set data directory (database and logs)"`
	HomeDir string `name:"home" short:"H" placeholder:"PATH" env:"STHOMEDIR" help:"Set configuration and data directory"`

	Serve serveCmd `cmd:"" help:"Run Syncthing (default)" default:"withargs"`
	CLI   cli.CLI  `cmd:"" help:"Command line interface for Syncthing"`

	Browser  browserCmd   `cmd:"" help:"Open GUI in browser, then exit"`
	Decrypt  decrypt.CLI  `cmd:"" help:"Decrypt or verify an encrypted folder"`
	DeviceID deviceIDCmd  `cmd:"" help:"Show device ID, then exit"`
	Generate generate.CLI `cmd:"" help:"Generate key and config, then exit"`
	Paths    pathsCmd     `cmd:"" help:"Show configuration paths, then exit"`
	Upgrade  upgradeCmd   `cmd:"" help:"Perform or check for upgrade, then exit"`
	Version  versionCmd   `cmd:"" help:"Show current version, then exit"`
	Debug    debugCmd     `cmd:"" help:"Various debugging commands"`

	InstallCompletions kongplete.InstallCompletions `cmd:"" help:"Print commands to install shell completions"`
}

func (c *CLI) AfterApply() error {
	// Executed after parsing command line options but before running actual
	// subcommands
	return setConfigDataLocationsFromFlags(c.HomeDir, c.ConfDir, c.DataDir)
}

// serveCmd are the options for the `syncthing serve` command.
type serveCmd struct {
	buildSpecificOptions

	AllowNewerConfig          bool          `help:"Allow loading newer than current config version" env:"STALLOWNEWERCONFIG"`
	Audit                     bool          `help:"Write events to audit file" env:"STAUDIT"`
	AuditFile                 string        `name:"auditfile" help:"Specify audit file (use \"-\" for stdout, \"--\" for stderr)" placeholder:"PATH" env:"STAUDITFILE"`
	DBMaintenanceInterval     time.Duration `help:"Database maintenance interval" default:"8h" env:"STDBMAINTENANCEINTERVAL"`
	DBDeleteRetentionInterval time.Duration `help:"Database deleted item retention interval" default:"4320h" env:"STDBDELETERETENTIONINTERVAL"`
	GUIAddress                string        `name:"gui-address" help:"Override GUI address (e.g. \"http://192.0.2.42:8443\")" placeholder:"URL" env:"STGUIADDRESS"`
	GUIAPIKey                 string        `name:"gui-apikey" help:"Override GUI API key" placeholder:"API-KEY" env:"STGUIAPIKEY"`
	LogFile                   string        `name:"logfile" help:"Log file name (see below)" default:"${logFile}" placeholder:"PATH" env:"STLOGFILE"`
	LogFlags                  int           `name:"logflags" help:"Select information in log line prefix (see below)" default:"${logFlags}" placeholder:"BITS" env:"STLOGFLAGS"`
	LogMaxFiles               int           `name:"log-max-old-files" help:"Number of old files to keep (zero to keep only current)" default:"${logMaxFiles}" placeholder:"N" env:"STLOGMAXOLDFILES"`
	LogMaxSize                int           `help:"Maximum size of any file (zero to disable log rotation)" default:"${logMaxSize}" placeholder:"BYTES" env:"STLOGMAXSIZE"`
	NoBrowser                 bool          `help:"Do not start browser" env:"STNOBROWSER"`
	NoPortProbing             bool          `help:"Don't try to find free ports for GUI and listen addresses on first startup" env:"STNOPORTPROBING"`
	NoRestart                 bool          `help:"Do not restart Syncthing when exiting due to API/GUI command, upgrade, or crash" env:"STNORESTART"`
	NoUpgrade                 bool          `help:"Disable automatic upgrades" env:"STNOUPGRADE"`
	Paused                    bool          `help:"Start with all devices and folders paused" env:"STPAUSED"`
	Unpaused                  bool          `help:"Start with all devices and folders unpaused" env:"STUNPAUSED"`
	Verbose                   bool          `help:"Print verbose log output" env:"STVERBOSE"`

	// Debug options below
	DebugGUIAssetsDir   string `help:"Directory to load GUI assets from" placeholder:"PATH" env:"STGUIASSETS"`
	DebugPerfStats      bool   `help:"Write running performance statistics to perf-$pid.csv (Unix only)" env:"STPERFSTATS"`
	DebugProfileBlock   bool   `help:"Write block profiles to block-$pid-$timestamp.pprof every 20 seconds" env:"STBLOCKPROFILE"`
	DebugProfileCPU     bool   `help:"Write a CPU profile to cpu-$pid.pprof on exit" env:"STCPUPROFILE"`
	DebugProfileHeap    bool   `help:"Write heap profiles to heap-$pid-$timestamp.pprof each time heap usage increases" env:"STHEAPPROFILE"`
	DebugProfilerListen string `help:"Network profiler listen address" placeholder:"ADDR" env:"STPROFILER" `
	DebugResetDeltaIdxs bool   `help:"Reset delta index IDs, forcing a full index exchange"`

	// Internal options, not shown to users
	InternalRestarting   bool `env:"STRESTART" hidden:"1"`
	InternalInnerProcess bool `env:"STMONITORED" hidden:"1"`
}

func defaultVars() kong.Vars {
	vars := kong.Vars{}

	vars["logFlags"] = strconv.Itoa(logger.DefaultFlags)
	vars["logMaxSize"] = strconv.Itoa(10 << 20) // 10 MiB
	vars["logMaxFiles"] = "3"                   // plus the current one

	if os.Getenv("STTRACE") != "" {
		vars["logFlags"] = strconv.Itoa(logger.DebugFlags)
	}

	// On non-Windows, we explicitly default to "-" which means stdout. On
	// Windows, the "default" options.logFile will later be replaced with the
	// default path, unless the user has manually specified "-" or
	// something else.
	if build.IsWindows {
		vars["logFile"] = "default"
	} else {
		vars["logFile"] = "-"
	}

	return vars
}

func main() {
	// Create a parser with an overridden help function to print our extra
	// help info.
	var entrypoint CLI
	parser, err := kong.New(
		&entrypoint,
		kong.ConfigureHelp(kong.HelpOptions{
			NoExpandSubcommands: true,
			Compact:             true,
		}),
		kong.Help(helpHandler),
		defaultVars(),
	)
	if err != nil {
		log.Fatal(err)
	}

	kongplete.Complete(parser)
	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)
	ctx.BindTo(l, (*logger.Logger)(nil)) // main logger available to subcommands
	err = ctx.Run()
	parser.FatalIfErrorf(err)
}

func helpHandler(options kong.HelpOptions, ctx *kong.Context) error {
	if err := kong.DefaultHelpPrinter(options, ctx); err != nil {
		return err
	}
	if ctx.Command() == "serve" {
		// Help was requested for `syncthing serve`, so we add our extra
		// usage info afte the normal options output.
		fmt.Printf(extraUsage, debugFacilities())
	}
	return nil
}

// serveCmd.Run() is the entrypoint for `syncthing serve`
func (c *serveCmd) Run() error {
	l.SetFlags(c.LogFlags)

	if c.GUIAddress != "" {
		// The config picks this up from the environment.
		os.Setenv("STGUIADDRESS", c.GUIAddress)
	}
	if c.GUIAPIKey != "" {
		// The config picks this up from the environment.
		os.Setenv("STGUIAPIKEY", c.GUIAPIKey)
	}

	if c.HideConsole {
		osutil.HideConsole()
	}

	// Treat an explicitly empty log file name as no log file
	if c.LogFile == "" {
		c.LogFile = "-"
	}
	if c.LogFile != "default" {
		// We must set this *after* expandLocations above.
		if err := locations.Set(locations.LogFile, c.LogFile); err != nil {
			l.Warnln("Setting log file path:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
	}

	if c.DebugGUIAssetsDir != "" {
		// The asset dir is blank if STGUIASSETS wasn't set, in which case we
		// should look for extra assets in the default place.
		if err := locations.Set(locations.GUIAssets, c.DebugGUIAssetsDir); err != nil {
			l.Warnln("Setting GUI assets path:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
	}

	// Ensure that our config and data directories exist.
	for _, loc := range []locations.BaseDirEnum{locations.ConfigBaseDir, locations.DataBaseDir} {
		if err := syncthing.EnsureDir(locations.GetBaseDir(loc), 0o700); err != nil {
			l.Warnln("Failed to ensure directory exists:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
	}

	if c.InternalInnerProcess {
		c.syncthingMain()
	} else {
		c.monitorMain()
	}
	return nil
}

func openGUI() error {
	cfg, err := loadOrDefaultConfig()
	if err != nil {
		return err
	}
	if guiCfg := cfg.GUI(); guiCfg.Enabled {
		if err := openURL(guiCfg.URL()); err != nil {
			return err
		}
	} else {
		l.Warnln("Browser: GUI is currently disabled")
	}
	return nil
}

func debugFacilities() string {
	facilities := l.Facilities()

	// Get a sorted list of names
	var names []string
	maxLen := 0
	for name := range facilities {
		names = append(names, name)
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	slices.Sort(names)

	// Format the choices
	b := new(bytes.Buffer)
	for _, name := range names {
		fmt.Fprintf(b, " %-*s - %s\n", maxLen, name, facilities[name])
	}
	return b.String()
}

type errNoUpgrade struct {
	current, latest string
}

func (e *errNoUpgrade) Error() string {
	return fmt.Sprintf("no upgrade available (current %q >= latest %q).", e.current, e.latest)
}

func checkUpgrade() (upgrade.Release, error) {
	cfg, err := loadOrDefaultConfig()
	if err != nil {
		return upgrade.Release{}, err
	}
	opts := cfg.Options()
	release, err := upgrade.LatestRelease(opts.ReleasesURL, build.Version, opts.UpgradeToPreReleases)
	if err != nil {
		return upgrade.Release{}, err
	}

	if upgrade.CompareVersions(release.Tag, build.Version) <= 0 {
		return upgrade.Release{}, &errNoUpgrade{build.Version, release.Tag}
	}

	l.Infof("Upgrade available (current %q < latest %q)", build.Version, release.Tag)
	return release, nil
}

func upgradeViaRest() error {
	cfg, err := loadOrDefaultConfig()
	if err != nil {
		return err
	}

	u, err := url.Parse(cfg.GUI().URL())
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, "rest/system/upgrade")
	target := u.String()
	r, _ := http.NewRequest("POST", target, nil)
	r.Header.Set("X-API-Key", cfg.GUI().APIKey)

	tr := &http.Transport{
		DialContext:     dialer.DialContext,
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   60 * time.Second,
	}
	resp, err := client.Do(r)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(bs))
	}

	return err
}

func (c *serveCmd) syncthingMain() {
	if c.DebugProfileBlock {
		startBlockProfiler()
	}
	if c.DebugProfileHeap {
		startHeapProfiler()
	}
	if c.DebugPerfStats {
		startPerfStats()
	}

	// Set a log prefix similar to the ID we will have later on, or early log
	// lines look ugly.
	l.SetPrefix("[start] ")

	// Print our version information up front, so any crash that happens
	// early etc. will have it available.
	l.Infoln(build.LongVersion)

	// Ensure that we have a certificate and key.
	cert, err := syncthing.LoadOrGenerateCertificate(
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	)
	if err != nil {
		l.Warnln("Failed to load/generate certificate:", err)
		os.Exit(1)
	}

	// Ensure we are the only running instance
	lf := flock.New(locations.Get(locations.LockFile))
	locked, err := lf.TryLock()
	if err != nil {
		l.Warnln("Failed to acquire lock:", err)
		os.Exit(1)
	} else if !locked {
		l.Warnln("Failed to acquire lock: is another Syncthing instance already running?")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// earlyService is a supervisor that runs the services needed for or
	// before app startup; the event logger, and the config service.
	spec := svcutil.SpecWithDebugLogger(l)
	earlyService := suture.New("early", spec)
	earlyService.ServeBackground(ctx)

	evLogger := events.NewLogger()
	earlyService.Add(evLogger)

	cfgWrapper, err := syncthing.LoadConfigAtStartup(locations.Get(locations.ConfigFile), cert, evLogger, c.AllowNewerConfig, c.NoPortProbing)
	if err != nil {
		l.Warnln("Failed to initialize config:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}
	earlyService.Add(cfgWrapper)
	config.RegisterInfoMetrics(cfgWrapper)

	// Candidate builds should auto upgrade. Make sure the option is set,
	// unless we are in a build where it's disabled or the STNOUPGRADE
	// environment variable is set.

	if build.IsCandidate && !upgrade.DisabledByCompilation && !c.NoUpgrade {
		cfgWrapper.Modify(func(cfg *config.Configuration) {
			l.Infoln("Automatic upgrade is always enabled for candidate releases.")
			if cfg.Options.AutoUpgradeIntervalH == 0 || cfg.Options.AutoUpgradeIntervalH > 24 {
				cfg.Options.AutoUpgradeIntervalH = 12
				// Set the option into the config as well, as the auto upgrade
				// loop expects to read a valid interval from there.
			}
			// We don't tweak the user's choice of upgrading to pre-releases or
			// not, as otherwise they cannot step off the candidate channel.
		})
	}

	if err := syncthing.TryMigrateDatabase(c.DBDeleteRetentionInterval); err != nil {
		l.Warnln("Failed to migrate old-style database:", err)
		os.Exit(1)
	}

	sdb, err := syncthing.OpenDatabase(locations.Get(locations.Database), c.DBDeleteRetentionInterval)
	if err != nil {
		l.Warnln("Error opening database:", err)
		os.Exit(1)
	}

	// Check if auto-upgrades is possible, and if yes, and it's enabled do an initial
	// upgrade immediately. The auto-upgrade routine can only be started
	// later after App is initialised.
	autoUpgradePossible := c.autoUpgradePossible()
	if autoUpgradePossible && cfgWrapper.Options().AutoUpgradeEnabled() {
		// try to do upgrade directly and log the error if relevant.
		miscDB := db.NewMiscDB(sdb)
		release, err := initialAutoUpgradeCheck(miscDB)
		if err == nil {
			err = upgrade.To(release)
		}
		if err != nil {
			if _, ok := err.(*errNoUpgrade); ok || err == errTooEarlyUpgradeCheck || err == errTooEarlyUpgrade {
				l.Debugln("Initial automatic upgrade:", err)
			} else {
				l.Infoln("Initial automatic upgrade:", err)
			}
		} else {
			l.Infof("Upgraded to %q, should exit now.", release.Tag)
			os.Exit(svcutil.ExitUpgrade.AsInt())
		}
	}

	if c.Unpaused {
		setPauseState(cfgWrapper, false)
	} else if c.Paused {
		setPauseState(cfgWrapper, true)
	}

	appOpts := syncthing.Options{
		NoUpgrade:             c.NoUpgrade,
		ProfilerAddr:          c.DebugProfilerListen,
		ResetDeltaIdxs:        c.DebugResetDeltaIdxs,
		Verbose:               c.Verbose,
		DBMaintenanceInterval: c.DBMaintenanceInterval,
	}

	if c.Audit || cfgWrapper.Options().AuditEnabled {
		l.Infoln("Auditing is enabled.")

		auditFile := cfgWrapper.Options().AuditFile

		// Ignore config option if command-line option is set
		if c.AuditFile != "" {
			l.Debugln("Using the audit file from the command-line parameter.")
			auditFile = c.AuditFile
		}

		appOpts.AuditWriter = auditWriter(auditFile)
	}

	app, err := syncthing.New(cfgWrapper, sdb, evLogger, cert, appOpts)
	if err != nil {
		l.Warnln("Failed to start Syncthing:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}

	if autoUpgradePossible {
		go autoUpgrade(cfgWrapper, app, evLogger)
	}

	setupSignalHandling(app)

	if c.DebugProfileCPU {
		f, err := os.Create(fmt.Sprintf("cpu-%d.pprof", os.Getpid()))
		if err != nil {
			l.Warnln("Creating profile:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			l.Warnln("Starting profile:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
	}

	if err := app.Start(); err != nil {
		os.Exit(svcutil.ExitError.AsInt())
	}

	cleanConfigDirectory()

	if cfgWrapper.Options().StartBrowser && !c.NoBrowser && !c.InternalRestarting {
		// Can potentially block if the utility we are invoking doesn't
		// fork, and just execs, hence keep it in its own routine.
		go func() { _ = openURL(cfgWrapper.GUI().URL()) }()
	}

	status := app.Wait()

	if status == svcutil.ExitError {
		l.Warnln("Syncthing stopped with error:", app.Error())
	}

	if c.DebugProfileCPU {
		pprof.StopCPUProfile()
	}

	// Best effort remove lockfile, doesn't matter if it succeeds
	_ = lf.Unlock()
	_ = os.Remove(locations.Get(locations.LockFile))

	os.Exit(int(status))
}

func setupSignalHandling(app *syncthing.App) {
	// Exit cleanly with "restarting" code on SIGHUP.

	restartSign := make(chan os.Signal, 1)
	signal.Notify(restartSign, syscall.SIGHUP)
	go func() {
		<-restartSign
		app.Stop(svcutil.ExitRestart)
	}()

	// Exit with "success" code (no restart) on INT/TERM

	stopSign := make(chan os.Signal, 1)
	signal.Notify(stopSign, os.Interrupt, sigTerm)
	go func() {
		<-stopSign
		app.Stop(svcutil.ExitSuccess)
	}()
}

// loadOrDefaultConfig creates a temporary, minimal configuration wrapper if no file
// exists.  As it disregards some command-line options, that should never be persisted.
func loadOrDefaultConfig() (config.Wrapper, error) {
	cfgFile := locations.Get(locations.ConfigFile)
	cfg, _, err := config.Load(cfgFile, protocol.EmptyDeviceID, events.NoopLogger)
	if err != nil {
		newCfg := config.New(protocol.EmptyDeviceID)
		return config.Wrap(cfgFile, newCfg, protocol.EmptyDeviceID, events.NoopLogger), nil
	}

	return cfg, err
}

func auditWriter(auditFile string) io.Writer {
	var fd io.Writer
	var err error
	var auditDest string
	var auditFlags int

	if auditFile == "-" {
		fd = os.Stdout
		auditDest = "stdout"
	} else if auditFile == "--" {
		fd = os.Stderr
		auditDest = "stderr"
	} else {
		if auditFile == "" {
			auditFile = locations.GetTimestamped(locations.AuditLog)
			auditFlags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		} else {
			auditFlags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		}
		fd, err = os.OpenFile(auditFile, auditFlags, 0o600)
		if err != nil {
			l.Warnln("Audit:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
		auditDest = auditFile
	}

	l.Infoln("Audit log in", auditDest)

	return fd
}

func (c *serveCmd) autoUpgradePossible() bool {
	if upgrade.DisabledByCompilation {
		return false
	}
	if c.NoUpgrade {
		l.Infof("No automatic upgrades; STNOUPGRADE environment variable defined.")
		return false
	}
	return true
}

func autoUpgrade(cfg config.Wrapper, app *syncthing.App, evLogger events.Logger) {
	timer := time.NewTimer(upgradeCheckInterval)
	sub := evLogger.Subscribe(events.DeviceConnected)
	for {
		select {
		case event := <-sub.C():
			data, ok := event.Data.(map[string]string)
			if !ok || data["clientName"] != "syncthing" || upgrade.CompareVersions(data["clientVersion"], build.Version) != upgrade.Newer {
				continue
			}
			if cfg.Options().AutoUpgradeEnabled() {
				l.Infof("Connected to device %s with a newer version (current %q < remote %q). Checking for upgrades.", data["id"], build.Version, data["clientVersion"])
			}
		case <-timer.C:
		}

		opts := cfg.Options()
		if !opts.AutoUpgradeEnabled() {
			timer.Reset(upgradeCheckInterval)
			continue
		}

		checkInterval := time.Duration(opts.AutoUpgradeIntervalH) * time.Hour
		rel, err := upgrade.LatestRelease(opts.ReleasesURL, build.Version, opts.UpgradeToPreReleases)
		if err == upgrade.ErrUpgradeUnsupported {
			sub.Unsubscribe()
			return
		}
		if err != nil {
			// Don't complain too loudly here; we might simply not have
			// internet connectivity, or the upgrade server might be down.
			l.Infoln("Automatic upgrade:", err)
			timer.Reset(checkInterval)
			continue
		}

		if upgrade.CompareVersions(rel.Tag, build.Version) != upgrade.Newer {
			// Skip equal, older or majorly newer (incompatible) versions
			timer.Reset(checkInterval)
			continue
		}

		l.Infof("Automatic upgrade (current %q < latest %q)", build.Version, rel.Tag)
		err = upgrade.To(rel)
		if err != nil {
			l.Warnln("Automatic upgrade:", err)
			timer.Reset(checkInterval)
			continue
		}
		sub.Unsubscribe()
		l.Warnf("Automatically upgraded to version %q. Restarting in 1 minute.", rel.Tag)
		time.Sleep(time.Minute)
		app.Stop(svcutil.ExitUpgrade)
		return
	}
}

func initialAutoUpgradeCheck(misc *db.Typed) (upgrade.Release, error) {
	if last, ok, err := misc.Time(upgradeCheckKey); err == nil && ok && time.Since(last) < upgradeCheckInterval {
		return upgrade.Release{}, errTooEarlyUpgradeCheck
	}
	_ = misc.PutTime(upgradeCheckKey, time.Now())
	release, err := checkUpgrade()
	if err != nil {
		return upgrade.Release{}, err
	}
	if upgrade.CompareVersions(release.Tag, build.Version) == upgrade.MajorNewer {
		return upgrade.Release{}, errors.New("higher major version")
	}

	if lastVersion, ok, err := misc.String(upgradeVersionKey); err == nil && ok && lastVersion == release.Tag {
		// Only check time if we try to upgrade to the same release.
		if lastTime, ok, err := misc.Time(upgradeTimeKey); err == nil && ok && time.Since(lastTime) < upgradeRetryInterval {
			return upgrade.Release{}, errTooEarlyUpgrade
		}
	}
	_ = misc.PutString(upgradeVersionKey, release.Tag)
	_ = misc.PutTime(upgradeTimeKey, time.Now())
	return release, nil
}

// cleanConfigDirectory removes old, unused configuration and index formats, a
// suitable time after they have gone out of fashion.
func cleanConfigDirectory() {
	patterns := map[string]time.Duration{
		"panic-*.log":        7 * 24 * time.Hour,  // keep panic logs for a week
		"audit-*.log":        7 * 24 * time.Hour,  // keep audit logs for a week
		"index":              14 * 24 * time.Hour, // keep old index format for two weeks
		"index-v0.11.0.db":   14 * 24 * time.Hour, // keep old index format for two weeks
		"index-v0.13.0.db":   14 * 24 * time.Hour, // keep old index format for two weeks
		"index*.converted":   14 * 24 * time.Hour, // keep old converted indexes for two weeks
		"config.xml.v*":      30 * 24 * time.Hour, // old config versions for a month
		"*.idx.gz":           30 * 24 * time.Hour, // these should for sure no longer exist
		"backup-of-v0.8":     30 * 24 * time.Hour, // these neither
		"tmp-index-sorter.*": time.Minute,         // these should never exist on startup
		"support-bundle-*":   30 * 24 * time.Hour, // keep old support bundle zip or folder for a month
		"csrftokens.txt":     0,                   // deprecated, remove immediately
	}

	for pat, dur := range patterns {
		fs := fs.NewFilesystem(fs.FilesystemTypeBasic, locations.GetBaseDir(locations.ConfigBaseDir))
		files, err := fs.Glob(pat)
		if err != nil {
			l.Infoln("Cleaning:", err)
			continue
		}

		for _, file := range files {
			info, err := fs.Lstat(file)
			if err != nil {
				l.Infoln("Cleaning:", err)
				continue
			}

			if time.Since(info.ModTime()) > dur {
				if err = fs.RemoveAll(file); err != nil {
					l.Infoln("Cleaning:", err)
				} else {
					l.Infoln("Cleaned away old file", filepath.Base(file))
				}
			}
		}
	}
}

func setPauseState(cfgWrapper config.Wrapper, paused bool) {
	_, err := cfgWrapper.Modify(func(cfg *config.Configuration) {
		for i := range cfg.Devices {
			cfg.Devices[i].Paused = paused
		}
		for i := range cfg.Folders {
			cfg.Folders[i].Paused = paused
		}
	})
	if err != nil {
		l.Warnln("Cannot adjust paused state:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}
}

func exitCodeForUpgrade(err error) int {
	if _, ok := err.(*errNoUpgrade); ok {
		return svcutil.ExitNoUpgradeAvailable.AsInt()
	}
	return svcutil.ExitError.AsInt()
}

type versionCmd struct{}

func (versionCmd) Run() error {
	fmt.Println(build.LongVersion)
	return nil
}

type deviceIDCmd struct{}

func (deviceIDCmd) Run() error {
	cert, err := tls.LoadX509KeyPair(
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	)
	if err != nil {
		l.Warnln("Error reading device ID:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}

	fmt.Println(protocol.NewDeviceID(cert.Certificate[0]))
	return nil
}

type pathsCmd struct{}

func (pathsCmd) Run() error {
	fmt.Print(locations.PrettyPaths())
	return nil
}

type upgradeCmd struct {
	CheckOnly bool   `short:"c" help:"Check for available upgrade, then exit"`
	From      string `short:"u" placeholder:"URL" help:"Force upgrade directly from specified URL"`
}

func (u upgradeCmd) Run() error {
	if u.CheckOnly {
		if _, err := checkUpgrade(); err != nil {
			l.Warnln("Checking for upgrade:", err)
			os.Exit(exitCodeForUpgrade(err))
		}
		return nil
	}

	if u.From != "" {
		err := upgrade.ToURL(u.From)
		if err != nil {
			l.Warnln("Error while Upgrading:", err)
			os.Exit(svcutil.ExitError.AsInt())
		}
		l.Infoln("Upgraded from", u.From)
		return nil
	}

	release, err := checkUpgrade()
	if err == nil {
		lf := flock.New(locations.Get(locations.LockFile))
		var locked bool
		locked, err = lf.TryLock()
		if err != nil {
			l.Warnln("Upgrade:", err)
			os.Exit(1)
		} else if locked {
			err = upgradeViaRest()
		} else {
			err = upgrade.To(release)
		}
	}
	if err != nil {
		l.Warnln("Upgrade:", err)
		os.Exit(exitCodeForUpgrade(err))
	}
	l.Infof("Upgraded to %q", release.Tag)
	os.Exit(svcutil.ExitUpgrade.AsInt())
	return nil
}

type browserCmd struct{}

func (browserCmd) Run() error {
	if err := openGUI(); err != nil {
		l.Warnln("Failed to open web UI:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}
	return nil
}

type debugCmd struct {
	ResetDatabase      resetDatabaseCmd `cmd:"" help:"Reset the database, forcing a full rescan and resync"`
	DatabaseStatistics databaseStatsCmd `cmd:"" help:"Display database size statistics"`
}

type resetDatabaseCmd struct{}

func (resetDatabaseCmd) Run() error {
	l.Infoln("Removing database in", locations.Get(locations.Database))
	if err := os.RemoveAll(locations.Get(locations.Database)); err != nil {
		l.Warnln("Resetting database:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}
	l.Infoln("Successfully reset database - it will be rebuilt after next start.")
	return nil
}

type databaseStatsCmd struct{}

func (c databaseStatsCmd) Run() error {
	db, err := sqlite.Open(locations.Get(locations.Database))
	if err != nil {
		return err
	}
	ds, err := db.Statistics()
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 2, 2, 2, ' ', 0)
	hdr := fmt.Sprintf("%s\t%s\t%s\t%12s\t%7s\n", "DATABASE", "FOLDER ID", "TABLE", "SIZE", "FILL")
	fmt.Fprint(tw, hdr)
	fmt.Fprint(tw, regexp.MustCompile(`[A-Z]`).ReplaceAllString(hdr, "="))
	c.printStat(tw, ds)
	return tw.Flush()
}

func (c databaseStatsCmd) printStat(w io.Writer, s *sqlite.DatabaseStatistics) {
	for _, table := range s.Tables {
		fmt.Fprintf(w, "%s\t%s\t%s\t%8d KiB\t%5.01f %%\n", s.Name, cmp.Or(s.FolderID, "-"), table.Name, table.Size/1024, float64(table.Size-table.Unused)*100/float64(table.Size))
	}
	for _, next := range s.Children {
		c.printStat(w, &next)
		s.Total.Size += next.Total.Size
		s.Total.Unused += next.Total.Unused
	}

	totalName := s.Name
	if len(s.Children) > 0 {
		totalName += " + children"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%8d KiB\t%5.01f %%\n", totalName, cmp.Or(s.FolderID, "-"), "(total)", s.Total.Size/1024, float64(s.Total.Size-s.Total.Unused)*100/float64(s.Total.Size))
}

func setConfigDataLocationsFromFlags(homeDir, confDir, dataDir string) error {
	homeSet := homeDir != ""
	confSet := confDir != ""
	dataSet := dataDir != ""
	switch {
	case dataSet != confSet:
		return errors.New("either both or none of --config and --data must be given, use --home to set both at once")
	case homeSet && dataSet:
		return errors.New("--home must not be used together with --config and --data")
	case homeSet:
		confDir = homeDir
		dataDir = homeDir
		fallthrough
	case dataSet:
		if err := locations.SetBaseDir(locations.ConfigBaseDir, confDir); err != nil {
			return err
		}
		return locations.SetBaseDir(locations.DataBaseDir, dataDir)
	}
	return nil
}
