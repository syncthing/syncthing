package envpprof

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
)

var (
	pprofDir = filepath.Join(os.Getenv("HOME"), "pprof")
	heap     bool
)

func writeHeapProfile() {
	os.Mkdir(pprofDir, 0750)
	f, err := ioutil.TempFile(pprofDir, "heap")
	if err != nil {
		log.Printf("error creating heap profile file: %s", err)
		return
	}
	defer f.Close()
	pprof.WriteHeapProfile(f)
	log.Printf("wrote heap profile to %q", f.Name())
}

func Stop() {
	pprof.StopCPUProfile()
	if heap {
		writeHeapProfile()
	}
}

func init() {
	_var := os.Getenv("GOPPROF")
	if _var == "" {
		return
	}
	for _, item := range strings.Split(os.Getenv("GOPPROF"), ",") {
		equalsPos := strings.IndexByte(item, '=')
		var key, value string
		if equalsPos < 0 {
			key = item
		} else {
			key = item[:equalsPos]
			value = item[equalsPos+1:]
		}
		if value != "" {
			log.Printf("values not yet supported")
		}
		switch key {
		case "http":
			go func() {
				var l net.Listener
				for port := uint16(6061); port != 6060; port++ {
					var err error
					l, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
					if err == nil {
						break
					}
				}
				if l == nil {
					log.Print("unable to create envpprof listener for http")
					return
				}
				defer l.Close()
				log.Printf("envpprof serving http://%s", l.Addr())
				log.Printf("error serving http on envpprof listener: %s", http.Serve(l, nil))
			}()
		case "cpu":
			os.Mkdir(pprofDir, 0750)
			f, err := ioutil.TempFile(pprofDir, "cpu")
			if err != nil {
				log.Printf("error creating cpu pprof file: %s", err)
				break
			}
			err = pprof.StartCPUProfile(f)
			if err != nil {
				log.Printf("error starting cpu profiling: %s", err)
				break
			}
			log.Printf("cpu profiling to file %q", f.Name())
		case "block":
			runtime.SetBlockProfileRate(1)
		case "heap":
			heap = true
		default:
			log.Printf("unexpected GOPPROF key %q", key)
		}
	}
}
