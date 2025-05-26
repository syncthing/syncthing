// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	githubToken = os.Getenv("GITHUB_TOKEN")
	githubRepo  = cmp.Or(os.Getenv("GITHUB_REPOSITORY"), "syncthing/syncthing")
)

func main() {
	ver := flag.String("new-ver", "", "New version tag")
	prevVer := flag.String("prev-ver", "", "Previous version tag")
	branch := flag.String("branch", "HEAD", "Branch to release from")
	flag.Parse()

	log.SetOutput(os.Stderr)

	if *ver == "" {
		log.Fatalln("Must set --new-ver")
	}
	if githubToken == "" {
		log.Fatalln("Must set $GITHUB_TOKEN")
	}

	addl, err := additionalNotes(*ver)
	if err != nil {
		log.Fatalln("Gathering additional notes:", err)
	}
	notes, err := generatedNotes(*ver, *branch, *prevVer)
	if err != nil {
		log.Fatalln("Gathering github notes:", err)
	}

	if addl != "" {
		fmt.Println(addl)
	}
	fmt.Println(notes)
}

// Load potential additional release notes from within the repo
func additionalNotes(newVer string) (string, error) {
	ver, _, _ := strings.Cut(newVer, "-")
	bs, err := os.ReadFile(fmt.Sprintf("relnotes/%s.md", ver))
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(bs), err
}

// Load generated release notes (list of pull requests and contributors)
// from GitHub.
func generatedNotes(newVer, targetCommit, prevVer string) (string, error) {
	fields := map[string]string{
		"tag_name":          newVer,
		"target_commitish":  targetCommit,
		"previous_tag_name": prevVer,
	}
	bs, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/repos/"+githubRepo+"/releases/generate-notes", bytes.NewReader(bs)) //nolint:noctx
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("X-Github-Api-Version", "2022-11-28")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(res.Body)
		log.Print(string(bs))
		return "", errors.New(res.Status) //nolint:err113
	}
	defer res.Body.Close()

	var resJSON struct {
		Body string
	}
	if err := json.NewDecoder(res.Body).Decode(&resJSON); err != nil {
		return "", err
	}
	return resJSON.Body, nil
}
