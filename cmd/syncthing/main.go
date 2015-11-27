// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay"
	"github.com/syncthing/syncthing/lib/symlinks"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/upgrade"

	"github.com/thejerf/suture"
)

var (
	Version     = "unknown-dev"
	Codename    = "Beryllium Bedbug"
	BuildEnv    = "default"
	BuildStamp  = "0"
	BuildDate   time.Time
	BuildHost   = "unknown"
	BuildUser   = "unknown"
	IsRelease   bool
	IsBeta      bool
	LongVersion string
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
	pingEventInterval    = time.Minute
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
		exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9]+)*(\+\d+-g[0-9a-f]+)?(-dirty)?$`)
		if !exp.MatchString(Version) {
			l.Fatalf("Invalid version string %q;\n\tdoes not match regexp %v", Version, exp)
		}
	}

	// Check for a clean release build. A release is something like "v0.1.2",
	// with an optional suffix of letters and dot separated numbers like
	// "-beta3.47". If there's more stuff, like a plus sign and a commit hash
	// and so on, then it's not a release. If there's a dash anywhere in
	// there, it's some kind of beta or prerelease version.

	exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z]+[\d\.]+)?$`)
	IsRelease = exp.MatchString(Version)
	IsBeta = strings.Contains(Version, "-")

	stamp, _ := strconv.Atoi(BuildStamp)
	BuildDate = time.Unix(int64(stamp), 0)

	date := BuildDate.UTC().Format("2006-01-02 15:04:05 MST")
	LongVersion = fmt.Sprintf(`syncthing %s "%s" (%s %s-%s %s) %s@%s %s`, Version, Codename, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildEnv, BuildUser, BuildHost, date)

	if os.Getenv("STTRACE") != "" {
		logFlags = log.Ltime | log.Ldate | log.Lmicroseconds | log.Lshortfile
	}
}

var (
	myID     protocol.DeviceID
	confDir  string
	logFlags = log.Ltime
	stop     = make(chan int)
	cert     tls.Certificate
	lans     []*net.IPNet
)

const (
	usage      = "syncthing [options]"
	extraUsage = `
The default configuration directory is:

  %s


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

The following environment variables modify syncthing's behavior in ways that
are mostly useful for developers. Use with care.

 STGUIASSETS     Directory to load GUI assets from. Overrides compiled in
                 assets.

 STTRACE         A comma separated string of facilities to trace. The valid
                 facility strings listed below.

 STPROFILER      Set to a listen address such as "127.0.0.1:9090" to start the
                 profiler with HTTP access.

 STCPUPROFILE    Write a CPU profile to cpu-$pid.pprof on exit.

 STHEAPPROFILE   Write heap profiles to heap-$pid-$timestamp.pprof each time
                 heap usage increases.

 STBLOCKPROFILE  Write block profiles to block-$pid-$timestamp.pprof every 20
                 seconds.

 STPERFSTATS     Write running performance statistics to perf-$pid.csv. Not
                 supported on Windows.

 STNOUPGRADE     Disable automatic upgrades.

 GOMAXPROCS      Set the maximum number of CPU cores to use. Defaults to all
                 available CPU cores.

 GOGC            Percentage of heap growth at which to trigger GC. Default is
                 100. Lower numbers keep peak memory usage down, at the price
                 of CPU usage (ie. performance).


Debugging Facilities
--------------------

The following are valid values for the STTRACE variable:

%s`
)

// Command line and environment options
var (
	reset          bool
	showVersion    bool
	doUpgrade      bool
	doUpgradeCheck bool
	upgradeTo      string
	noBrowser      bool
	noConsole      bool
	logFile        string
	auditEnabled   bool
	verbose        bool
	paused         bool
	noRestart      = os.Getenv("STNORESTART") != ""
	noUpgrade      = os.Getenv("STNOUPGRADE") != ""
	profiler       = os.Getenv("STPROFILER")
	guiAssets      = os.Getenv("STGUIASSETS")
	cpuProfile     = os.Getenv("STCPUPROFILE") != ""
	stRestarting   = os.Getenv("STRESTART") != ""
	innerProcess   = os.Getenv("STNORESTART") != "" || os.Getenv("STMONITORED") != ""
)

func main() {
	if runtime.GOOS == "windows" {
		// On Windows, we use a log file by default. Setting the -logfile flag
		// to "-" disables this behavior.
		flag.StringVar(&logFile, "logfile", "", "Log file name (use \"-\" for stdout)")

		// We also add an option to hide the console window
		flag.BoolVar(&noConsole, "no-console", false, "Hide console window")
	} else {
		flag.StringVar(&logFile, "logfile", "-", "Log file name (use \"-\" for stdout)")
	}

	var guiAddress, guiAPIKey string
	var generateDir string
	flag.StringVar(&generateDir, "generate", "", "Generate key and config in specified dir, then exit")
	flag.StringVar(&guiAddress, "gui-address", guiAddress, "Override GUI address")
	flag.StringVar(&guiAPIKey, "gui-apikey", guiAPIKey, "Override GUI API key")
	flag.StringVar(&confDir, "home", "", "Set configuration directory")
	flag.IntVar(&logFlags, "logflags", logFlags, "Select information in log line prefix")
	flag.BoolVar(&noBrowser, "no-browser", false, "Do not start browser")
	flag.BoolVar(&noRestart, "no-restart", noRestart, "Do not restart; just exit")
	flag.BoolVar(&reset, "reset", false, "Reset the database")
	flag.BoolVar(&doUpgrade, "upgrade", false, "Perform upgrade")
	flag.BoolVar(&doUpgradeCheck, "upgrade-check", false, "Check for available upgrade")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.StringVar(&upgradeTo, "upgrade-to", upgradeTo, "Force upgrade directly from specified URL")
	flag.BoolVar(&auditEnabled, "audit", false, "Write events to audit file")
	flag.BoolVar(&verbose, "verbose", false, "Print verbose log output")
	flag.BoolVar(&paused, "paused", false, "Start with all devices paused")

	longUsage := fmt.Sprintf(extraUsage, baseDirs["config"], debugFacilities())
	flag.Usage = usageFor(flag.CommandLine, usage, longUsage)
	flag.Parse()

	if guiAddress != "" {
		// The config picks this up from the environment.
		os.Setenv("STGUIADDRESS", guiAddress)
	}
	if guiAPIKey != "" {
		// The config picks this up from the environment.
		os.Setenv("STGUIAPIKEY", guiAPIKey)
	}

	if noConsole {
		osutil.HideConsole()
	}

	if confDir != "" {
		// Not set as default above because the string can be really long.
		baseDirs["config"] = confDir
	}

	if err := expandLocations(); err != nil {
		l.Fatalln(err)
	}

	if guiAssets == "" {
		guiAssets = locations[locGUIAssets]
	}

	if logFile == "" {
		// Use the default log file location
		logFile = locations[locLogFile]
	}

	if showVersion {
		fmt.Println(LongVersion)
		return
	}

	l.SetFlags(logFlags)

	if generateDir != "" {
		generate(generateDir)
		return
	}

	if info, err := os.Stat(baseDirs["config"]); err == nil && !info.IsDir() {
		l.Fatalln("Config directory", baseDirs["config"], "is not a directory")
	}

	// Ensure that our home directory exists.
	ensureDir(baseDirs["config"], 0700)

	if upgradeTo != "" {
		err := upgrade.ToURL(upgradeTo)
		if err != nil {
			l.Fatalln("Upgrade:", err) // exits 1
		}
		l.Okln("Upgraded from", upgradeTo)
		return
	}

	if doUpgrade || doUpgradeCheck {
		releasesURL := "https://api.github.com/repos/syncthing/syncthing/releases?per_page=30"
		if cfg, _, err := loadConfig(locations[locConfigFile]); err == nil {
			releasesURL = cfg.Options().ReleasesURL
		}
		rel, err := upgrade.LatestRelease(releasesURL, Version)
		if err != nil {
			l.Fatalln("Upgrade:", err) // exits 1
		}

		if upgrade.CompareVersions(rel.Tag, Version) <= 0 {
			l.Infof("No upgrade available (current %q >= latest %q).", Version, rel.Tag)
			os.Exit(exitNoUpgradeAvailable)
		}

		l.Infof("Upgrade available (current %q < latest %q)", Version, rel.Tag)

		if doUpgrade {
			// Use leveldb database locks to protect against concurrent upgrades
			_, err = db.Open(locations[locDatabase])
			if err != nil {
				l.Infoln("Attempting upgrade through running Syncthing...")
				err = upgradeViaRest()
				if err != nil {
					l.Fatalln("Upgrade:", err)
				}
				l.Okln("Syncthing upgrading")
				return
			}

			err = upgrade.To(rel)
			if err != nil {
				l.Fatalln("Upgrade:", err) // exits 1
			}
			l.Okf("Upgraded to %q", rel.Tag)
		}

		return
	}

	if reset {
		resetDB()
		return
	}

	if noRestart {
		syncthingMain()
	} else {
		monitorMain()
	}
}

func generate(generateDir string) {
	dir, err := osutil.ExpandTilde(generateDir)
	if err != nil {
		l.Fatalln("generate:", err)
	}

	info, err := os.Stat(dir)
	if err == nil && !info.IsDir() {
		l.Fatalln(dir, "is not a directory")
	}
	if err != nil && os.IsNotExist(err) {
		err = osutil.MkdirAll(dir, 0700)
		if err != nil {
			l.Fatalln("generate:", err)
		}
	}

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
	var myName, _ = os.Hostname()
	var newCfg = defaultConfig(myName)
	var cfg = config.Wrap(cfgFile, newCfg)
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

func upgradeViaRest() error {
	cfg, err := config.Load(locations[locConfigFile], protocol.LocalDeviceID)
	if err != nil {
		return err
	}
	target := cfg.GUI().URL()
	r, _ := http.NewRequest("POST", target+"/rest/system/upgrade", nil)
	r.Header.Set("X-API-Key", cfg.GUI().APIKey())

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

func syncthingMain() {
	// Create a main service manager. We'll add things to this as we go along.
	// We want any logging it does to go through our log system.
	mainSvc := suture.New("main", suture.Spec{
		Log: func(line string) {
			l.Debugln(line)
		},
	})
	mainSvc.ServeBackground()

	// Set a log prefix similar to the ID we will have later on, or early log
	// lines look ugly.
	l.SetPrefix("[start] ")

	if auditEnabled {
		startAuditing(mainSvc)
	}

	if verbose {
		mainSvc.Add(newVerboseSvc())
	}

	errors := logger.NewRecorder(l, logger.LevelWarn, maxSystemErrors, 0)
	systemLog := logger.NewRecorder(l, logger.LevelDebug, maxSystemLog, initialSystemLog)

	// Event subscription for the API; must start early to catch the early events.
	apiSub := events.NewBufferedSubscription(events.Default.Subscribe(events.AllEvents), 1000)

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Attempt to increase the limit on number of open files to the maximum
	// allowed, in case we have many peers. We don't really care enough to
	// report the error if there is one.
	osutil.MaximizeOpenFileLimit()

	// Ensure that that we have a certificate and key.
	cert, err := tls.LoadX509KeyPair(locations[locCertFile], locations[locKeyFile])
	if err != nil {
		l.Infof("Generating ECDSA key and certificate for %s...", tlsDefaultCommonName)
		cert, err = tlsutil.NewCertificate(locations[locCertFile], locations[locKeyFile], tlsDefaultCommonName, bepRSABits)
		if err != nil {
			l.Fatalln(err)
		}
	}

	// We reinitialize the predictable RNG with our device ID, to get a
	// sequence that is always the same but unique to this syncthing instance.
	predictableRandom.Seed(seedFromBytes(cert.Certificate[0]))

	myID = protocol.NewDeviceID(cert.Certificate[0])
	l.SetPrefix(fmt.Sprintf("[%s] ", myID.String()[:5]))

	l.Infoln(LongVersion)
	l.Infoln("My ID:", myID)
	printHashRate()

	// Emit the Starting event, now that we know who we are.

	events.Default.Log(events.Starting, map[string]string{
		"home": baseDirs["config"],
		"myID": myID.String(),
	})

	// Prepare to be able to save configuration

	cfgFile := locations[locConfigFile]

	// Load the configuration file, if it exists.
	// If it does not, create a template.

	cfg, myName, err := loadConfig(cfgFile)
	if err != nil {
		if os.IsNotExist(err) {
			l.Infoln("No config file; starting with empty defaults")
			myName, _ = os.Hostname()
			newCfg := defaultConfig(myName)
			cfg = config.Wrap(cfgFile, newCfg)
			cfg.Save()
			l.Infof("Edit %s to taste or use the GUI\n", cfgFile)
		} else {
			l.Fatalln("Loading config:", err)
		}
	}

	if cfg.Raw().OriginalVersion != config.CurrentVersion {
		l.Infoln("Archiving a copy of old config file format")
		// Archive a copy
		osutil.Rename(cfgFile, cfgFile+fmt.Sprintf(".v%d", cfg.Raw().OriginalVersion))
		// Save the new version
		cfg.Save()
	}

	if err := checkShortIDs(cfg); err != nil {
		l.Fatalln("Short device IDs are in conflict. Unlucky!\n  Regenerate the device ID of one if the following:\n  ", err)
	}

	if len(profiler) > 0 {
		go func() {
			l.Debugln("Starting profiler on", profiler)
			runtime.SetBlockProfileRate(1)
			err := http.ListenAndServe(profiler, nil)
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
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
	}

	// If the read or write rate should be limited, set up a rate limiter for it.
	// This will be used on connections created in the connect and listen routines.

	opts := cfg.Options()

	if !opts.SymlinksEnabled {
		symlinks.Supported = false
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

	// Pack and optimize the database
	if err := ldb.Compact(); err != nil {
		// I don't think this is fatal, but who knows. If it is, we'll surely
		// get an error when trying to write to the db later.
		l.Infoln("Compacting database:", err)
	}

	m := model.NewModel(cfg, myID, myName, "syncthing", Version, ldb, protectedFiles)
	cfg.Subscribe(m)

	if t := os.Getenv("STDEADLOCKTIMEOUT"); len(t) > 0 {
		it, err := strconv.Atoi(t)
		if err == nil {
			m.StartDeadlockDetector(time.Duration(it) * time.Second)
		}
	} else if !IsRelease || IsBeta {
		m.StartDeadlockDetector(20 * time.Minute)
	}

	if paused {
		for device := range cfg.Devices() {
			m.PauseDevice(device)
		}
	}

	// Clear out old indexes for other devices. Otherwise we'll start up and
	// start needing a bunch of files which are nowhere to be found. This
	// needs to be changed when we correctly do persistent indexes.
	for _, folderCfg := range cfg.Folders() {
		m.AddFolder(folderCfg)
		for _, device := range folderCfg.DeviceIDs() {
			if device == myID {
				continue
			}
			m.Index(device, folderCfg.ID, nil, 0, nil)
		}
		// Routine to pull blocks from other devices to synchronize the local
		// folder. Does not run when we are in read only (publish only) mode.
		if folderCfg.ReadOnly {
			m.StartFolderRO(folderCfg.ID)
		} else {
			m.StartFolderRW(folderCfg.ID)
		}
	}

	mainSvc.Add(m)

	// The default port we announce, possibly modified by setupUPnP next.

	uri, err := url.Parse(opts.ListenAddress[0])
	if err != nil {
		l.Fatalf("Failed to parse listen address %s: %v", opts.ListenAddress[0], err)
	}

	addr, err := net.ResolveTCPAddr("tcp", uri.Host)
	if err != nil {
		l.Fatalln("Bad listen address:", err)
	}

	// The externalAddr tracks our external addresses for discovery purposes.

	var addrList *addressLister

	// Start UPnP

	if opts.UPnPEnabled {
		upnpSvc := newUPnPSvc(cfg, addr.Port)
		mainSvc.Add(upnpSvc)

		// The external address tracker needs to know about the UPnP service
		// so it can check for an external mapped port.
		addrList = newAddressLister(upnpSvc, cfg)
	} else {
		addrList = newAddressLister(nil, cfg)
	}

	// Start relay management

	var relaySvc *relay.Svc
	if opts.RelaysEnabled && (opts.GlobalAnnEnabled || opts.RelayWithoutGlobalAnn) {
		relaySvc = relay.NewSvc(cfg, tlsCfg)
		mainSvc.Add(relaySvc)
	}

	// Start discovery

	cachedDiscovery := discover.NewCachingMux()
	mainSvc.Add(cachedDiscovery)

	if cfg.Options().GlobalAnnEnabled {
		for _, srv := range cfg.GlobalDiscoveryServers() {
			l.Infoln("Using discovery server", srv)
			gd, err := discover.NewGlobal(srv, cert, addrList, relaySvc)
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
		bcd, err := discover.NewLocal(myID, fmt.Sprintf(":%d", cfg.Options().LocalAnnPort), addrList, relaySvc)
		if err != nil {
			l.Warnln("IPv4 local discovery:", err)
		} else {
			cachedDiscovery.Add(bcd, 0, 0, ipv4LocalDiscoveryPriority)
		}
		// v6 multicasts
		mcd, err := discover.NewLocal(myID, cfg.Options().LocalAnnMCAddr, addrList, relaySvc)
		if err != nil {
			l.Warnln("IPv6 local discovery:", err)
		} else {
			cachedDiscovery.Add(mcd, 0, 0, ipv6LocalDiscoveryPriority)
		}
	}

	// GUI

	setupGUI(mainSvc, cfg, m, apiSub, cachedDiscovery, relaySvc, errors, systemLog)

	// Start connection management

	connectionSvc := connections.NewConnectionSvc(cfg, myID, m, tlsCfg, cachedDiscovery, relaySvc, bepProtocolName, tlsDefaultCommonName, lans)
	mainSvc.Add(connectionSvc)

	if cpuProfile {
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

	if opts.URAccepted > 0 && opts.URAccepted < usageReportVersion {
		l.Infoln("Anonymous usage report has changed; revoking acceptance")
		opts.URAccepted = 0
		opts.URUniqueID = ""
		cfg.SetOptions(opts)
	}
	if opts.URAccepted >= usageReportVersion {
		if opts.URUniqueID == "" {
			// Previously the ID was generated from the node ID. We now need
			// to generate a new one.
			opts.URUniqueID = randomString(8)
			cfg.SetOptions(opts)
			cfg.Save()
		}
	}

	// The usageReportingManager registers itself to listen to configuration
	// changes, and there's nothing more we need to tell it from the outside.
	// Hence we don't keep the returned pointer.
	newUsageReportingManager(cfg, m)

	if opts.RestartOnWakeup {
		go standbyMonitor()
	}

	if opts.AutoUpgradeIntervalH > 0 {
		if noUpgrade {
			l.Infof("No automatic upgrades; STNOUPGRADE environment variable defined.")
		} else if IsRelease {
			go autoUpgrade(cfg)
		} else {
			l.Infof("No automatic upgrades; %s is not a release version.", Version)
		}
	}

	events.Default.Log(events.StartupComplete, map[string]string{
		"myID": myID.String(),
	})
	go generatePingEvents()

	cleanConfigDirectory()

	code := <-stop

	mainSvc.Stop()

	l.Okln("Exiting")

	if cpuProfile {
		pprof.StopCPUProfile()
	}

	os.Exit(code)
}

// printHashRate prints the hashing performance in MB/s, formatting it with
// appropriate precision for the value, i.e. 182 MB/s, 18 MB/s, 1.8 MB/s, 0.18
// MB/s.
func printHashRate() {
	hashRate := cpuBench(3, 100*time.Millisecond)

	decimals := 0
	if hashRate < 1 {
		decimals = 2
	} else if hashRate < 10 {
		decimals = 1
	}

	l.Infof("Single thread hash performance is ~%.*f MB/s", decimals, hashRate)
}

func loadConfig(cfgFile string) (*config.Wrapper, string, error) {
	info, err := os.Stat(cfgFile)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("configuration is not a file")
	}

	cfg, err := config.Load(cfgFile, myID)
	if err != nil {
		return nil, "", err
	}

	myCfg := cfg.Devices()[myID]
	myName := myCfg.Name
	if myName == "" {
		myName, _ = os.Hostname()
	}

	return cfg, myName, nil
}

func startAuditing(mainSvc *suture.Supervisor) {
	auditFile := timestampedLoc(locAuditLog)
	fd, err := os.OpenFile(auditFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		l.Fatalln("Audit:", err)
	}

	auditSvc := newAuditSvc(fd)
	mainSvc.Add(auditSvc)

	// We wait for the audit service to fully start before we return, to
	// ensure we capture all events from the start.
	auditSvc.WaitForStart()

	l.Infoln("Audit log in", auditFile)
}

func setupGUI(mainSvc *suture.Supervisor, cfg *config.Wrapper, m *model.Model, apiSub *events.BufferedSubscription, discoverer *discover.CachingMux, relaySvc *relay.Svc, errors, systemLog *logger.Recorder) {
	guiCfg := cfg.GUI()

	if !guiCfg.Enabled {
		return
	}

	if guiCfg.InsecureAdminAccess {
		l.Warnln("Insecure admin access is enabled.")
	}

	api, err := newAPISvc(myID, cfg, guiAssets, m, apiSub, discoverer, relaySvc, errors, systemLog)
	if err != nil {
		l.Fatalln("Cannot start GUI:", err)
	}
	cfg.Subscribe(api)
	mainSvc.Add(api)

	if cfg.Options().StartBrowser && !noBrowser && !stRestarting {
		// Can potentially block if the utility we are invoking doesn't
		// fork, and just execs, hence keep it in it's own routine.
		go openURL(guiCfg.URL())
	}
}

func defaultConfig(myName string) config.Configuration {
	defaultFolder := config.NewFolderConfiguration("default", locations[locDefFolder])
	defaultFolder.RescanIntervalS = 60
	defaultFolder.MinDiskFreePct = 1
	defaultFolder.Devices = []config.FolderDeviceConfiguration{{DeviceID: myID}}
	defaultFolder.AutoNormalize = true
	defaultFolder.MaxConflicts = -1

	thisDevice := config.NewDeviceConfiguration(myID, myName)
	thisDevice.Addresses = []string{"dynamic"}

	newCfg := config.New(myID)
	newCfg.Folders = []config.FolderConfiguration{defaultFolder}
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
	newCfg.Options.ListenAddress = []string{fmt.Sprintf("tcp://0.0.0.0:%d", port)}
	return newCfg
}

func generatePingEvents() {
	for {
		time.Sleep(pingEventInterval)
		events.Default.Log(events.Ping, nil)
	}
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

func ensureDir(dir string, mode int) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err := osutil.MkdirAll(dir, 0700)
		if err != nil {
			l.Fatalln(err)
		}
	} else if mode >= 0 && err == nil && int(fi.Mode()&0777) != mode {
		err := os.Chmod(dir, os.FileMode(mode))
		// This can fail on crappy filesystems, nothing we can do about it.
		if err != nil {
			l.Warnln(err)
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
	restartDelay := time.Duration(60 * time.Second)
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

		rel, err := upgrade.LatestRelease(cfg.Options().ReleasesURL, Version)
		if err == upgrade.ErrUpgradeUnsupported {
			events.Default.Unsubscribe(sub)
			return
		}
		if err != nil {
			// Don't complain too loudly here; we might simply not have
			// internet connectivity, or the upgrade server might be down.
			l.Infoln("Automatic upgrade:", err)
			timer.Reset(time.Duration(cfg.Options().AutoUpgradeIntervalH) * time.Hour)
			continue
		}

		if upgrade.CompareVersions(rel.Tag, Version) != upgrade.Newer {
			// Skip equal, older or majorly newer (incompatible) versions
			timer.Reset(time.Duration(cfg.Options().AutoUpgradeIntervalH) * time.Hour)
			continue
		}

		l.Infof("Automatic upgrade (current %q < latest %q)", Version, rel.Tag)
		err = upgrade.To(rel)
		if err != nil {
			l.Warnln("Automatic upgrade:", err)
			timer.Reset(time.Duration(cfg.Options().AutoUpgradeIntervalH) * time.Hour)
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
		"panic-*.log":    7 * 24 * time.Hour,  // keep panic logs for a week
		"audit-*.log":    7 * 24 * time.Hour,  // keep audit logs for a week
		"index":          14 * 24 * time.Hour, // keep old index format for two weeks
		"config.xml.v*":  30 * 24 * time.Hour, // old config versions for a month
		"*.idx.gz":       30 * 24 * time.Hour, // these should for sure no longer exist
		"backup-of-v0.8": 30 * 24 * time.Hour, // these neither
	}

	for pat, dur := range patterns {
		pat = filepath.Join(baseDirs["config"], pat)
		files, err := osutil.Glob(pat)
		if err != nil {
			l.Infoln("Cleaning:", err)
			continue
		}

		for _, file := range files {
			info, err := osutil.Lstat(file)
			if err != nil {
				l.Infoln("Cleaning:", err)
				continue
			}

			if time.Since(info.ModTime()) > dur {
				if err = os.RemoveAll(file); err != nil {
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
	exists := make(map[uint64]protocol.DeviceID)
	for deviceID := range cfg.Devices() {
		shortID := deviceID.Short()
		if otherID, ok := exists[shortID]; ok {
			return fmt.Errorf("%v in conflict with %v", deviceID, otherID)
		}
		exists[shortID] = deviceID
	}
	return nil
}
