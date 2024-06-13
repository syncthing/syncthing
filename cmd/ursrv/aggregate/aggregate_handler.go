package aggregate

import (
	"fmt"
	"math"
	"net"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/geoip"
	"github.com/syncthing/syncthing/lib/ur"
)

// CounterHelper is a helper object. It collects the values from the usage
// reports and stores it in a structural manner, making it easy to calculate the
// average, median, sum, min, max, percentile values.
type AggregateHandler struct {
	mutex      sync.Mutex
	floats     map[string][]float64          // fieldName -> data for float statistics
	ints       map[string][]int64            // fieldName -> data for int statistics
	mapStrings map[string]map[string]int64   // fieldName -> mapped value -> data for histogram
	mapInts    map[string]map[string][]int64 // fieldName -> mapped value -> data for int statistics

	keySince map[string]int // Map keys to their 'since' value (report-version)

	totalReports   int // All handled reports counter
	totalV2Reports int
	totalV3Reports int
}

func newAggregateHandler() *AggregateHandler {
	return &AggregateHandler{
		floats:     make(map[string][]float64),
		ints:       make(map[string][]int64),
		mapStrings: make(map[string]map[string]int64),
		mapInts:    make(map[string]map[string][]int64),
		keySince:   make(map[string]int),
	}
}

func (h *AggregateHandler) addFloat(label string, value float64) {
	if value == 0.0 || math.IsNaN(value) {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.floats[label]
	if res == nil {
		res = make([]float64, 0)
	}
	res = append(res, value)
	h.floats[label] = res
}

func (h *AggregateHandler) addInt(label string, value int) {
	if value == 0 {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.ints[label]
	if res == nil {
		res = make([]int64, 0)
	}
	res = append(res, int64(value))
	h.ints[label] = res
}

func (h *AggregateHandler) addMapStrings(label, key string, value int64) {
	if value == 0 || key == "" {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.mapStrings[label]
	if res == nil {
		res = make(map[string]int64)
	}
	res[key] += value
	h.mapStrings[label] = res
}

func (h *AggregateHandler) addIntArr(label string, value []int) {
	for _, v := range value {
		h.addInt(label, v)
	}
}

func (h *AggregateHandler) addMapInts(label string, value map[string]int) {
	if len(value) == 0 {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.mapInts[label]
	if res == nil {
		res = make(map[string][]int64)
	}
	for k, v := range value {
		if k == "" || v == 0 {
			continue
		}

		res[k] = append(res[k], int64(v))
	}
	h.mapInts[label] = res
}

func (h *AggregateHandler) incReportCounter(version int) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.totalReports++
	switch version {
	case 3:
		h.totalV3Reports++
		fallthrough
	case 2:
		h.totalV2Reports++
	}
}

func (h *AggregateHandler) calculateSummary(date time.Time) *ur.Aggregation {
	// Summarises the data to a single report. This includes calculating the
	// required values, like average, median, sum, etc.
	ap := &ur.Aggregation{
		Date:       date.UTC().Unix(),
		Count:      int64(h.totalReports),   // all reports
		CountV2:    int64(h.totalV2Reports), // v2 repots
		CountV3:    int64(h.totalV3Reports), // v3 reports
		Statistics: make(map[string]ur.Statistic),
	}

	res := make(map[string]ur.Statistic)
	var wg sync.WaitGroup
	var mutex sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex.Lock()
		defer mutex.Unlock()

		for label, v := range h.floats {
			res[label] =
				ur.Statistic{
					Key:       label,
					Statistic: &ur.Statistic_Float{Float: floatStats(v)},
					Since:     int64(h.keySince[label]),
				}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex.Lock()
		defer mutex.Unlock()

		for label, v := range h.ints {
			res[label] =
				ur.Statistic{
					Key:       label,
					Statistic: &ur.Statistic_Integer{Integer: intStats(v)},
					Since:     int64(h.keySince[label]),
				}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex.Lock()
		defer mutex.Unlock()

		for label, child := range h.mapInts {
			mapStats := &ur.MapIntegerStatistic{Map: make(map[string]ur.IntegerStatistic)}
			for k, v := range child {
				mapStats.Map[k] = *intStats(v)
			}

			res[label] = ur.Statistic{
				Key:       label,
				Statistic: &ur.Statistic_MappedInteger{MappedInteger: mapStats},
				Since:     int64(h.keySince[label]),
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex.Lock()
		defer mutex.Unlock()

		for label, child := range h.mapStrings {
			stats := &ur.MapHistogram{Map: make(map[string]int64)}
			for k, v := range child {
				stats.Map[k] = v
				stats.Count += v
			}
			res[label] = ur.Statistic{
				Key:       label,
				Statistic: &ur.Statistic_Histogram{Histogram: stats},
				Since:     int64(h.keySince[label]),
			}
		}
	}()
	wg.Wait()

	ap.Statistics = res

	return ap
}

func (h *AggregateHandler) handleMiscStats(longVersion string) {
	if m := compilerRe.FindStringSubmatch(longVersion); len(m) == 3 {
		h.addMapStrings("compiler", m[1], 1)
		h.addMapStrings("builder", m[2], 1)
	loop:
		for _, d := range knownDistributions {
			if d.matcher.MatchString(longVersion) {
				h.addMapStrings("distribution", d.distribution, 1)
				break loop
			}
		}
	}
}

func (h *AggregateHandler) parseGeoLocation(geoip *geoip.Provider, addr string) {
	if addr == "" || geoip == nil {
		return
	}

	if addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(addr, "0")); err == nil {
		city, err := geoip.City(addr.IP)
		if err == nil {
			country := city.Country.Names["en"]
			if country == "" {
				country = "Unkown"
			}
			h.addMapStrings(countryTag, country, 1)
			h.addMapStrings(locationTag, fmt.Sprintf("%f~%f", city.Location.Latitude, city.Location.Longitude), 1)
		}
	}
}

func (h *AggregateHandler) aggregateReportData(v any, urVersion int, parent string) {
	s := reflect.ValueOf(v)

	if s.Kind() != reflect.Struct {
		// Sanity check, this will otherwise cause a panic.
		return
	}

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := s.Type().Field(i).Tag
		h.handleValue(f, tag, urVersion, parent)
	}
}

func (h *AggregateHandler) handleValue(value reflect.Value, tag reflect.StructTag, urVersion int, parent string) {
	parsedTag := parseTag(tag.Get("json"), parent)
	sinceInt, err := strconv.Atoi(tag.Get("since"))

	// Keep track of field's since value so we can append it to the analysis
	// object.
	h.mutex.Lock()
	h.keySince[parsedTag] = sinceInt
	h.mutex.Unlock()

	if err != nil || urVersion < sinceInt {
		return
	}
	// Some fields are either handled separately or not relevant, we should skip
	// those when iterating over the report fields.
	if shouldSkipField(parsedTag) {
		return
	}

	if value.Kind() == reflect.Struct {
		// Walk through the child-struct, append this field's label as
		// parent-label.
		h.aggregateReportData(value.Interface(), urVersion, parsedTag)

		// This field itself has nothing relevant to analyse.
		return
	}

	// Handle the content of the field depending on the type.
	switch t := value.Interface().(type) {
	case string:
		if t == "" {
			return
		}
		h.addMapStrings(parsedTag, t, 1)
	case int:
		if t == 0 {
			return
		}
		h.addInt(parsedTag, t)
	case float64:
		if t == 0 || math.IsNaN(t) {
			return
		}
		h.addFloat(parsedTag, t)
	case bool:
		if !t {
			return
		}
		h.addInt(parsedTag, 1)
	case []int:
		if len(t) == 0 {
			return
		}
		h.addIntArr(parsedTag, t)
	case map[string]int:
		if len(t) == 0 {
			return
		}
		h.addMapInts(parsedTag, t)
	default:
		return
	}
}
