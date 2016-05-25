package missinggo

import (
	"strconv"
)

func StringTruth(s string) (ret bool) {
	if s == "" {
		return false
	}
	ret, err := strconv.ParseBool(s)
	if err == nil {
		return
	}
	i, err := strconv.ParseInt(s, 0, 0)
	if err == nil {
		return i != 0
	}
	return true
}
