package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	keyFile           = getEnvDefault("UR_KEY_FILE", "key.pem")
	certFile          = getEnvDefault("UR_CRT_FILE", "crt.pem")
	dbConn            = getEnvDefault("UR_DB_URL", "postgres://user:password@localhost/ur?sslmode=disable")
	listenAddr        = getEnvDefault("UR_LISTEN", "0.0.0.0:8443")
	tpl               *template.Template
	compilerRe        = regexp.MustCompile(`\(([A-Za-z0-9()., -]+) [\w-]+ \w+\) ([\w@-]+)`)
	aggregateVersions = []string{"v0.7", "v0.8", "v0.9", "v0.10"}
)

var funcs = map[string]interface{}{
	"commatize": commatize,
	"number":    number,
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

type report struct {
	Received time.Time // Only from DB

	UniqueID       string
	Version        string
	LongVersion    string
	Platform       string
	NumFolders     int
	NumDevices     int
	TotFiles       int
	FolderMaxFiles int
	TotMiB         int
	FolderMaxMiB   int
	MemoryUsageMiB int
	SHA256Perf     float64
	MemorySize     int

	Date string
}

func (r *report) Validate() error {
	if r.UniqueID == "" || r.Version == "" || r.Platform == "" {
		return fmt.Errorf("missing required field")
	}
	if len(r.Date) != 8 {
		return fmt.Errorf("date not initialized")
	}
	return nil
}

func setupDB(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS Reports (
		Received TIMESTAMP NOT NULL,
		UniqueID VARCHAR(32) NOT NULL,
		Version VARCHAR(32) NOT NULL,
		LongVersion VARCHAR(256) NOT NULL,
		Platform VARCHAR(32) NOT NULL,
		NumFolders INTEGER NOT NULL,
		NumDevices INTEGER NOT NULL,
		TotFiles INTEGER NOT NULL,
		FolderMaxFiles INTEGER NOT NULL,
		TotMiB INTEGER NOT NULL,
		FolderMaxMiB INTEGER NOT NULL,
		MemoryUsageMiB INTEGER NOT NULL,
		SHA256Perf DOUBLE PRECISION NOT NULL,
		MemorySize INTEGER NOT NULL,
		Date VARCHAR(8) NOT NULL
	)`)
	if err != nil {
		return err
	}

	row := db.QueryRow(`SELECT 'UniqueIDIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE UNIQUE INDEX UniqueIDIndex ON Reports (Date, UniqueID)`)
	}

	row = db.QueryRow(`SELECT 'ReceivedIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX ReceivedIndex ON Reports (Received)`)
	}

	return err
}

func insertReport(db *sql.DB, r report) error {
	_, err := db.Exec(`INSERT INTO Reports VALUES (TIMEZONE('UTC', NOW()), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		r.UniqueID, r.Version, r.LongVersion, r.Platform, r.NumFolders,
		r.NumDevices, r.TotFiles, r.FolderMaxFiles, r.TotMiB, r.FolderMaxMiB,
		r.MemoryUsageMiB, r.SHA256Perf, r.MemorySize, r.Date)

	return err
}

type withDBFunc func(*sql.DB, http.ResponseWriter, *http.Request)

func withDB(db *sql.DB, f withDBFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(db, w, r)
	})
}

func main() {
	log.SetFlags(log.Ltime | log.Ldate)
	log.SetOutput(os.Stdout)

	// Template

	fd, err := os.Open("static/index.html")
	if err != nil {
		log.Fatalln("template:", err)
	}
	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Fatalln("template:", err)
	}
	fd.Close()
	tpl = template.Must(template.New("index.html").Funcs(funcs).Parse(string(bs)))

	// DB

	db, err := sql.Open("postgres", dbConn)
	if err != nil {
		log.Fatalln("database:", err)
	}
	err = setupDB(db)
	if err != nil {
		log.Fatalln("database:", err)
	}

	// TLS

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalln("tls:", err)
	}

	cfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		SessionTicketsDisabled: true,
	}

	// HTTPS

	listener, err := tls.Listen("tcp", listenAddr, cfg)
	if err != nil {
		log.Fatalln("https:", err)
	}

	srv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	http.HandleFunc("/", withDB(db, rootHandler))
	http.HandleFunc("/newdata", withDB(db, newDataHandler))
	http.HandleFunc("/summary.json", withDB(db, summaryHandler))
	http.HandleFunc("/movement.json", withDB(db, movementHandler))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	err = srv.Serve(listener)
	if err != nil {
		log.Fatalln("https:", err)
	}
}

var (
	cacheData []byte
	cacheTime time.Time
	cacheMut  sync.Mutex
)

const maxCacheTime = 5 * 60 * time.Second

func rootHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		cacheMut.Lock()
		defer cacheMut.Unlock()

		if time.Since(cacheTime) > maxCacheTime {
			rep := getReport(db)
			buf := new(bytes.Buffer)
			err := tpl.Execute(buf, rep)
			if err != nil {
				log.Println(err)
				http.Error(w, "Template Error", http.StatusInternalServerError)
				return
			}
			cacheData = buf.Bytes()
			cacheTime = time.Now()
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(cacheData)
	} else {
		http.Error(w, "Not found", 404)
		return
	}
}

func newDataHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var rep report
	rep.Date = time.Now().UTC().Format("20060102")

	lr := &io.LimitedReader{R: r.Body, N: 10240}
	if err := json.NewDecoder(lr).Decode(&rep); err != nil {
		log.Println("json decode:", err)
		http.Error(w, "JSON Decode Error", http.StatusInternalServerError)
		return
	}

	if err := rep.Validate(); err != nil {
		log.Println("validate:", err)
		log.Printf("%#v", rep)
		http.Error(w, "Validation Error", http.StatusInternalServerError)
		return
	}

	if err := insertReport(db, rep); err != nil {
		log.Println("insert:", err)
		log.Printf("%#v", rep)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}
}

func summaryHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getSummary(db)
	if err != nil {
		log.Println("summaryHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := s.MarshalJSON()
	if err != nil {
		log.Println("summaryHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

func movementHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getMovement(db)
	if err != nil {
		log.Println("movementHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := json.Marshal(s)
	if err != nil {
		log.Println("movementHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

type category struct {
	Values [4]float64
	Key    string
	Descr  string
	Unit   string
	Binary bool
}

func getReport(db *sql.DB) map[string]interface{} {
	nodes := 0
	var versions []string
	var platforms []string
	var oses []string
	var numFolders []int
	var numDevices []int
	var totFiles []int
	var maxFiles []int
	var totMiB []int
	var maxMiB []int
	var memoryUsage []int
	var sha256Perf []float64
	var memorySize []int
	var compilers []string
	var builders []string

	rows, err := db.Query(`SELECT * FROM Reports WHERE Received > now() - '1 day'::INTERVAL`)
	if err != nil {
		log.Println("sql:", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {

		var rep report
		err := rows.Scan(&rep.Received, &rep.UniqueID, &rep.Version,
			&rep.LongVersion, &rep.Platform, &rep.NumFolders, &rep.NumDevices,
			&rep.TotFiles, &rep.FolderMaxFiles, &rep.TotMiB, &rep.FolderMaxMiB,
			&rep.MemoryUsageMiB, &rep.SHA256Perf, &rep.MemorySize, &rep.Date)

		if err != nil {
			log.Println("sql:", err)
			return nil
		}

		nodes++
		versions = append(versions, transformVersion(rep.Version))
		platforms = append(platforms, rep.Platform)
		ps := strings.Split(rep.Platform, "-")
		oses = append(oses, ps[0])
		if m := compilerRe.FindStringSubmatch(rep.LongVersion); len(m) == 3 {
			compilers = append(compilers, m[1])
			builders = append(builders, m[2])
		}
		if rep.NumFolders > 0 {
			numFolders = append(numFolders, rep.NumFolders)
		}
		if rep.NumDevices > 0 {
			numDevices = append(numDevices, rep.NumDevices)
		}
		if rep.TotFiles > 0 {
			totFiles = append(totFiles, rep.TotFiles)
		}
		if rep.FolderMaxFiles > 0 {
			maxFiles = append(maxFiles, rep.FolderMaxFiles)
		}
		if rep.TotMiB > 0 {
			totMiB = append(totMiB, rep.TotMiB*(1<<20))
		}
		if rep.FolderMaxMiB > 0 {
			maxMiB = append(maxMiB, rep.FolderMaxMiB*(1<<20))
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
		Descr:  "Files Managed per Device",
	})

	categories = append(categories, category{
		Values: statsForInts(maxFiles),
		Descr:  "Files in Largest Folder",
	})

	categories = append(categories, category{
		Values: statsForInts(totMiB),
		Descr:  "Data Managed per Device",
		Unit:   "B",
		Binary: true,
	})

	categories = append(categories, category{
		Values: statsForInts(maxMiB),
		Descr:  "Data in Largest Folder",
		Unit:   "B",
		Binary: true,
	})

	categories = append(categories, category{
		Values: statsForInts(numDevices),
		Descr:  "Number of Devices in Cluster",
	})

	categories = append(categories, category{
		Values: statsForInts(numFolders),
		Descr:  "Number of Folders Configured",
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
	r["nodes"] = nodes
	r["categories"] = categories
	r["versions"] = analyticsFor(versions, 10)
	r["platforms"] = analyticsFor(platforms, 0)
	r["os"] = analyticsFor(oses, 0)
	r["compilers"] = analyticsFor(compilers, 12)
	r["builders"] = analyticsFor(builders, 12)

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

	// Truncate old versions to just the generation part
	for _, agg := range aggregateVersions {
		if strings.HasPrefix(v, agg) {
			return agg + ".x"
		}
	}

	return v
}

type summary struct {
	versions map[string]int   // version string to count index
	rows     map[string][]int // date to list of counts
}

func newSummary() summary {
	return summary{
		versions: make(map[string]int),
		rows:     make(map[string][]int),
	}
}

func (s *summary) setCount(date, version string, count int) {
	idx, ok := s.versions[version]
	if !ok {
		idx = len(s.versions)
		s.versions[version] = idx
	}

	row := s.rows[date]
	if len(row) <= idx {
		old := row
		row = make([]int, idx+1)
		copy(row, old)
		s.rows[date] = row
	}

	row[idx] = count
}

func (s *summary) MarshalJSON() ([]byte, error) {
	var versions []string
	for v := range s.versions {
		versions = append(versions, v)
	}
	sort.Strings(versions)

	headerRow := []interface{}{"Day"}
	for _, v := range versions {
		headerRow = append(headerRow, v)
	}

	var table [][]interface{}
	table = append(table, headerRow)

	var dates []string
	for k := range s.rows {
		dates = append(dates, k)
	}
	sort.Strings(dates)

	for _, date := range dates {
		row := []interface{}{date}
		for _, ver := range versions {
			idx := s.versions[ver]
			if len(s.rows[date]) > idx && s.rows[date][idx] > 0 {
				row = append(row, s.rows[date][idx])
			} else {
				row = append(row, nil)
			}
		}
		table = append(table, row)
	}

	return json.Marshal(table)
}

func getSummary(db *sql.DB) (summary, error) {
	s := newSummary()

	rows, err := db.Query(`SELECT Day, Version, Count FROM VersionSummary WHERE Day > now() - '1 year'::INTERVAL;`)
	if err != nil {
		return summary{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var day time.Time
		var ver string
		var num int
		err := rows.Scan(&day, &ver, &num)
		if err != nil {
			return summary{}, err
		}

		if ver == "v0.0" {
			// ?
			continue
		}

		// SUPER UGLY HACK to avoid having to do sorting properly
		if len(ver) == 4 { // v0.x
			ver = ver[:3] + "0" + ver[3:] // now v0.0x
		}

		s.setCount(day.Format("2006-01-02"), ver, num)
	}

	return s, nil
}

func getMovement(db *sql.DB) ([][]interface{}, error) {
	rows, err := db.Query(`SELECT Day, Added, Removed FROM UserMovement WHERE Day > now() - '1 year'::INTERVAL ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "Joined", "Left"},
	}

	for rows.Next() {
		var day time.Time
		var added, removed int
		err := rows.Scan(&day, &added, &removed)
		if err != nil {
			return nil, err
		}

		row := []interface{}{day.Format("2006-01-02"), added, -removed}
		if removed == 0 {
			row[2] = nil
		}

		res = append(res, row)
	}

	return res, nil
}
