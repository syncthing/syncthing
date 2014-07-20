// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha1"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/calmh/syncthing/config"
	"github.com/calmh/syncthing/discover"
	"github.com/calmh/syncthing/events"
	"github.com/calmh/syncthing/logger"
	"github.com/calmh/syncthing/model"
	"github.com/calmh/syncthing/osutil"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/upnp"
	"github.com/juju/ratelimit"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	Version     = "unknown-dev"
	BuildEnv    = "default"
	BuildStamp  = "0"
	BuildDate   time.Time
	BuildHost   = "unknown"
	BuildUser   = "unknown"
	LongVersion string
)

var l = logger.DefaultLogger

func init() {
	if Version != "unknown-dev" {
		// If not a generic dev build, version string should come from git describe
		exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-beta\d+)?(-\d+-g[0-9a-f]+)?(-dirty)?$`)
		if !exp.MatchString(Version) {
			l.Fatalf("Invalid version string %q;\n\tdoes not match regexp %v", Version, exp)
		}
	}

	stamp, _ := strconv.Atoi(BuildStamp)
	BuildDate = time.Unix(int64(stamp), 0)

	date := BuildDate.UTC().Format("2006-01-02 15:04:05 MST")
	LongVersion = fmt.Sprintf("syncthing %s (%s %s-%s %s) %s@%s %s", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildEnv, BuildUser, BuildHost, date)

	if os.Getenv("STTRACE") != "" {
		logFlags = log.Ltime | log.Ldate | log.Lmicroseconds | log.Lshortfile
	}
}

var (
	cfg        config.Configuration
	myID       protocol.NodeID
	confDir    string
	logFlags   int = log.Ltime
	rateBucket *ratelimit.Bucket
	stop       = make(chan bool)
	discoverer *discover.Discoverer
)

const (
	usage      = "syncthing [options]"
	extraUsage = `The value for the -logflags option is a sum of the following:

   1  Date
   2  Time
   4  Microsecond time
   8  Long filename
  16  Short filename

I.e. to prefix each log line with date and time, set -logflags=3 (1 + 2 from
above). The value 0 is used to disable all of the above. The default is to
show time only (2).

The following enviroment variables are interpreted by syncthing:

 STNORESTART   Do not attempt to restart when requested to, instead just exit.
               Set this variable when running under a service manager such as
               runit, launchd, etc.

 STPROFILER    Set to a listen address such as "127.0.0.1:9090" to start the
               profiler with HTTP access.

 STTRACE       A comma separated string of facilities to trace. The valid
               facility strings:
               - "beacon"   (the beacon package)
               - "discover" (the discover package)
               - "files"    (the files package)
               - "net"      (the main package; connections & network messages)
               - "model"    (the model package)
               - "scanner"  (the scanner package)
               - "upnp"     (the upnp package)
               - "xdr"      (the xdr package)
               - "all"      (all of the above)

 STCPUPROFILE  Write CPU profile to the specified file.

 STGUIASSETS   Directory to load GUI assets from. Overrides compiled in assets.

 STDEADLOCKTIMEOUT  Alter deadlock detection timeout (seconds; default 1200).`
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var reset bool
	var showVersion bool
	var doUpgrade bool
	flag.StringVar(&confDir, "home", getDefaultConfDir(), "Set configuration directory")
	flag.BoolVar(&reset, "reset", false, "Prepare to resync from cluster")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&doUpgrade, "upgrade", false, "Perform upgrade")
	flag.IntVar(&logFlags, "logflags", logFlags, "Set log flags")
	flag.Usage = usageFor(flag.CommandLine, usage, extraUsage)
	flag.Parse()

	if showVersion {
		fmt.Println(LongVersion)
		return
	}

	l.SetFlags(logFlags)

	if doUpgrade {
		err := upgrade()
		if err != nil {
			l.Fatalln(err)
		}
		return
	}

	if len(os.Getenv("GOGC")) == 0 {
		debug.SetGCPercent(25)
	}

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	confDir = expandTilde(confDir)

	events.Default.Log(events.Starting, map[string]string{"home": confDir})

	if _, err := os.Stat(confDir); err != nil && confDir == getDefaultConfDir() {
		// We are supposed to use the default configuration directory. It
		// doesn't exist. In the past our default has been ~/.syncthing, so if
		// that directory exists we move it to the new default location and
		// continue. We don't much care if this fails at this point, we will
		// be checking that later.

		var oldDefault string
		if runtime.GOOS == "windows" {
			oldDefault = filepath.Join(os.Getenv("AppData"), "Syncthing")
		} else {
			oldDefault = expandTilde("~/.syncthing")
		}
		if _, err := os.Stat(oldDefault); err == nil {
			os.MkdirAll(filepath.Dir(confDir), 0700)
			if err := os.Rename(oldDefault, confDir); err == nil {
				l.Infoln("Moved config dir", oldDefault, "to", confDir)
			}
		}
	}

	// Ensure that our home directory exists and that we have a certificate and key.

	ensureDir(confDir, 0700)
	cert, err := loadCert(confDir, "")
	if err != nil {
		newCertificate(confDir, "")
		cert, err = loadCert(confDir, "")
		l.FatalErr(err)
	}

	myID = protocol.NewNodeID(cert.Certificate[0])
	l.SetPrefix(fmt.Sprintf("[%s] ", myID.String()[:5]))

	l.Infoln(LongVersion)
	l.Infoln("My ID:", myID)

	// Prepare to be able to save configuration

	cfgFile := filepath.Join(confDir, "config.xml")
	go saveConfigLoop(cfgFile)

	// Load the configuration file, if it exists.
	// If it does not, create a template.

	cf, err := os.Open(cfgFile)
	if err == nil {
		// Read config.xml
		cfg, err = config.Load(cf, myID)
		if err != nil {
			l.Fatalln(err)
		}
		cf.Close()
	} else {
		l.Infoln("No config file; starting with empty defaults")
		name, _ := os.Hostname()
		defaultRepo := filepath.Join(getHomeDir(), "Sync")
		ensureDir(defaultRepo, 0755)

		cfg, err = config.Load(nil, myID)
		cfg.Repositories = []config.RepositoryConfiguration{
			{
				ID:        "default",
				Directory: defaultRepo,
				Nodes:     []config.NodeConfiguration{{NodeID: myID}},
			},
		}
		cfg.Nodes = []config.NodeConfiguration{
			{
				NodeID:    myID,
				Addresses: []string{"dynamic"},
				Name:      name,
			},
		}

		port, err := getFreePort("127.0.0.1", 8080)
		l.FatalErr(err)
		cfg.GUI.Address = fmt.Sprintf("127.0.0.1:%d", port)

		port, err = getFreePort("0.0.0.0", 22000)
		l.FatalErr(err)
		cfg.Options.ListenAddress = []string{fmt.Sprintf("0.0.0.0:%d", port)}

		saveConfig()
		l.Infof("Edit %s to taste or use the GUI\n", cfgFile)
	}

	if reset {
		resetRepositories()
		return
	}

	if profiler := os.Getenv("STPROFILER"); len(profiler) > 0 {
		go func() {
			l.Debugln("Starting profiler on", profiler)
			runtime.SetBlockProfileRate(1)
			err := http.ListenAndServe(profiler, nil)
			if err != nil {
				l.Fatalln(err)
			}
		}()
	}

	if len(os.Getenv("STRESTART")) > 0 {
		waitForParentExit()
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

	// If the write rate should be limited, set up a rate limiter for it.
	// This will be used on connections created in the connect and listen routines.

	if cfg.Options.MaxSendKbps > 0 {
		rateBucket = ratelimit.NewBucketWithRate(float64(1000*cfg.Options.MaxSendKbps), int64(5*1000*cfg.Options.MaxSendKbps))
	}

	removeLegacyIndexes()
	db, err := leveldb.OpenFile(filepath.Join(confDir, "index"), nil)
	if err != nil {
		l.Fatalln("leveldb.OpenFile():", err)
	}
	m := model.NewModel(confDir, &cfg, "syncthing", Version, db)

nextRepo:
	for i, repo := range cfg.Repositories {
		if repo.Invalid != "" {
			continue
		}

		repo.Directory = expandTilde(repo.Directory)

		// Safety check. If the cached index contains files but the repository
		// doesn't exist, we have a problem. We would assume that all files
		// have been deleted which might not be the case, so abort instead.

		id := fmt.Sprintf("%x", sha1.Sum([]byte(repo.Directory)))
		idxFile := filepath.Join(confDir, id+".idx.gz")
		if _, err := os.Stat(idxFile); err == nil {
			if fi, err := os.Stat(repo.Directory); err != nil || !fi.IsDir() {
				cfg.Repositories[i].Invalid = "repo directory missing"
				continue nextRepo
			}
		}

		ensureDir(repo.Directory, -1)
		m.AddRepo(repo)
	}

	// GUI
	if cfg.GUI.Enabled && cfg.GUI.Address != "" {
		addr, err := net.ResolveTCPAddr("tcp", cfg.GUI.Address)
		if err != nil {
			l.Fatalf("Cannot start GUI on %q: %v", cfg.GUI.Address, err)
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
			if cfg.GUI.UseTLS {
				proto = "https"
			}

			l.Infof("Starting web GUI on %s://%s:%d/", proto, hostShow, addr.Port)
			err := startGUI(cfg.GUI, os.Getenv("STGUIASSETS"), m)
			if err != nil {
				l.Fatalln("Cannot start GUI:", err)
			}
			if cfg.Options.StartBrowser && len(os.Getenv("STRESTART")) == 0 {
				openURL(fmt.Sprintf("%s://%s:%d", proto, hostOpen, addr.Port))
			}
		}
	}

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	m.CleanRepos()
	l.Infoln("Performing initial repository scan")
	m.ScanRepos()

	// Remove all .idx* files that don't belong to an active repo.

	validIndexes := make(map[string]bool)
	for _, repo := range cfg.Repositories {
		dir := expandTilde(repo.Directory)
		id := fmt.Sprintf("%x", sha1.Sum([]byte(dir)))
		validIndexes[id] = true
	}

	allIndexes, err := filepath.Glob(filepath.Join(confDir, "*.idx*"))
	if err == nil {
		for _, idx := range allIndexes {
			bn := filepath.Base(idx)
			fs := strings.Split(bn, ".")
			if len(fs) > 1 {
				if _, ok := validIndexes[fs[0]]; !ok {
					l.Infoln("Removing old index", bn)
					os.Remove(idx)
				}
			}
		}
	}

	// UPnP

	var externalPort = 0
	if cfg.Options.UPnPEnabled {
		// We seed the random number generator with the node ID to get a
		// repeatable sequence of random external ports.
		externalPort = setupUPnP(rand.NewSource(certSeed(cert.Certificate[0])))
	}

	// Routine to connect out to configured nodes
	discoverer = discovery(externalPort)
	go listenConnect(myID, m, tlsCfg)

	for _, repo := range cfg.Repositories {
		if repo.Invalid != "" {
			continue
		}

		// Routine to pull blocks from other nodes to synchronize the local
		// repository. Does not run when we are in read only (publish only) mode.
		if repo.ReadOnly {
			l.Okf("Ready to synchronize %s (read only; no external updates accepted)", repo.ID)
			m.StartRepoRO(repo.ID)
		} else {
			l.Okf("Ready to synchronize %s (read-write)", repo.ID)
			m.StartRepoRW(repo.ID, cfg.Options.ParallelRequests)
		}
	}

	if cpuprof := os.Getenv("STCPUPROFILE"); len(cpuprof) > 0 {
		f, err := os.Create(cpuprof)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	for _, node := range cfg.Nodes {
		if len(node.Name) > 0 {
			l.Infof("Node %s is %q at %v", node.NodeID, node.Name, node.Addresses)
		}
	}

	if cfg.Options.URAccepted > 0 && cfg.Options.URAccepted < usageReportVersion {
		l.Infoln("Anonymous usage report has changed; revoking acceptance")
		cfg.Options.URAccepted = 0
	}
	if cfg.Options.URAccepted >= usageReportVersion {
		go usageReportingLoop(m)
		go func() {
			time.Sleep(10 * time.Minute)
			err := sendUsageReport(m)
			if err != nil {
				l.Infoln("Usage report:", err)
			}
		}()
	}

	events.Default.Log(events.StartupComplete, nil)
	go generateEvents()

	<-stop

	l.Okln("Exiting")
}

func generateEvents() {
	for {
		time.Sleep(300 * time.Second)
		events.Default.Log(events.Ping, nil)
	}
}

func waitForParentExit() {
	l.Infoln("Waiting for parent to exit...")
	// Wait for the listen address to become free, indicating that the parent has exited.
	for {
		ln, err := net.Listen("tcp", cfg.Options.ListenAddress[0])
		if err == nil {
			ln.Close()
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	l.Okln("Continuing")
}

func setupUPnP(r rand.Source) int {
	var externalPort = 0
	if len(cfg.Options.ListenAddress) == 1 {
		_, portStr, err := net.SplitHostPort(cfg.Options.ListenAddress[0])
		if err != nil {
			l.Warnln(err)
		} else {
			// Set up incoming port forwarding, if necessary and possible
			port, _ := strconv.Atoi(portStr)
			igd, err := upnp.Discover()
			if err == nil {
				for i := 0; i < 10; i++ {
					r := 1024 + int(r.Int63()%(65535-1024))
					err := igd.AddPortMapping(upnp.TCP, r, port, "syncthing", 0)
					if err == nil {
						externalPort = r
						l.Infoln("Created UPnP port mapping - external port", externalPort)
						break
					}
				}
				if externalPort == 0 {
					l.Warnln("Failed to create UPnP port mapping")
				}
			} else {
				l.Infof("No UPnP gateway detected")
				if debugNet {
					l.Debugf("UPnP: %v", err)
				}
			}
		}
	} else {
		l.Warnln("Multiple listening addresses; not attempting UPnP port mapping")
	}
	return externalPort
}

func resetRepositories() {
	suffix := fmt.Sprintf(".syncthing-reset-%d", time.Now().UnixNano())
	for _, repo := range cfg.Repositories {
		if _, err := os.Stat(repo.Directory); err == nil {
			l.Infof("Reset: Moving %s -> %s", repo.Directory, repo.Directory+suffix)
			os.Rename(repo.Directory, repo.Directory+suffix)
		}
	}

	idx := filepath.Join(confDir, "index")
	os.RemoveAll(idx)
}

func removeLegacyIndexes() {
	pat := filepath.Join(confDir, "*.idx.gz*")
	idxs, err := filepath.Glob(pat)
	if err == nil {
		for _, idx := range idxs {
			l.Infof("Reset: Removing %s", idx)
			os.Remove(idx)
		}
	}
}

func restart() {
	l.Infoln("Restarting")
	if os.Getenv("SMF_FMRI") != "" || os.Getenv("STNORESTART") != "" {
		// Solaris SMF
		l.Infoln("Service manager detected; exit instead of restart")
		stop <- true
		return
	}

	env := os.Environ()
	if len(os.Getenv("STRESTART")) == 0 {
		env = append(env, "STRESTART=1")
	}
	pgm, err := exec.LookPath(os.Args[0])
	if err != nil {
		l.Warnln(err)
		return
	}
	proc, err := os.StartProcess(pgm, os.Args, &os.ProcAttr{
		Env:   env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		l.Fatalln(err)
	}
	proc.Release()
	stop <- true
}

func shutdown() {
	stop <- true
}

var saveConfigCh = make(chan struct{})

func saveConfigLoop(cfgFile string) {
	for _ = range saveConfigCh {
		fd, err := os.Create(cfgFile + ".tmp")
		if err != nil {
			l.Warnln(err)
			continue
		}

		err = config.Save(fd, cfg)
		if err != nil {
			l.Warnln(err)
			fd.Close()
			continue
		}

		err = fd.Close()
		if err != nil {
			l.Warnln(err)
			continue
		}

		err = osutil.Rename(cfgFile+".tmp", cfgFile)
		if err != nil {
			l.Warnln(err)
		}
	}
}

func saveConfig() {
	saveConfigCh <- struct{}{}
}

func listenConnect(myID protocol.NodeID, m *model.Model, tlsCfg *tls.Config) {
	var conns = make(chan *tls.Conn)

	// Listen
	for _, addr := range cfg.Options.ListenAddress {
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
		remoteID := protocol.NewNodeID(certs[0].Raw)

		if remoteID == myID {
			l.Infof("Connected to myself (%s) - should not happen", remoteID)
			conn.Close()
			continue
		}

		if m.ConnectedTo(remoteID) {
			l.Infof("Connected to already connected node (%s)", remoteID)
			conn.Close()
			continue
		}

		for _, nodeCfg := range cfg.Nodes {
			if nodeCfg.NodeID == remoteID {
				var wr io.Writer = conn
				if rateBucket != nil {
					wr = &limitedWriter{conn, rateBucket}
				}
				name := fmt.Sprintf("%s-%s", conn.LocalAddr(), conn.RemoteAddr())
				protoConn := protocol.NewConnection(remoteID, conn, wr, m, name)

				l.Infof("Established secure connection to %s at %s", remoteID, name)
				if debugNet {
					l.Debugf("cipher suite %04X", conn.ConnectionState().CipherSuite)
				}
				events.Default.Log(events.NodeConnected, map[string]string{
					"id":   remoteID.String(),
					"addr": conn.RemoteAddr().String(),
				})

				m.AddConnection(conn, protoConn)
				continue next
			}
		}

		l.Infof("Connection from %s with unknown node ID %s; ignoring", conn.RemoteAddr(), remoteID)
		conn.Close()
	}
}

func listenTLS(conns chan *tls.Conn, addr string, tlsCfg *tls.Config) {
	if debugNet {
		l.Debugln("listening on", addr)
	}

	tcaddr, err := net.ResolveTCPAddr("tcp", addr)
	l.FatalErr(err)
	listener, err := net.ListenTCP("tcp", tcaddr)
	l.FatalErr(err)

	for {
		conn, err := listener.Accept()
		if err != nil {
			l.Warnln(err)
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
			l.Warnln(err)
			tc.Close()
			continue
		}

		conns <- tc
	}

}

func dialTLS(m *model.Model, conns chan *tls.Conn, tlsCfg *tls.Config) {
	var delay time.Duration = 1 * time.Second
	for {
	nextNode:
		for _, nodeCfg := range cfg.Nodes {
			if nodeCfg.NodeID == myID {
				continue
			}

			if m.ConnectedTo(nodeCfg.NodeID) {
				continue
			}

			var addrs []string
			for _, addr := range nodeCfg.Addresses {
				if addr == "dynamic" {
					if discoverer != nil {
						t := discoverer.Lookup(nodeCfg.NodeID)
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
					l.Debugln("dial", nodeCfg.NodeID, addr)
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
					l.Warnln(err)
					tc.Close()
					continue
				}

				conns <- tc
				continue nextNode
			}
		}

		time.Sleep(delay)
		delay *= 2
		if maxD := time.Duration(cfg.Options.ReconnectIntervalS) * time.Second; delay > maxD {
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
	disc, err := discover.NewDiscoverer(myID, cfg.Options.ListenAddress, cfg.Options.LocalAnnPort)
	if err != nil {
		l.Warnf("No discovery possible (%v)", err)
		return nil
	}

	if cfg.Options.LocalAnnEnabled {
		l.Infoln("Sending local discovery announcements")
		disc.StartLocal()
	}

	if cfg.Options.GlobalAnnEnabled {
		l.Infoln("Sending global discovery announcements")
		disc.StartGlobal(cfg.Options.GlobalAnnServer, uint16(extPort))
	}

	return disc
}

func ensureDir(dir string, mode int) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0700)
		l.FatalErr(err)
	} else if mode >= 0 && err == nil && int(fi.Mode()&0777) != mode {
		err := os.Chmod(dir, os.FileMode(mode))
		l.FatalErr(err)
	}
}

func getDefaultConfDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LocalAppData"), "Syncthing")

	case "darwin":
		return expandTilde("~/Library/Application Support/Syncthing")

	default:
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			return filepath.Join(xdgCfg, "syncthing")
		} else {
			return expandTilde("~/.config/syncthing")
		}
	}
}

func expandTilde(p string) string {
	if p == "~" {
		return getHomeDir()
	}

	p = filepath.FromSlash(p)
	if !strings.HasPrefix(p, fmt.Sprintf("~%c", os.PathSeparator)) {
		return p
	}

	return filepath.Join(getHomeDir(), p[2:])
}

func getHomeDir() string {
	var home string

	switch runtime.GOOS {
	case "windows":
		home = filepath.Join(os.Getenv("HomeDrive"), os.Getenv("HomePath"))
		if home == "" {
			home = os.Getenv("UserProfile")
		}
	default:
		home = os.Getenv("HOME")
	}

	if home == "" {
		l.Fatalln("No home directory found - set $HOME (or the platform equivalent).")
	}

	return home
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
	addr := c.Addr().String()
	c.Close()

	_, portstr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}

	port, err := strconv.Atoi(portstr)
	if err != nil {
		return 0, err
	}

	return port, nil
}
