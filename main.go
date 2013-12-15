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
	"github.com/calmh/syncthing/protocol"
	docopt "github.com/docopt/docopt.go"
)

const (
	confDirName  = ".syncthing"
	confFileName = "syncthing.ini"
	usage        = `Usage:
  syncthing [options]

Options:
  -l <addr>        Listening address [default: :22000]
  -p <addr>        Enable HTTP profiler on addr
  --home <path>    Home directory
  --delete         Delete files that were deleted on a peer node
  --ro             Local repository is read only
  --scan-intv <s>  Repository scan interval, in seconds [default: 60]
  --conn-intv <s>  Node reconnect interval, in seconds [default: 15]
  --no-stats       Don't print transfer statistics

Help Options:
  -h, --help       Show this help
  --version        Show version

Debug Options:
  --trace-file     Trace file operations
  --trace-net      Trace network operations
  --trace-idx      Trace sent indexes
`
)

var (
	config    ini.Config
	nodeAddrs = make(map[string][]string)
)

// Options
var (
	confDir    = path.Join(getHomeDir(), confDirName)
	addr       string
	prof       string
	readOnly   bool
	scanIntv   int
	connIntv   int
	traceNet   bool
	traceFile  bool
	traceIdx   bool
	printStats bool
	doDelete   bool
)

func main() {
	// Useful for debugging; to be adjusted.
	log.SetFlags(log.Ltime | log.Lshortfile)

	arguments, _ := docopt.Parse(usage, nil, true, "syncthing 0.1", false)

	addr = arguments["-l"].(string)
	prof, _ = arguments["-p"].(string)
	readOnly, _ = arguments["--ro"].(bool)

	if arguments["--home"] != nil {
		confDir, _ = arguments["--home"].(string)
	}

	scanIntv, _ = strconv.Atoi(arguments["--scan-intv"].(string))
	if scanIntv == 0 {
		fatalln("Invalid --scan-intv")
	}

	connIntv, _ = strconv.Atoi(arguments["--conn-intv"].(string))
	if connIntv == 0 {
		fatalln("Invalid --conn-intv")
	}

	doDelete = arguments["--delete"].(bool)
	traceFile = arguments["--trace-file"].(bool)
	traceNet = arguments["--trace-net"].(bool)
	traceIdx = arguments["--trace-idx"].(bool)
	printStats = !arguments["--no-stats"].(bool)

	// Ensure that our home directory exists and that we have a certificate and key.

	ensureDir(confDir)
	cert, err := loadCert(confDir)
	if err != nil {
		newCertificate(confDir)
		cert, err = loadCert(confDir)
		fatalErr(err)
	}

	myID := string(certId(cert.Certificate[0]))
	infoln("My ID:", myID)

	if prof != "" {
		okln("Profiler listening on", prof)
		go func() {
			http.ListenAndServe(prof, nil)
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

	cf, err := os.Open(path.Join(confDir, confFileName))
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

	m := NewModel(dir)

	// Walk the repository and update the local model before establishing any
	// connections to other nodes.

	infoln("Initial repository scan in progress")
	loadIndex(m)
	updateLocalModel(m)

	// Routine to listen for incoming connections
	infoln("Listening for incoming connections")
	go listen(myID, addr, m, cfg)

	// Routine to connect out to configured nodes
	infoln("Attempting to connect to other nodes")
	go connect(myID, addr, nodeAddrs, m, cfg)

	// Routine to pull blocks from other nodes to synchronize the local
	// repository. Does not run when we are in read only (publish only) mode.
	if !readOnly {
		infoln("Cleaning out incomplete synchronizations")
		CleanTempFiles(dir)
		okln("Ready to synchronize")
		m.Start()
	}

	// Periodically scan the repository and update the local model.
	// XXX: Should use some fsnotify mechanism.
	go func() {
		for {
			time.Sleep(time.Duration(scanIntv) * time.Second)
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

		if traceNet {
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

		warnln("Connect from unknown node", remoteID)
		conn.Close()
	}
}

func connect(myID string, addr string, nodeAddrs map[string][]string, m *Model, cfg *tls.Config) {
	_, portstr, err := net.SplitHostPort(addr)
	fatalErr(err)
	port, _ := strconv.Atoi(portstr)

	infoln("Starting local discovery")
	disc, err := discover.NewDiscoverer(myID, port)
	if err != nil {
		warnln("No local discovery possible")
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

				if traceNet {
					debugln("NET: Dial", nodeID, addr)
				}
				conn, err := tls.Dial("tcp", addr, cfg)
				if err != nil {
					if traceNet {
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
				if traceNet {
					t0 := time.Now()
					nc.Ping()
					timing("NET: Ping reply", t0)
				}
				continue nextNode
			}
		}

		time.Sleep(time.Duration(connIntv) * time.Second)
	}
}

func updateLocalModel(m *Model) {
	files := Walk(m.Dir(), m)
	m.ReplaceLocal(files)
	saveIndex(m)
}

func saveIndex(m *Model) {
	fname := fmt.Sprintf("%x.idx", sha1.Sum([]byte(m.Dir())))
	idxf, err := os.Create(path.Join(confDir, fname))
	if err != nil {
		return
	}
	protocol.WriteIndex(idxf, m.ProtocolIndex())
	idxf.Close()
}

func loadIndex(m *Model) {
	fname := fmt.Sprintf("%x.idx", sha1.Sum([]byte(m.Dir())))
	idxf, err := os.Open(path.Join(confDir, fname))
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

func ensureDir(dir string) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0700)
		fatalErr(err)
	} else if fi.Mode()&0077 != 0 {
		err := os.Chmod(dir, 0700)
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
