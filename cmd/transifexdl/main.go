// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
)

type stat struct {
	Translated   int `json:"translated_entities"`
	Untranslated int `json:"untranslated_entities"`
}

type translation struct {
	Content string
}

func main() {
	log.SetFlags(log.Lshortfile)

	if u, p := userPass(); u == "" || p == "" {
		log.Fatal("Need environment variables TRANSIFEX_USER and TRANSIFEX_PASS")
	}

	resp := req("https://www.transifex.com/api/2/project/syncthing/resource/gui/stats")

	var stats map[string]stat
	err := json.NewDecoder(resp.Body).Decode(&stats)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()

	var langs []string
	for code, stat := range stats {
		shortCode := code[:2]
		if pct := 100 * stat.Translated / (stat.Translated + stat.Untranslated); pct < 95 {
			log.Printf("Skipping language %q (too low completion ratio %d%%)", shortCode, pct)
			os.Remove("lang-" + shortCode + ".json")
			continue
		}

		langs = append(langs, shortCode)
		if shortCode == "en" {
			continue
		}

		log.Printf("Updating language %q", shortCode)

		resp := req("https://www.transifex.com/api/2/project/syncthing/resource/gui/translation/" + code)
		var t translation
		err := json.NewDecoder(resp.Body).Decode(&t)
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()

		fd, err := os.Create("lang-" + shortCode + ".json")
		if err != nil {
			log.Fatal(err)
		}
		fd.WriteString(t.Content)
		fd.Close()
	}

	sort.Strings(langs)
	fmt.Print("var validLangs = ")
	json.NewEncoder(os.Stdout).Encode(langs)
}

func userPass() (string, string) {
	user := os.Getenv("TRANSIFEX_USER")
	pass := os.Getenv("TRANSIFEX_PASS")
	return user, pass
}

func req(url string) *http.Response {
	user, pass := userPass()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	return resp
}
