// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/calmh/logger"
	"github.com/juju/ratelimit"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/discover"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/files"
	"github.com/syncthing/syncthing/internal/model"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syncthing/syncthing/internal/upgrade"
	"github.com/syncthing/syncthing/internal/upnp"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var (
	Version     = "unknown-dev"
	BuildEnv    = "default"
	BuildStamp  = "0"
	BuildDate   time.Time
	BuildHost   = "unknown"
	BuildUser   = "unknown"
	IsRelease   bool
	IsBeta      bool
	LongVersion string
	GoArchExtra string // "", "v5", "v6", "v7"
)

const (
	exitSuccess            = 0
	exitError              = 1
	exitNoUpgradeAvailable = 2
	exitRestarting         = 3
	exitUpgrading          = 4
)

var l = logger.DefaultLogger

func init() {
	if Version != "unknown-dev" {
		// If not a generic dev build, version string should come from git describe
		exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9]+)*(\+\d+-g[0-9a-f]+)?(-dirty)?$`)
		if !exp.MatchString(Version) {
			l.Fatalf("Invalid version string %q;\n\tdoes not match regexp %v", Version, exp)
		}
	}

	// Check for a clean release build.
	exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-beta[\d\.]+)?$`)
	IsRelease = exp.MatchString(Version)

	// Check for a beta build
	IsBeta = strings.Contains(Version, "-beta")

	stamp, _ := strconv.Atoi(BuildStamp)
	BuildDate = time.Unix(int64(stamp), 0)

	date := BuildDate.UTC().Format("2006-01-02 15:04:05 MST")
	LongVersion = fmt.Sprintf("syncthing %s (%s %s-%s %s) %s@%s %s", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildEnv, BuildUser, BuildHost, date)

	if os.Getenv("STTRACE") != "" {
		logFlags = log.Ltime | log.Ldate | log.Lmicroseconds | log.Lshortfile
	}
}

var (
	cfg            *config.ConfigWrapper
	myID           protocol.DeviceID
	confDir        string
	logFlags       int = log.Ltime
	writeRateLimit *ratelimit.Bucket
	readRateLimit  *ratelimit.Bucket
	stop           = make(chan int)
	discoverer     *discover.Discoverer
	externalPort   int
	igd            *upnp.IGD
	cert           tls.Certificate
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

 STGUIASSETS   Directory to load GUI assets from. Overrides compiled in assets.

 STTRACE       A comma separated string of facilities to trace. The valid
               facility strings are:

               - "beacon"   (the beacon package)
               - "discover" (the discover package)
               - "events"   (the events package)
               - "files"    (the files package)
               - "net"      (the main package; connections & network messages)
               - "model"    (the model package)
               - "scanner"  (the scanner package)
               - "stats"    (the stats package)
               - "upnp"     (the upnp package)
               - "xdr"      (the xdr package)
               - "all"      (all of the above)

 STPROFILER    Set to a listen address such as "127.0.0.1:9090" to start the
               profiler with HTTP access.

 STCPUPROFILE  Write a CPU profile to cpu-$pid.pprof on exit.

 STHEAPPROFILE Write heap profiles to heap-$pid-$timestamp.pprof each time
               heap usage increases.

 STPERFSTATS   Write running performance statistics to perf-$pid.csv. Not
               supported on Windows.

 GOMAXPROCS    Set the maximum number of CPU cores to use. Defaults to all
               available CPU cores.`
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Command line and environment options
var (
	reset             bool
	showVersion       bool
	doUpgrade         bool
	doUpgradeCheck    bool
	noBrowser         bool
	generateDir       string
	logFile           string
	noRestart         = os.Getenv("STNORESTART") != ""
	guiAddress        = os.Getenv("STGUIADDRESS") // legacy
	guiAuthentication = os.Getenv("STGUIAUTH")    // legacy
	guiAPIKey         = os.Getenv("STGUIAPIKEY")  // legacy
	profiler          = os.Getenv("STPROFILER")
	guiAssets         = os.Getenv("STGUIASSETS")
	cpuProfile        = os.Getenv("STCPUPROFILE") != ""
	stRestarting      = os.Getenv("STRESTART") != ""
	innerProcess      = os.Getenv("STNORESTART") != "" || os.Getenv("STMONITORED") != ""
)

func main() {
	defConfDir, err := getDefaultConfDir()
	if err != nil {
		l.Fatalln("home:", err)
	}

	if runtime.GOOS == "windows" {
		// On Windows, we use a log file by default. Setting the -logfile flag
		// to the empty string disables this behavior.

		logFile = filepath.Join(defConfDir, "syncthing.log")
		flag.StringVar(&logFile, "logfile", logFile, "Log file name (blank for stdout)")
	}

	flag.StringVar(&generateDir, "generate", "", "Generate key and config in specified dir, then exit")
	flag.StringVar(&guiAddress, "gui-address", guiAddress, "Override GUI address")
	flag.StringVar(&guiAuthentication, "gui-authentication", guiAuthentication, "Override GUI authentication; username:password")
	flag.StringVar(&guiAPIKey, "gui-apikey", guiAPIKey, "Override GUI API key")
	flag.StringVar(&confDir, "home", "", "Set configuration directory")
	flag.IntVar(&logFlags, "logflags", logFlags, "Select information in log line prefix")
	flag.BoolVar(&noBrowser, "no-browser", false, "Do not start browser")
	flag.BoolVar(&noRestart, "no-restart", noRestart, "Do not restart; just exit")
	flag.BoolVar(&reset, "reset", false, "Prepare to resync from cluster")
	flag.BoolVar(&doUpgrade, "upgrade", false, "Perform upgrade")
	flag.BoolVar(&doUpgradeCheck, "upgrade-check", false, "Check for available upgrade")
	flag.BoolVar(&showVersion, "version", false, "Show version")

	flag.Usage = usageFor(flag.CommandLine, usage, fmt.Sprintf(extraUsage, defConfDir))
	flag.Parse()

	if confDir == "" {
		// Not set as default above because the string can be really long.
		confDir = defConfDir
	}

	if confDir != defConfDir && filepath.Dir(logFile) == defConfDir {
		// The user changed the config dir with -home, but not the log file
		// location. In this case we assume they meant for the logfile to
		// still live in it's default location *relative to the config dir*.
		logFile = filepath.Join(confDir, "syncthing.log")
	}

	if showVersion {
		fmt.Println(LongVersion)
		return
	}

	l.SetFlags(logFlags)

	if generateDir != "" {
		dir, err := osutil.ExpandTilde(generateDir)
		if err != nil {
			l.Fatalln("generate:", err)
		}

		info, err := os.Stat(dir)
		if err != nil {
			l.Fatalln("generate:", err)
		}
		if !info.IsDir() {
			l.Fatalln(dir, "is not a directory")
		}

		cert, err := loadCert(dir, "")
		if err == nil {
			l.Warnln("Key exists; will not overwrite.")
			l.Infoln("Device ID:", protocol.NewDeviceID(cert.Certificate[0]))
		} else {
			newCertificate(dir, "")
			cert, err = loadCert(dir, "")
			myID = protocol.NewDeviceID(cert.Certificate[0])
			if err != nil {
				l.Fatalln("load cert:", err)
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

		return
	}

	confDir, err := osutil.ExpandTilde(confDir)
	if err != nil {
		l.Fatalln("home:", err)
	}

	if info, err := os.Stat(confDir); err == nil && !info.IsDir() {
		l.Fatalln("Config directory", confDir, "is not a directory")
	}

	// Ensure that our home directory exists.
	ensureDir(confDir, 0700)

	if doUpgrade || doUpgradeCheck {
		rel, err := upgrade.LatestRelease(IsBeta)
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
			_, err = leveldb.OpenFile(filepath.Join(confDir, "index"), &opt.Options{CachedOpenFiles: 100})
			if err != nil {
				l.Fatalln("Cannot upgrade, database seems to be locked. Is another copy of Syncthing already running?")
			}

			err = upgrade.UpgradeTo(rel, GoArchExtra)
			if err != nil {
				l.Fatalln("Upgrade:", err) // exits 1
			}
			l.Okf("Upgraded to %q", rel.Tag)
			return
		} else {
			return
		}
	}

	if reset {
		resetFolders()
		return
	}

	if noRestart {
		syncthingMain()
	} else {
		monitorMain()
	}
}

func syncthingMain() {
	var err error

	if len(os.Getenv("GOGC")) == 0 {
		debug.SetGCPercent(25)
	}

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	events.Default.Log(events.Starting, map[string]string{"home": confDir})

	// Ensure that that we have a certificate and key.
	cert, err = loadCert(confDir, "")
	if err != nil {
		newCertificate(confDir, "")
		cert, err = loadCert(confDir, "")
		if err != nil {
			l.Fatalln("load cert:", err)
		}
	}

	myID = protocol.NewDeviceID(cert.Certificate[0])
	l.SetPrefix(fmt.Sprintf("[%s] ", myID.String()[:5]))

	l.Infoln(LongVersion)
	l.Infoln("My ID:", myID)

	// Prepare to be able to save configuration

	cfgFile := filepath.Join(confDir, "config.xml")

	var myName string

	// Load the configuration file, if it exists.
	// If it does not, create a template.

	if info, err := os.Stat(cfgFile); err == nil {
		if !info.Mode().IsRegular() {
			l.Fatalln("Config file is not a file?")
		}
		cfg, err = config.Load(cfgFile, myID)
		if err == nil {
			myCfg := cfg.Devices()[myID]
			if myCfg.Name == "" {
				myName, _ = os.Hostname()
			} else {
				myName = myCfg.Name
			}
		} else {
			l.Fatalln("Configuration:", err)
		}
	} else {
		l.Infoln("No config file; starting with empty defaults")
		myName, _ = os.Hostname()
		newCfg := defaultConfig(myName)
		cfg = config.Wrap(cfgFile, newCfg)
		cfg.Save()
		l.Infof("Edit %s to taste or use the GUI\n", cfgFile)
	}

	if cfg.Raw().OriginalVersion != config.CurrentVersion {
		l.Infoln("Archiving a copy of old config file format")
		// Archive a copy
		osutil.Rename(cfgFile, cfgFile+fmt.Sprintf(".v%d", cfg.Raw().OriginalVersion))
		// Save the new version
		cfg.Save()
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
		NextProtos:             []string{"bep/1.0"},
		ServerName:             myID.String(),
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
		MinVersion:             tls.VersionTLS12,
	}

	// If the read or write rate should be limited, set up a rate limiter for it.
	// This will be used on connections created in the connect and listen routines.

	opts := cfg.Options()

	if opts.MaxSendKbps > 0 {
		writeRateLimit = ratelimit.NewBucketWithRate(float64(1000*opts.MaxSendKbps), int64(5*1000*opts.MaxSendKbps))
	}
	if opts.MaxRecvKbps > 0 {
		readRateLimit = ratelimit.NewBucketWithRate(float64(1000*opts.MaxRecvKbps), int64(5*1000*opts.MaxRecvKbps))
	}

	db, err := leveldb.OpenFile(filepath.Join(confDir, "index"), &opt.Options{CachedOpenFiles: 100})
	if err != nil {
		l.Fatalln("Cannot open database:", err, "- Is another copy of Syncthing already running?")
	}

	// Remove database entries for folders that no longer exist in the config
	folders := cfg.Folders()
	for _, folder := range files.ListFolders(db) {
		if _, ok := folders[folder]; !ok {
			l.Infof("Cleaning data for dropped folder %q", folder)
			files.DropFolder(db, folder)
		}
	}

	m := model.NewModel(cfg, myName, "syncthing", Version, db)

	sanityCheckFolders(cfg, m)

	// GUI

	setupGUI(cfg, m)

	// Clear out old indexes for other devices. Otherwise we'll start up and
	// start needing a bunch of files which are nowhere to be found. This
	// needs to be changed when we correctly do persistent indexes.
	for _, folderCfg := range cfg.Folders() {
		if folderCfg.Invalid != "" {
			continue
		}
		for _, device := range folderCfg.DeviceIDs() {
			if device == myID {
				continue
			}
			m.Index(device, folderCfg.ID, nil)
		}
	}

	// The default port we announce, possibly modified by setupUPnP next.

	addr, err := net.ResolveTCPAddr("tcp", opts.ListenAddress[0])
	if err != nil {
		l.Fatalln("Bad listen address:", err)
	}
	externalPort = addr.Port

	// UPnP
	igd = nil

	if opts.UPnPEnabled {
		setupUPnP()
	}

	// Routine to connect out to configured devices
	discoverer = discovery(externalPort)
	go listenConnect(myID, m, tlsCfg)

	for _, folder := range cfg.Folders() {
		if folder.Invalid != "" {
			continue
		}

		// Routine to pull blocks from other devices to synchronize the local
		// folder. Does not run when we are in read only (publish only) mode.
		if folder.ReadOnly {
			l.Okf("Ready to synchronize %s (read only; no external updates accepted)", folder.ID)
			m.StartFolderRO(folder.ID)
		} else {
			l.Okf("Ready to synchronize %s (read-write)", folder.ID)
			m.StartFolderRW(folder.ID)
		}
	}

	if cpuProfile {
		f, err := os.Create(fmt.Sprintf("cpu-%d.pprof", os.Getpid()))
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	for _, device := range cfg.Devices() {
		if len(device.Name) > 0 {
			l.Infof("Device %s is %q at %v", device.DeviceID, device.Name, device.Addresses)
		}
	}

	if opts.URAccepted > 0 && opts.URAccepted < usageReportVersion {
		l.Infoln("Anonymous usage report has changed; revoking acceptance")
		opts.URAccepted = 0
		cfg.SetOptions(opts)
	}
	if opts.URAccepted >= usageReportVersion {
		go usageReportingLoop(m)
		go func() {
			time.Sleep(10 * time.Minute)
			err := sendUsageReport(m)
			if err != nil {
				l.Infoln("Usage report:", err)
			}
		}()
	}

	if opts.RestartOnWakeup {
		go standbyMonitor()
	}

	if opts.AutoUpgradeIntervalH > 0 {
		if IsRelease {
			go autoUpgrade()
		} else {
			l.Infof("No automatic upgrades; %s is not a relase version.", Version)
		}
	}

	events.Default.Log(events.StartupComplete, nil)
	go generateEvents()

	code := <-stop

	l.Okln("Exiting")
	os.Exit(code)
}

func setupGUI(cfg *config.ConfigWrapper, m *model.Model) {
	opts := cfg.Options()
	guiCfg := overrideGUIConfig(cfg.GUI(), guiAddress, guiAuthentication, guiAPIKey)

	if guiCfg.Enabled && guiCfg.Address != "" {
		addr, err := net.ResolveTCPAddr("tcp", guiCfg.Address)
		if err != nil {
			l.Fatalf("Cannot start GUI on %q: %v", guiCfg.Address, err)
		} else {
			var hostOpen, hostShow string
			switch {
			case addr.IP == nil:
				hostOpen = "localhost"
				hostShow = "0.0.0.0"
			case addr.IP.IsUnspecified():
				hostOpen = "localhost"
				hostShow = addr.IP.String()
			default:
				hostOpen = addr.IP.String()
				hostShow = hostOpen
			}

			var proto = "http"
			if guiCfg.UseTLS {
				proto = "https"
			}

			urlShow := fmt.Sprintf("%s://%s/", proto, net.JoinHostPort(hostShow, strconv.Itoa(addr.Port)))
			l.Infoln("Starting web GUI on", urlShow)
			err := startGUI(guiCfg, guiAssets, m)
			if err != nil {
				l.Fatalln("Cannot start GUI:", err)
			}
			if opts.StartBrowser && !noBrowser && !stRestarting {
				urlOpen := fmt.Sprintf("%s://%s/", proto, net.JoinHostPort(hostOpen, strconv.Itoa(addr.Port)))
				openURL(urlOpen)
			}
		}
	}
}

func sanityCheckFolders(cfg *config.ConfigWrapper, m *model.Model) {
nextFolder:
	for id, folder := range cfg.Folders() {
		if folder.Invalid != "" {
			continue
		}
		m.AddFolder(folder)

		fi, err := os.Stat(folder.Path)
		if m.CurrentLocalVersion(id) > 0 {
			// Safety check. If the cached index contains files but the
			// folder doesn't exist, we have a problem. We would assume
			// that all files have been deleted which might not be the case,
			// so mark it as invalid instead.
			if err != nil || !fi.IsDir() {
				l.Warnf("Stopping folder %q - path does not exist, but has files in index", folder.ID)
				cfg.InvalidateFolder(id, "folder path missing")
				continue nextFolder
			} else if !folder.HasMarker() {
				l.Warnf("Stopping folder %q - path exists, but folder marker missing, check for mount issues", folder.ID)
				cfg.InvalidateFolder(id, "folder marker missing")
				continue nextFolder
			}
		} else if os.IsNotExist(err) {
			// If we don't have any files in the index, and the directory
			// doesn't exist, try creating it.
			err = os.MkdirAll(folder.Path, 0700)
			if err != nil {
				l.Warnf("Stopping folder %q - %v", folder.ID, err)
				cfg.InvalidateFolder(id, err.Error())
				continue nextFolder
			}
			err = folder.CreateMarker()
		} else if !folder.HasMarker() {
			// If we don't have any files in the index, and the path does exist
			// but the marker is not there, create it.
			err = folder.CreateMarker()
		}

		if err != nil {
			// If there was another error or we could not create the
			// path, the folder is invalid.
			l.Warnf("Stopping folder %q - %v", folder.ID, err)
			cfg.InvalidateFolder(id, err.Error())
			continue nextFolder
		}
	}
}

func defaultConfig(myName string) config.Configuration {
	defaultFolder, err := osutil.ExpandTilde("~/Sync")
	if err != nil {
		l.Fatalln("home:", err)
	}

	newCfg := config.New(myID)
	newCfg.Folders = []config.FolderConfiguration{
		{
			ID:              "default",
			Path:            defaultFolder,
			RescanIntervalS: 60,
			Devices:         []config.FolderDeviceConfiguration{{DeviceID: myID}},
		},
	}
	newCfg.Devices = []config.DeviceConfiguration{
		{
			DeviceID:  myID,
			Addresses: []string{"dynamic"},
			Name:      myName,
		},
	}

	port, err := getFreePort("127.0.0.1", 8080)
	if err != nil {
		l.Fatalln("get free port (GUI):", err)
	}
	newCfg.GUI.Address = fmt.Sprintf("127.0.0.1:%d", port)

	port, err = getFreePort("0.0.0.0", 22000)
	if err != nil {
		l.Fatalln("get free port (BEP):", err)
	}
	newCfg.Options.ListenAddress = []string{fmt.Sprintf("0.0.0.0:%d", port)}
	return newCfg
}

func generateEvents() {
	for {
		time.Sleep(300 * time.Second)
		events.Default.Log(events.Ping, nil)
	}
}

func setupUPnP() {
	if opts := cfg.Options(); len(opts.ListenAddress) == 1 {
		_, portStr, err := net.SplitHostPort(opts.ListenAddress[0])
		if err != nil {
			l.Warnln("Bad listen address:", err)
		} else {
			// Set up incoming port forwarding, if necessary and possible
			port, _ := strconv.Atoi(portStr)
			igds := upnp.Discover()
			if len(igds) > 0 {
				// Configure the first discovered IGD only. This is a work-around until we have a better mechanism
				// for handling multiple IGDs, which will require changes to the global discovery service
				igd = igds[0]

				externalPort = setupExternalPort(igd, port)
				if externalPort == 0 {
					l.Warnln("Failed to create UPnP port mapping")
				} else {
					l.Infof("Created UPnP port mapping for external port %d on UPnP device %s.", externalPort, igd.FriendlyIdentifier())

					if opts.UPnPRenewal > 0 {
						go renewUPnP(port)
					}
				}
			}
		}
	} else {
		l.Warnln("Multiple listening addresses; not attempting UPnP port mapping")
	}
}

func setupExternalPort(igd *upnp.IGD, port int) int {
	if igd == nil {
		return 0
	}

	// We seed the random number generator with the node ID to get a
	// repeatable sequence of random external ports.
	rnd := rand.NewSource(certSeed(cert.Certificate[0]))
	for i := 0; i < 10; i++ {
		r := 1024 + int(rnd.Int63()%(65535-1024))
		err := igd.AddPortMapping(upnp.TCP, r, port, "syncthing", cfg.Options().UPnPLease*60)
		if err == nil {
			return r
		}
	}
	return 0
}

func renewUPnP(port int) {
	for {
		opts := cfg.Options()
		time.Sleep(time.Duration(opts.UPnPRenewal) * time.Minute)

		// Make sure our IGD reference isn't nil
		if igd == nil {
			if debugNet {
				l.Debugln("Undefined IGD during UPnP port renewal. Re-discovering...")
			}
			igds := upnp.Discover()
			if len(igds) > 0 {
				// Configure the first discovered IGD only. This is a work-around until we have a better mechanism
				// for handling multiple IGDs, which will require changes to the global discovery service
				igd = igds[0]
			} else {
				if debugNet {
					l.Debugln("Failed to discover IGD during UPnP port mapping renewal.")
				}
				continue
			}
		}

		// Just renew the same port that we already have
		if externalPort != 0 {
			err := igd.AddPortMapping(upnp.TCP, externalPort, port, "syncthing", opts.UPnPLease*60)
			if err != nil {
				l.Warnf("Error renewing UPnP port mapping for external port %d on device %s: %s", externalPort, igd.FriendlyIdentifier(), err.Error())
			} else if debugNet {
				l.Debugf("Renewed UPnP port mapping for external port %d on device %s.", externalPort, igd.FriendlyIdentifier())
			}

			continue
		}

		// Something strange has happened. We didn't have an external port before?
		// Or perhaps the gateway has changed?
		// Retry the same port sequence from the beginning.
		if debugNet {
			l.Debugln("No UPnP port mapping defined, updating...")
		}

		forwardedPort := setupExternalPort(igd, port)
		if forwardedPort != 0 {
			externalPort = forwardedPort
			discoverer.StopGlobal()
			discoverer.StartGlobal(opts.GlobalAnnServer, uint16(forwardedPort))
			if debugNet {
				l.Debugf("Updated UPnP port mapping for external port %d on device %s.", forwardedPort, igd.FriendlyIdentifier())
			}
		} else {
			l.Warnf("Failed to update UPnP port mapping for external port on device " + igd.FriendlyIdentifier() + ".")
		}
	}
}

func resetFolders() {
	suffix := fmt.Sprintf(".syncthing-reset-%d", time.Now().UnixNano())
	for _, folder := range cfg.Folders() {
		if _, err := os.Stat(folder.Path); err == nil {
			l.Infof("Reset: Moving %s -> %s", folder.Path, folder.Path+suffix)
			os.Rename(folder.Path, folder.Path+suffix)
		}
	}

	idx := filepath.Join(confDir, "index")
	os.RemoveAll(idx)
}

func restart() {
	l.Infoln("Restarting")
	stop <- exitRestarting
}

func shutdown() {
	l.Infoln("Shutting down")
	stop <- exitSuccess
}

func listenConnect(myID protocol.DeviceID, m *model.Model, tlsCfg *tls.Config) {
	var conns = make(chan *tls.Conn)

	// Listen
	for _, addr := range cfg.Options().ListenAddress {
		go listenTLS(conns, addr, tlsCfg)
	}

	// Connect
	go dialTLS(m, conns, tlsCfg)

next:
	for conn := range conns {
		certs := conn.ConnectionState().PeerCertificates
		if cl := len(certs); cl != 1 {
			l.Infof("Got peer certificate list of length %d != 1 from %s; protocol error", cl, conn.RemoteAddr())
			conn.Close()
			continue
		}
		remoteCert := certs[0]
		remoteID := protocol.NewDeviceID(remoteCert.Raw)

		if remoteID == myID {
			l.Infof("Connected to myself (%s) - should not happen", remoteID)
			conn.Close()
			continue
		}

		if m.ConnectedTo(remoteID) {
			l.Infof("Connected to already connected device (%s)", remoteID)
			conn.Close()
			continue
		}

		for deviceID, deviceCfg := range cfg.Devices() {
			if deviceID == remoteID {
				// Verify the name on the certificate. By default we set it to
				// "syncthing" when generating, but the user may have replaced
				// the certificate and used another name.
				certName := deviceCfg.CertName
				if certName == "" {
					certName = "syncthing"
				}
				err := remoteCert.VerifyHostname(certName)
				if err != nil {
					// Incorrect certificate name is something the user most
					// likely wants to know about, since it's an advanced
					// config. Warn instead of Info.
					l.Warnf("Bad certificate from %s (%v): %v", remoteID, conn.RemoteAddr(), err)
					conn.Close()
					continue next
				}

				// If rate limiting is set, we wrap the connection in a
				// limiter.
				var wr io.Writer = conn
				if writeRateLimit != nil {
					wr = &limitedWriter{conn, writeRateLimit}
				}

				var rd io.Reader = conn
				if readRateLimit != nil {
					rd = &limitedReader{conn, readRateLimit}
				}

				name := fmt.Sprintf("%s-%s", conn.LocalAddr(), conn.RemoteAddr())
				protoConn := protocol.NewConnection(remoteID, rd, wr, m, name, deviceCfg.Compression)

				l.Infof("Established secure connection to %s at %s", remoteID, name)
				if debugNet {
					l.Debugf("cipher suite %04X", conn.ConnectionState().CipherSuite)
				}
				events.Default.Log(events.DeviceConnected, map[string]string{
					"id":   remoteID.String(),
					"addr": conn.RemoteAddr().String(),
				})

				m.AddConnection(conn, protoConn)
				continue next
			}
		}

		events.Default.Log(events.DeviceRejected, map[string]string{
			"device":  remoteID.String(),
			"address": conn.RemoteAddr().String(),
		})
		l.Infof("Connection from %s with unknown device ID %s; ignoring", conn.RemoteAddr(), remoteID)
		conn.Close()
	}
}

func listenTLS(conns chan *tls.Conn, addr string, tlsCfg *tls.Config) {
	if debugNet {
		l.Debugln("listening on", addr)
	}

	tcaddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		l.Fatalln("listen (BEP):", err)
	}
	listener, err := net.ListenTCP("tcp", tcaddr)
	if err != nil {
		l.Fatalln("listen (BEP):", err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			l.Warnln("Accepting connection:", err)
			continue
		}

		if debugNet {
			l.Debugln("connect from", conn.RemoteAddr())
		}

		tcpConn := conn.(*net.TCPConn)
		setTCPOptions(tcpConn)

		tc := tls.Server(conn, tlsCfg)
		err = tc.Handshake()
		if err != nil {
			l.Infoln("TLS handshake:", err)
			tc.Close()
			continue
		}

		conns <- tc
	}

}

func dialTLS(m *model.Model, conns chan *tls.Conn, tlsCfg *tls.Config) {
	var delay time.Duration = 1 * time.Second
	for {
	nextDevice:
		for deviceID, deviceCfg := range cfg.Devices() {
			if deviceID == myID {
				continue
			}

			if m.ConnectedTo(deviceID) {
				continue
			}

			var addrs []string
			for _, addr := range deviceCfg.Addresses {
				if addr == "dynamic" {
					if discoverer != nil {
						t := discoverer.Lookup(deviceID)
						if len(t) == 0 {
							continue
						}
						addrs = append(addrs, t...)
					}
				} else {
					addrs = append(addrs, addr)
				}
			}

			for _, addr := range addrs {
				host, port, err := net.SplitHostPort(addr)
				if err != nil && strings.HasPrefix(err.Error(), "missing port") {
					// addr is on the form "1.2.3.4"
					addr = net.JoinHostPort(addr, "22000")
				} else if err == nil && port == "" {
					// addr is on the form "1.2.3.4:"
					addr = net.JoinHostPort(host, "22000")
				}
				if debugNet {
					l.Debugln("dial", deviceCfg.DeviceID, addr)
				}

				raddr, err := net.ResolveTCPAddr("tcp", addr)
				if err != nil {
					if debugNet {
						l.Debugln(err)
					}
					continue
				}

				conn, err := net.DialTCP("tcp", nil, raddr)
				if err != nil {
					if debugNet {
						l.Debugln(err)
					}
					continue
				}

				setTCPOptions(conn)

				tc := tls.Client(conn, tlsCfg)
				err = tc.Handshake()
				if err != nil {
					l.Infoln("TLS handshake:", err)
					tc.Close()
					continue
				}

				conns <- tc
				continue nextDevice
			}
		}

		time.Sleep(delay)
		delay *= 2
		if maxD := time.Duration(cfg.Options().ReconnectIntervalS) * time.Second; delay > maxD {
			delay = maxD
		}
	}
}

func setTCPOptions(conn *net.TCPConn) {
	var err error
	if err = conn.SetLinger(0); err != nil {
		l.Infoln(err)
	}
	if err = conn.SetNoDelay(false); err != nil {
		l.Infoln(err)
	}
	if err = conn.SetKeepAlivePeriod(60 * time.Second); err != nil {
		l.Infoln(err)
	}
	if err = conn.SetKeepAlive(true); err != nil {
		l.Infoln(err)
	}
}

func discovery(extPort int) *discover.Discoverer {
	opts := cfg.Options()
	disc := discover.NewDiscoverer(myID, opts.ListenAddress)

	if opts.LocalAnnEnabled {
		l.Infoln("Starting local discovery announcements")
		disc.StartLocal(opts.LocalAnnPort, opts.LocalAnnMCAddr)
	}

	if opts.GlobalAnnEnabled {
		l.Infoln("Starting global discovery announcements")
		disc.StartGlobal(opts.GlobalAnnServer, uint16(extPort))
	}

	return disc
}

func ensureDir(dir string, mode int) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0700)
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

func getDefaultConfDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LocalAppData"), "Syncthing"), nil

	case "darwin":
		return osutil.ExpandTilde("~/Library/Application Support/Syncthing")

	default:
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			return filepath.Join(xdgCfg, "syncthing"), nil
		} else {
			return osutil.ExpandTilde("~/.config/syncthing")
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

func overrideGUIConfig(cfg config.GUIConfiguration, address, authentication, apikey string) config.GUIConfiguration {
	if address != "" {
		cfg.Enabled = true

		if !strings.Contains(address, "//") {
			// Assume just an IP was given. Don't touch he TLS setting.
			cfg.Address = address
		} else {
			parsed, err := url.Parse(address)
			if err != nil {
				l.Fatalln(err)
			}
			cfg.Address = parsed.Host
			switch parsed.Scheme {
			case "http":
				cfg.UseTLS = false
			case "https":
				cfg.UseTLS = true
			default:
				l.Fatalln("Unknown scheme:", parsed.Scheme)
			}
		}
	}

	if authentication != "" {
		authenticationParts := strings.SplitN(authentication, ":", 2)

		hash, err := bcrypt.GenerateFromPassword([]byte(authenticationParts[1]), 0)
		if err != nil {
			l.Fatalln("Invalid GUI password:", err)
		}

		cfg.User = authenticationParts[0]
		cfg.Password = string(hash)
	}

	if apikey != "" {
		cfg.APIKey = apikey
	}
	return cfg
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

func autoUpgrade() {
	var skipped bool
	interval := time.Duration(cfg.Options().AutoUpgradeIntervalH) * time.Hour
	for {
		if skipped {
			time.Sleep(interval)
		} else {
			skipped = true
		}

		rel, err := upgrade.LatestRelease(IsBeta)
		if err == upgrade.ErrUpgradeUnsupported {
			return
		}
		if err != nil {
			// Don't complain too loudly here; we might simply not have
			// internet connectivity, or the upgrade server might be down.
			l.Infoln("Automatic upgrade:", err)
			continue
		}

		if upgrade.CompareVersions(rel.Tag, Version) <= 0 {
			continue
		}

		l.Infof("Automatic upgrade (current %q < latest %q)", Version, rel.Tag)
		err = upgrade.UpgradeTo(rel, GoArchExtra)
		if err != nil {
			l.Warnln("Automatic upgrade:", err)
			continue
		}
		l.Warnf("Automatically upgraded to version %q. Restarting in 1 minute.", rel.Tag)
		time.Sleep(time.Minute)
		stop <- exitUpgrading
		return
	}
}
