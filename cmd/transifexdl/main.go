package main

import (
	"encoding/json"
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
		if shortCode == "en" {
			continue
		}
		if pct := 100 * stat.Translated / (stat.Translated + stat.Untranslated); pct < 95 {
			log.Printf("Skipping language %q (too low completion ratio %d%%)", shortCode, pct)
			os.Remove("lang-" + shortCode + ".json")
			continue
		}

		log.Printf("Updating language %q", shortCode)

		langs = append(langs, shortCode)
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
	json.NewEncoder(os.Stdout).Encode(langs)
}

func req(url string) *http.Response {
	user := os.Getenv("TRANSIFEX_USER")
	pass := os.Getenv("TRANSIFEX_PASS")

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
