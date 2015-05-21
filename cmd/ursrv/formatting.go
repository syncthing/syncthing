package main

import (
	"bytes"
	"fmt"
	"strings"
)

func number(isBinary bool, v float64) string {
	if isBinary {
		return binary(v)
	} else {
		return metric(v)
	}
}

type prefix struct {
	Prefix     string
	Multiplier float64
}

var metricPrefixes = []prefix{
	{"G", 1e9},
	{"M", 1e6},
	{"k", 1e3},
}

var binaryPrefixes = []prefix{
	{"Gi", 1 << 30},
	{"Mi", 1 << 20},
	{"Ki", 1 << 10},
}

func metric(v float64) string {
	return withPrefix(v, metricPrefixes)
}

func binary(v float64) string {
	return withPrefix(v, binaryPrefixes)
}

func withPrefix(v float64, ps []prefix) string {
	for _, p := range ps {
		if v >= p.Multiplier {
			return fmt.Sprintf("%.1f %s", v/p.Multiplier, p.Prefix)
		}
	}
	return fmt.Sprint(v)
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
