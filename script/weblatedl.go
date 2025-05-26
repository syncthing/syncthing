// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
)

type stat struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Total      int    `json:"total"`
	Translated int    `json:"translated"`
	Fuzzy      int    `json:"fuzzy"`
}

type translation map[string]any

func main() {
	log.SetFlags(log.Lshortfile)

	token := os.Getenv("WEBLATE_TOKEN")
	if token == "" {
		log.Fatal("Need environment variable WEBLATE_TOKEN")
	}

	curValidLangs := make(map[string]struct{})
	for _, lang := range loadValidLangs() {
		curValidLangs[lang] = struct{}{}
	}
	log.Println(curValidLangs)

	resp := req("https://hosted.weblate.org/api/components/syncthing/gui/statistics/", token)
	var statRes struct {
		Results []stat
	}
	if err := json.NewDecoder(resp.Body).Decode(&statRes); err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()

	names := make(map[string]string)

	var langs []string
	for _, stat := range statRes.Results {
		code := reformatLanguageCode(stat.Code)
		pct := 100 * stat.Translated / stat.Total
		if _, valid := curValidLangs[code]; pct < 75 || !valid && pct < 95 {
			log.Printf("Language %q too low completion ratio %d%%", code, pct)
		} else {
			langs = append(langs, code)
			names[code] = stat.Name
		}
		if code == "en" {
			continue
		}

		log.Printf("Updating language %q", code)

		resp := req("https://hosted.weblate.org/api/translations/syncthing/gui/"+stat.Code+"/file/", token)
		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()

		var t translation
		if err := json.Unmarshal(bs, &t); err != nil {
			log.Fatal(err)
		}

		fd, err := os.Create("lang-" + code + ".json")
		if err != nil {
			log.Fatal(err)
		}
		fd.Write(bs)
		fd.Close()
	}

	saveValidLangs(langs)
	saveLanguageNames(names)
}

func reformatLanguageCode(origCode string) string {
	switch origCode {
	case "ko":
		return "ko-KR"
	case "nb_NO":
		return "nb"
	case "ro":
		return "ro-RO"
	case "zh_Hans":
		return "zh-CN"
	case "zh_Hant":
		return "zh-TW"
	case "zh_Hant_HK":
		return "zh-HK"
	default:
		return strings.Replace(origCode, "_", "-", 1)
	}
}

func saveValidLangs(langs []string) {
	slices.Sort(langs)
	fd, err := os.Create("valid-langs.js")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(fd, "var validLangs = ")
	json.NewEncoder(fd).Encode(langs)
	fd.Close()
}

func saveLanguageNames(names map[string]string) {
	fd, err := os.Create("prettyprint.js")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(fd, "var langPrettyprint = ")
	json.NewEncoder(fd).Encode(names)
	fd.Close()
}

func req(url, token string) *http.Response {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Token "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	return resp
}

func loadValidLangs() []string {
	fd, err := os.Open("valid-langs.js")
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()
	bs, err := io.ReadAll(fd)
	if err != nil {
		log.Fatal(err)
	}

	var langs []string
	exp := regexp.MustCompile(`\[([a-zA-Z@",-_]+)\]`)
	if matches := exp.FindSubmatch(bs); len(matches) == 2 {
		langs = strings.Split(string(matches[1]), ",")
		for i := range langs {
			// Remove quotes
			langs[i] = langs[i][1 : len(langs[i])-1]
		}
	}

	return langs
}
