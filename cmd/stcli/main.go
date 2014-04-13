package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/calmh/syncthing/protocol"
)

var (
	exit    bool
	cmd     string
	confDir string
	target  string
	get     string
	pc      protocol.Connection
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	flag.StringVar(&cmd, "cmd", "idx", "Command")
	flag.StringVar(&confDir, "home", ".", "Certificates directory")
	flag.StringVar(&target, "target", "127.0.0.1:22000", "Target node")
	flag.StringVar(&get, "get", "", "Get file")
	flag.BoolVar(&exit, "exit", false, "Exit after command")
	flag.Parse()

	connect(target)

	select {}
}

func connect(target string) {
	cert, err := loadCert(confDir)
	if err != nil {
		log.Fatal(err)
	}

	myID := string(certID(cert.Certificate[0]))

	tlsCfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{"bep/1.0"},
		ServerName:             myID,
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
		MinVersion:             tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", target, tlsCfg)
	if err != nil {
		log.Fatal(err)
	}

	remoteID := certID(conn.ConnectionState().PeerCertificates[0].Raw)

	pc = protocol.NewConnection(remoteID, conn, conn, Model{})

	select {}
}

type Model struct {
}

func prtIndex(files []protocol.FileInfo) {
	for _, f := range files {
		log.Printf("%q (v:%d mod:%d flags:0%o nblocks:%d)", f.Name, f.Version, f.Modified, f.Flags, len(f.Blocks))
		for _, b := range f.Blocks {
			log.Printf("    %6d %x", b.Size, b.Hash)
		}
	}
}

func (m Model) Index(nodeID string, repo string, files []protocol.FileInfo) {
	log.Printf("Received index for repo %q", repo)
	if cmd == "idx" {
		prtIndex(files)
		if get != "" {
			for _, f := range files {
				if f.Name == get {
					go getFile(f)
					break
				}
			}
		} else if exit {
			os.Exit(0)
		}
	}
}

func getFile(f protocol.FileInfo) {
	fn := filepath.Base(f.Name)
	fd, err := os.Create(fn)
	if err != nil {
		log.Fatal(err)
	}

	var offset int64
	for _, b := range f.Blocks {
		log.Printf("Request %q %d - %d", f.Name, offset, offset+int64(b.Size))
		bs, err := pc.Request("default", f.Name, offset, int(b.Size))
		log.Printf(" - got %d bytes", len(bs))
		if err != nil {
			log.Fatal(err)
		}
		offset += int64(b.Size)
		fd.Write(bs)
	}

	fd.Close()
}

func (m Model) IndexUpdate(nodeID string, repo string, files []protocol.FileInfo) {
	log.Printf("Received index update for repo %q", repo)
	if cmd == "idx" {
		prtIndex(files)
		if exit {
			os.Exit(0)
		}
	}
}

func (m Model) ClusterConfig(nodeID string, config protocol.ClusterConfigMessage) {
	log.Println("Received cluster config")
	log.Printf("%#v", config)
}

func (m Model) Request(nodeID, repo string, name string, offset int64, size int) ([]byte, error) {
	log.Println("Received request")
	return nil, io.EOF
}

func (m Model) Close(nodeID string, err error) {
	log.Println("Received close")
}
