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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/calmh/syncthing/discover"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/upnp"
	"github.com/juju/ratelimit"
)

const BlockSize = 128 * 1024

var (
	Version     = "unknown-dev"
	BuildStamp  = "0"
	BuildDate   time.Time
	BuildHost   = "unknown"
	BuildUser   = "unknown"
	LongVersion string
)

func init() {
	stamp, _ := strconv.Atoi(BuildStamp)
	BuildDate = time.Unix(int64(stamp), 0)

	date := BuildDate.UTC().Format("2006-01-02 15:04:05 MST")
	LongVersion = fmt.Sprintf("syncthing %s (%s %s-%s) %s@%s %s", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildUser, BuildHost, date)
}

var (
	cfg        Configuration
	myID       string
	confDir    string
	rateBucket *ratelimit.Bucket
	stop       = make(chan bool)
	discoverer *discover.Discoverer
)

const (
	usage      = "syncthing [options]"
	extraUsage = `The following enviroment variables are interpreted by syncthing:

 STNORESTART   Do not attempt to restart when requested to, instead just exit.
               Set this variable when running under a service manager such as
               runit, launchd, etc.

 STPROFILER    Set to a listen address such as "127.0.0.1:9090" to start the
               profiler with HTTP access.

 STTRACE       A comma separated string of facilities to trace. The valid
               facility strings:
               - "discover" (the node discovery package)
               - "files"    (file set store)
               - "idx"      (index sending and receiving)
               - "mc"       (multicast beacon)
               - "need"     (file need calculations)
               - "net"      (connecting and disconnecting, network messages)
               - "pull"     (file pull activity)
               - "scanner"  (the file change scanner)
               - "upnp"     (the upnp port mapper)

 STCPUPROFILE  Write CPU profile to the specified file.`
)

func main() {
	var reset bool
	var showVersion bool
	var doUpgrade bool
	flag.StringVar(&confDir, "home", getDefaultConfDir(), "Set configuration directory")
	flag.BoolVar(&reset, "reset", false, "Prepare to resync from cluster")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&doUpgrade, "upgrade", false, "Perform upgrade")
	flag.Usage = usageFor(flag.CommandLine, usage, extraUsage)
	flag.Parse()

	if len(os.Getenv("STRESTART")) > 0 {
		// Give the parent process time to exit and release sockets etc.
		time.Sleep(1 * time.Second)
	}

	if showVersion {
		fmt.Println(LongVersion)
		return
	}

	if doUpgrade {
		err := upgrade()
		if err != nil {
			fatalln(err)
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

	if _, err := os.Stat(confDir); err != nil && confDir == getDefaultConfDir() {
		// We are supposed to use the default configuration directory. It
		// doesn't exist. In the past our default has been ~/.syncthing, so if
		// that directory exists we move it to the new default location and
		// continue. We don't much care if this fails at this point, we will
		// be checking that later.

		oldDefault := expandTilde("~/.syncthing")
		if _, err := os.Stat(oldDefault); err == nil {
			os.MkdirAll(filepath.Dir(confDir), 0700)
			if err := os.Rename(oldDefault, confDir); err == nil {
				infoln("Moved config dir", oldDefault, "to", confDir)
			}
		}
	}

	// Ensure that our home directory exists and that we have a certificate and key.

	ensureDir(confDir, 0700)
	cert, err := loadCert(confDir)
	if err != nil {
		newCertificate(confDir)
		cert, err = loadCert(confDir)
		fatalErr(err)
	}

	myID = certID(cert.Certificate[0])
	log.SetPrefix("[" + myID[0:5] + "] ")
	logger.SetPrefix("[" + myID[0:5] + "] ")

	infoln(LongVersion)
	infoln("My ID:", myID)

	// Prepare to be able to save configuration

	cfgFile := filepath.Join(confDir, "config.xml")
	go saveConfigLoop(cfgFile)

	// Load the configuration file, if it exists.
	// If it does not, create a template.

	cf, err := os.Open(cfgFile)
	if err == nil {
		// Read config.xml
		cfg, err = readConfigXML(cf, myID)
		if err != nil {
			fatalln(err)
		}
		cf.Close()
	} else {
		infoln("No config file; starting with empty defaults")
		name, _ := os.Hostname()
		defaultRepo := filepath.Join(getHomeDir(), "Sync")
		ensureDir(defaultRepo, 0755)

		cfg, err = readConfigXML(nil, myID)
		cfg.Repositories = []RepositoryConfiguration{
			{
				ID:        "default",
				Directory: defaultRepo,
				Nodes:     []NodeConfiguration{{NodeID: myID}},
			},
		}
		cfg.Nodes = []NodeConfiguration{
			{
				NodeID:    myID,
				Addresses: []string{"dynamic"},
				Name:      name,
			},
		}

		port, err := getFreePort("127.0.0.1", 8080)
		fatalErr(err)
		cfg.GUI.Address = fmt.Sprintf("127.0.0.1:%d", port)

		port, err = getFreePort("", 22000)
		fatalErr(err)
		cfg.Options.ListenAddress = []string{fmt.Sprintf(":%d", port)}

		saveConfig()
		infof("Edit %s to taste or use the GUI\n", cfgFile)
	}

	if reset {
		resetRepositories()
		return
	}

	if profiler := os.Getenv("STPROFILER"); len(profiler) > 0 {
		go func() {
			dlog.Println("Starting profiler on", profiler)
			err := http.ListenAndServe(profiler, nil)
			if err != nil {
				dlog.Fatal(err)
			}
		}()
	}

	// The TLS configuration is used for both the listening socket and outgoing
	// connections.

	tlsCfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{"bep/1.0"},
		ServerName:             myID,
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

	m := NewModel(cfg.Options.MaxChangeKbps * 1000)

	for _, repo := range cfg.Repositories {
		if repo.Invalid != "" {
			continue
		}
		dir := expandTilde(repo.Directory)
		m.AddRepo(repo.ID, dir, repo.Nodes)
	}

	// GUI
	if cfg.GUI.Enabled && cfg.GUI.Address != "" {
		addr, err := net.ResolveTCPAddr("tcp", cfg.GUI.Address)
		if err != nil {
			fatalf("Cannot start GUI on %q: %v", cfg.GUI.Address, err)
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

			infof("Starting web GUI on http://%s:%d/", hostShow, addr.Port)
			err := startGUI(cfg.GUI, m)
			if err != nil {
				fatalln("Cannot start GUI:", err)
			}
			if cfg.Options.StartBrowser && len(os.Getenv("STRESTART")) == 0 {
				openURL(fmt.Sprintf("http://%s:%d", hostOpen, addr.Port))
			}
		}
	}

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	infoln("Populating repository index")
	m.LoadIndexes(confDir)

	for _, repo := range cfg.Repositories {
		if repo.Invalid != "" {
			continue
		}

		dir := expandTilde(repo.Directory)

		// Safety check. If the cached index contains files but the repository
		// doesn't exist, we have a problem. We would assume that all files
		// have been deleted which might not be the case, so abort instead.

		if files, _, _ := m.LocalSize(repo.ID); files > 0 {
			if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
				warnf("Configured repository %q has index but directory %q is missing; not starting.", repo.ID, repo.Directory)
				fatalf("Ensure that directory is present or remove repository from configuration.")
			}
		}

		// Ensure that repository directories exist for newly configured repositories.
		ensureDir(dir, -1)
	}

	m.ScanRepos()
	m.SaveIndexes(confDir)

	// UPnP

	var externalPort = 0
	if cfg.Options.UPnPEnabled {
		// We seed the random number generator with the node ID to get a
		// repeatable sequence of random external ports.
		rand.Seed(certSeed(cert.Certificate[0]))
		externalPort = setupUPnP()
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
			okf("Ready to synchronize %s (read only; no external updates accepted)", repo.ID)
			m.StartRepoRO(repo.ID)
		} else {
			okf("Ready to synchronize %s (read-write)", repo.ID)
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

	<-stop
}

func setupUPnP() int {
	var externalPort = 0
	if len(cfg.Options.ListenAddress) == 1 {
		_, portStr, err := net.SplitHostPort(cfg.Options.ListenAddress[0])
		if err != nil {
			warnln(err)
		} else {
			// Set up incoming port forwarding, if necessary and possible
			port, _ := strconv.Atoi(portStr)
			igd, err := upnp.Discover()
			if err == nil {
				for i := 0; i < 10; i++ {
					r := 1024 + rand.Intn(65535-1024)
					err := igd.AddPortMapping(upnp.TCP, r, port, "syncthing", 0)
					if err == nil {
						externalPort = r
						infoln("Created UPnP port mapping - external port", externalPort)
						break
					}
				}
				if externalPort == 0 {
					warnln("Failed to create UPnP port mapping")
				}
			} else {
				infof("No UPnP IGD device found, no port mapping created (%v)", err)
			}
		}
	} else {
		warnln("Multiple listening addresses; not attempting UPnP port mapping")
	}
	return externalPort
}

func resetRepositories() {
	suffix := fmt.Sprintf(".syncthing-reset-%d", time.Now().UnixNano())
	for _, repo := range cfg.Repositories {
		if _, err := os.Stat(repo.Directory); err == nil {
			infof("Reset: Moving %s -> %s", repo.Directory, repo.Directory+suffix)
			os.Rename(repo.Directory, repo.Directory+suffix)
		}
	}

	pat := filepath.Join(confDir, "*.idx.gz")
	idxs, err := filepath.Glob(pat)
	if err == nil {
		for _, idx := range idxs {
			infof("Reset: Removing %s", idx)
			os.Remove(idx)
		}
	}
}

func restart() {
	infoln("Restarting")
	if os.Getenv("SMF_FMRI") != "" || os.Getenv("STNORESTART") != "" {
		// Solaris SMF
		infoln("Service manager detected; exit instead of restart")
		stop <- true
		return
	}

	env := os.Environ()
	if len(os.Getenv("STRESTART")) == 0 {
		env = append(env, "STRESTART=1")
	}
	pgm, err := exec.LookPath(os.Args[0])
	if err != nil {
		warnln(err)
		return
	}
	proc, err := os.StartProcess(pgm, os.Args, &os.ProcAttr{
		Env:   env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		fatalln(err)
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
			warnln(err)
			continue
		}

		err = writeConfigXML(fd, cfg)
		if err != nil {
			warnln(err)
			fd.Close()
			continue
		}

		err = fd.Close()
		if err != nil {
			warnln(err)
			continue
		}

		err = Rename(cfgFile+".tmp", cfgFile)
		if err != nil {
			warnln(err)
		}
	}
}

func saveConfig() {
	saveConfigCh <- struct{}{}
}

func listenConnect(myID string, m *Model, tlsCfg *tls.Config) {
	var conns = make(chan *tls.Conn)

	// Listen
	for _, addr := range cfg.Options.ListenAddress {
		addr := addr
		go func() {
			if debugNet {
				dlog.Println("listening on", addr)
			}
			l, err := tls.Listen("tcp", addr, tlsCfg)
			fatalErr(err)

			for {
				conn, err := l.Accept()
				if err != nil {
					warnln(err)
					continue
				}

				if debugNet {
					dlog.Println("connect from", conn.RemoteAddr())
				}

				tc := conn.(*tls.Conn)
				err = tc.Handshake()
				if err != nil {
					warnln(err)
					tc.Close()
					continue
				}

				conns <- tc
			}
		}()
	}

	// Connect
	go func() {
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
						dlog.Println("dial", nodeCfg.NodeID, addr)
					}
					conn, err := tls.Dial("tcp", addr, tlsCfg)
					if err != nil {
						if debugNet {
							dlog.Println(err)
						}
						continue
					}

					conns <- conn
					continue nextNode
				}
			}

			time.Sleep(time.Duration(cfg.Options.ReconnectIntervalS) * time.Second)
		}
	}()

next:
	for conn := range conns {
		certs := conn.ConnectionState().PeerCertificates
		if l := len(certs); l != 1 {
			warnf("Got peer certificate list of length %d != 1; protocol error", l)
			conn.Close()
			continue
		}
		remoteID := certID(certs[0].Raw)

		if remoteID == myID {
			warnf("Connected to myself (%s) - should not happen", remoteID)
			conn.Close()
			continue
		}

		if m.ConnectedTo(remoteID) {
			warnf("Connected to already connected node (%s)", remoteID)
			conn.Close()
			continue
		}

		for _, nodeCfg := range cfg.Nodes {
			if nodeCfg.NodeID == remoteID {
				var wr io.Writer = conn
				if rateBucket != nil {
					wr = &limitedWriter{conn, rateBucket}
				}
				protoConn := protocol.NewConnection(remoteID, conn, wr, m)
				m.AddConnection(conn, protoConn)
				continue next
			}
		}
		conn.Close()
	}
}

func discovery(extPort int) *discover.Discoverer {
	disc, err := discover.NewDiscoverer(myID, cfg.Options.ListenAddress)
	if err != nil {
		warnf("No discovery possible (%v)", err)
		return nil
	}

	if cfg.Options.LocalAnnEnabled {
		infoln("Sending local discovery announcements")
		disc.StartLocal()
	}

	if cfg.Options.GlobalAnnEnabled {
		infoln("Sending global discovery announcements")
		disc.StartGlobal(cfg.Options.GlobalAnnServer, uint16(extPort))
	}

	return disc
}

func ensureDir(dir string, mode int) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0700)
		fatalErr(err)
	} else if mode >= 0 && err == nil && int(fi.Mode()&0777) != mode {
		err := os.Chmod(dir, os.FileMode(mode))
		fatalErr(err)
	}
}

func getDefaultConfDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("AppData"), "Syncthing")

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
	if runtime.GOOS == "windows" || !strings.HasPrefix(p, "~/") {
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
		fatalln("No home directory found - set $HOME (or the platform equivalent).")
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
