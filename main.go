package main

import (
	"compress/gzip"
	"crypto/sha1"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/calmh/ini"
	"github.com/calmh/syncthing/discover"
	flags "github.com/calmh/syncthing/github.com/jessevdk/go-flags"
	"github.com/calmh/syncthing/protocol"
)

type Options struct {
	ConfDir    string           `short:"c" long:"cfg" description:"Configuration directory" default:"~/.syncthing" value-name:"DIR"`
	Listen     string           `short:"l" long:"listen" description:"Listen address" default:":22000" value-name:"ADDR"`
	ReadOnly   bool             `short:"r" long:"ro" description:"Repository is read only"`
	Delete     bool             `short:"d" long:"delete" description:"Delete files deleted from cluster"`
	Rehash     bool             `long:"rehash" description:"Ignore cache and rehash all files in repository"`
	NoSymlinks bool             `long:"no-symlinks" description:"Don't follow first level symlinks in the repo"`
	NoStats    bool             `long:"no-stats" description:"Don't print model and connection statistics"`
	GUIAddr    string           `long:"gui" description:"GUI listen address" default:"" value-name:"ADDR"`
	Discovery  DiscoveryOptions `group:"Discovery Options"`
	Advanced   AdvancedOptions  `group:"Advanced Options"`
	Debug      DebugOptions     `group:"Debugging Options"`
}

type DebugOptions struct {
	LogSource bool   `long:"log-source"`
	TraceFile bool   `long:"trace-file"`
	TraceNet  bool   `long:"trace-net"`
	TraceIdx  bool   `long:"trace-idx"`
	TraceNeed bool   `long:"trace-need"`
	Profiler  string `long:"profiler" value-name:"ADDR"`
}

type DiscoveryOptions struct {
	ExternalServer      string `long:"ext-server" description:"External discovery server" value-name:"NAME" default:"syncthing.nym.se"`
	ExternalPort        int    `short:"e" long:"ext-port" description:"External listen port" value-name:"PORT" default:"22000"`
	NoExternalDiscovery bool   `short:"n" long:"no-ext-announce" description:"Do not announce presence externally"`
	NoLocalDiscovery    bool   `short:"N" long:"no-local-announce" description:"Do not announce presence locally"`
}

type AdvancedOptions struct {
	RequestsInFlight int           `long:"reqs-in-flight" description:"Parallell in flight requests per file" default:"4" value-name:"REQS"`
	FilesInFlight    int           `long:"files-in-flight" description:"Parallell in flight file pulls" default:"8" value-name:"FILES"`
	ScanInterval     time.Duration `long:"scan-intv" description:"Repository scan interval" default:"60s" value-name:"INTV"`
	ConnInterval     time.Duration `long:"conn-intv" description:"Node reconnect interval" default:"60s" value-name:"INTV"`
}

var opts Options
var Version string = "unknown-dev"

const (
	confDirName  = ".syncthing"
	confFileName = "syncthing.ini"
)

var (
	config    ini.Config
	nodeAddrs = make(map[string][]string)
)

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(0)
	}
	if opts.Debug.TraceFile || opts.Debug.TraceIdx || opts.Debug.TraceNet || opts.Debug.LogSource {
		logger = log.New(os.Stderr, "", log.Lshortfile|log.Ldate|log.Ltime|log.Lmicroseconds)
	}
	if strings.HasPrefix(opts.ConfDir, "~/") {
		opts.ConfDir = strings.Replace(opts.ConfDir, "~", getHomeDir(), 1)
	}

	infoln("Version", Version)

	// Ensure that our home directory exists and that we have a certificate and key.

	ensureDir(opts.ConfDir, 0700)
	cert, err := loadCert(opts.ConfDir)
	if err != nil {
		newCertificate(opts.ConfDir)
		cert, err = loadCert(opts.ConfDir)
		fatalErr(err)
	}

	myID := string(certId(cert.Certificate[0]))
	infoln("My ID:", myID)

	if opts.Debug.Profiler != "" {
		go func() {
			err := http.ListenAndServe(opts.Debug.Profiler, nil)
			if err != nil {
				warnln(err)
			}
		}()
	}

	// The TLS configuration is used for both the listening socket and outgoing
	// connections.

	cfg := &tls.Config{
		ClientAuth:         tls.RequestClientCert,
		ServerName:         "syncthing",
		NextProtos:         []string{"bep/1.0"},
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
	}

	// Load the configuration file, if it exists.

	cf, err := os.Open(path.Join(opts.ConfDir, confFileName))
	if err != nil {
		fatalln("No config file")
		config = ini.Config{}
	}
	config = ini.Parse(cf)
	cf.Close()

	var dir = config.Get("repository", "dir")

	// Create a map of desired node connections based on the configuration file
	// directives.

	for nodeID, addrs := range config.OptionMap("nodes") {
		addrs := strings.Fields(addrs)
		nodeAddrs[nodeID] = addrs
	}

	ensureDir(dir, -1)
	m := NewModel(dir)

	// GUI
	if opts.GUIAddr != "" {
		startGUI(opts.GUIAddr, m)
	}

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	if !opts.Rehash {
		infoln("Loading index cache")
		loadIndex(m)
	}
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
		infoln("Cleaning out incomplete synchronizations")
		CleanTempFiles(dir)
		okln("Ready to synchronize")
		m.Start()
	}

	// Periodically scan the repository and update the local model.
	// XXX: Should use some fsnotify mechanism.
	go func() {
		for {
			time.Sleep(opts.Advanced.ScanInterval)
			updateLocalModel(m)
		}
	}()

	if !opts.NoStats {
		// Periodically print statistics
		go printStatsLoop(m)
	}

	select {}
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
				infof("%s: %sb/s in, %sb/s out", node, MetricPrefix(inbps), MetricPrefix(outbps))
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
			infof("%6d files, %9sB in to synchronize", len(needFiles), BinaryPrefix(bytes))
		}
	}
}

func listen(myID string, addr string, m *Model, cfg *tls.Config) {
	l, err := tls.Listen("tcp", addr, cfg)
	fatalErr(err)

listen:
	for {
		conn, err := l.Accept()
		if err != nil {
			warnln(err)
			continue
		}

		if opts.Debug.TraceNet {
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
				m.AddConnection(conn, remoteID)
				continue listen
			}
		}
		conn.Close()
	}
}

func connect(myID string, addr string, nodeAddrs map[string][]string, m *Model, cfg *tls.Config) {
	_, portstr, err := net.SplitHostPort(addr)
	fatalErr(err)
	port, _ := strconv.Atoi(portstr)

	if opts.Discovery.NoLocalDiscovery {
		port = -1
	} else {
		infoln("Sending local discovery announcements")
	}

	if opts.Discovery.NoExternalDiscovery {
		opts.Discovery.ExternalPort = -1
	} else {
		infoln("Sending external discovery announcements")
	}

	disc, err := discover.NewDiscoverer(myID, port, opts.Discovery.ExternalPort, opts.Discovery.ExternalServer)

	if err != nil {
		warnf("No discovery possible (%v)", err)
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

				if opts.Debug.TraceNet {
					debugln("NET: Dial", nodeID, addr)
				}
				conn, err := tls.Dial("tcp", addr, cfg)
				if err != nil {
					if opts.Debug.TraceNet {
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

				m.AddConnection(conn, remoteID)
				continue nextNode
			}
		}

		time.Sleep(opts.Advanced.ConnInterval)
	}
}

func updateLocalModel(m *Model) {
	files := Walk(m.Dir(), m, !opts.NoSymlinks)
	m.ReplaceLocal(files)
	saveIndex(m)
}

func saveIndex(m *Model) {
	name := fmt.Sprintf("%x.idx.gz", sha1.Sum([]byte(m.Dir())))
	fullName := path.Join(opts.ConfDir, name)
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
	fname := fmt.Sprintf("%x.idx.gz", sha1.Sum([]byte(m.Dir())))
	idxf, err := os.Open(path.Join(opts.ConfDir, fname))
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
	m.SeedIndex(idx)
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

func getHomeDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		fatalln("No home directory?")
	}
	return home
}
