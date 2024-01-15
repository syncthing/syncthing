// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package aggregate

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/oschwald/geoip2-golang"
	"github.com/syncthing/syncthing/cmd/ursrv/blob"
	"github.com/syncthing/syncthing/cmd/ursrv/report"
	"github.com/syncthing/syncthing/lib/sliceutil"
	"github.com/syncthing/syncthing/lib/ur/contract"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	compilerRe                = regexp.MustCompile(`\(([A-Za-z0-9()., -]+) \w+-\w+(?:| android| default)\) ([\w@.-]+)`)
	featureOrder              = []string{"Various", "Folder", "Device", "Connection", "GUI"}
	knownVersions             = []string{"v2", "v3"}
	invalidBlockstatsVersions = []string{"v0.14.40", "v0.14.39", "v0.14.38"}
	knownDistributions        = []distributionMatch{
		// Maps well known builders to the official distribution method that
		// they represent

		{regexp.MustCompile(`\steamcity@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sjenkins@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sbuilder@github\.syncthing\.net`), "GitHub"},

		{regexp.MustCompile(`\sdeb@build\.syncthing\.net`), "APT"},
		{regexp.MustCompile(`\sdebian@github\.syncthing\.net`), "APT"},

		{regexp.MustCompile(`\sdocker@syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@build.syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@github.syncthing\.net`), "Docker Hub"},

		{regexp.MustCompile(`\sandroid-builder@github\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*teamcity@build\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*vagrant@basebox-stretch64`), "F-Droid"},
		{regexp.MustCompile(`\svagrant@bullseye`), "F-Droid"},
		{regexp.MustCompile(`\sbuilduser@(archlinux|svetlemodry)`), "Arch (3rd party)"},
		{regexp.MustCompile(`@debian`), "Debian (3rd party)"},
		{regexp.MustCompile(`@fedora`), "Fedora (3rd party)"},
		{regexp.MustCompile(`\sbrew@`), "Homebrew (3rd party)"},
		{regexp.MustCompile(`\sroot@buildkitsandbox`), "LinuxServer.io (3rd party)"},
		{regexp.MustCompile(`.`), "Others"},
	}
)

type distributionMatch struct {
	matcher      *regexp.Regexp
	distribution string
}

type CLI struct {
	DBConn    string `env:"UR_DB_URL" default:"postgres://user:password@localhost/ur?sslmode=disable"`
	GeoIPPath string `env:"UR_GEOIP" default:"GeoLite2-City.mmdb"`
	Migrate   bool   `env:"UR_MIGRATE"` // Migration support (to be removed post-migration).
	From      string `env:"UR_MIGRATE_FROM" default:"2014-06-11"`
	To        string `env:"UR_MIGRATE_TO"`
}

func (cli *CLI) Run(s3Config blob.S3Config) error {
	log.SetFlags(log.Ltime | log.Ldate)
	log.SetOutput(os.Stdout)

	// Deprecated (remove post-migration).
	db, err := sql.Open("postgres", cli.DBConn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}

	// Initialize the storage and store.
	b := blob.NewBlobStorage(s3Config)
	store := blob.NewUrsrvStore(b)

	// Migration support (to be removed post-migration).
	if cli.Migrate {
		log.Println("Starting migration")
		if err := runMigration(db, store, cli.GeoIPPath, cli.From, cli.To); err != nil {
			log.Println("Migration failed:", err)
			return err
		}
		log.Println("Migration complete")

		// Skip the regular aggregation.
		return nil
	}

	for {
		runAggregation(store, cli.GeoIPPath, time.Now().UTC().AddDate(0, 0, -1))

		// Sleep until one minute past next midnight
		sleepUntilNext(24*time.Hour, 1*time.Minute)
	}
}

func sleepUntilNext(intv, margin time.Duration) {
	now := time.Now().UTC()
	next := now.Truncate(intv).Add(intv).Add(margin)

	log.Println("Sleeping until", next)
	time.Sleep(next.Sub(now))
}

func runAggregation(store *blob.UrsrvStore, geoIPPath string, aggregateDate time.Time) {
	// Prepare the geo location database.
	geoip, err := geoip2.Open(geoIPPath)
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	// Retreive the usage reports for the given date.
	reps, err := store.ListUsageReportsForDate(aggregateDate)
	if err != nil {
		log.Printf("error while listing reports for the previous day, %v", err)
		return
	}

	if len(reps) == 0 {
		log.Printf("no reports to aggregate")
		return
	}

	// Aggregate the obtained usage reports to a single daily summary.
	ar, err := aggregateUserReports(geoip, aggregateDate, reps)
	if err != nil {
		log.Printf("error while aggregating reports, %v", err)
		return
	}

	// Store the aggregated report.
	err = store.PutAggregatedReport(ar)
	if err != nil {
		log.Printf("error while storing the aggregated report, skipping cleanup %v", err)
		return
	}
}

func aggregateUserReports(geoip *geoip2.Reader, date time.Time, reps []contract.Report) (*report.AggregatedReport, error) {
	// Initialize the report.
	ar := &report.AggregatedReport{
		Date:        date.UTC(),
		Performance: report.Performance{},
		BlockStats:  report.BlockStats{},
	}

	// Initialize variables which are used as a mediator.
	nodes := 0
	blockstatNodes := 0
	countriesTotal := 0
	var versions []string
	var platforms []string
	var numFolders []int
	var numDevices []int
	var totFiles []int
	var maxFiles []int
	var totMiB []int64
	var maxMiB []int64
	var memoryUsage []int64
	var sha256Perf []float64
	var memorySize []int64
	var uptime []int
	var compilers []string
	var builders []string
	var distributions []string
	locations := report.NewLocationsMap()
	countries := make(map[string]int)
	versionCount := make(map[string]int)
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

	// Handle each report.
	for _, rep := range reps {
		if geoip != nil && rep.Address != "" {
			if addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(rep.Address, "0")); err == nil {
				city, err := geoip.City(addr.IP)
				if err == nil {
					locations.Add(city.Location.Latitude, city.Location.Longitude)
					country := city.Country.Names["en"]
					if country == "" {
						country = "Unkown"
					}
					countries[country]++
					countriesTotal++
				}
			}
		}

		nodes++
		versions = append(versions, transformVersion(rep.Version))
		inc(versionCount, transformVersion(rep.Version), 1)
		platforms = append(platforms, rep.Platform)

		if m := compilerRe.FindStringSubmatch(rep.LongVersion); len(m) == 3 {
			compilers = append(compilers, m[1])
			builders = append(builders, m[2])
		loop:
			for _, d := range knownDistributions {
				if d.matcher.MatchString(rep.LongVersion) {
					distributions = append(distributions, d.distribution)
					break loop
				}
			}
		}

		if rep.NumFolders > 0 {
			numFolders = append(numFolders, rep.NumFolders)
		}
		if rep.NumDevices > 0 {
			numDevices = append(numDevices, rep.NumDevices)
		}
		if rep.FolderMaxFiles > 0 {
			maxFiles = append(maxFiles, rep.FolderMaxFiles)
		}
		if rep.FolderMaxMiB > 0 {
			maxMiB = append(maxMiB, int64(rep.FolderMaxMiB)*(1<<20))
		}
		if rep.Uptime > 0 {
			uptime = append(uptime, rep.Uptime)
		}

		// Performance
		// Some custom implementation reported bytes when we expect megabytes,
		// cap at petabyte
		if rep.MemorySize < 1073741824 {
			if rep.TotFiles > 0 {
				totFiles = append(totFiles, rep.TotFiles)
			}
			if rep.TotMiB > 0 {
				totMiB = append(totMiB, int64(rep.TotMiB)*(1<<20))
			}
			if rep.SHA256Perf > 0 {
				sha256Perf = append(sha256Perf, rep.SHA256Perf*(1<<20))
			}
			if rep.MemorySize > 0 {
				memorySize = append(memorySize, int64(rep.MemorySize)*(1<<20))
			}
			if rep.MemoryUsageMiB > 0 {
				memoryUsage = append(memoryUsage, int64(rep.MemoryUsageMiB)*(1<<20))
			}
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
				natType = strings.ReplaceAll(natType, "unknown", "Unknown")
				natType = strings.ReplaceAll(natType, "Symetric", "Symmetric")
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
			inc(features["Folder"]["v3"], "Case sensitive FS", rep.FolderUsesV3.CaseSensitiveFS)
			inc(features["Folder"]["v3"], "Mode, receive encrypted", rep.FolderUsesV3.ReceiveEncrypted)

			add(featureGroups["Folder"]["v3"], "Conflicts", "Disabled", rep.FolderUsesV3.ConflictsDisabled)
			add(featureGroups["Folder"]["v3"], "Conflicts", "Unlimited", rep.FolderUsesV3.ConflictsUnlimited)
			add(featureGroups["Folder"]["v3"], "Conflicts", "Limited", rep.FolderUsesV3.ConflictsOther)

			for key, value := range rep.FolderUsesV3.PullOrder {
				add(featureGroups["Folder"]["v3"], "Pull Order", prettyCase(key), value)
			}

			for key, value := range rep.FolderUsesV3.CopyRangeMethod {
				add(featureGroups["Folder"]["v3"], "Copy Range Method", prettyCase(key), value)
			}

			inc(features["Device"]["v3"], "Untrusted", rep.DeviceUsesV3.Untrusted)

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
				add(featureGroups["Connection"]["v3"], "Transport", cases.Title(language.English).String(transport), count)
				if strings.HasSuffix(transport, "4") {
					add(featureGroups["Connection"]["v3"], "IP version", "IPv4", count)
				} else if strings.HasSuffix(transport, "6") {
					add(featureGroups["Connection"]["v3"], "IP version", "IPv6", count)
				} else {
					add(featureGroups["Connection"]["v3"], "IP version", "Unknown", count)
				}
			}

			if shouldIncludeBlockstats(rep.Version, rep.URVersion) {
				blockstatNodes++

				// Blockstats
				ar.BlockStats.Total += float64(rep.BlockStats.Total)
				ar.BlockStats.Renamed += float64(rep.BlockStats.Renamed)
				ar.BlockStats.Reused += float64(rep.BlockStats.Reused)
				ar.BlockStats.Pulled += float64(rep.BlockStats.Pulled)
				ar.BlockStats.CopyOrigin += float64(rep.BlockStats.CopyOrigin)
				ar.BlockStats.CopyOriginShifted += float64(rep.BlockStats.CopyOriginShifted)
				ar.BlockStats.CopyElsewhere += float64(rep.BlockStats.CopyElsewhere)
			}
		}
	}

	categories := []report.Category{
		{
			Values: statsForInts(totFiles),
			Descr:  "Files Managed per Device",
		}, {
			Values: statsForInts(maxFiles),
			Descr:  "Files in Largest Folder",
		}, {
			Values: statsForInt64s(totMiB),
			Descr:  "Data Managed per Device",
			Unit:   "B",
			Type:   report.NumberBinary,
		}, {
			Values: statsForInt64s(maxMiB),
			Descr:  "Data in Largest Folder",
			Unit:   "B",
			Type:   report.NumberBinary,
		}, {
			Values: statsForInts(numDevices),
			Descr:  "Number of Devices in Cluster",
		}, {
			Values: statsForInts(numFolders),
			Descr:  "Number of Folders Configured",
		}, {
			Values: statsForInt64s(memoryUsage),
			Descr:  "Memory Usage",
			Unit:   "B",
			Type:   report.NumberBinary,
		}, {
			Values: statsForInt64s(memorySize),
			Descr:  "System Memory",
			Unit:   "B",
			Type:   report.NumberBinary,
		}, {
			Values: statsForFloats(sha256Perf),
			Descr:  "SHA-256 Hashing Performance",
			Unit:   "B/s",
			Type:   report.NumberBinary,
		}, {
			Values: statsForInts(numCPU),
			Descr:  "Number of CPU cores",
		}, {
			Values: statsForInts(uptime),
			Descr:  "Uptime (v3)",
			Type:   report.NumberDuration,
		},
	}

	reportFeatures := make(map[string][]report.Feature)
	for featureType, versions := range features {
		var featureList []report.Feature
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
				featureList = append(featureList, report.Feature{
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

	reportFeatureGroups := make(map[string][]report.FeatureGroup)
	for featureType, versions := range featureGroups {
		var featureList []report.FeatureGroup
		for version, featureMap := range versions {
			for key, counts := range featureMap {
				featureList = append(featureList, report.FeatureGroup{
					Key:     key,
					Version: version,
					Counts:  counts,
				})
			}
		}
		reportFeatureGroups[featureType] = featureList
	}

	var countryList []report.Feature
	for country, count := range countries {
		countryList = append(countryList, report.Feature{
			Key:   country,
			Count: count,
			Pct:   (100 * float64(count)) / float64(countriesTotal),
		})
		sort.Sort(sort.Reverse(sortableFeatureList(countryList)))
	}

	ar.Features = reportFeatures
	ar.FeatureGroups = reportFeatureGroups
	ar.Nodes = nodes
	ar.VersionNodes = reports
	ar.Categories = categories
	ar.Versions = group(byVersion, analyticsFor(versions, 2000), 5, 1.0)
	ar.VersionPenetrations = penetrationLevels(analyticsFor(versions, 2000), []float64{50, 75, 90, 95})
	ar.Platforms = group(byPlatform, analyticsFor(platforms, 2000), 10, 0.0)
	ar.Compilers = group(byCompiler, analyticsFor(compilers, 2000), 5, 1.0)
	ar.Builders = analyticsFor(builders, 12)
	ar.Distributions = analyticsFor(distributions, len(knownDistributions))
	ar.FeatureOrder = featureOrder
	ar.Locations = locations.WeightedLocations()
	ar.Countries = countryList

	// Versions
	ar.VersionCount = versionCount

	// Performance
	ar.Performance.TotFiles = sliceutil.Average(totFiles)
	ar.Performance.TotMib = sliceutil.Average(totMiB)
	ar.Performance.Sha256Perf = sliceutil.Average(sha256Perf)
	ar.Performance.MemorySize = sliceutil.Average(memorySize)
	ar.Performance.MemoryUsageMib = sliceutil.Average(memoryUsage)

	// Blockstats
	ar.BlockStats.NodeCount = blockstatNodes

	return ar, nil
}

// Migration support (to be removed post-migration).
func runMigration(db *sql.DB, store *blob.UrsrvStore, geoIPPath, from, to string) error {
	geoip, err := geoip2.Open(geoIPPath)
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	var toDate, fromDate time.Time

	// Default is v1.0.0 release date.
	fromDate, err = time.Parse(time.DateOnly, from)
	if err != nil {
		return err
	}

	if to == "" {
		// No end-date was set, default is yesterday.
		to = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	}
	toDate, err = time.Parse(time.DateOnly, to)
	if err != nil {
		return err
	}

	// Aggregate the reports of all the days prior to today, as all the usage
	// reports for those days should be put in the db already.
	for fromDate.Before(toDate) {
		// Obtain the reports for the given date from the db.
		reports, err := reportsFromDB(db, fromDate)
		if err != nil {
			return fmt.Errorf("error while retrieving reports for date %v: %w", fromDate, err)
		}
		if len(reports) == 0 {
			// No valid reports were obtained for this date.
			fromDate = fromDate.AddDate(0, 0, 1)
			continue
		}

		// Aggregate the reports.
		aggregated, err := aggregateUserReports(geoip, fromDate, reports)
		if err != nil {
			log.Println("migrate aggregation failed", fromDate, err)
		}

		// Store the aggregated report in the new storage location.
		err = store.PutAggregatedReport(aggregated)
		if err != nil {
			log.Println("migrate aggregated report failed", fromDate, err)
		}

		// Continue to the next day.
		fromDate = fromDate.AddDate(0, 0, 1)
	}

	return nil
}

// Migration support (to be removed post-migration).
func reportsFromDB(db *sql.DB, date time.Time) ([]contract.Report, error) {
	var reports []contract.Report

	// Select all the rows where the received day is equal to the given timestamp's day.
	rows, err := db.Query(`
	SELECT Received, Report FROM ReportsJson WHERE DATE_TRUNC('day', Received) = DATE_TRUNC('day', $1::timestamp)`, date)
	if err != nil {
		return reports, err
	}

	// Parse the row-data and append it to a slice.
	var rep contract.Report
	for rows.Next() {
		err := rows.Scan(&rep.Received, &rep)
		if err != nil {
			log.Println("sql:", err)
			continue
		}
		if err = rep.Validate(); err != nil {
			continue
		}
		reports = append(reports, rep)
	}

	return reports, nil
}

func shouldIncludeBlockstats(version string, urVersion int) bool {
	if urVersion < 3 {
		return false
	}

	for _, iv := range invalidBlockstatsVersions {
		if strings.HasPrefix(version, iv) {
			return false
		}
	}
	return true
}
