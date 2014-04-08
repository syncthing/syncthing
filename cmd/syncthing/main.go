package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/calmh/syncthing/discover"
	"github.com/calmh/syncthing/protocol"
	"github.com/juju/ratelimit"
)

const BlockSize = 128 * 1024

var cfg Configuration
var Version = "unknown-dev"

var (
	myID       string
	confDir    string
	rateBucket *ratelimit.Bucket
)

const (
	usage      = "syncthing [options]"
	extraUsage = `The following enviroment variables are interpreted by syncthing:

 STNORESTART  Do not attempt to restart when requested to, instead just exit.
              Set this variable when running under a service manager such as
              runit, launchd, etc.

 STPROFILER   Set to a listen address such as "127.0.0.1:9090" to start the
              profiler with HTTP access.

 STTRACE      A comma separated string of facilities to trace. The valid
              facility strings:
              - "discover" (the node discovery package)
              - "files"    (file set store)
              - "idx"      (index sending and receiving)
              - "mc"       (multicast beacon)
              - "need"     (file need calculations)
              - "net"      (connecting and disconnecting, network messages)
              - "pull"     (file pull activity)
              - "scanner"  (the file change scanner)
              `
)

func main() {
	var reset bool
	var showVersion bool
	flag.StringVar(&confDir, "home", getDefaultConfDir(), "Set configuration directory")
	flag.BoolVar(&reset, "reset", false, "Prepare to resync from cluster")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Usage = usageFor(flag.CommandLine, usage, extraUsage)
	flag.Parse()

	if len(os.Getenv("STRESTART")) > 0 {
		// Give the parent process time to exit and release sockets etc.
		time.Sleep(1 * time.Second)
	}

	if showVersion {
		fmt.Printf("syncthing %s (%s %s-%s)\n", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if len(os.Getenv("GOGC")) == 0 {
		debug.SetGCPercent(25)
	}

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	confDir = expandTilde(confDir)

	// Ensure that our home directory exists and that we have a certificate and key.

	ensureDir(confDir, 0700)
	cert, err := loadCert(confDir)
	if err != nil {
		newCertificate(confDir)
		cert, err = loadCert(confDir)
		fatalErr(err)
	}

	myID = string(certID(cert.Certificate[0]))
	log.SetPrefix("[" + myID[0:5] + "] ")
	logger.SetPrefix("[" + myID[0:5] + "] ")

	infoln("Version", Version)
	infoln("My ID:", myID)

	// Prepare to be able to save configuration

	cfgFile := filepath.Join(confDir, "config.xml")
	go saveConfigLoop(cfgFile)

	// Load the configuration file, if it exists.
	// If it does not, create a template.

	cf, err := os.Open(cfgFile)
	if err == nil {
		// Read config.xml
		cfg, err = readConfigXML(cf)
		if err != nil {
			fatalln(err)
		}
		cf.Close()
	}

	if len(cfg.Repositories) == 0 {
		infoln("No config file; starting with empty defaults")

		cfg, err = readConfigXML(nil)
		cfg.Repositories = []RepositoryConfiguration{
			{
				ID:        "default",
				Directory: filepath.Join(getHomeDir(), "Sync"),
				Nodes:     []NodeConfiguration{{NodeID: myID}},
			},
		}
		cfg.Nodes = []NodeConfiguration{
			{NodeID: myID, Addresses: []string{"dynamic"}},
		}

		saveConfig()
		infof("Edit %s to taste or use the GUI\n", cfgFile)
	}

	if reset {
		resetRepositories()
		os.Exit(0)
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

	for i := range cfg.Repositories {
		cfg.Repositories[i].Nodes = cleanNodeList(cfg.Repositories[i].Nodes, myID)
		dir := expandTilde(cfg.Repositories[i].Directory)
		ensureDir(dir, -1)
		m.AddRepo(cfg.Repositories[i].ID, dir, cfg.Repositories[i].Nodes)
	}

	// GUI
	if cfg.Options.GUIEnabled && cfg.Options.GUIAddress != "" {
		addr, err := net.ResolveTCPAddr("tcp", cfg.Options.GUIAddress)
		if err != nil {
			warnf("Cannot start GUI on %q: %v", cfg.Options.GUIAddress, err)
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
			startGUI(cfg.Options.GUIAddress, m)
			if cfg.Options.StartBrowser && len(os.Getenv("STRESTART")) == 0 {
				openURL(fmt.Sprintf("http://%s:%d", hostOpen, addr.Port))
			}
		}
	}

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	infoln("Populating repository index")
	m.LoadIndexes(confDir)
	m.ScanRepos()
	m.SaveIndexes(confDir)

	connOpts := map[string]string{
		"clientId":      "syncthing",
		"clientVersion": Version,
		"clusterHash":   clusterHash(cfg.Repositories[0].Nodes),
	}

	// Routine to connect out to configured nodes
	disc := discovery()
	go listenConnect(myID, disc, m, tlsCfg, connOpts)

	for _, repo := range cfg.Repositories {
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

	select {}
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
		os.Exit(0)
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
	os.Exit(0)
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

func listenConnect(myID string, disc *discover.Discoverer, m *Model, tlsCfg *tls.Config, connOpts map[string]string) {
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
						if disc != nil {
							t := disc.Lookup(nodeCfg.NodeID)
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
		remoteID := certID(conn.ConnectionState().PeerCertificates[0].Raw)

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

		for _, nodeCfg := range cfg.Repositories[0].Nodes {
			if nodeCfg.NodeID == remoteID {
				var wr io.Writer = conn
				if rateBucket != nil {
					wr = &limitedWriter{conn, rateBucket}
				}
				protoConn := protocol.NewConnection(remoteID, conn, wr, m, connOpts)
				m.AddConnection(conn, protoConn)
				continue next
			}
		}
		conn.Close()
	}
}

func discovery() *discover.Discoverer {
	if !cfg.Options.LocalAnnEnabled {
		return nil
	}

	infoln("Sending local discovery announcements")

	if !cfg.Options.GlobalAnnEnabled {
		cfg.Options.GlobalAnnServer = ""
	} else {
		infoln("Sending external discovery announcements")
	}

	disc, err := discover.NewDiscoverer(myID, cfg.Options.ListenAddress, cfg.Options.GlobalAnnServer)

	if err != nil {
		warnf("No discovery possible (%v)", err)
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

func expandTilde(p string) string {
	if runtime.GOOS == "windows" {
		return p
	}

	if strings.HasPrefix(p, "~/") {
		return strings.Replace(p, "~", getUnixHomeDir(), 1)
	}
	return p
}

func getUnixHomeDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		fatalln("No home directory?")
	}
	return home
}

func getHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return getUnixHomeDir()
}

func getDefaultConfDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("AppData"), "syncthing")
	}
	return expandTilde("~/.syncthing")
}
