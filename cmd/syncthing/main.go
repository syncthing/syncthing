// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/weakhash"

	"github.com/thejerf/suture"

	_ "net/http/pprof" // Need to import this to support STPROFILER.
)

var (
	Version           = "unknown-dev"
	Codename          = "Dysprosium Dragonfly"
	BuildStamp        = "0"
	BuildDate         time.Time
	BuildHost         = "unknown"
	BuildUser         = "unknown"
	IsRelease         bool
	IsCandidate       bool
	IsBeta            bool
	LongVersion       string
	BuildTags         []string
	allowedVersionExp = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9]+)*(\.\d+)*(\+\d+-g[0-9a-f]+)?(-[^\s]+)?$`)
)

const (
	exitSuccess            = 0
	exitError              = 1
	exitNoUpgradeAvailable = 2
	exitRestarting         = 3
	exitUpgrading          = 4
)

const (
	bepProtocolName      = "bep/1.0"
	tlsDefaultCommonName = "syncthing"
	httpsRSABits         = 2048
	bepRSABits           = 0 // 384 bit ECDSA used instead
	defaultEventTimeout  = time.Minute
	maxSystemErrors      = 5
	initialSystemLog     = 10
	maxSystemLog         = 250
)

// The discovery results are sorted by their source priority.
const (
	ipv6LocalDiscoveryPriority = iota
	ipv4LocalDiscoveryPriority
	globalDiscoveryPriority
)

func init() {
	if Version != "unknown-dev" {
		// If not a generic dev build, version string should come from git describe
		if !allowedVersionExp.MatchString(Version) {
			l.Fatalf("Invalid version string %q;\n\tdoes not match regexp %v", Version, allowedVersionExp)
		}
	}
}

func setBuildMetadata() {
	// Check for a clean release build. A release is something like
	// "v0.1.2", with an optional suffix of letters and dot separated
	// numbers like "-beta3.47". If there's more stuff, like a plus sign and
	// a commit hash and so on, then it's not a release. If it has a dash in
	// it, it's some sort of beta, release candidate or special build. If it
	// has "-rc." in it, like "v0.14.35-rc.42", then it's a candidate build.
	//
	// So, every build that is not a stable release build has IsBeta = true.
	// This is used to enable some extra debugging (the deadlock detector).
	//
	// Release candidate builds are also "betas" from this point of view and
	// will have that debugging enabled. In addition, some features are
	// forced for release candidates - auto upgrade, and usage reporting.

	exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z]+[\d\.]+)?$`)
	IsRelease = exp.MatchString(Version)
	IsCandidate = strings.Contains(Version, "-rc.")
	IsBeta = strings.Contains(Version, "-")

	stamp, _ := strconv.Atoi(BuildStamp)
	BuildDate = time.Unix(int64(stamp), 0)

	date := BuildDate.UTC().Format("2006-01-02 15:04:05 MST")
	LongVersion = fmt.Sprintf(`syncthing %s "%s" (%s %s-%s) %s@%s %s`, Version, Codename, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildUser, BuildHost, date)

	if len(BuildTags) > 0 {
		LongVersion = fmt.Sprintf("%s [%s]", LongVersion, strings.Join(BuildTags, ", "))
	}
}

var (
	myID protocol.DeviceID
	stop = make(chan int)
	lans []*net.IPNet
)

const (
	usage      = "syncthing [options]"
	extraUsage = `
The -logflags value is a sum of the following:

   1  Date
   2  Time
   4  Microsecond time
   8  Long filename
  16  Short filename

I.e. to prefix each log line with date and time, set -logflags=3 (1 + 2 from
above). The value 0 is used to disable all of the above. The default is to
show time only (2).


Development Settings
--------------------

The following environment variables modify Syncthing's behavior in ways that
are mostly useful for developers. Use with care.

 STNODEFAULTFOLDER Don't create a default folder when starting for the first
                   time. This variable will be ignored anytime after the first
                   run.

 STGUIASSETS       Directory to load GUI assets from. Overrides compiled in
                   assets.

 STTRACE           A comma separated string of facilities to trace. The valid
                   facility strings listed below.

 STPROFILER        Set to a listen address such as "127.0.0.1:9090" to start
                   the profiler with HTTP access.

 STCPUPROFILE      Write a CPU profile to cpu-$pid.pprof on exit.

 STHEAPPROFILE     Write heap profiles to heap-$pid-$timestamp.pprof each time
                   heap usage increases.

 STBLOCKPROFILE    Write block profiles to block-$pid-$timestamp.pprof every 20
                   seconds.

 STPERFSTATS       Write running performance statistics to perf-$pid.csv. Not
                   supported on Windows.

 STDEADLOCK        Used for debugging internal deadlocks. Use only under
                   direction of a developer.

 STDEADLOCKTIMEOUT Used for debugging internal deadlocks; sets debug
                   sensitivity. Use only under direction of a developer.

 STDEADLOCKTHRESHOLD Used for debugging internal deadlocks; sets debug
                     sensitivity.  Use only under direction of a developer.

 STNORESTART       Equivalent to the -no-restart argument. Disable the
                   Syncthing monitor process which handles restarts for some
                   configuration changes, upgrades, crashes and also log file
                   writing (stdout is still written).

 STNOUPGRADE       Disable automatic upgrades.

 STHASHING         Select the SHA256 hashing package to use. Possible values
                   are "standard" for the Go standard library implementation,
                   "minio" for the github.com/minio/sha256-simd implementation,
                   and blank (the default) for auto detection.

 GOMAXPROCS        Set the maximum number of CPU cores to use. Defaults to all
                   available CPU cores.

 GOGC              Percentage of heap growth at which to trigger GC. Default is
                   100. Lower numbers keep peak memory usage down, at the price
                   of CPU usage (i.e. performance).


Debugging Facilities
--------------------

The following are valid values for the STTRACE variable:

%s`
)

// Environment options
var (
	noUpgradeFromEnv = os.Getenv("STNOUPGRADE") != ""
	innerProcess     = os.Getenv("STNORESTART") != "" || os.Getenv("STMONITORED") != ""
	noDefaultFolder  = os.Getenv("STNODEFAULTFOLDER") != ""
)

type RuntimeOptions struct {
	confDir        string
	resetDatabase  bool
	resetDeltaIdxs bool
	showVersion    bool
	showPaths      bool
	showDeviceId   bool
	doUpgrade      bool
	doUpgradeCheck bool
	upgradeTo      string
	noBrowser      bool
	browserOnly    bool
	hideConsole    bool
	logFile        string
	auditEnabled   bool
	auditFile      string
	verbose        bool
	paused         bool
	unpaused       bool
	guiAddress     string
	guiAPIKey      string
	generateDir    string
	noRestart      bool
	profiler       string
	assetDir       string
	cpuProfile     bool
	stRestarting   bool
	logFlags       int
}

func defaultRuntimeOptions() RuntimeOptions {
	options := RuntimeOptions{
		noRestart:    os.Getenv("STNORESTART") != "",
		profiler:     os.Getenv("STPROFILER"),
		assetDir:     os.Getenv("STGUIASSETS"),
		cpuProfile:   os.Getenv("STCPUPROFILE") != "",
		stRestarting: os.Getenv("STRESTART") != "",
		logFlags:     log.Ltime,
	}

	if os.Getenv("STTRACE") != "" {
		options.logFlags = logger.DebugFlags
	}

	if runtime.GOOS != "windows" {
		// On non-Windows, we explicitly default to "-" which means stdout. On
		// Windows, the blank options.logFile will later be replaced with the
		// default path, unless the user has manually specified "-" or
		// something else.
		options.logFile = "-"
	}

	return options
}

func parseCommandLineOptions() RuntimeOptions {
	options := defaultRuntimeOptions()

	flag.StringVar(&options.generateDir, "generate", "", "Generate key and config in specified dir, then exit")
	flag.StringVar(&options.guiAddress, "gui-address", options.guiAddress, "Override GUI address (e.g. \"http://192.0.2.42:8443\")")
	flag.StringVar(&options.guiAPIKey, "gui-apikey", options.guiAPIKey, "Override GUI API key")
	flag.StringVar(&options.confDir, "home", "", "Set configuration directory")
	flag.IntVar(&options.logFlags, "logflags", options.logFlags, "Select information in log line prefix (see below)")
	flag.BoolVar(&options.noBrowser, "no-browser", false, "Do not start browser")
	flag.BoolVar(&options.browserOnly, "browser-only", false, "Open GUI in browser")
	flag.BoolVar(&options.noRestart, "no-restart", options.noRestart, "Disable monitor process, managed restarts and log file writing")
	flag.BoolVar(&options.resetDatabase, "reset-database", false, "Reset the database, forcing a full rescan and resync")
	flag.BoolVar(&options.resetDeltaIdxs, "reset-deltas", false, "Reset delta index IDs, forcing a full index exchange")
	flag.BoolVar(&options.doUpgrade, "upgrade", false, "Perform upgrade")
	flag.BoolVar(&options.doUpgradeCheck, "upgrade-check", false, "Check for available upgrade")
	flag.BoolVar(&options.showVersion, "version", false, "Show version")
	flag.BoolVar(&options.showPaths, "paths", false, "Show configuration paths")
	flag.BoolVar(&options.showDeviceId, "device-id", false, "Show the device ID")
	flag.StringVar(&options.upgradeTo, "upgrade-to", options.upgradeTo, "Force upgrade directly from specified URL")
	flag.BoolVar(&options.auditEnabled, "audit", false, "Write events to audit file")
	flag.BoolVar(&options.verbose, "verbose", false, "Print verbose log output")
	flag.BoolVar(&options.paused, "paused", false, "Start with all devices and folders paused")
	flag.BoolVar(&options.unpaused, "unpaused", false, "Start with all devices and folders unpaused")
	flag.StringVar(&options.logFile, "logfile", options.logFile, "Log file name (use \"-\" for stdout)")
	flag.StringVar(&options.auditFile, "auditfile", options.auditFile, "Specify audit file (use \"-\" for stdout, \"--\" for stderr)")
	if runtime.GOOS == "windows" {
		// Allow user to hide the console window
		flag.BoolVar(&options.hideConsole, "no-console", false, "Hide console window")
	}

	longUsage := fmt.Sprintf(extraUsage, debugFacilities())
	flag.Usage = usageFor(flag.CommandLine, usage, longUsage)
	flag.Parse()

	if len(flag.Args()) > 0 {
		flag.Usage()
		os.Exit(2)
	}

	return options
}

func main() {
	setBuildMetadata()

	options := parseCommandLineOptions()
	l.SetFlags(options.logFlags)

	if options.guiAddress != "" {
		// The config picks this up from the environment.
		os.Setenv("STGUIADDRESS", options.guiAddress)
	}
	if options.guiAPIKey != "" {
		// The config picks this up from the environment.
		os.Setenv("STGUIAPIKEY", options.guiAPIKey)
	}

	// Check for options which are not compatible with each other. We have
	// to check logfile before it's set to the default below - we only want
	// to complain if they set -logfile explicitly, not if it's set to its
	// default location
	if options.noRestart && (options.logFile != "" && options.logFile != "-") {
		l.Fatalln("-logfile may not be used with -no-restart or STNORESTART")
	}

	if options.hideConsole {
		osutil.HideConsole()
	}

	if options.confDir != "" {
		// Not set as default above because the string can be really long.
		if !filepath.IsAbs(options.confDir) {
			var err error
			options.confDir, err = filepath.Abs(options.confDir)
			if err != nil {
				l.Fatalln(err)
			}
		}
		baseDirs["config"] = options.confDir
	}

	if err := expandLocations(); err != nil {
		l.Fatalln(err)
	}

	if options.logFile == "" {
		// Blank means use the default logfile location. We must set this
		// *after* expandLocations above.
		options.logFile = locations[locLogFile]
	}

	if options.assetDir == "" {
		// The asset dir is blank if STGUIASSETS wasn't set, in which case we
		// should look for extra assets in the default place.
		options.assetDir = locations[locGUIAssets]
	}

	if options.showVersion {
		fmt.Println(LongVersion)
		return
	}

	if options.showPaths {
		showPaths()
		return
	}

	if options.showDeviceId {
		cert, err := tls.LoadX509KeyPair(locations[locCertFile], locations[locKeyFile])
		if err != nil {
			l.Fatalln("Error reading device ID:", err)
		}

		myID = protocol.NewDeviceID(cert.Certificate[0])
		fmt.Println(myID)
		return
	}

	if options.browserOnly {
		openGUI()
		return
	}

	if options.generateDir != "" {
		generate(options.generateDir)
		return
	}

	// Ensure that our home directory exists.
	ensureDir(baseDirs["config"], 0700)

	if options.upgradeTo != "" {
		err := upgrade.ToURL(options.upgradeTo)
		if err != nil {
			l.Fatalln("Upgrade:", err) // exits 1
		}
		l.Infoln("Upgraded from", options.upgradeTo)
		return
	}

	if options.doUpgradeCheck {
		checkUpgrade()
		return
	}

	if options.doUpgrade {
		release := checkUpgrade()
		performUpgrade(release)
		return
	}

	if options.resetDatabase {
		resetDB()
		return
	}

	if innerProcess || options.noRestart {
		syncthingMain(options)
	} else {
		monitorMain(options)
	}
}

func openGUI() {
	cfg, _ := loadOrDefaultConfig()
	if cfg.GUI().Enabled {
		openURL(cfg.GUI().URL())
	} else {
		l.Warnln("Browser: GUI is currently disabled")
	}
}

func generate(generateDir string) {
	dir, err := fs.ExpandTilde(generateDir)
	if err != nil {
		l.Fatalln("generate:", err)
	}
	ensureDir(dir, 0700)

	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err == nil {
		l.Warnln("Key exists; will not overwrite.")
		l.Infoln("Device ID:", protocol.NewDeviceID(cert.Certificate[0]))
	} else {
		cert, err = tlsutil.NewCertificate(certFile, keyFile, tlsDefaultCommonName, bepRSABits)
		if err != nil {
			l.Fatalln("Create certificate:", err)
		}
		myID = protocol.NewDeviceID(cert.Certificate[0])
		if err != nil {
			l.Fatalln("Load certificate:", err)
		}
		if err == nil {
			l.Infoln("Device ID:", protocol.NewDeviceID(cert.Certificate[0]))
		}
	}

	cfgFile := filepath.Join(dir, "config.xml")
	if _, err := os.Stat(cfgFile); err == nil {
		l.Warnln("Config exists; will not overwrite.")
		return
	}
	var cfg = defaultConfig(cfgFile)
	err = cfg.Save()
	if err != nil {
		l.Warnln("Failed to save config", err)
	}
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
	sort.Strings(names)

	// Format the choices
	b := new(bytes.Buffer)
	for _, name := range names {
		fmt.Fprintf(b, " %-*s - %s\n", maxLen, name, facilities[name])
	}
	return b.String()
}

func checkUpgrade() upgrade.Release {
	cfg, _ := loadOrDefaultConfig()
	opts := cfg.Options()
	release, err := upgrade.LatestRelease(opts.ReleasesURL, Version, opts.UpgradeToPreReleases)
	if err != nil {
		l.Fatalln("Upgrade:", err)
	}

	if upgrade.CompareVersions(release.Tag, Version) <= 0 {
		noUpgradeMessage := "No upgrade available (current %q >= latest %q)."
		l.Infof(noUpgradeMessage, Version, release.Tag)
		os.Exit(exitNoUpgradeAvailable)
	}

	l.Infof("Upgrade available (current %q < latest %q)", Version, release.Tag)
	return release
}

func performUpgrade(release upgrade.Release) {
	// Use leveldb database locks to protect against concurrent upgrades
	_, err := db.Open(locations[locDatabase])
	if err == nil {
		err = upgrade.To(release)
		if err != nil {
			l.Fatalln("Upgrade:", err)
		}
		l.Infof("Upgraded to %q", release.Tag)
	} else {
		l.Infoln("Attempting upgrade through running Syncthing...")
		err = upgradeViaRest()
		if err != nil {
			l.Fatalln("Upgrade:", err)
		}
		l.Infoln("Syncthing upgrading")
		os.Exit(exitUpgrading)
	}
}

func upgradeViaRest() error {
	cfg, _ := loadOrDefaultConfig()
	u, err := url.Parse(cfg.GUI().URL())
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, "rest/system/upgrade")
	target := u.String()
	r, _ := http.NewRequest("POST", target, nil)
	r.Header.Set("X-API-Key", cfg.GUI().APIKey)

	tr := &http.Transport{
		Dial:            dialer.Dial,
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
	if resp.StatusCode != 200 {
		bs, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return err
		}
		return errors.New(string(bs))
	}

	return err
}

func syncthingMain(runtimeOptions RuntimeOptions) {
	setupSignalHandling()

	// Create a main service manager. We'll add things to this as we go along.
	// We want any logging it does to go through our log system.
	mainService := suture.New("main", suture.Spec{
		Log: func(line string) {
			l.Debugln(line)
		},
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
	defaultSub := events.NewBufferedSubscription(events.Default.Subscribe(defaultEventMask), eventSubBufferSize)
	diskSub := events.NewBufferedSubscription(events.Default.Subscribe(diskEventMask), eventSubBufferSize)

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Attempt to increase the limit on number of open files to the maximum
	// allowed, in case we have many peers. We don't really care enough to
	// report the error if there is one.
	osutil.MaximizeOpenFileLimit()

	// Ensure that we have a certificate and key.
	cert, err := tls.LoadX509KeyPair(locations[locCertFile], locations[locKeyFile])
	if err != nil {
		l.Infof("Generating ECDSA key and certificate for %s...", tlsDefaultCommonName)
		cert, err = tlsutil.NewCertificate(locations[locCertFile], locations[locKeyFile], tlsDefaultCommonName, bepRSABits)
		if err != nil {
			l.Fatalln(err)
		}
	}

	myID = protocol.NewDeviceID(cert.Certificate[0])
	l.SetPrefix(fmt.Sprintf("[%s] ", myID.String()[:5]))

	l.Infoln(LongVersion)
	l.Infoln("My ID:", myID)

	// Select SHA256 implementation and report. Affected by the
	// STHASHING environment variable.
	sha256.SelectAlgo()
	sha256.Report()

	// Emit the Starting event, now that we know who we are.

	events.Default.Log(events.Starting, map[string]string{
		"home": baseDirs["config"],
		"myID": myID.String(),
	})

	cfg := loadConfigAtStartup()

	if err := checkShortIDs(cfg); err != nil {
		l.Fatalln("Short device IDs are in conflict. Unlucky!\n  Regenerate the device ID of one of the following:\n  ", err)
	}

	if len(runtimeOptions.profiler) > 0 {
		go func() {
			l.Debugln("Starting profiler on", runtimeOptions.profiler)
			runtime.SetBlockProfileRate(1)
			err := http.ListenAndServe(runtimeOptions.profiler, nil)
			if err != nil {
				l.Fatalln(err)
			}
		}()
	}

	// The TLS configuration is used for both the listening socket and outgoing
	// connections.

	tlsCfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{bepProtocolName},
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
		MinVersion:             tls.VersionTLS12,
		CipherSuites: []uint16{
			0xCCA8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305, Go 1.8
			0xCCA9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, Go 1.8
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
	}

	opts := cfg.Options()

	if opts.WeakHashSelectionMethod == config.WeakHashAuto {
		perfWithWeakHash := cpuBench(3, 150*time.Millisecond, true)
		l.Infof("Hashing performance with weak hash is %.02f MB/s", perfWithWeakHash)
		perfWithoutWeakHash := cpuBench(3, 150*time.Millisecond, false)
		l.Infof("Hashing performance without weak hash is %.02f MB/s", perfWithoutWeakHash)

		if perfWithoutWeakHash*0.8 > perfWithWeakHash {
			l.Infof("Weak hash disabled, as it has an unacceptable performance impact.")
			weakhash.Enabled = false
		} else {
			l.Infof("Weak hash enabled, as it has an acceptable performance impact.")
			weakhash.Enabled = true
		}
	} else if opts.WeakHashSelectionMethod == config.WeakHashNever {
		l.Infof("Disabling weak hash")
		weakhash.Enabled = false
	} else if opts.WeakHashSelectionMethod == config.WeakHashAlways {
		l.Infof("Enabling weak hash")
		weakhash.Enabled = true
	}

	if (opts.MaxRecvKbps > 0 || opts.MaxSendKbps > 0) && !opts.LimitBandwidthInLan {
		lans, _ = osutil.GetLans()
		for _, lan := range opts.AlwaysLocalNets {
			_, ipnet, err := net.ParseCIDR(lan)
			if err != nil {
				l.Infoln("Network", lan, "is malformed:", err)
				continue
			}
			lans = append(lans, ipnet)
		}

		networks := make([]string, len(lans))
		for i, lan := range lans {
			networks[i] = lan.String()
		}
		l.Infoln("Local networks:", strings.Join(networks, ", "))
	}

	dbFile := locations[locDatabase]
	ldb, err := db.Open(dbFile)

	if err != nil {
		l.Fatalln("Cannot open database:", err, "- Is another copy of Syncthing already running?")
	}

	if runtimeOptions.resetDeltaIdxs {
		l.Infoln("Reinitializing delta index IDs")
		ldb.DropDeltaIndexIDs()
	}

	protectedFiles := []string{
		locations[locDatabase],
		locations[locConfigFile],
		locations[locCertFile],
		locations[locKeyFile],
	}

	// Remove database entries for folders that no longer exist in the config
	folders := cfg.Folders()
	for _, folder := range ldb.ListFolders() {
		if _, ok := folders[folder]; !ok {
			l.Infof("Cleaning data for dropped folder %q", folder)
			db.DropFolder(ldb, folder)
		}
	}

	if cfg.RawCopy().OriginalVersion == 15 {
		// The config version 15->16 migration is about handling ignores and
		// delta indexes and requires that we drop existing indexes that
		// have been incorrectly ignore filtered.
		ldb.DropDeltaIndexIDs()
	}
	if cfg.RawCopy().OriginalVersion < 19 {
		// Converts old symlink types to new in the entire database.
		ldb.ConvertSymlinkTypes()
	}

	m := model.NewModel(cfg, myID, "syncthing", Version, ldb, protectedFiles)

	if t := os.Getenv("STDEADLOCKTIMEOUT"); len(t) > 0 {
		it, err := strconv.Atoi(t)
		if err == nil {
			m.StartDeadlockDetector(time.Duration(it) * time.Second)
		}
	} else if !IsRelease || IsBeta {
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

	// Start connection management

	connectionsService := connections.NewService(cfg, myID, m, tlsCfg, cachedDiscovery, bepProtocolName, tlsDefaultCommonName, lans)
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
			cachedDiscovery.Add(gd, 5*time.Minute, time.Minute, globalDiscoveryPriority)
		}
	}

	if cfg.Options().LocalAnnEnabled {
		// v4 broadcasts
		bcd, err := discover.NewLocal(myID, fmt.Sprintf(":%d", cfg.Options().LocalAnnPort), connectionsService)
		if err != nil {
			l.Warnln("IPv4 local discovery:", err)
		} else {
			cachedDiscovery.Add(bcd, 0, 0, ipv4LocalDiscoveryPriority)
		}
		// v6 multicasts
		mcd, err := discover.NewLocal(myID, cfg.Options().LocalAnnMCAddr, connectionsService)
		if err != nil {
			l.Warnln("IPv6 local discovery:", err)
		} else {
			cachedDiscovery.Add(mcd, 0, 0, ipv6LocalDiscoveryPriority)
		}
	}

	// GUI

	setupGUI(mainService, cfg, m, defaultSub, diskSub, cachedDiscovery, connectionsService, errors, systemLog, runtimeOptions)

	if runtimeOptions.cpuProfile {
		f, err := os.Create(fmt.Sprintf("cpu-%d.pprof", os.Getpid()))
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	for _, device := range cfg.Devices() {
		if len(device.Name) > 0 {
			l.Infof("Device %s is %q at %v", device.DeviceID, device.Name, device.Addresses)
		}
	}

	// Candidate builds always run with usage reporting.

	if IsCandidate {
		l.Infoln("Anonymous usage reporting is always enabled for candidate releases.")
		opts.URAccepted = usageReportVersion
		// Unique ID will be set and config saved below if necessary.
	}

	if opts.URUniqueID == "" {
		opts.URUniqueID = rand.String(8)
		cfg.SetOptions(opts)
		cfg.Save()
	}

	usageReportingSvc := newUsageReportingService(cfg, m, connectionsService)
	mainService.Add(usageReportingSvc)

	if opts.RestartOnWakeup {
		go standbyMonitor()
	}

	// Candidate builds should auto upgrade. Make sure the option is set,
	// unless we are in a build where it's disabled or the STNOUPGRADE
	// environment variable is set.

	if IsCandidate && !upgrade.DisabledByCompilation && !noUpgradeFromEnv {
		l.Infoln("Automatic upgrade is always enabled for candidate releases.")
		if opts.AutoUpgradeIntervalH == 0 || opts.AutoUpgradeIntervalH > 24 {
			opts.AutoUpgradeIntervalH = 12
			// Set the option into the config as well, as the auto upgrade
			// loop expects to read a valid interval from there.
			cfg.SetOptions(opts)
			cfg.Save()
		}
		// We don't tweak the user's choice of upgrading to pre-releases or
		// not, as otherwise they cannot step off the candidate channel.
	}

	if opts.AutoUpgradeIntervalH > 0 {
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

	code := <-stop

	mainService.Stop()

	l.Infoln("Exiting")

	if runtimeOptions.cpuProfile {
		pprof.StopCPUProfile()
	}

	os.Exit(code)
}

func setupSignalHandling() {
	// Exit cleanly with "restarting" code on SIGHUP.

	restartSign := make(chan os.Signal, 1)
	sigHup := syscall.Signal(1)
	signal.Notify(restartSign, sigHup)
	go func() {
		<-restartSign
		stop <- exitRestarting
	}()

	// Exit with "success" code (no restart) on INT/TERM

	stopSign := make(chan os.Signal, 1)
	sigTerm := syscall.Signal(15)
	signal.Notify(stopSign, os.Interrupt, sigTerm)
	go func() {
		<-stopSign
		stop <- exitSuccess
	}()
}

func loadOrDefaultConfig() (*config.Wrapper, error) {
	cfgFile := locations[locConfigFile]
	cfg, err := config.Load(cfgFile, myID)

	if err != nil {
		cfg = defaultConfig(cfgFile)
	}

	return cfg, err
}

func loadConfigAtStartup() *config.Wrapper {
	cfgFile := locations[locConfigFile]
	cfg, err := config.Load(cfgFile, myID)
	if os.IsNotExist(err) {
		cfg := defaultConfig(cfgFile)
		cfg.Save()
		l.Infof("Default config saved. Edit %s to taste or use the GUI\n", cfg.ConfigPath())
	} else if err == io.EOF {
		l.Fatalln("Failed to load config: unexpected end of file. Truncated or empty configuration?")
	} else if err != nil {
		l.Fatalln("Failed to load config:", err)
	}

	if cfg.RawCopy().OriginalVersion != config.CurrentVersion {
		err = archiveAndSaveConfig(cfg)
		if err != nil {
			l.Fatalln("Config archive:", err)
		}
	}

	return cfg
}

func archiveAndSaveConfig(cfg *config.Wrapper) error {
	// Copy the existing config to an archive copy
	archivePath := cfg.ConfigPath() + fmt.Sprintf(".v%d", cfg.RawCopy().OriginalVersion)
	l.Infoln("Archiving a copy of old config file format at:", archivePath)
	if err := copyFile(cfg.ConfigPath(), archivePath); err != nil {
		return err
	}

	// Do a regular atomic config sve
	return cfg.Save()
}

func copyFile(src, dst string) error {
	bs, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(dst, bs, 0600); err != nil {
		// Attempt to clean up
		os.Remove(dst)
		return err
	}

	return nil
}

func startAuditing(mainService *suture.Supervisor, auditFile string) {

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
			auditFile = timestampedLoc(locAuditLog)
			auditFlags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		} else {
			auditFlags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		}
		fd, err = os.OpenFile(auditFile, auditFlags, 0600)
		if err != nil {
			l.Fatalln("Audit:", err)
		}
		auditDest = auditFile
	}

	auditService := newAuditService(fd)
	mainService.Add(auditService)

	// We wait for the audit service to fully start before we return, to
	// ensure we capture all events from the start.
	auditService.WaitForStart()

	l.Infoln("Audit log in", auditDest)
}

func setupGUI(mainService *suture.Supervisor, cfg *config.Wrapper, m *model.Model, defaultSub, diskSub events.BufferedSubscription, discoverer discover.CachingMux, connectionsService *connections.Service, errors, systemLog logger.Recorder, runtimeOptions RuntimeOptions) {
	guiCfg := cfg.GUI()

	if !guiCfg.Enabled {
		return
	}

	if guiCfg.InsecureAdminAccess {
		l.Warnln("Insecure admin access is enabled.")
	}

	cpu := newCPUService()
	mainService.Add(cpu)

	api := newAPIService(myID, cfg, locations[locHTTPSCertFile], locations[locHTTPSKeyFile], runtimeOptions.assetDir, m, defaultSub, diskSub, discoverer, connectionsService, errors, systemLog, cpu)
	cfg.Subscribe(api)
	mainService.Add(api)

	if cfg.Options().StartBrowser && !runtimeOptions.noBrowser && !runtimeOptions.stRestarting {
		// Can potentially block if the utility we are invoking doesn't
		// fork, and just execs, hence keep it in its own routine.
		<-api.startedOnce
		go openURL(guiCfg.URL())
	}
}

func defaultConfig(cfgFile string) *config.Wrapper {
	myName, _ := os.Hostname()

	var defaultFolder config.FolderConfiguration

	if !noDefaultFolder {
		l.Infoln("Default folder created and/or linked to new config")
		defaultFolder = config.NewFolderConfiguration("default", fs.FilesystemTypeBasic, locations[locDefFolder])
		defaultFolder.Label = "Default Folder"
		defaultFolder.RescanIntervalS = 60
		defaultFolder.FSWatcherDelayS = 10
		defaultFolder.MinDiskFree = config.Size{Value: 1, Unit: "%"}
		defaultFolder.Devices = []config.FolderDeviceConfiguration{{DeviceID: myID}}
		defaultFolder.AutoNormalize = true
		defaultFolder.MaxConflicts = -1
	} else {
		l.Infoln("We will skip creation of a default folder on first start since the proper envvar is set")
	}

	thisDevice := config.NewDeviceConfiguration(myID, myName)
	thisDevice.Addresses = []string{"dynamic"}

	newCfg := config.New(myID)
	if !noDefaultFolder {
		newCfg.Folders = []config.FolderConfiguration{defaultFolder}
	}
	newCfg.Devices = []config.DeviceConfiguration{thisDevice}

	port, err := getFreePort("127.0.0.1", 8384)
	if err != nil {
		l.Fatalln("get free port (GUI):", err)
	}
	newCfg.GUI.RawAddress = fmt.Sprintf("127.0.0.1:%d", port)

	port, err = getFreePort("0.0.0.0", 22000)
	if err != nil {
		l.Fatalln("get free port (BEP):", err)
	}
	if port == 22000 {
		newCfg.Options.ListenAddresses = []string{"default"}
	} else {
		newCfg.Options.ListenAddresses = []string{
			fmt.Sprintf("tcp://%s", net.JoinHostPort("0.0.0.0", strconv.Itoa(port))),
			"dynamic+https://relays.syncthing.net/endpoint",
		}
	}

	return config.Wrap(cfgFile, newCfg)
}

func resetDB() error {
	return os.RemoveAll(locations[locDatabase])
}

func restart() {
	l.Infoln("Restarting")
	stop <- exitRestarting
}

func shutdown() {
	l.Infoln("Shutting down")
	stop <- exitSuccess
}

func ensureDir(dir string, mode fs.FileMode) {
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)
	err := fs.MkdirAll(".", mode)
	if err != nil {
		l.Fatalln(err)
	}

	if fi, err := fs.Stat("."); err == nil {
		// Apprently the stat may fail even though the mkdirall passed. If it
		// does, we'll just assume things are in order and let other things
		// fail (like loading or creating the config...).
		currentMode := fi.Mode() & 0777
		if currentMode != mode {
			err := fs.Chmod(".", mode)
			// This can fail on crappy filesystems, nothing we can do about it.
			if err != nil {
				l.Warnln(err)
			}
		}
	}
}

// getFreePort returns a free TCP port fort listening on. The ports given are
// tried in succession and the first to succeed is returned. If none succeed,
// a random high port is returned.
func getFreePort(host string, ports ...int) (int, error) {
	for _, port := range ports {
		c, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err == nil {
			c.Close()
			return port, nil
		}
	}

	c, err := net.Listen("tcp", host+":0")
	if err != nil {
		return 0, err
	}
	addr := c.Addr().(*net.TCPAddr)
	c.Close()
	return addr.Port, nil
}

func standbyMonitor() {
	restartDelay := 60 * time.Second
	now := time.Now()
	for {
		time.Sleep(10 * time.Second)
		if time.Since(now) > 2*time.Minute {
			l.Infof("Paused state detected, possibly woke up from standby. Restarting in %v.", restartDelay)

			// We most likely just woke from standby. If we restart
			// immediately chances are we won't have networking ready. Give
			// things a moment to stabilize.
			time.Sleep(restartDelay)

			restart()
			return
		}
		now = time.Now()
	}
}

func autoUpgrade(cfg *config.Wrapper) {
	timer := time.NewTimer(0)
	sub := events.Default.Subscribe(events.DeviceConnected)
	for {
		select {
		case event := <-sub.C():
			data, ok := event.Data.(map[string]string)
			if !ok || data["clientName"] != "syncthing" || upgrade.CompareVersions(data["clientVersion"], Version) != upgrade.Newer {
				continue
			}
			l.Infof("Connected to device %s with a newer version (current %q < remote %q). Checking for upgrades.", data["id"], Version, data["clientVersion"])
		case <-timer.C:
		}

		opts := cfg.Options()
		checkInterval := time.Duration(opts.AutoUpgradeIntervalH) * time.Hour
		if checkInterval < time.Hour {
			// We shouldn't be here if AutoUpgradeIntervalH < 1, but for
			// safety's sake.
			checkInterval = time.Hour
		}

		rel, err := upgrade.LatestRelease(opts.ReleasesURL, Version, opts.UpgradeToPreReleases)
		if err == upgrade.ErrUpgradeUnsupported {
			events.Default.Unsubscribe(sub)
			return
		}
		if err != nil {
			// Don't complain too loudly here; we might simply not have
			// internet connectivity, or the upgrade server might be down.
			l.Infoln("Automatic upgrade:", err)
			timer.Reset(checkInterval)
			continue
		}

		if upgrade.CompareVersions(rel.Tag, Version) != upgrade.Newer {
			// Skip equal, older or majorly newer (incompatible) versions
			timer.Reset(checkInterval)
			continue
		}

		l.Infof("Automatic upgrade (current %q < latest %q)", Version, rel.Tag)
		err = upgrade.To(rel)
		if err != nil {
			l.Warnln("Automatic upgrade:", err)
			timer.Reset(checkInterval)
			continue
		}
		events.Default.Unsubscribe(sub)
		l.Warnf("Automatically upgraded to version %q. Restarting in 1 minute.", rel.Tag)
		time.Sleep(time.Minute)
		stop <- exitUpgrading
		return
	}
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
	}

	for pat, dur := range patterns {
		fs := fs.NewFilesystem(fs.FilesystemTypeBasic, baseDirs["config"])
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

// checkShortIDs verifies that the configuration won't result in duplicate
// short ID:s; that is, that the devices in the cluster all have unique
// initial 64 bits.
func checkShortIDs(cfg *config.Wrapper) error {
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

func showPaths() {
	fmt.Printf("Configuration file:\n\t%s\n\n", locations[locConfigFile])
	fmt.Printf("Database directory:\n\t%s\n\n", locations[locDatabase])
	fmt.Printf("Device private key & certificate files:\n\t%s\n\t%s\n\n", locations[locKeyFile], locations[locCertFile])
	fmt.Printf("HTTPS private key & certificate files:\n\t%s\n\t%s\n\n", locations[locHTTPSKeyFile], locations[locHTTPSCertFile])
	fmt.Printf("Log file:\n\t%s\n\n", locations[locLogFile])
	fmt.Printf("GUI override directory:\n\t%s\n\n", locations[locGUIAssets])
	fmt.Printf("Default sync folder directory:\n\t%s\n\n", locations[locDefFolder])
}

func setPauseState(cfg *config.Wrapper, paused bool) {
	raw := cfg.RawCopy()
	for i := range raw.Devices {
		raw.Devices[i].Paused = paused
	}
	for i := range raw.Folders {
		raw.Folders[i].Paused = paused
	}
	if err := cfg.Replace(raw); err != nil {
		l.Fatalln("Cannot adjust paused state:", err)
	}
}
