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

	// v2 fields

	URVersion  int
	NumCPU     int
	FolderUses struct {
		ReadOnly            int
		IgnorePerms         int
		IgnoreDelete        int
		AutoNormalize       int
		SimpleVersioning    int
		ExternalVersioning  int
		StaggeredVersioning int
		TrashcanVersioning  int
	}
	DeviceUses struct {
		Introducer       int
		CustomCertName   int
		CompressAlways   int
		CompressMetadata int
		CompressNever    int
		DynamicAddr      int
		StaticAddr       int
	}
	Announce struct {
		GlobalEnabled     bool
		LocalEnabled      bool
		DefaultServersDNS int
		DefaultServersIP  int
		OtherServers      int
	}
	Relays struct {
		Enabled        bool
		DefaultServers int
		OtherServers   int
	}
	UsesRateLimit        bool
	UpgradeAllowedManual bool
	UpgradeAllowedAuto   bool

	// Generated

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

func (r *report) FieldPointers() []interface{} {
	// All the fields of the report, in the same order as the database fields.
	return []interface{}{
		&r.Received, &r.UniqueID, &r.Version, &r.LongVersion, &r.Platform,
		&r.NumFolders, &r.NumDevices, &r.TotFiles, &r.FolderMaxFiles,
		&r.TotMiB, &r.FolderMaxMiB, &r.MemoryUsageMiB, &r.SHA256Perf,
		&r.MemorySize, &r.Date, &r.URVersion, &r.NumCPU,
		&r.FolderUses.ReadOnly, &r.FolderUses.IgnorePerms, &r.FolderUses.IgnoreDelete,
		&r.FolderUses.AutoNormalize, &r.DeviceUses.Introducer,
		&r.DeviceUses.CustomCertName, &r.DeviceUses.CompressAlways,
		&r.DeviceUses.CompressMetadata, &r.DeviceUses.CompressNever,
		&r.DeviceUses.DynamicAddr, &r.DeviceUses.StaticAddr,
		&r.Announce.GlobalEnabled, &r.Announce.LocalEnabled,
		&r.Announce.DefaultServersDNS, &r.Announce.DefaultServersIP,
		&r.Announce.OtherServers, &r.Relays.Enabled, &r.Relays.DefaultServers,
		&r.Relays.OtherServers, &r.UsesRateLimit, &r.UpgradeAllowedManual,
		&r.UpgradeAllowedAuto,
		&r.FolderUses.SimpleVersioning, &r.FolderUses.ExternalVersioning,
		&r.FolderUses.StaggeredVersioning, &r.FolderUses.TrashcanVersioning,
	}
}

func (r *report) FieldNames() []string {
	// The database fields that back this struct in PostgreSQL
	return []string{
		// V1
		"Received",
		"UniqueID",
		"Version",
		"LongVersion",
		"Platform",
		"NumFolders",
		"NumDevices",
		"TotFiles",
		"FolderMaxFiles",
		"TotMiB",
		"FolderMaxMiB",
		"MemoryUsageMiB",
		"SHA256Perf",
		"MemorySize",
		"Date",
		// V2
		"ReportVersion",
		"NumCPU",
		"FolderRO",
		"FolderIgnorePerms",
		"FolderIgnoreDelete",
		"FolderAutoNormalize",
		"DeviceIntroducer",
		"DeviceCustomCertName",
		"DeviceCompressAlways",
		"DeviceCompressMetadata",
		"DeviceCompressNever",
		"DeviceDynamicAddr",
		"DeviceStaticAddr",
		"AnnounceGlobalEnabled",
		"AnnounceLocalEnabled",
		"AnnounceDefaultServersDNS",
		"AnnounceDefaultServersIP",
		"AnnounceOtherServers",
		"RelayEnabled",
		"RelayDefaultServers",
		"RelayOtherServers",
		"RateLimitEnabled",
		"UpgradeAllowedManual",
		"UpgradeAllowedAuto",
		// v0.12.19+
		"FolderSimpleVersioning",
		"FolderExternalVersioning",
		"FolderStaggeredVersioning",
		"FolderTrashcanVersioning",
	}
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

	var t string
	row := db.QueryRow(`SELECT 'UniqueIDIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		log.Println(err)
		if _, err = db.Exec(`CREATE UNIQUE INDEX UniqueIDIndex ON Reports (Date, UniqueID)`); err != nil {
			return err
		}
	}

	row = db.QueryRow(`SELECT 'ReceivedIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		if _, err = db.Exec(`CREATE INDEX ReceivedIndex ON Reports (Received)`); err != nil {
			return err
		}
	}

	row = db.QueryRow(`SELECT attname FROM pg_attribute WHERE attrelid = (SELECT oid FROM pg_class WHERE relname = 'reports') AND attname = 'reportversion'`)
	if err := row.Scan(&t); err != nil {
		// The ReportVersion column doesn't exist; add the new columns.
		_, err = db.Exec(`ALTER TABLE Reports
		ADD COLUMN ReportVersion INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN NumCPU INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderRO  INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderIgnorePerms INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderIgnoreDelete INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderAutoNormalize INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceIntroducer INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceCustomCertName INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceCompressAlways INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceCompressMetadata INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceCompressNever INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceDynamicAddr INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN DeviceStaticAddr INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN AnnounceGlobalEnabled BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN AnnounceLocalEnabled BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN AnnounceDefaultServersDNS INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN AnnounceDefaultServersIP INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN AnnounceOtherServers INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN RelayEnabled BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN RelayDefaultServers INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN RelayOtherServers INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN RateLimitEnabled BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN UpgradeAllowedManual BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN UpgradeAllowedAuto BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN FolderSimpleVersioning INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderExternalVersioning INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderStaggeredVersioning INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderTrashcanVersioning INTEGER NOT NULL DEFAULT 0
		`)
		if err != nil {
			return err
		}
	}

	row = db.QueryRow(`SELECT 'ReportVersionIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		if _, err = db.Exec(`CREATE INDEX ReportVersionIndex ON Reports (ReportVersion)`); err != nil {
			return err
		}
	}

	return nil
}

func insertReport(db *sql.DB, r report) error {
	r.Received = time.Now().UTC()
	fields := r.FieldPointers()
	params := make([]string, len(fields))
	for i := range params {
		params[i] = fmt.Sprintf("$%d", i+1)
	}
	query := "INSERT INTO Reports (" + strings.Join(r.FieldNames(), ", ") + ") VALUES (" + strings.Join(params, ", ") + ")"
	_, err := db.Exec(query, fields...)

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

type feature struct {
	Key string
	Pct int
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

	v2Reports := 0
	features := map[string]int{
		"Rate limiting":                          0,
		"Upgrades allowed (automatic)":           0,
		"Upgrades allowed (manual)":              0,
		"Folders, automatic normalization":       0,
		"Folders, ignore deletes":                0,
		"Folders, ignore permissions":            0,
		"Folders, master mode":                   0,
		"Folders, simple versioning":             0,
		"Folders, external versioning":           0,
		"Folders, staggered versioning":          0,
		"Folders, trashcan versioning":           0,
		"Devices, compress always":               0,
		"Devices, compress metadata":             0,
		"Devices, compress nothing":              0,
		"Devices, custom certificate":            0,
		"Devices, dynamic addresses":             0,
		"Devices, static addresses":              0,
		"Devices, introducer":                    0,
		"Relaying, enabled":                      0,
		"Relaying, default relays":               0,
		"Relaying, other relays":                 0,
		"Discovery, global enabled":              0,
		"Discovery, local enabled":               0,
		"Discovery, default servers (using DNS)": 0,
		"Discovery, default servers (using IP)":  0,
		"Discovery, other servers":               0,
	}
	var numCPU []int

	var rep report

	rows, err := db.Query(`SELECT ` + strings.Join(rep.FieldNames(), ",") + ` FROM Reports WHERE Received > now() - '1 day'::INTERVAL`)
	if err != nil {
		log.Println("sql:", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(rep.FieldPointers()...)

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

		if rep.URVersion >= 2 {
			v2Reports++
			numCPU = append(numCPU, rep.NumCPU)
			if rep.UsesRateLimit {
				features["Rate limiting"]++
			}
			if rep.UpgradeAllowedAuto {
				features["Upgrades allowed (automatic)"]++
			}
			if rep.UpgradeAllowedManual {
				features["Upgrades allowed (manual)"]++
			}
			if rep.FolderUses.AutoNormalize > 0 {
				features["Folders, automatic normalization"]++
			}
			if rep.FolderUses.IgnoreDelete > 0 {
				features["Folders, ignore deletes"]++
			}
			if rep.FolderUses.IgnorePerms > 0 {
				features["Folders, ignore permissions"]++
			}
			if rep.FolderUses.ReadOnly > 0 {
				features["Folders, master mode"]++
			}
			if rep.FolderUses.SimpleVersioning > 0 {
				features["Folders, simple versioning"]++
			}
			if rep.FolderUses.ExternalVersioning > 0 {
				features["Folders, external versioning"]++
			}
			if rep.FolderUses.StaggeredVersioning > 0 {
				features["Folders, staggered versioning"]++
			}
			if rep.FolderUses.TrashcanVersioning > 0 {
				features["Folders, trashcan versioning"]++
			}
			if rep.DeviceUses.CompressAlways > 0 {
				features["Devices, compress always"]++
			}
			if rep.DeviceUses.CompressMetadata > 0 {
				features["Devices, compress metadata"]++
			}
			if rep.DeviceUses.CompressNever > 0 {
				features["Devices, compress nothing"]++
			}
			if rep.DeviceUses.CustomCertName > 0 {
				features["Devices, custom certificate"]++
			}
			if rep.DeviceUses.DynamicAddr > 0 {
				features["Devices, dynamic addresses"]++
			}
			if rep.DeviceUses.StaticAddr > 0 {
				features["Devices, static addresses"]++
			}
			if rep.DeviceUses.Introducer > 0 {
				features["Devices, introducer"]++
			}
			if rep.Relays.Enabled {
				features["Relaying, enabled"]++
			}
			if rep.Relays.DefaultServers > 0 {
				features["Relaying, default relays"]++
			}
			if rep.Relays.OtherServers > 0 {
				features["Relaying, other relays"]++
			}
			if rep.Announce.GlobalEnabled {
				features["Discovery, global enabled"]++
			}
			if rep.Announce.LocalEnabled {
				features["Discovery, local enabled"]++
			}
			if rep.Announce.DefaultServersDNS > 0 {
				features["Discovery, default servers (using DNS)"]++
			}
			if rep.Announce.DefaultServersIP > 0 {
				features["Discovery, default servers (using IP)"]++
			}
			if rep.Announce.DefaultServersIP > 0 {
				features["Discovery, other servers"]++
			}
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

	categories = append(categories, category{
		Values: statsForInts(numCPU),
		Descr:  "Number of CPU cores",
	})

	var featureList []feature
	var featureNames []string
	for key := range features {
		featureNames = append(featureNames, key)
	}
	sort.Strings(featureNames)
	if v2Reports > 0 {
		for _, key := range featureNames {
			featureList = append(featureList, feature{
				Key: key,
				Pct: (100 * features[key]) / v2Reports,
			})
		}
		sort.Sort(sort.Reverse(sortableFeatureList(featureList)))
	}

	r := make(map[string]interface{})
	r["nodes"] = nodes
	r["v2nodes"] = v2Reports
	r["categories"] = categories
	r["versions"] = analyticsFor(versions, 10)
	r["platforms"] = analyticsFor(platforms, 0)
	r["os"] = analyticsFor(oses, 0)
	r["compilers"] = analyticsFor(compilers, 12)
	r["builders"] = analyticsFor(builders, 12)
	r["features"] = featureList

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
	rows, err := db.Query(`SELECT Day, Added, Removed, Bounced FROM UserMovement WHERE Day > now() - '1 year'::INTERVAL ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "Joined", "Left", "Bounced"},
	}

	for rows.Next() {
		var day time.Time
		var added, removed, bounced int
		err := rows.Scan(&day, &added, &removed, &bounced)
		if err != nil {
			return nil, err
		}

		row := []interface{}{day.Format("2006-01-02"), added, -removed, bounced}
		if removed == 0 {
			row[2] = nil
		}
		if bounced == 0 {
			row[3] = nil
		}

		res = append(res, row)
	}

	return res, nil
}

type sortableFeatureList []feature

func (l sortableFeatureList) Len() int {
	return len(l)
}
func (l sortableFeatureList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l sortableFeatureList) Less(a, b int) bool {
	if l[a].Pct != l[b].Pct {
		return l[a].Pct < l[b].Pct
	}
	return l[a].Key > l[b].Key
}
