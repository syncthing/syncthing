package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	keyFile  = flag.String("key", "", "Key file")
	certFile = flag.String("cert", "", "Certificate file")
	dbDir    = flag.String("db", "", "Database directory")
	port     = flag.Int("port", 8443, "Listen port")
	tpl      *template.Template
)

var funcs = map[string]interface{}{
	"commatize": commatize,
	"number":    number,
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	fd, err := os.Open("static/index.html")
	if err != nil {
		log.Fatal(err)
	}
	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()
	tpl = template.Must(template.New("index.html").Funcs(funcs).Parse(string(bs)))

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/newdata", newDataHandler)
	http.HandleFunc("/report", reportHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Fatal(err)
	}

	cfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		SessionTicketsDisabled: true,
	}

	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", *port), cfg)
	if err != nil {
		log.Fatal(err)
	}

	srv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	err = srv.Serve(listener)
	if err != nil {
		log.Fatal(err)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		k := timestamp()
		rep := getReport(k)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err := tpl.Execute(w, rep)
		if err != nil {
			log.Println(err)
		}
	} else {
		http.Error(w, "Not found", 404)
	}
}

func reportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	k := timestamp()
	rep := getReport(k)
	json.NewEncoder(w).Encode(rep)
}

func newDataHandler(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("20060102")
	dir := filepath.Join(*dbDir, today)
	ensureDir(dir, 0700)

	var m map[string]interface{}
	lr := &io.LimitedReader{R: r.Body, N: 10240}
	err := json.NewDecoder(lr).Decode(&m)

	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), 500)
		return
	}

	id, ok := m["uniqueID"]
	if ok {
		idStr, ok := id.(string)
		if !ok {
			if err != nil {
				log.Println("No ID")
				http.Error(w, "No ID", 500)
				return
			}
		}

		f, err := os.Create(path.Join(dir, idStr+".json"))
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(f).Encode(m)
		f.Close()
	} else {
		log.Println("No ID")
		http.Error(w, "No ID", 500)
		return
	}
}

type report struct {
	UniqueID       string
	Version        string
	Platform       string
	NumRepos       int
	NumNodes       int
	TotFiles       int
	RepoMaxFiles   int
	TotMiB         int
	RepoMaxMiB     int
	MemoryUsageMiB int
	SHA256Perf     float64
	MemorySize     int
}

func fileList() ([]string, error) {
	files := make(map[string]string)
	t0 := time.Now().Add(-24 * time.Hour).Format("20060102")
	t1 := time.Now().Format("20060102")

	dir := filepath.Join(*dbDir, t0)
	gr, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range gr {
		bn := filepath.Base(f)
		files[bn] = f
	}

	dir = filepath.Join(*dbDir, t1)
	gr, err = filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range gr {
		bn := filepath.Base(f)
		files[bn] = f
	}

	l := make([]string, 0, len(files))
	for _, f := range files {
		si, err := os.Stat(f)
		if err != nil {
			continue
		}
		if time.Since(si.ModTime()) < 24*time.Hour {
			l = append(l, f)
		}
	}

	return l, nil
}

type category struct {
	Values [4]float64
	Key    string
	Descr  string
	Unit   string
	Binary bool
}

var reportCache map[string]interface{}
var reportMutex sync.Mutex

func getReport(key string) map[string]interface{} {
	reportMutex.Lock()
	defer reportMutex.Unlock()

	if k := reportCache["key"]; k == key {
		return reportCache
	}

	files, err := fileList()
	if err != nil {
		return nil
	}

	nodes := 0
	var versions []string
	var platforms []string
	var oses []string
	var numRepos []int
	var numNodes []int
	var totFiles []int
	var maxFiles []int
	var totMiB []int
	var maxMiB []int
	var memoryUsage []int
	var sha256Perf []float64
	var memorySize []int

	for _, fn := range files {
		f, err := os.Open(fn)
		if err != nil {
			continue
		}

		var rep report
		err = json.NewDecoder(f).Decode(&rep)
		if err != nil {
			continue
		}
		f.Close()

		nodes++
		versions = append(versions, transformVersion(rep.Version))
		platforms = append(platforms, rep.Platform)
		ps := strings.Split(rep.Platform, "-")
		oses = append(oses, ps[0])
		if rep.NumRepos > 0 {
			numRepos = append(numRepos, rep.NumRepos)
		}
		if rep.NumNodes > 0 {
			numNodes = append(numNodes, rep.NumNodes)
		}
		if rep.TotFiles > 0 {
			totFiles = append(totFiles, rep.TotFiles)
		}
		if rep.RepoMaxFiles > 0 {
			maxFiles = append(maxFiles, rep.RepoMaxFiles)
		}
		if rep.TotMiB > 0 {
			totMiB = append(totMiB, rep.TotMiB*(1<<20))
		}
		if rep.RepoMaxMiB > 0 {
			maxMiB = append(maxMiB, rep.RepoMaxMiB*(1<<20))
		}
		if rep.MemoryUsageMiB > 0 {
			memoryUsage = append(memoryUsage, rep.MemoryUsageMiB*(1<<20))
		}
		if rep.SHA256Perf > 0 {
			sha256Perf = append(sha256Perf, rep.SHA256Perf*(1<<20))
		}
		if rep.MemorySize > 0 {
			memorySize = append(memorySize, rep.MemorySize*(1<<20))
		}
	}

	var categories []category
	categories = append(categories, category{
		Values: statsForInts(totFiles),
		Descr:  "Files Managed per Node",
	})

	categories = append(categories, category{
		Values: statsForInts(maxFiles),
		Descr:  "Files in Largest Repo",
	})

	categories = append(categories, category{
		Values: statsForInts(totMiB),
		Descr:  "Data Managed per Node",
		Unit:   "B",
		Binary: true,
	})

	categories = append(categories, category{
		Values: statsForInts(maxMiB),
		Descr:  "Data in Largest Repo",
		Unit:   "B",
		Binary: true,
	})

	categories = append(categories, category{
		Values: statsForInts(numNodes),
		Descr:  "Number of Nodes in Cluster",
	})

	categories = append(categories, category{
		Values: statsForInts(numRepos),
		Descr:  "Number of Repositories Configured",
	})

	categories = append(categories, category{
		Values: statsForInts(memoryUsage),
		Descr:  "Memory Usage",
		Unit:   "B",
		Binary: true,
	})

	categories = append(categories, category{
		Values: statsForInts(memorySize),
		Descr:  "System Memory",
		Unit:   "B",
		Binary: true,
	})

	categories = append(categories, category{
		Values: statsForFloats(sha256Perf),
		Descr:  "SHA-256 Hashing Performance",
		Unit:   "B/s",
		Binary: true,
	})

	r := make(map[string]interface{})
	r["key"] = key
	r["nodes"] = nodes
	r["categories"] = categories
	r["versions"] = analyticsFor(versions)
	r["platforms"] = analyticsFor(platforms)
	r["os"] = analyticsFor(oses)

	reportCache = r

	return r
}

func ensureDir(dir string, mode int) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		os.MkdirAll(dir, 0700)
	} else if mode >= 0 && err == nil && int(fi.Mode()&0777) != mode {
		os.Chmod(dir, os.FileMode(mode))
	}
}

var vRe = regexp.MustCompile(`^(v\d+\.\d+\.\d+(?:-[a-z]\w+)?)[+\.-]`)

// transformVersion returns a version number formatted correctly, with all
// development versions aggregated into one.
func transformVersion(v string) string {
	if v == "unknown-dev" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if m := vRe.FindStringSubmatch(v); len(m) > 0 {
		return m[1] + " (+dev)"
	}
	return v
}

// timestamp returns a time stamp for the current hour, to be used as a cache key
func timestamp() string {
	return time.Now().Format("20060102T15")
}
