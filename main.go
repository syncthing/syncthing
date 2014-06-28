package main

import (
	"bytes"
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
	"sort"
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

type category struct {
	Key   string
	Descr string
	Unit  string
}

var categories = []category{
	{Key: "totFiles", Descr: "Files Managed per Node", Unit: ""},
	{Key: "maxFiles", Descr: "Files in Largest Repo", Unit: ""},
	{Key: "totMiB", Descr: "Data Managed per Node", Unit: "MiB"},
	{Key: "maxMiB", Descr: "Data in Largest Repo", Unit: "MiB"},
	{Key: "numNodes", Descr: "Number of Nodes in Cluster", Unit: ""},
	{Key: "numRepos", Descr: "Number of Repositories Configured", Unit: ""},
	{Key: "memoryUsage", Descr: "Memory Usage", Unit: "MiB"},
	{Key: "memorySize", Descr: "System Memory", Unit: "MiB"},
	{Key: "sha256Perf", Descr: "SHA-256 Hashing Performance", Unit: "MiB/s"},
}

var numRe = regexp.MustCompile(`\d\d\d$`)
var funcs = map[string]interface{}{
	"number": func(n interface{}) string {
		var s string
		switch n := n.(type) {
		case int:
			s = fmt.Sprint(n)
		case float64:
			s = fmt.Sprintf("%.02f", n)
		default:
			return fmt.Sprint(n)
		}

		var b bytes.Buffer
		l := len(s)
		for i := range s {
			b.Write([]byte{s[i]})
			if (l-i)%3 == 1 {
				b.WriteString(",")
			}
		}
		return b.String()
	},
	"commatize": commatize,
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

	err = http.ListenAndServeTLS(fmt.Sprintf(":%d", *port), *certFile, *keyFile, nil)
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
		l = append(l, f)
	}

	return l, nil
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
			totMiB = append(totMiB, rep.TotMiB)
		}
		if rep.RepoMaxMiB > 0 {
			maxMiB = append(maxMiB, rep.RepoMaxMiB)
		}
		if rep.MemoryUsageMiB > 0 {
			memoryUsage = append(memoryUsage, rep.MemoryUsageMiB)
		}
		if rep.SHA256Perf > 0 {
			sha256Perf = append(sha256Perf, rep.SHA256Perf)
		}
		if rep.MemorySize > 0 {
			memorySize = append(memorySize, rep.MemorySize)
		}
	}

	r := make(map[string]interface{})
	r["key"] = key
	r["nodes"] = nodes
	r["categories"] = categories
	r["versions"] = analyticsFor(versions)
	r["platforms"] = analyticsFor(platforms)
	r["os"] = analyticsFor(oses)
	r["numRepos"] = statsForInts(numRepos)
	r["numNodes"] = statsForInts(numNodes)
	r["totFiles"] = statsForInts(totFiles)
	r["maxFiles"] = statsForInts(maxFiles)
	r["totMiB"] = statsForInts(totMiB)
	r["maxMiB"] = statsForInts(maxMiB)
	r["memoryUsage"] = statsForInts(memoryUsage)
	r["sha256Perf"] = statsForFloats(sha256Perf)
	r["memorySize"] = statsForInts(memorySize)

	reportCache = r

	return r
}

type analytic struct {
	Key        string
	Count      int
	Percentage float64
}

type analyticList []analytic

func (l analyticList) Less(a, b int) bool {
	return l[b].Count < l[a].Count // inverse
}

func (l analyticList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l analyticList) Len() int {
	return len(l)
}

// Returns a list of frequency analytics for a given list of strings.
func analyticsFor(ss []string) []analytic {
	m := make(map[string]int)
	t := 0
	for _, s := range ss {
		m[s]++
		t++
	}

	l := make([]analytic, 0, len(m))
	for k, c := range m {
		l = append(l, analytic{k, c, 100 * float64(c) / float64(t)})
	}

	sort.Sort(analyticList(l))
	return l
}

func statsForInts(data []int) map[string]int {
	sort.Ints(data)
	res := make(map[string]int, 4)
	if len(data) == 0 {
		return res
	}
	res["fp"] = data[int(float64(len(data))*0.05)]
	res["med"] = data[len(data)/2]
	res["nfp"] = data[int(float64(len(data))*0.95)]
	res["max"] = data[len(data)-1]
	return res
}

func statsForFloats(data []float64) map[string]float64 {
	sort.Float64s(data)
	res := make(map[string]float64, 4)
	if len(data) == 0 {
		return res
	}
	res["fp"] = data[int(float64(len(data))*0.05)]
	res["med"] = data[len(data)/2]
	res["nfp"] = data[int(float64(len(data))*0.95)]
	res["max"] = data[len(data)-1]
	return res
}

func ensureDir(dir string, mode int) {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		os.MkdirAll(dir, 0700)
	} else if mode >= 0 && err == nil && int(fi.Mode()&0777) != mode {
		os.Chmod(dir, os.FileMode(mode))
	}
}

var vRe = regexp.MustCompile(`^(v\d+\.\d+\.\d+)-.+`)

// transformVersion returns a version number formatted correctly, with all
// development versions aggregated into one.
func transformVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if m := vRe.FindStringSubmatch(v); len(m) > 0 {
		return m[1] + " (+dev)"
	}
	return v
}

// commatize returns a number with sep as thousands separators. Handles
// integers and plain floats.
func commatize(sep, s string) string {
	var b bytes.Buffer
	fs := strings.SplitN(s, ".", 2)

	l := len(fs[0])
	for i := range fs[0] {
		b.Write([]byte{s[i]})
		if i < l-1 && (l-i)%3 == 1 {
			b.WriteString(sep)
		}
	}

	if len(fs) > 1 && len(fs[1]) > 0 {
		b.WriteString(".")
		b.WriteString(fs[1])
	}

	return b.String()
}

// timestamp returns a time stamp for the current hour, to be used as a cache key
func timestamp() string {
	return time.Now().Format("20060102T15")
}
