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

var (
	specLocalestrings = `(Name|GenericName|Comment|Keywords)` // only these and Icon are allowed per spec, and icons have nothing to do with Transifex anyway

	specLan       = `[a-z]{2}[a-z]?`                      // language is 2 or 3 lowercase letters: ISO-639 code (en, arb)
	specCou       = `(_[A-Z]{2})?`                        // _COUNTRY is 2 uppercase letters: ISO 3166-1 code (FR, CN) - optional
	specEnc       = `(\.\S+)?`                            // .encoding can be anything non-whitespace (utf8, MACCYRILLIC, iso-8859-15) - optional and ignored upon parsing per spec
	specMod       = `(@\S+)?`                             // @modifier can be anything non-whitespace (euro, valencia, saaho) - optional
	specLanguages = specLan + specCou + specEnc + specMod // language_COUNTRY.encoding@modifier (per spec)

	locRe       = regexp.MustCompile(`^` + specLocalestrings + `=.*\S*.*`)                   // these lines are to be translated
	transRe     = regexp.MustCompile(`^` + specLocalestrings + `\[` + specLanguages + `\]=`) // these are translated lines, we ditch them and regenerate
	validLangRe = regexp.MustCompile(`^` + specLanguages + `$`)                              // these are valid language codes
	groupRe     = regexp.MustCompile(`^\[Desktop Entry\]$`)                                  // we only process [Desktop Entry] section, all others are to be preserved verbatim
	badRe       = regexp.MustCompile(`\n`)                                                   // we don't want newlines in our translated string

	langs = make([]string, 0)
)

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

// walkerLanguages is for translation directory. It just looks at filenames and builds our list of available languages.
func walkerLanguages(file string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if filepath.Ext(file) == ".json" && filepath.Base(file)[0:5] == "lang-" && info.Mode().IsRegular() {

		// Our filenames are lang-LANGUAGE.json. We only need the LANGUAGE part.
		lang := strings.TrimSuffix(filepath.Base(file)[5:], ".json")

		// Transifex LANGUAGE format differs from spec (es-ES instead of es_ES), so convert.
		for i := 2; i < 4; i++ {
			lang = replaceAtIndex(lang, '-', '_', i)
		}

		// If the resulting LANGUAGE looks spec-conforming, it goes to the list.
		if validLangRe.MatchString(lang) {
			langs = append(langs, lang)
		}
	}

	return nil
}

// replaceAtIndex replaces f for r in string in position i, otherwise returns unmodified string.
func replaceAtIndex(in string, f rune, r rune, i int) string {
	out := []rune(in)
	if len(out) > i && out[i] == f {
		out[i] = r
	}
	return string(out)
}

// walkerDesktop is for .desktop files directory. It looks for .desktop files and processes them in place.
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

		lines := strings.Split(string(bs), "\n") // this is what's in out .desktop file, line by line
		linesNew := []string{}                   // and this will be what we write in its stead when done

		// We only will process [Desktop Entry] section. Spec explicitly requires us to leave any other section alone.
		in := false

		for _, line := range lines {

			// This is an already translated line. We discard it.
			if in && transRe.MatchString(line) {
				continue
			}

			// All other lines need to be kept verbatim.
			linesNew = append(linesNew, line)

			// This is "[Desktop Entry]" line. We will process the lines below it.
			if groupRe.MatchString(line) {
				in = true
				continue
			}

			// This line starts another section. We don't process the lines below it.
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				in = false
				continue
			}

			// This is one of our lines that need to be translated. We append its translations to our output.
			if in && locRe.MatchString(line) {
				trans := translate(line)
				linesNew = append(linesNew, trans...)
			}
		}

		// Now we take the result of our work and write it to .desktop file.
		lNew := strings.Join(linesNew, "\n") // this way we avoid appending trailing newline to the file

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

// translate takes "Key=Value" and returns []"Key[language]=Translated value"
func translate(line string) []string {
	translated := []string{}
	values := strings.SplitN(line, "=", 2) // as values may by spec contain "=" symbols, too
	trans := make(map[string]string)       // trans["language"] = "Translated value"

	// One special case is Keywords, as there can be many of them, and each gets translated separately
	if values[0] == "Keywords" {
		trans = getKeywordTrans(values[1])
	} else {
		trans = getTrans(values[1])
	}

	// Construct whole lines to return
	for lang, tran := range trans {
		newLine := values[0] + "[" + lang + "]" + "=" + tran
		translated = append(translated, newLine)
	}

	return translated
}

//getTrans takes "phrase" and returns map["language"]"translation of phrase" with only valid translations
func getTrans(line string) map[string]string {
	trans := make(map[string]string) // trans["language"] = "Translated value"
	for _, lang := range langs {
		translation := getTranslation(lang, line)
		if translation != "" {
			trans[lang] = translation
		}
	}
	return trans
}

//getKeywordTrans is like getTrans, but for multi-values lines
func getKeywordTrans(line string) map[string]string {
	trans := make(map[string]string) // trans["language"] = "Translated value;another one;yet another"
	words := strings.Split(line, ";")

	// for each language we have
	for _, lang := range langs {
		tr := []string{} // translations for individual words
		tl := ""         // "Translated value;another one;yet another"

		// translate each word separately
		for _, word := range words {
			translation := getTranslation(lang, word)
			if translation != "" {
				tr = append(tr, translation)
			}
		}

		// now concatenate into one line
		for _, tran := range tr {
			tl = tl + tran + ";"
		}

		// and add to the list of good translations
		if tl != "" {
			trans[lang] = tl
		}
	}
	return trans
}

//getTranslation returns translation of line in lang, returns "" if it is not available
func getTranslation(lang string, line string) string {

	// change our LANGUAGE format from .desktop to Transifex and look in the corresponding file
	for i := 2; i < 4; i++ {
		lang = replaceAtIndex(lang, '_', '-', i)
	}
	langFile := "lang-" + lang + ".json"
	langFile = filepath.Join(os.Args[2], langFile)

	fd, err := os.Open(langFile)
	if err != nil {
		log.Fatal(err)
	}

	trans := make(map[string]string) // all the translations in the language file
	err = json.NewDecoder(fd).Decode(&trans)
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()

	line = strings.TrimSpace(line) // that's how the line was written to Transifex, so we look it up the same way

	// This check is probably redundant depending on how Tansifex really works,
	// but "\n" would really damage our files (and is illegal), so we'll have this check just in case.
	if badRe.MatchString(trans[line]) {
		return ""
	}

	return trans[line]
}
