package httptoo

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type BytesContentRange struct {
	First, Last, Length int64
}

type BytesRange struct {
	First, Last int64
}

var (
	httpBytesRangeRegexp = regexp.MustCompile(`bytes[ =](\d+)-(\d*)`)
)

func ParseBytesRange(s string) (ret BytesRange, ok bool) {
	ss := httpBytesRangeRegexp.FindStringSubmatch(s)
	if ss == nil {
		return
	}
	var err error
	ret.First, err = strconv.ParseInt(ss[1], 10, 64)
	if err != nil {
		return
	}
	if ss[2] == "" {
		ret.Last = math.MaxInt64
	} else {
		ret.Last, err = strconv.ParseInt(ss[2], 10, 64)
		if err != nil {
			return
		}
	}
	ok = true
	return
}

func parseUnitRanges(s string) (unit, ranges string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " =")
	if i == -1 {
		return
	}
	unit = s[:i]
	ranges = s[i+1:]
	return
}

func parseFirstLast(s string) (first, last int64) {
	ss := strings.SplitN(s, "-", 2)
	first, err := strconv.ParseInt(ss[0], 10, 64)
	if err != nil {
		panic(err)
	}
	last, err = strconv.ParseInt(ss[1], 10, 64)
	if err != nil {
		panic(err)
	}
	return
}

func parseContentRange(s string) (ret BytesContentRange) {
	ss := strings.SplitN(s, "/", 2)
	firstLast := strings.TrimSpace(ss[0])
	if firstLast == "*" {
		ret.First = -1
		ret.Last = -1
	} else {
		ret.First, ret.Last = parseFirstLast(firstLast)
	}
	il := strings.TrimSpace(ss[1])
	if il == "*" {
		ret.Length = -1
	} else {
		var err error
		ret.Length, err = strconv.ParseInt(il, 10, 64)
		if err != nil {
			panic(err)
		}
	}
	return
}

func ParseBytesContentRange(s string) (ret BytesContentRange, ok bool) {
	unit, ranges := parseUnitRanges(s)
	if unit != "bytes" {
		return
	}
	ret = parseContentRange(ranges)
	ok = true
	return
}
