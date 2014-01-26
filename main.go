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
	"github.com/calmh/syncthing/model"
	"github.com/calmh/syncthing/protocol"
)

var opts Options
var Version string = "unknown-dev"

const (
	confFileName = "syncthing.ini"
)

var (
	myID      string
	config    ini.Config
	nodeAddrs = make(map[string][]string)
)

var (
	showVersion bool
	showConfig  bool
	confDir     string
	trace       string
	profiler    string
)

func main() {
	log.SetOutput(os.Stderr)
	logger = log.New(os.Stderr, "", log.Flags())

	flag.StringVar(&confDir, "home", "~/.syncthing", "Set configuration directory")
	flag.BoolVar(&showConfig, "config", false, "Print current configuration")
	flag.StringVar(&trace, "debug.trace", "", "(connect,net,idx,file,pull)")
	flag.StringVar(&profiler, "debug.profiler", "", "(addr)")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Usage = usageFor(flag.CommandLine, "syncthing [options]")
	flag.Parse()

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

	// Load the configuration file, if it exists.
	// If it does not, create a template.

	cfgFile := path.Join(confDir, confFileName)
	cf, err := os.Open(cfgFile)

	if err != nil {
		infoln("My ID:", myID)

		infoln("No config file; creating a template")

		loadConfig(nil, &opts) //loads defaults
		fd, err := os.Create(cfgFile)
		if err != nil {
			fatalln(err)
		}

		writeConfig(fd, "~/Sync", map[string]string{myID: "dynamic"}, opts, true)
		fd.Close()
		infof("Edit %s to suit and restart syncthing.", cfgFile)

		os.Exit(0)
	}

	config = ini.Parse(cf)
	cf.Close()

	loadConfig(config.OptionMap("settings"), &opts)

	if showConfig {
		writeConfig(os.Stdout,
			config.Get("repository", "dir"),
			config.OptionMap("nodes"), opts, false)
		os.Exit(0)
	}

	infoln("Version", Version)
	infoln("My ID:", myID)

	var dir = expandTilde(config.Get("repository", "dir"))
	if len(dir) == 0 {
		fatalln("No repository directory. Set dir under [repository] in syncthing.ini.")
	}

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

	cfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{"bep/1.0"},
		ServerName:             myID,
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
		MinVersion:             tls.VersionTLS12,
	}

	// Create a map of desired node connections based on the configuration file
	// directives.

	for nodeID, addrs := range config.OptionMap("nodes") {
		addrs := strings.Fields(addrs)
		nodeAddrs[nodeID] = addrs
	}

	ensureDir(dir, -1)
	m := model.NewModel(dir, opts.MaxChangeBW*1000)
	for _, t := range strings.Split(trace, ",") {
		m.Trace(t)
	}
	if opts.LimitRate > 0 {
		m.LimitRate(opts.LimitRate)
	}

	// GUI
	if opts.GUI && opts.GUIAddr != "" {
		host, port, err := net.SplitHostPort(opts.GUIAddr)
		if err != nil {
			warnf("Cannot start GUI on %q: %v", opts.GUIAddr, err)
		} else {
			if len(host) > 0 {
				infof("Starting web GUI on http://%s", opts.GUIAddr)
			} else {
				infof("Starting web GUI on port %s", port)
			}
			startGUI(opts.GUIAddr, m)
		}
	}

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	infoln("Populating repository index")
	updateLocalModel(m)

	// Routine to listen for incoming connections
	infoln("Listening for incoming connections")
	go listen(myID, opts.Listen, m, cfg)

	// Routine to connect out to configured nodes
	infoln("Attempting to connect to other nodes")
	go connect(myID, opts.Listen, nodeAddrs, m, cfg)

	// Routine to pull blocks from other nodes to synchronize the local
	// repository. Does not run when we are in read only (publish only) mode.
	if !opts.ReadOnly {
		if opts.Delete {
			infoln("Deletes from peer nodes are allowed")
		} else {
			infoln("Deletes from peer nodes will be ignored")
		}
		okln("Ready to synchronize (read-write)")
		m.StartRW(opts.Delete, opts.ParallelRequests)
	} else {
		okln("Ready to synchronize (read only; no external updates accepted)")
	}

	// Periodically scan the repository and update the local model.
	// XXX: Should use some fsnotify mechanism.
	go func() {
		for {
			time.Sleep(opts.ScanInterval)
			if m.LocalAge() > opts.ScanInterval.Seconds()/2 {
				updateLocalModel(m)
			}
		}
	}()

	// Periodically print statistics
	go printStatsLoop(m)

	select {}
}

func printStatsLoop(m *model.Model) {
	var lastUpdated int64
	var lastStats = make(map[string]model.ConnectionInfo)

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

func listen(myID string, addr string, m *model.Model, cfg *tls.Config) {
	l, err := tls.Listen("tcp", addr, cfg)
	fatalErr(err)

	connOpts := map[string]string{
		"clientId":      "syncthing",
		"clientVersion": Version,
	}

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

		for nodeID := range nodeAddrs {
			if nodeID == remoteID {
				protoConn := protocol.NewConnection(remoteID, conn, conn, m, connOpts)
				m.AddConnection(conn, protoConn)
				continue listen
			}
		}
		conn.Close()
	}
}

func connect(myID string, addr string, nodeAddrs map[string][]string, m *model.Model, cfg *tls.Config) {
	_, portstr, err := net.SplitHostPort(addr)
	fatalErr(err)
	port, _ := strconv.Atoi(portstr)

	if !opts.LocalDiscovery {
		port = -1
	} else {
		infoln("Sending local discovery announcements")
	}

	if !opts.ExternalDiscovery {
		opts.ExternalServer = ""
	} else {
		infoln("Sending external discovery announcements")
	}

	disc, err := discover.NewDiscoverer(myID, port, opts.ExternalServer)

	if err != nil {
		warnf("No discovery possible (%v)", err)
	}

	connOpts := map[string]string{
		"clientId":      "syncthing",
		"clientVersion": Version,
	}

	for {
	nextNode:
		for nodeID, addrs := range nodeAddrs {
			if nodeID == myID {
				continue
			}
			if m.ConnectedTo(nodeID) {
				continue
			}
			for _, addr := range addrs {
				if addr == "dynamic" {
					var ok bool
					if disc != nil {
						addr, ok = disc.Lookup(nodeID)
					}
					if !ok {
						continue
					}
				}

				if strings.Contains(trace, "connect") {
					debugln("NET: Dial", nodeID, addr)
				}
				conn, err := tls.Dial("tcp", addr, cfg)
				if err != nil {
					if strings.Contains(trace, "connect") {
						debugln("NET:", err)
					}
					continue
				}

				remoteID := certId(conn.ConnectionState().PeerCertificates[0].Raw)
				if remoteID != nodeID {
					warnln("Unexpected nodeID", remoteID, "!=", nodeID)
					conn.Close()
					continue
				}

				protoConn := protocol.NewConnection(remoteID, conn, conn, m, connOpts)
				m.AddConnection(conn, protoConn)
				continue nextNode
			}
		}

		time.Sleep(opts.ConnInterval)
	}
}

func updateLocalModel(m *model.Model) {
	files, _ := m.Walk(opts.Symlinks)
	m.ReplaceLocal(files)
	saveIndex(m)
}

func saveIndex(m *model.Model) {
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

func loadIndex(m *model.Model) {
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
