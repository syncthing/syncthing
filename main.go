package main

import (
	"compress/gzip"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/calmh/ini"
	"github.com/calmh/syncthing/discover"
	"github.com/calmh/syncthing/protocol"
)

var cfg Configuration
var Version string = "unknown-dev"

var (
	myID   string
	config ini.Config
)

var (
	showVersion  bool
	confDir      string
	trace        string
	profiler     string
	verbose      bool
	startupDelay int
)

func main() {
	flag.StringVar(&confDir, "home", "~/.syncthing", "Set configuration directory")
	flag.StringVar(&trace, "debug.trace", "", "(connect,net,idx,file,pull)")
	flag.StringVar(&profiler, "debug.profiler", "", "(addr)")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&verbose, "v", false, "Be more verbose")
	flag.IntVar(&startupDelay, "delay", 0, "Startup delay (s)")
	flag.Usage = usageFor(flag.CommandLine, "syncthing [options]")
	flag.Parse()

	if startupDelay > 0 {
		time.Sleep(time.Duration(startupDelay) * time.Second)
	}

	if showVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if len(os.Getenv("GOGC")) == 0 {
		debug.SetGCPercent(25)
	}

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	if len(trace) > 0 {
		log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime | log.Lmicroseconds)
		logger.SetFlags(log.Lshortfile | log.Ldate | log.Ltime | log.Lmicroseconds)
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

	myID = string(certId(cert.Certificate[0]))
	log.SetPrefix("[" + myID[0:5] + "] ")
	logger.SetPrefix("[" + myID[0:5] + "] ")

	infoln("Version", Version)
	infoln("My ID:", myID)

	// Prepare to be able to save configuration

	cfgFile := path.Join(confDir, "config.xml")
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
	} else {
		// No config.xml, let's try the old syncthing.ini
		iniFile := path.Join(confDir, "syncthing.ini")
		cf, err := os.Open(iniFile)
		if err == nil {
			infoln("Migrating syncthing.ini to config.xml")
			iniCfg := ini.Parse(cf)
			cf.Close()
			os.Rename(iniFile, path.Join(confDir, "migrated_syncthing.ini"))

			cfg, _ = readConfigXML(nil)
			cfg.Repositories = []RepositoryConfiguration{
				{Directory: iniCfg.Get("repository", "dir")},
			}
			readConfigINI(iniCfg.OptionMap("settings"), &cfg.Options)
			for name, addrs := range iniCfg.OptionMap("nodes") {
				n := NodeConfiguration{
					NodeID:    name,
					Addresses: strings.Fields(addrs),
				}
				cfg.Repositories[0].Nodes = append(cfg.Repositories[0].Nodes, n)
			}

			saveConfig()
		}
	}

	if len(cfg.Repositories) == 0 {
		infoln("No config file; starting with empty defaults")

		cfg, err = readConfigXML(nil)
		cfg.Repositories = []RepositoryConfiguration{
			{
				Directory: "~/Sync",
				Nodes: []NodeConfiguration{
					{NodeID: myID, Addresses: []string{"dynamic"}},
				},
			},
		}

		saveConfig()
		infof("Edit %s to taste or use the GUI\n", cfgFile)
	}

	// Make sure the local node is in the node list.
	cfg.Repositories[0].Nodes = cleanNodeList(cfg.Repositories[0].Nodes, myID)

	var dir = expandTilde(cfg.Repositories[0].Directory)

	if len(profiler) > 0 {
		go func() {
			err := http.ListenAndServe(profiler, nil)
			if err != nil {
				warnln(err)
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

	ensureDir(dir, -1)
	m := NewModel(dir, cfg.Options.MaxChangeKbps*1000)
	for _, t := range strings.Split(trace, ",") {
		m.Trace(t)
	}
	if cfg.Options.MaxSendKbps > 0 {
		m.LimitRate(cfg.Options.MaxSendKbps)
	}

	// GUI
	if cfg.Options.GUIEnabled && cfg.Options.GUIAddress != "" {
		host, port, err := net.SplitHostPort(cfg.Options.GUIAddress)
		if err != nil {
			warnf("Cannot start GUI on %q: %v", cfg.Options.GUIAddress, err)
		} else {
			if len(host) > 0 {
				infof("Starting web GUI on http://%s", cfg.Options.GUIAddress)
			} else {
				infof("Starting web GUI on port %s", port)
			}
			startGUI(cfg.Options.GUIAddress, m)
		}
	}

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	if verbose {
		infoln("Populating repository index")
	}
	loadIndex(m)
	updateLocalModel(m)

	connOpts := map[string]string{
		"clientId":      "syncthing",
		"clientVersion": Version,
		"clusterHash":   clusterHash(cfg.Repositories[0].Nodes),
	}

	// Routine to listen for incoming connections
	if verbose {
		infoln("Listening for incoming connections")
	}
	for _, addr := range cfg.Options.ListenAddress {
		go listen(myID, addr, m, tlsCfg, connOpts)
	}

	// Routine to connect out to configured nodes
	if verbose {
		infoln("Attempting to connect to other nodes")
	}
	disc := discovery(cfg.Options.ListenAddress[0])
	go connect(myID, disc, m, tlsCfg, connOpts)

	// Routine to pull blocks from other nodes to synchronize the local
	// repository. Does not run when we are in read only (publish only) mode.
	if !cfg.Options.ReadOnly {
		if verbose {
			if cfg.Options.AllowDelete {
				infoln("Deletes from peer nodes are allowed")
			} else {
				infoln("Deletes from peer nodes will be ignored")
			}
			okln("Ready to synchronize (read-write)")
		}
		m.StartRW(cfg.Options.AllowDelete, cfg.Options.ParallelRequests)
	} else if verbose {
		okln("Ready to synchronize (read only; no external updates accepted)")
	}

	// Periodically scan the repository and update the local
	// XXX: Should use some fsnotify mechanism.
	go func() {
		td := time.Duration(cfg.Options.RescanIntervalS) * time.Second
		for {
			time.Sleep(td)
			if m.LocalAge() > (td / 2).Seconds() {
				updateLocalModel(m)
			}
		}
	}()

	if verbose {
		// Periodically print statistics
		go printStatsLoop(m)
	}

	select {}
}

func restart() {
	infoln("Restarting")
	args := os.Args
	doAppend := true
	for _, arg := range args {
		if arg == "-delay" {
			doAppend = false
			break
		}
	}
	if doAppend {
		args = append(args, "-delay", "2")
	}
	proc, err := os.StartProcess(os.Args[0], args, &os.ProcAttr{
		Env:   os.Environ(),
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

		err = os.Rename(cfgFile+".tmp", cfgFile)
		if err != nil {
			warnln(err)
		}
	}
}

func saveConfig() {
	saveConfigCh <- struct{}{}
}

func printStatsLoop(m *Model) {
	var lastUpdated int64
	var lastStats = make(map[string]ConnectionInfo)

	for {
		time.Sleep(60 * time.Second)

		for node, stats := range m.ConnectionStats() {
			secs := time.Since(lastStats[node].At).Seconds()
			inbps := 8 * int(float64(stats.InBytesTotal-lastStats[node].InBytesTotal)/secs)
			outbps := 8 * int(float64(stats.OutBytesTotal-lastStats[node].OutBytesTotal)/secs)

			if inbps+outbps > 0 {
				infof("%s: %sb/s in, %sb/s out", node[0:5], MetricPrefix(inbps), MetricPrefix(outbps))
			}

			lastStats[node] = stats
		}

		if lu := m.Generation(); lu > lastUpdated {
			lastUpdated = lu
			files, _, bytes := m.GlobalSize()
			infof("%6d files, %9sB in cluster", files, BinaryPrefix(bytes))
			files, _, bytes = m.LocalSize()
			infof("%6d files, %9sB in local repo", files, BinaryPrefix(bytes))
			needFiles, bytes := m.NeedFiles()
			infof("%6d files, %9sB to synchronize", len(needFiles), BinaryPrefix(bytes))
		}
	}
}

func listen(myID string, addr string, m *Model, tlsCfg *tls.Config, connOpts map[string]string) {
	if strings.Contains(trace, "connect") {
		debugln("NET: Listening on", addr)
	}
	l, err := tls.Listen("tcp", addr, tlsCfg)
	fatalErr(err)

listen:
	for {
		conn, err := l.Accept()
		if err != nil {
			warnln(err)
			continue
		}

		if strings.Contains(trace, "connect") {
			debugln("NET: Connect from", conn.RemoteAddr())
		}

		tc := conn.(*tls.Conn)
		err = tc.Handshake()
		if err != nil {
			warnln(err)
			tc.Close()
			continue
		}

		remoteID := certId(tc.ConnectionState().PeerCertificates[0].Raw)

		if remoteID == myID {
			warnf("Connect from myself (%s) - should not happen", remoteID)
			conn.Close()
			continue
		}

		if m.ConnectedTo(remoteID) {
			warnf("Connect from connected node (%s)", remoteID)
		}

		for _, nodeCfg := range cfg.Repositories[0].Nodes {
			if nodeCfg.NodeID == remoteID {
				protoConn := protocol.NewConnection(remoteID, conn, conn, m, connOpts)
				m.AddConnection(conn, protoConn)
				continue listen
			}
		}
		conn.Close()
	}
}

func discovery(addr string) *discover.Discoverer {
	_, portstr, err := net.SplitHostPort(addr)
	fatalErr(err)
	port, _ := strconv.Atoi(portstr)

	if !cfg.Options.LocalAnnEnabled {
		port = -1
	} else if verbose {
		infoln("Sending local discovery announcements")
	}

	if !cfg.Options.GlobalAnnEnabled {
		cfg.Options.GlobalAnnServer = ""
	} else if verbose {
		infoln("Sending external discovery announcements")
	}

	disc, err := discover.NewDiscoverer(myID, port, cfg.Options.GlobalAnnServer)

	if err != nil {
		warnf("No discovery possible (%v)", err)
	}

	return disc
}

func connect(myID string, disc *discover.Discoverer, m *Model, tlsCfg *tls.Config, connOpts map[string]string) {
	for {
	nextNode:
		for _, nodeCfg := range cfg.Repositories[0].Nodes {
			if nodeCfg.NodeID == myID {
				continue
			}
			if m.ConnectedTo(nodeCfg.NodeID) {
				continue
			}
			for _, addr := range nodeCfg.Addresses {
				if addr == "dynamic" {
					var ok bool
					if disc != nil {
						addr, ok = disc.Lookup(nodeCfg.NodeID)
					}
					if !ok {
						continue
					}
				}

				if strings.Contains(trace, "connect") {
					debugln("NET: Dial", nodeCfg.NodeID, addr)
				}
				conn, err := tls.Dial("tcp", addr, tlsCfg)
				if err != nil {
					if strings.Contains(trace, "connect") {
						debugln("NET:", err)
					}
					continue
				}

				remoteID := certId(conn.ConnectionState().PeerCertificates[0].Raw)
				if remoteID != nodeCfg.NodeID {
					warnln("Unexpected nodeID", remoteID, "!=", nodeCfg.NodeID)
					conn.Close()
					continue
				}

				protoConn := protocol.NewConnection(remoteID, conn, conn, m, connOpts)
				m.AddConnection(conn, protoConn)
				continue nextNode
			}
		}

		time.Sleep(time.Duration(cfg.Options.ReconnectIntervalS) * time.Second)
	}
}

func updateLocalModel(m *Model) {
	files, _ := m.Walk(cfg.Options.FollowSymlinks)
	m.ReplaceLocal(files)
	saveIndex(m)
}

func saveIndex(m *Model) {
	name := m.RepoID() + ".idx.gz"
	fullName := path.Join(confDir, name)
	idxf, err := os.Create(fullName + ".tmp")
	if err != nil {
		return
	}

	gzw := gzip.NewWriter(idxf)

	protocol.WriteIndex(gzw, m.ProtocolIndex())
	gzw.Close()
	idxf.Close()
	os.Rename(fullName+".tmp", fullName)
}

func loadIndex(m *Model) {
	name := m.RepoID() + ".idx.gz"
	idxf, err := os.Open(path.Join(confDir, name))
	if err != nil {
		return
	}
	defer idxf.Close()

	gzr, err := gzip.NewReader(idxf)
	if err != nil {
		return
	}
	defer gzr.Close()

	idx, err := protocol.ReadIndex(gzr)
	if err != nil {
		return
	}
	m.SeedLocal(idx)
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
	if strings.HasPrefix(p, "~/") {
		return strings.Replace(p, "~", getHomeDir(), 1)
	}
	return p
}

func getHomeDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		fatalln("No home directory?")
	}
	return home
}
