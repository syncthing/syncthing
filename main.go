package main

import (
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
	ConfDir      string           `short:"c" long:"cfg" description:"Configuration directory" default:"~/.syncthing" value-name:"DIR"`
	Listen       string           `short:"l" long:"listen" description:"Listen address" default:":22000" value-name:"ADDR"`
	ReadOnly     bool             `short:"r" long:"ro" description:"Repository is read only"`
	Delete       bool             `short:"d" long:"delete" description:"Delete files from repo when deleted from cluster"`
	NoSymlinks   bool             `long:"no-symlinks" description:"Don't follow first level symlinks in the repo"`
	ScanInterval time.Duration    `long:"scan-intv" description:"Repository scan interval" default:"60s" value-name:"INTV"`
	ConnInterval time.Duration    `long:"conn-intv" description:"Node reconnect interval" default:"60s" value-name:"INTV"`
	Discovery    DiscoveryOptions `group:"Discovery Options"`
	Debug        DebugOptions     `group:"Debugging Options"`
}

type DebugOptions struct {
	TraceFile bool   `long:"trace-file"`
	TraceNet  bool   `long:"trace-net"`
	TraceIdx  bool   `long:"trace-idx"`
	Profiler  string `long:"profiler" value-name:"ADDR"`
}

type DiscoveryOptions struct {
	ExternalServer      string `long:"ext-server" description:"External discovery server" value-name:"NAME" default:"syncthing.nym.se"`
	ExternalPort        int    `short:"e" long:"ext-port" description:"External listen port" value-name:"PORT" default:"22000"`
	NoExternalDiscovery bool   `short:"n" long:"no-ext-announce" description:"Do not announce presence externally"`
	NoLocalDiscovery    bool   `short:"N" long:"no-local-announce" description:"Do not announce presence locally"`
}

var opts Options
var Version string

const (
	confDirName  = ".syncthing"
	confFileName = "syncthing.ini"
)

var (
	config    ini.Config
	nodeAddrs = make(map[string][]string)
)

// Options
var (
	ConfDir = path.Join(getHomeDir(), confDirName)
)

func main() {
	// Useful for debugging; to be adjusted.
	log.SetFlags(log.Ltime | log.Lshortfile)

	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(0)
	}
	if strings.HasPrefix(opts.ConfDir, "~/") {
		opts.ConfDir = strings.Replace(opts.ConfDir, "~", getHomeDir(), 1)
	}

	infoln("Version", Version)

	// Ensure that our home directory exists and that we have a certificate and key.

	ensureDir(ConfDir, 0700)
	cert, err := loadCert(ConfDir)
	if err != nil {
		newCertificate(ConfDir)
		cert, err = loadCert(ConfDir)
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

	cf, err := os.Open(path.Join(ConfDir, confFileName))
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

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	infoln("Initial repository scan in progress")
	loadIndex(m)
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
			time.Sleep(opts.ScanInterval)
			updateLocalModel(m)
		}
	}()

	select {}
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
				nc := protocol.NewConnection(remoteID, conn, conn, m)
				m.AddNode(nc)
				okln("Connected to nodeID", remoteID, "(in)")
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

				nc := protocol.NewConnection(nodeID, conn, conn, m)
				okln("Connected to node", remoteID, "(out)")
				m.AddNode(nc)
				if opts.Debug.TraceNet {
					t0 := time.Now()
					nc.Ping()
					timing("NET: Ping reply", t0)
				}
				continue nextNode
			}
		}

		time.Sleep(opts.ConnInterval)
	}
}

func updateLocalModel(m *Model) {
	files := Walk(m.Dir(), m, !opts.NoSymlinks)
	m.ReplaceLocal(files)
	saveIndex(m)
}

func saveIndex(m *Model) {
	fname := fmt.Sprintf("%x.idx", sha1.Sum([]byte(m.Dir())))
	idxf, err := os.Create(path.Join(ConfDir, fname))
	if err != nil {
		return
	}
	protocol.WriteIndex(idxf, m.ProtocolIndex())
	idxf.Close()
}

func loadIndex(m *Model) {
	fname := fmt.Sprintf("%x.idx", sha1.Sum([]byte(m.Dir())))
	idxf, err := os.Open(path.Join(ConfDir, fname))
	if err != nil {
		return
	}
	defer idxf.Close()

	idx, err := protocol.ReadIndex(idxf)
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
