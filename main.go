package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var (
	keyFile  = flag.String("key", "", "Key file")
	certFile = flag.String("cert", "", "Certificate file")
	dbDir    = flag.String("db", "", "Database directory")
	port     = flag.Int("port", 8443, "Listen port")
)

func main() {
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	http.HandleFunc("/newdata", newDataHandler)
	http.HandleFunc("/report", reportHandler)
	http.Handle("/", http.FileServer(http.Dir("static")))

	err := http.ListenAndServeTLS(fmt.Sprintf(":%d", *port), *certFile, *keyFile, nil)
	if err != nil {
		log.Fatal(err)
	}
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

var cache map[string]interface{}
var cacheDate string
var cacheMut sync.Mutex

func reportHandler(w http.ResponseWriter, r *http.Request) {
	yesterday := time.Now().Add(-24 * time.Hour).Format("20060102")

	cacheMut.Lock()
	cacheMut.Unlock()

	if cacheDate != yesterday {
		cache = make(map[string]interface{})

		dir := filepath.Join(*dbDir, yesterday)

		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			http.Error(w, "Glob error", 500)
			return
		}

		nodes := 0
		versions := make(map[string]int)
		platforms := make(map[string]int)
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
			versions[rep.Version]++
			platforms[rep.Platform]++
			numRepos = append(numRepos, rep.NumRepos)
			numNodes = append(numNodes, rep.NumNodes)
			totFiles = append(totFiles, rep.TotFiles)
			maxFiles = append(maxFiles, rep.RepoMaxFiles)
			totMiB = append(totMiB, rep.TotMiB)
			maxMiB = append(maxMiB, rep.RepoMaxMiB)
			memoryUsage = append(memoryUsage, rep.MemoryUsageMiB)
			sha256Perf = append(sha256Perf, rep.SHA256Perf)
			if rep.MemorySize > 0 {
				memorySize = append(memorySize, rep.MemorySize)
			}
		}

		cache = make(map[string]interface{})
		cache["nodes"] = nodes
		cache["versions"] = versions
		cache["platforms"] = platforms
		cache["numRepos"] = statsForInts(numRepos)
		cache["numNodes"] = statsForInts(numNodes)
		cache["totFiles"] = statsForInts(totFiles)
		cache["maxFiles"] = statsForInts(maxFiles)
		cache["totMiB"] = statsForInts(totMiB)
		cache["maxMiB"] = statsForInts(maxMiB)
		cache["memoryUsage"] = statsForInts(memoryUsage)
		cache["sha256Perf"] = statsForFloats(sha256Perf)
		cache["memorySize"] = statsForInts(memorySize)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cache)
}

func statsForInts(data []int) map[string]int {
	sort.Ints(data)
	res := make(map[string]int, 4)
	res["min"] = data[0]
	res["med"] = data[len(data)/2]
	res["nfp"] = data[int(float64(len(data))*0.95)]
	res["max"] = data[len(data)-1]
	return res
}

func statsForFloats(data []float64) map[string]float64 {
	sort.Float64s(data)
	res := make(map[string]float64, 4)
	res["min"] = data[0]
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
