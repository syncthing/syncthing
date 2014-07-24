package main

import (
	"bytes"
	"strconv"
	"strings"
)

type githubRelease struct {
	Tag        string        `json:"tag_name"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

func compareVersions(a, b string) int {
	return bytes.Compare(versionParts(a), versionParts(b))
}

func versionParts(v string) []byte {
	parts := strings.Split(v, "-")
	fields := strings.Split(parts[0], ".")
	res := make([]byte, len(fields))
	for i, s := range fields {
		v, _ := strconv.Atoi(s)
		res[i] = byte(v)
	}
	return res
}
