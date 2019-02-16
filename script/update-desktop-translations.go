// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var groupRe = regexp.MustCompile(`^\[Desktop Entry\]$`)
var locRe = regexp.MustCompile(`^(Name|GenericName|Comment|Keywords)=.*\S*.*`)
var transRe = regexp.MustCompile(`^(Name|GenericName|Comment|Keywords)\[[a-z]{2}[a-z]?(_[A-Z]{2})?(\..+)?(@\w+)?\]=`)
var validLangRe = regexp.MustCompile(`^[a-z]{2}[a-z]?(_[A-Z]{2})?(\..+)?(@\w+)?$`)
var badRe = regexp.MustCompile(`\n`)

var langs = make([]string, 0)

func main() {
	err := filepath.Walk(os.Args[2], walkerLanguages)
	if err != nil {
		log.Fatal(err)
	}

	err = filepath.Walk(os.Args[1], walkerDesktop)
	if err != nil {
		log.Fatal(err)
	}
}

func walkerLanguages(file string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if filepath.Ext(file) == ".json" && filepath.Base(file)[0:5] == "lang-" && info.Mode().IsRegular() {
		lang := strings.TrimSuffix(filepath.Base(file)[5:], ".json")
		for i := 2; i < 4; i++ {
			lang = replaceAtIndex(lang, '-', '_', i)
		}
		if validLangRe.MatchString(lang) {
			langs = append(langs, lang)
		}
	}

	return nil
}

func replaceAtIndex(in string, f rune, r rune, i int) string {
	out := []rune(in)
	if len(out) > i && out[i] == f {
		out[i] = r
	}
	return string(out)
}

func walkerDesktop(file string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if filepath.Ext(file) == ".desktop" && info.Mode().IsRegular() {
		fd, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}

		defer fd.Close()

		bs, err := ioutil.ReadAll(fd)
		if err != nil {
			log.Fatal(err)
		}

		lines := strings.Split(string(bs), "\n")
		linesNew := []string{}

		in := false

		for _, line := range lines {
			if in && transRe.MatchString(line) {
				continue
			}

			linesNew = append(linesNew, line)

			if groupRe.MatchString(line) {
				in = true
				continue
			}

			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				in = false
				continue
			}

			if in && locRe.MatchString(line) {
				trans := translate(line)
				linesNew = append(linesNew, trans...)
			}
		}

		lNew := strings.Join(linesNew, "\n")

		out, err := os.Create(file)
		if err != nil {
			log.Fatal(err)
		}
		defer out.Close()

		_, err = out.WriteString(lNew)
		if err != nil {
			log.Fatal(err)
		}
		err = out.Sync()
		if err != nil {
			log.Fatal(err)
		}

	}

	return (nil)
}

func translate(line string) []string {
	translated := []string{}
	values := strings.SplitN(line, "=", 2)
	trans := make(map[string]string)
	if values[0] == "Keywords" {
		trans = getKeywordTrans(values[1])
	} else {
		trans = getTrans(values[1])
	}
	for lang, tran := range trans {
		newLine := values[0] + "[" + lang + "]" + "=" + tran
		translated = append(translated, newLine)
	}
	return translated
}

func getTrans(line string) map[string]string {
	trans := make(map[string]string)
	for _, lang := range langs {
		translation := getTranslation(lang, line)
		if translation != "" {
			trans[lang] = translation
		}
	}
	return trans
}

func getTranslation(lang string, line string) string {
	for i := 2; i < 4; i++ {
		lang = replaceAtIndex(lang, '_', '-', i)
	}
	langFile := "lang-" + lang + ".json"
	langFile = filepath.Join(os.Args[2], langFile)

	fd, err := os.Open(langFile)
	if err != nil {
		log.Fatal(err)
	}

	trans := make(map[string]string)
	err = json.NewDecoder(fd).Decode(&trans)
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()

	line = strings.TrimSpace(line)

	// This check is probably redundant depending on how Tansifex really works,
	// but "\n" would really damage our files, so we'll have this check just in case.
	if badRe.MatchString(line) {
		line = ""
	}

	return trans[line]
}

func getKeywordTrans(line string) map[string]string {
	trans := make(map[string]string)
	words := strings.Split(line, ";")
	for _, lang := range langs {
		tr := []string{}
		tl := ""
		for _, word := range words {
			translation := getTranslation(lang, word)
			if translation != "" {
				tr = append(tr, translation)
			}
		}
		for _, tran := range tr {
			tl = tl + tran + ";"
		}
		if tl != "" {
			trans[lang] = tl
		}
	}
	return trans
}
