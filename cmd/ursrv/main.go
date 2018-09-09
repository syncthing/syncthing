package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lib/pq"
	"github.com/oschwald/geoip2-golang"
)

var (
	useHTTP          = os.Getenv("UR_USE_HTTP") != ""
	debug            = os.Getenv("UR_DEBUG") != ""
	keyFile          = getEnvDefault("UR_KEY_FILE", "key.pem")
	certFile         = getEnvDefault("UR_CRT_FILE", "crt.pem")
	dbConn           = getEnvDefault("UR_DB_URL", "postgres://user:password@localhost/ur?sslmode=disable")
	listenAddr       = getEnvDefault("UR_LISTEN", "0.0.0.0:8443")
	geoIPPath        = getEnvDefault("UR_GEOIP", "GeoLite2-City.mmdb")
	tpl              *template.Template
	compilerRe       = regexp.MustCompile(`\(([A-Za-z0-9()., -]+) \w+-\w+(?:| android| default)\) ([\w@.-]+)`)
	progressBarClass = []string{"", "progress-bar-success", "progress-bar-info", "progress-bar-warning", "progress-bar-danger"}
	featureOrder     = []string{"Various", "Folder", "Device", "Connection", "GUI"}
	knownVersions    = []string{"v2", "v3"}
)

var funcs = map[string]interface{}{
	"commatize":  commatize,
	"number":     number,
	"proportion": proportion,
	"counter": func() *counter {
		return &counter{}
	},
	"progressBarClassByIndex": func(a int) string {
		return progressBarClass[a%len(progressBarClass)]
	},
	"slice": func(numParts, whichPart int, input []feature) []feature {
		var part []feature
		perPart := (len(input) / numParts) + len(input)%2

		parts := make([][]feature, 0, numParts)
		for len(input) >= perPart {
			part, input = input[:perPart], input[perPart:]
			parts = append(parts, part)
		}
		if len(input) > 0 {
			parts = append(parts, input[:])
		}
		return parts[whichPart-1]
	},
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

type IntMap map[string]int

func (p IntMap) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *IntMap) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("Type assertion .([]byte) failed.")
	}

	var i map[string]int
	err := json.Unmarshal(source, &i)
	if err != nil {
		return err
	}

	*p = i
	return nil
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
		SendOnly            int
		ReceiveOnly         int
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

	// V2.5 fields (fields that were in v2 but never added to the database
	UpgradeAllowedPre bool
	RescanIntvs       pq.Int64Array

	// v3 fields

	Uptime                     int
	NATType                    string
	AlwaysLocalNets            bool
	CacheIgnoredFiles          bool
	OverwriteRemoteDeviceNames bool
	ProgressEmitterEnabled     bool
	CustomDefaultFolderPath    bool
	WeakHashSelection          string
	CustomTrafficClass         bool
	CustomTempIndexMinBlocks   bool
	TemporariesDisabled        bool
	TemporariesCustom          bool
	LimitBandwidthInLan        bool
	CustomReleaseURL           bool
	RestartOnWakeup            bool
	CustomStunServers          bool

	FolderUsesV3 struct {
		ScanProgressDisabled    int
		ConflictsDisabled       int
		ConflictsUnlimited      int
		ConflictsOther          int
		DisableSparseFiles      int
		DisableTempIndexes      int
		AlwaysWeakHash          int
		CustomWeakHashThreshold int
		FsWatcherEnabled        int
		PullOrder               IntMap
		FilesystemType          IntMap
		FsWatcherDelays         pq.Int64Array
	}

	GUIStats struct {
		Enabled                   int
		UseTLS                    int
		UseAuth                   int
		InsecureAdminAccess       int
		Debugging                 int
		InsecureSkipHostCheck     int
		InsecureAllowFrameLoading int
		ListenLocal               int
		ListenUnspecified         int
		Theme                     IntMap
	}

	BlockStats struct {
		Total             int
		Renamed           int
		Reused            int
		Pulled            int
		CopyOrigin        int
		CopyOriginShifted int
		CopyElsewhere     int
	}

	TransportStats IntMap

	IgnoreStats struct {
		Lines           int
		Inverts         int
		Folded          int
		Deletable       int
		Rooted          int
		Includes        int
		EscapedIncludes int
		DoubleStars     int
		Stars           int
	}

	// V3 fields added late in the RC
	WeakHashEnabled bool

	// Generated

	Date    string
	Address string
}

func (r *report) Validate() error {
	if r.UniqueID == "" || r.Version == "" || r.Platform == "" {
		return fmt.Errorf("missing required field")
	}
	if len(r.Date) != 8 {
		return fmt.Errorf("date not initialized")
	}

	// Some fields may not be null.
	if r.RescanIntvs == nil {
		r.RescanIntvs = []int64{}
	}
	if r.FolderUsesV3.FsWatcherDelays == nil {
		r.FolderUsesV3.FsWatcherDelays = []int64{}
	}

	return nil
}

func (r *report) FieldPointers() []interface{} {
	// All the fields of the report, in the same order as the database fields.
	return []interface{}{
		&r.Received, &r.UniqueID, &r.Version, &r.LongVersion, &r.Platform,
		&r.NumFolders, &r.NumDevices, &r.TotFiles, &r.FolderMaxFiles,
		&r.TotMiB, &r.FolderMaxMiB, &r.MemoryUsageMiB, &r.SHA256Perf,
		&r.MemorySize, &r.Date,
		// V2
		&r.URVersion, &r.NumCPU, &r.FolderUses.SendOnly, &r.FolderUses.IgnorePerms,
		&r.FolderUses.IgnoreDelete, &r.FolderUses.AutoNormalize, &r.DeviceUses.Introducer,
		&r.DeviceUses.CustomCertName, &r.DeviceUses.CompressAlways,
		&r.DeviceUses.CompressMetadata, &r.DeviceUses.CompressNever,
		&r.DeviceUses.DynamicAddr, &r.DeviceUses.StaticAddr,
		&r.Announce.GlobalEnabled, &r.Announce.LocalEnabled,
		&r.Announce.DefaultServersDNS, &r.Announce.DefaultServersIP,
		&r.Announce.OtherServers, &r.Relays.Enabled, &r.Relays.DefaultServers,
		&r.Relays.OtherServers, &r.UsesRateLimit, &r.UpgradeAllowedManual,
		&r.UpgradeAllowedAuto, &r.FolderUses.SimpleVersioning,
		&r.FolderUses.ExternalVersioning, &r.FolderUses.StaggeredVersioning,
		&r.FolderUses.TrashcanVersioning,

		// V2.5
		&r.UpgradeAllowedPre, &r.RescanIntvs,

		// V3
		&r.Uptime, &r.NATType, &r.AlwaysLocalNets, &r.CacheIgnoredFiles,
		&r.OverwriteRemoteDeviceNames, &r.ProgressEmitterEnabled, &r.CustomDefaultFolderPath,
		&r.WeakHashSelection, &r.CustomTrafficClass, &r.CustomTempIndexMinBlocks,
		&r.TemporariesDisabled, &r.TemporariesCustom, &r.LimitBandwidthInLan,
		&r.CustomReleaseURL, &r.RestartOnWakeup, &r.CustomStunServers,

		&r.FolderUsesV3.ScanProgressDisabled, &r.FolderUsesV3.ConflictsDisabled,
		&r.FolderUsesV3.ConflictsUnlimited, &r.FolderUsesV3.ConflictsOther,
		&r.FolderUsesV3.DisableSparseFiles, &r.FolderUsesV3.DisableTempIndexes,
		&r.FolderUsesV3.AlwaysWeakHash, &r.FolderUsesV3.CustomWeakHashThreshold,
		&r.FolderUsesV3.FsWatcherEnabled,

		&r.FolderUsesV3.PullOrder, &r.FolderUsesV3.FilesystemType,
		&r.FolderUsesV3.FsWatcherDelays,

		&r.GUIStats.Enabled, &r.GUIStats.UseTLS, &r.GUIStats.UseAuth,
		&r.GUIStats.InsecureAdminAccess,
		&r.GUIStats.Debugging, &r.GUIStats.InsecureSkipHostCheck,
		&r.GUIStats.InsecureAllowFrameLoading, &r.GUIStats.ListenLocal,
		&r.GUIStats.ListenUnspecified, &r.GUIStats.Theme,

		&r.BlockStats.Total, &r.BlockStats.Renamed,
		&r.BlockStats.Reused, &r.BlockStats.Pulled, &r.BlockStats.CopyOrigin,
		&r.BlockStats.CopyOriginShifted, &r.BlockStats.CopyElsewhere,

		&r.TransportStats,

		&r.IgnoreStats.Lines, &r.IgnoreStats.Inverts, &r.IgnoreStats.Folded,
		&r.IgnoreStats.Deletable, &r.IgnoreStats.Rooted, &r.IgnoreStats.Includes,
		&r.IgnoreStats.EscapedIncludes, &r.IgnoreStats.DoubleStars, &r.IgnoreStats.Stars,

		// V3 added late in the RC
		&r.WeakHashEnabled,
		&r.Address,

		// Receive only folders
		&r.FolderUses.ReceiveOnly,
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
		// V2.5
		"UpgradeAllowedPre",
		"RescanIntvs",
		// V3
		"Uptime",
		"NATType",
		"AlwaysLocalNets",
		"CacheIgnoredFiles",
		"OverwriteRemoteDeviceNames",
		"ProgressEmitterEnabled",
		"CustomDefaultFolderPath",
		"WeakHashSelection",
		"CustomTrafficClass",
		"CustomTempIndexMinBlocks",
		"TemporariesDisabled",
		"TemporariesCustom",
		"LimitBandwidthInLan",
		"CustomReleaseURL",
		"RestartOnWakeup",
		"CustomStunServers",

		"FolderScanProgressDisabled",
		"FolderConflictsDisabled",
		"FolderConflictsUnlimited",
		"FolderConflictsOther",
		"FolderDisableSparseFiles",
		"FolderDisableTempIndexes",
		"FolderAlwaysWeakHash",
		"FolderCustomWeakHashThreshold",
		"FolderFsWatcherEnabled",
		"FolderPullOrder",
		"FolderFilesystemType",
		"FolderFsWatcherDelays",

		"GUIEnabled",
		"GUIUseTLS",
		"GUIUseAuth",
		"GUIInsecureAdminAccess",
		"GUIDebugging",
		"GUIInsecureSkipHostCheck",
		"GUIInsecureAllowFrameLoading",
		"GUIListenLocal",
		"GUIListenUnspecified",
		"GUITheme",

		"BlocksTotal",
		"BlocksRenamed",
		"BlocksReused",
		"BlocksPulled",
		"BlocksCopyOrigin",
		"BlocksCopyOriginShifted",
		"BlocksCopyElsewhere",

		"Transport",

		"IgnoreLines",
		"IgnoreInverts",
		"IgnoreFolded",
		"IgnoreDeletable",
		"IgnoreRooted",
		"IgnoreIncludes",
		"IgnoreEscapedIncludes",
		"IgnoreDoubleStars",
		"IgnoreStars",

		// V3 added late in the RC
		"WeakHashEnabled",
		"Address",

		// Receive only folders
		"FolderRecvOnly",
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

	// V2

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

	// V2.5

	row = db.QueryRow(`SELECT attname FROM pg_attribute WHERE attrelid = (SELECT oid FROM pg_class WHERE relname = 'reports') AND attname = 'upgradeallowedpre'`)
	if err := row.Scan(&t); err != nil {
		// The ReportVersion column doesn't exist; add the new columns.
		_, err = db.Exec(`ALTER TABLE Reports
		ADD COLUMN UpgradeAllowedPre BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN RescanIntvs INT[] NOT NULL DEFAULT '{}'
		`)
		if err != nil {
			return err
		}
	}

	// V3

	row = db.QueryRow(`SELECT attname FROM pg_attribute WHERE attrelid = (SELECT oid FROM pg_class WHERE relname = 'reports') AND attname = 'uptime'`)
	if err := row.Scan(&t); err != nil {
		// The Uptime column doesn't exist; add the new columns.
		_, err = db.Exec(`ALTER TABLE Reports
		ADD COLUMN Uptime INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN NATType VARCHAR(32) NOT NULL DEFAULT '',
		ADD COLUMN AlwaysLocalNets BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN CacheIgnoredFiles BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN OverwriteRemoteDeviceNames BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN ProgressEmitterEnabled BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN CustomDefaultFolderPath BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN WeakHashSelection VARCHAR(32) NOT NULL DEFAULT '',
		ADD COLUMN CustomTrafficClass BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN CustomTempIndexMinBlocks BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN TemporariesDisabled BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN TemporariesCustom BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN LimitBandwidthInLan BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN CustomReleaseURL BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN RestartOnWakeup BOOLEAN NOT NULL DEFAULT FALSE,
		ADD COLUMN CustomStunServers BOOLEAN NOT NULL DEFAULT FALSE,

		ADD COLUMN FolderScanProgressDisabled INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderConflictsDisabled INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderConflictsUnlimited INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderConflictsOther INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderDisableSparseFiles INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderDisableTempIndexes INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderAlwaysWeakHash INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderCustomWeakHashThreshold INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderFsWatcherEnabled INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN FolderPullOrder JSONB NOT NULL DEFAULT '{}',
		ADD COLUMN FolderFilesystemType JSONB NOT NULL DEFAULT '{}',
		ADD COLUMN FolderFsWatcherDelays INT[] NOT NULL DEFAULT '{}',

		ADD COLUMN GUIEnabled INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIUseTLS INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIUseAuth INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIInsecureAdminAccess INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIDebugging INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIInsecureSkipHostCheck INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIInsecureAllowFrameLoading INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIListenLocal INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUIListenUnspecified INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN GUITheme JSONB NOT NULL DEFAULT '{}',

		ADD COLUMN BlocksTotal INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN BlocksRenamed INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN BlocksReused INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN BlocksPulled INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN BlocksCopyOrigin INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN BlocksCopyOriginShifted INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN BlocksCopyElsewhere INTEGER NOT NULL DEFAULT 0,

		ADD COLUMN Transport JSONB NOT NULL DEFAULT '{}',

		ADD COLUMN IgnoreLines INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreInverts INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreFolded INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreDeletable INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreRooted INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreIncludes INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreEscapedIncludes INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreDoubleStars INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IgnoreStars INTEGER NOT NULL DEFAULT 0
		`)
		if err != nil {
			return err
		}
	}

	// V3 added late in the RC

	row = db.QueryRow(`SELECT attname FROM pg_attribute WHERE attrelid = (SELECT oid FROM pg_class WHERE relname = 'reports') AND attname = 'weakhashenabled'`)
	if err := row.Scan(&t); err != nil {
		// The WeakHashEnabled column doesn't exist; add the new columns.
		_, err = db.Exec(`ALTER TABLE Reports
		ADD COLUMN WeakHashEnabled BOOLEAN NOT NULL DEFAULT FALSE
		ADD COLUMN Address VARCHAR(45) NOT NULL DEFAULT ''
		`)
		if err != nil {
			return err
		}
	}

	// Receive only added ad-hoc

	row = db.QueryRow(`SELECT attname FROM pg_attribute WHERE attrelid = (SELECT oid FROM pg_class WHERE relname = 'reports') AND attname = 'folderrecvonly'`)
	if err := row.Scan(&t); err != nil {
		// The RecvOnly column doesn't exist; add it.
		_, err = db.Exec(`ALTER TABLE Reports
		ADD COLUMN FolderRecvOnly INTEGER NOT NULL DEFAULT 0
		`)
		if err != nil {
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
	log.SetFlags(log.Ltime | log.Ldate | log.Lshortfile)
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

	// TLS & Listening

	var listener net.Listener
	if useHTTP {
		listener, err = net.Listen("tcp", listenAddr)
	} else {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Fatalln("tls:", err)
		}

		cfg := &tls.Config{
			Certificates:           []tls.Certificate{cert},
			SessionTicketsDisabled: true,
		}
		listener, err = tls.Listen("tcp", listenAddr, cfg)
	}
	if err != nil {
		log.Fatalln("listen:", err)
	}

	srv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	http.HandleFunc("/", withDB(db, rootHandler))
	http.HandleFunc("/newdata", withDB(db, newDataHandler))
	http.HandleFunc("/summary.json", withDB(db, summaryHandler))
	http.HandleFunc("/movement.json", withDB(db, movementHandler))
	http.HandleFunc("/performance.json", withDB(db, performanceHandler))
	http.HandleFunc("/blockstats.json", withDB(db, blockStatsHandler))
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

	addr := r.Header.Get("X-Forwarded-For")
	if addr != "" {
		addr = strings.Split(addr, ", ")[0]
	} else {
		addr = r.RemoteAddr
	}

	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}

	if net.ParseIP(addr) == nil {
		addr = ""
	}

	var rep report
	rep.Date = time.Now().UTC().Format("20060102")
	rep.Address = addr

	lr := &io.LimitedReader{R: r.Body, N: 40 * 1024}
	bs, _ := ioutil.ReadAll(lr)
	if err := json.Unmarshal(bs, &rep); err != nil {
		log.Println("decode:", err)
		if debug {
			log.Printf("%s", bs)
		}
		http.Error(w, "JSON Decode Error", http.StatusInternalServerError)
		return
	}

	if err := rep.Validate(); err != nil {
		log.Println("validate:", err)
		if debug {
			log.Printf("%#v", rep)
		}
		http.Error(w, "Validation Error", http.StatusInternalServerError)
		return
	}

	if err := insertReport(db, rep); err != nil {
		log.Println("insert:", err)
		if debug {
			log.Printf("%#v", rep)
		}
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

func performanceHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getPerformance(db)
	if err != nil {
		log.Println("performanceHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := json.Marshal(s)
	if err != nil {
		log.Println("performanceHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

func blockStatsHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getBlockStats(db)
	if err != nil {
		log.Println("blockStatsHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := json.Marshal(s)
	if err != nil {
		log.Println("blockStatsHandler:", err)
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
	Type   NumberType
}

type feature struct {
	Key     string
	Version string
	Count   int
	Pct     float64
}

type featureGroup struct {
	Key     string
	Version string
	Counts  map[string]int
}

// Used in the templates
type counter struct {
	n int
}

func (c *counter) Current() int {
	return c.n
}

func (c *counter) Increment() string {
	c.n++
	return ""
}

func (c *counter) DrawTwoDivider() bool {
	return c.n != 0 && c.n%2 == 0
}

// add sets a key in a nested map, initializing things if needed as we go.
func add(storage map[string]map[string]int, parent, child string, value int) {
	n, ok := storage[parent]
	if !ok {
		n = make(map[string]int)
		storage[parent] = n
	}
	n[child] += value
}

// inc makes sure that even for unused features, we initialize them in the
// feature map. Furthermore, this acts as a helper that accepts booleans
// to increment by one, or integers to increment by that integer.
func inc(storage map[string]int, key string, i interface{}) {
	cv := storage[key]
	switch v := i.(type) {
	case bool:
		if v {
			cv++
		}
	case int:
		cv += v
	}
	storage[key] = cv
}

type location struct {
	Latitude  float64
	Longitude float64
}

func getReport(db *sql.DB) map[string]interface{} {
	geoip, err := geoip2.Open(geoIPPath)
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	nodes := 0
	countriesTotal := 0
	var versions []string
	var platforms []string
	var numFolders []int
	var numDevices []int
	var totFiles []int
	var maxFiles []int
	var totMiB []int
	var maxMiB []int
	var memoryUsage []int
	var sha256Perf []float64
	var memorySize []int
	var uptime []int
	var compilers []string
	var builders []string
	locations := make(map[location]int)
	countries := make(map[string]int)

	reports := make(map[string]int)
	totals := make(map[string]int)

	// category -> version -> feature -> count
	features := make(map[string]map[string]map[string]int)
	// category -> version -> feature -> group -> count
	featureGroups := make(map[string]map[string]map[string]map[string]int)
	for _, category := range featureOrder {
		features[category] = make(map[string]map[string]int)
		featureGroups[category] = make(map[string]map[string]map[string]int)
		for _, version := range knownVersions {
			features[category][version] = make(map[string]int)
			featureGroups[category][version] = make(map[string]map[string]int)
		}
	}

	// Initialize some features that hide behind if conditions, and might not
	// be initialized.
	add(featureGroups["Various"]["v2"], "Upgrades", "Pre-release", 0)
	add(featureGroups["Various"]["v2"], "Upgrades", "Automatic", 0)
	add(featureGroups["Various"]["v2"], "Upgrades", "Manual", 0)
	add(featureGroups["Various"]["v2"], "Upgrades", "Disabled", 0)
	add(featureGroups["Various"]["v3"], "Temporary Retention", "Disabled", 0)
	add(featureGroups["Various"]["v3"], "Temporary Retention", "Custom", 0)
	add(featureGroups["Various"]["v3"], "Temporary Retention", "Default", 0)
	add(featureGroups["Connection"]["v3"], "IP version", "IPv4", 0)
	add(featureGroups["Connection"]["v3"], "IP version", "IPv6", 0)
	add(featureGroups["Connection"]["v3"], "IP version", "Unknown", 0)

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

		if geoip != nil && rep.Address != "" {
			if addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(rep.Address, "0")); err == nil {
				city, err := geoip.City(addr.IP)
				if err == nil {
					loc := location{
						Latitude:  city.Location.Latitude,
						Longitude: city.Location.Longitude,
					}
					locations[loc]++
					countries[city.Country.Names["en"]]++
					countriesTotal++
				}
			}
		}

		nodes++
		versions = append(versions, transformVersion(rep.Version))
		platforms = append(platforms, rep.Platform)
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
		if rep.Uptime > 0 {
			uptime = append(uptime, rep.Uptime)
		}

		totals["Device"] += rep.NumDevices
		totals["Folder"] += rep.NumFolders

		if rep.URVersion >= 2 {
			reports["v2"]++
			numCPU = append(numCPU, rep.NumCPU)

			// Various
			inc(features["Various"]["v2"], "Rate limiting", rep.UsesRateLimit)

			if rep.UpgradeAllowedPre {
				add(featureGroups["Various"]["v2"], "Upgrades", "Pre-release", 1)
			} else if rep.UpgradeAllowedAuto {
				add(featureGroups["Various"]["v2"], "Upgrades", "Automatic", 1)
			} else if rep.UpgradeAllowedManual {
				add(featureGroups["Various"]["v2"], "Upgrades", "Manual", 1)
			} else {
				add(featureGroups["Various"]["v2"], "Upgrades", "Disabled", 1)
			}

			// Folders
			inc(features["Folder"]["v2"], "Automatic normalization", rep.FolderUses.AutoNormalize)
			inc(features["Folder"]["v2"], "Ignore deletes", rep.FolderUses.IgnoreDelete)
			inc(features["Folder"]["v2"], "Ignore permissions", rep.FolderUses.IgnorePerms)
			inc(features["Folder"]["v2"], "Mode, send only", rep.FolderUses.SendOnly)
			inc(features["Folder"]["v2"], "Mode, receive only", rep.FolderUses.ReceiveOnly)

			add(featureGroups["Folder"]["v2"], "Versioning", "Simple", rep.FolderUses.SimpleVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "External", rep.FolderUses.ExternalVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "Staggered", rep.FolderUses.StaggeredVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "Trashcan", rep.FolderUses.TrashcanVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "Disabled", rep.NumFolders-rep.FolderUses.SimpleVersioning-rep.FolderUses.ExternalVersioning-rep.FolderUses.StaggeredVersioning-rep.FolderUses.TrashcanVersioning)

			// Device
			inc(features["Device"]["v2"], "Custom certificate", rep.DeviceUses.CustomCertName)
			inc(features["Device"]["v2"], "Introducer", rep.DeviceUses.Introducer)

			add(featureGroups["Device"]["v2"], "Compress", "Always", rep.DeviceUses.CompressAlways)
			add(featureGroups["Device"]["v2"], "Compress", "Metadata", rep.DeviceUses.CompressMetadata)
			add(featureGroups["Device"]["v2"], "Compress", "Nothing", rep.DeviceUses.CompressNever)

			add(featureGroups["Device"]["v2"], "Addresses", "Dynamic", rep.DeviceUses.DynamicAddr)
			add(featureGroups["Device"]["v2"], "Addresses", "Static", rep.DeviceUses.StaticAddr)

			// Connections
			inc(features["Connection"]["v2"], "Relaying, enabled", rep.Relays.Enabled)
			inc(features["Connection"]["v2"], "Discovery, global enabled", rep.Announce.GlobalEnabled)
			inc(features["Connection"]["v2"], "Discovery, local enabled", rep.Announce.LocalEnabled)

			add(featureGroups["Connection"]["v2"], "Discovery", "Default servers (using DNS)", rep.Announce.DefaultServersDNS)
			add(featureGroups["Connection"]["v2"], "Discovery", "Default servers (using IP)", rep.Announce.DefaultServersIP)
			add(featureGroups["Connection"]["v2"], "Discovery", "Other servers", rep.Announce.DefaultServersIP)

			add(featureGroups["Connection"]["v2"], "Relaying", "Default relays", rep.Relays.DefaultServers)
			add(featureGroups["Connection"]["v2"], "Relaying", "Other relays", rep.Relays.OtherServers)
		}

		if rep.URVersion >= 3 {
			reports["v3"]++

			inc(features["Various"]["v3"], "Custom LAN classification", rep.AlwaysLocalNets)
			inc(features["Various"]["v3"], "Ignore caching", rep.CacheIgnoredFiles)
			inc(features["Various"]["v3"], "Overwrite device names", rep.OverwriteRemoteDeviceNames)
			inc(features["Various"]["v3"], "Download progress disabled", !rep.ProgressEmitterEnabled)
			inc(features["Various"]["v3"], "Custom default path", rep.CustomDefaultFolderPath)
			inc(features["Various"]["v3"], "Custom traffic class", rep.CustomTrafficClass)
			inc(features["Various"]["v3"], "Custom temporary index threshold", rep.CustomTempIndexMinBlocks)
			inc(features["Various"]["v3"], "Weak hash enabled", rep.WeakHashEnabled)
			inc(features["Various"]["v3"], "LAN rate limiting", rep.LimitBandwidthInLan)
			inc(features["Various"]["v3"], "Custom release server", rep.CustomReleaseURL)
			inc(features["Various"]["v3"], "Restart after suspend", rep.RestartOnWakeup)
			inc(features["Various"]["v3"], "Custom stun servers", rep.CustomStunServers)
			inc(features["Various"]["v3"], "Ignore patterns", rep.IgnoreStats.Lines > 0)

			if rep.NATType != "" {
				natType := rep.NATType
				natType = strings.Replace(natType, "unknown", "Unknown", -1)
				natType = strings.Replace(natType, "Symetric", "Symmetric", -1)
				add(featureGroups["Various"]["v3"], "NAT Type", natType, 1)
			}

			if rep.TemporariesDisabled {
				add(featureGroups["Various"]["v3"], "Temporary Retention", "Disabled", 1)
			} else if rep.TemporariesCustom {
				add(featureGroups["Various"]["v3"], "Temporary Retention", "Custom", 1)
			} else {
				add(featureGroups["Various"]["v3"], "Temporary Retention", "Default", 1)
			}

			inc(features["Folder"]["v3"], "Scan progress disabled", rep.FolderUsesV3.ScanProgressDisabled)
			inc(features["Folder"]["v3"], "Disable sharing of partial files", rep.FolderUsesV3.DisableTempIndexes)
			inc(features["Folder"]["v3"], "Disable sparse files", rep.FolderUsesV3.DisableSparseFiles)
			inc(features["Folder"]["v3"], "Weak hash, always", rep.FolderUsesV3.AlwaysWeakHash)
			inc(features["Folder"]["v3"], "Weak hash, custom threshold", rep.FolderUsesV3.CustomWeakHashThreshold)
			inc(features["Folder"]["v3"], "Filesystem watcher", rep.FolderUsesV3.FsWatcherEnabled)

			add(featureGroups["Folder"]["v3"], "Conflicts", "Disabled", rep.FolderUsesV3.ConflictsDisabled)
			add(featureGroups["Folder"]["v3"], "Conflicts", "Unlimited", rep.FolderUsesV3.ConflictsUnlimited)
			add(featureGroups["Folder"]["v3"], "Conflicts", "Limited", rep.FolderUsesV3.ConflictsOther)

			for key, value := range rep.FolderUsesV3.PullOrder {
				add(featureGroups["Folder"]["v3"], "Pull Order", prettyCase(key), value)
			}

			totals["GUI"] += rep.GUIStats.Enabled

			inc(features["GUI"]["v3"], "Auth Enabled", rep.GUIStats.UseAuth)
			inc(features["GUI"]["v3"], "TLS Enabled", rep.GUIStats.UseTLS)
			inc(features["GUI"]["v3"], "Insecure Admin Access", rep.GUIStats.InsecureAdminAccess)
			inc(features["GUI"]["v3"], "Skip Host check", rep.GUIStats.InsecureSkipHostCheck)
			inc(features["GUI"]["v3"], "Allow Frame loading", rep.GUIStats.InsecureAllowFrameLoading)

			add(featureGroups["GUI"]["v3"], "Listen address", "Local", rep.GUIStats.ListenLocal)
			add(featureGroups["GUI"]["v3"], "Listen address", "Unspecified", rep.GUIStats.ListenUnspecified)
			add(featureGroups["GUI"]["v3"], "Listen address", "Other", rep.GUIStats.Enabled-rep.GUIStats.ListenLocal-rep.GUIStats.ListenUnspecified)

			for theme, count := range rep.GUIStats.Theme {
				add(featureGroups["GUI"]["v3"], "Theme", prettyCase(theme), count)
			}

			for transport, count := range rep.TransportStats {
				add(featureGroups["Connection"]["v3"], "Transport", strings.Title(transport), count)
				if strings.HasSuffix(transport, "4") {
					add(featureGroups["Connection"]["v3"], "IP version", "IPv4", count)
				} else if strings.HasSuffix(transport, "6") {
					add(featureGroups["Connection"]["v3"], "IP version", "IPv6", count)
				} else {
					add(featureGroups["Connection"]["v3"], "IP version", "Unknown", count)
				}
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
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInts(maxMiB),
		Descr:  "Data in Largest Folder",
		Unit:   "B",
		Type:   NumberBinary,
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
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInts(memorySize),
		Descr:  "System Memory",
		Unit:   "B",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForFloats(sha256Perf),
		Descr:  "SHA-256 Hashing Performance",
		Unit:   "B/s",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInts(numCPU),
		Descr:  "Number of CPU cores",
	})

	categories = append(categories, category{
		Values: statsForInts(uptime),
		Descr:  "Uptime (v3)",
		Type:   NumberDuration,
	})

	reportFeatures := make(map[string][]feature)
	for featureType, versions := range features {
		var featureList []feature
		for version, featureMap := range versions {
			// We count totals of the given feature type, for example number of
			// folders or devices, if that doesn't exist, we work out percentage
			// against the total of the version reports. Things like "Various"
			// never have counts.
			total, ok := totals[featureType]
			if !ok {
				total = reports[version]
			}
			for key, count := range featureMap {
				featureList = append(featureList, feature{
					Key:     key,
					Version: version,
					Count:   count,
					Pct:     (100 * float64(count)) / float64(total),
				})
			}
		}
		sort.Sort(sort.Reverse(sortableFeatureList(featureList)))
		reportFeatures[featureType] = featureList
	}

	reportFeatureGroups := make(map[string][]featureGroup)
	for featureType, versions := range featureGroups {
		var featureList []featureGroup
		for version, featureMap := range versions {
			for key, counts := range featureMap {
				featureList = append(featureList, featureGroup{
					Key:     key,
					Version: version,
					Counts:  counts,
				})
			}
		}
		reportFeatureGroups[featureType] = featureList
	}

	var countryList []feature
	for country, count := range countries {
		countryList = append(countryList, feature{
			Key:   country,
			Count: count,
			Pct:   (100 * float64(count)) / float64(countriesTotal),
		})
		sort.Sort(sort.Reverse(sortableFeatureList(countryList)))
	}

	r := make(map[string]interface{})
	r["features"] = reportFeatures
	r["featureGroups"] = reportFeatureGroups
	r["nodes"] = nodes
	r["versionNodes"] = reports
	r["categories"] = categories
	r["versions"] = group(byVersion, analyticsFor(versions, 2000), 10)
	r["versionPenetrations"] = penetrationLevels(analyticsFor(versions, 2000), []float64{50, 75, 90, 95})
	r["platforms"] = group(byPlatform, analyticsFor(platforms, 2000), 5)
	r["compilers"] = group(byCompiler, analyticsFor(compilers, 2000), 5)
	r["builders"] = analyticsFor(builders, 12)
	r["featureOrder"] = featureOrder
	r["locations"] = locations
	r["contries"] = countryList

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

var (
	plusRe  = regexp.MustCompile(`\+.*$`)
	plusStr = "(+dev)"
)

// transformVersion returns a version number formatted correctly, with all
// development versions aggregated into one.
func transformVersion(v string) string {
	if v == "unknown-dev" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	v = plusRe.ReplaceAllString(v, " "+plusStr)

	return v
}

type summary struct {
	versions map[string]int   // version string to count index
	max      map[string]int   // version string to max users per day
	rows     map[string][]int // date to list of counts
}

func newSummary() summary {
	return summary{
		versions: make(map[string]int),
		max:      make(map[string]int),
		rows:     make(map[string][]int),
	}
}

func (s *summary) setCount(date, version string, count int) {
	idx, ok := s.versions[version]
	if !ok {
		idx = len(s.versions)
		s.versions[version] = idx
	}

	if s.max[version] < count {
		s.max[version] = count
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

	var filtered []string
	for _, v := range versions {
		if s.max[v] > 50 {
			filtered = append(filtered, v)
		}
	}
	versions = filtered

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

	rows, err := db.Query(`SELECT Day, Version, Count FROM VersionSummary WHERE Day > now() - '2 year'::INTERVAL;`)
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
	rows, err := db.Query(`SELECT Day, Added, Removed, Bounced FROM UserMovement WHERE Day > now() - '2 year'::INTERVAL ORDER BY Day`)
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

func getPerformance(db *sql.DB) ([][]interface{}, error) {
	rows, err := db.Query(`SELECT Day, TotFiles, TotMiB, SHA256Perf, MemorySize, MemoryUsageMiB FROM Performance WHERE Day > '2014-06-20'::TIMESTAMP ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "TotFiles", "TotMiB", "SHA256Perf", "MemorySize", "MemoryUsageMiB"},
	}

	for rows.Next() {
		var day time.Time
		var sha256Perf float64
		var totFiles, totMiB, memorySize, memoryUsage int
		err := rows.Scan(&day, &totFiles, &totMiB, &sha256Perf, &memorySize, &memoryUsage)
		if err != nil {
			return nil, err
		}

		row := []interface{}{day.Format("2006-01-02"), totFiles, totMiB, float64(int(sha256Perf*10)) / 10, memorySize, memoryUsage}
		res = append(res, row)
	}

	return res, nil
}

func getBlockStats(db *sql.DB) ([][]interface{}, error) {
	rows, err := db.Query(`SELECT Day, Reports, Pulled, Renamed, Reused, CopyOrigin, CopyOriginShifted, CopyElsewhere FROM BlockStats WHERE Day > '2017-10-23'::TIMESTAMP ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "Number of Reports", "Transferred (GiB)", "Saved by renaming files (GiB)", "Saved by resuming transfer (GiB)", "Saved by reusing data from old file (GiB)", "Saved by reusing shifted data from old file (GiB)", "Saved by reusing data from other files (GiB)"},
	}
	blocksToGb := float64(8 * 1024)
	for rows.Next() {
		var day time.Time
		var reports, pulled, renamed, reused, copyOrigin, copyOriginShifted, copyElsewhere float64
		err := rows.Scan(&day, &reports, &pulled, &renamed, &reused, &copyOrigin, &copyOriginShifted, &copyElsewhere)
		if err != nil {
			return nil, err
		}
		row := []interface{}{
			day.Format("2006-01-02"),
			reports,
			pulled / blocksToGb,
			renamed / blocksToGb,
			reused / blocksToGb,
			copyOrigin / blocksToGb,
			copyOriginShifted / blocksToGb,
			copyElsewhere / blocksToGb,
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

func prettyCase(input string) string {
	output := ""
	for i, runeValue := range input {
		if i == 0 {
			runeValue = unicode.ToUpper(runeValue)
		} else if unicode.IsUpper(runeValue) {
			output += " "
		}
		output += string(runeValue)
	}
	return output
}
