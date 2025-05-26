package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var (
	githubToken = os.Getenv("GITHUB_TOKEN")
	githubRepo  = cmp.Or(os.Getenv("GITHUB_REPO"), "syncthing/syncthing")
)

func main() {
	ver := flag.String("new-ver", "", "New version tag")
	prevVer := flag.String("prev-ver", "", "Previous version tag")
	branch := flag.String("branch", "HEAD", "Branch to release from")
	flag.Parse()

	if *ver == "" {
		fmt.Println("Must set -new-ver")
		os.Exit(2)
	}

	addl, err := additionalNotes(*ver)
	if err != nil {
		fmt.Println("Gathering additional notes:", err)
		os.Exit(1)
	}
	notes, err := generatedNotes(*ver, *branch, *prevVer)
	if err != nil {
		fmt.Println("Gathering github notes:", err)
		os.Exit(1)
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
	bs, _ := json.Marshal(fields)
	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/repos/"+githubRepo+"/releases/generate-notes", bytes.NewReader(bs))
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
	defer res.Body.Close()

	var resJSON struct {
		Body string
	}
	if err := json.NewDecoder(res.Body).Decode(&resJSON); err != nil {
		return "", err
	}
	return resJSON.Body, nil
}
