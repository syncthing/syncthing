// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

// Updates the list of software copyrights in aboutModalView.html based on the
// output of `go mod graph`.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var copyrightMap = map[string]string{
	// https://github.com/aws/aws-sdk-go/blob/main/NOTICE.txt#L2
	"aws/aws-sdk-go": "Copyright &copy; 2015 Amazon.com, Inc. or its affiliates, Copyright 2014-2015 Stripe, Inc",
	// https://github.com/ccding/go-stun/blob/master/main.go#L1
	"ccding/go-stun": "Copyright &copy; 2016 Cong Ding",
	// https://github.com/search?q=repo%3Acertifi%2Fgocertifi%20copyright&type=code
	// "certifi/gocertifi": "No copyrights found",
	// https://github.com/search?q=repo%3Aebitengine%2Fpurego%20copyright&type=code
	"ebitengine/purego": "Copyright &copy; 2022 The Ebitengine Authors",
	// https://github.com/search?q=repo%3Agoogle%2Fpprof%20copyright&type=code
	"google/pprof": "Copyright &copy; 2016 Google Inc",
	// https://github.com/greatroar/blobloom/blob/master/README.md?plain=1#L74
	"greatroar/blobloom": "Copyright &copy; 2020-2024 the Blobloom authors",
	// https://github.com/jmespath/go-jmespath/blob/master/NOTICE#L2
	"jmespath/go-jmespath": "Copyright &copy; 2015 James Saryerwinnie",
	// https://github.com/maxmind/geoipupdate/blob/main/README.md?plain=1#L140
	"maxmind/geoipupdate": "Copyright &copy; 2018-2024 by MaxMind, Inc",
	// https://github.com/search?q=repo%3Aprometheus%2Fclient_golang%20copyright&type=code
	"prometheus/client_golang": "Copyright 2012-2015 The Prometheus Authors",
	// https://github.com/search?q=repo%3Apuzpuzpuz%2Fxsync%20copyright&type=code
	// "puzpuzpuz/xsync": "No copyrights found",
	// https://github.com/search?q=repo%3Atklauser%2Fnumcpus%20copyright&type=code
	"tklauser/numcpus": "Copyright &copy; 2018-2024 Tobias Klauser",
	// https://github.com/search?q=repo%3Auber-go%2Fmock%20copyright&type=code
	"go.uber.org/mock": "Copyright &copy; 2010-2022 Google LLC",
}

var urlMap = map[string]string{
	"fontawesome.io":           "https://github.com/FortAwesome/Font-Awesome",
	"go.uber.org/automaxprocs": "https://github.com/uber-go/automaxprocs",
	// "go.uber.org/mock":           "https://github.com/uber-go/mock",
	"google.golang.org/protobuf": "https://github.com/protocolbuffers/protobuf-go",
	// "gopkg.in/yaml.v2":           "", // ignore, as gopkg.in/yaml.v3 supersedes
	// "gopkg.in/yaml.v3":           "https://github.com/go-yaml/yaml",
	"sigs.k8s.io/yaml": "https://github.com/kubernetes-sigs/yaml",
}

const htmlFile = "gui/default/syncthing/core/aboutModalView.html"

type Type int

const (
	// TypeJS defines non-Go copyright notices
	TypeJS Type = iota
	// TypeKeep defines Go copyright notices for packages that are still used.
	TypeKeep
	// TypeToss defines Go copyright notices for packages that are no longer used.
	TypeToss
	// TypeNew defines Go copyright notices for new packages found via `go mod graph`.
	TypeNew
)

type CopyrightNotice struct {
	Type           Type
	Name           string
	HTML           string
	Module         string
	URL            string
	Copyright      string
	RepoURL        string
	RepoCopyrights []string
}

var copyrightRe = regexp.MustCompile(`(?s)id="copyright-notices">(.+?)</ul>`)

func main() {
	bs := readAll(htmlFile)
	matches := copyrightRe.FindStringSubmatch(string(bs))

	if len(matches) <= 1 {
		log.Fatal("Cannot find id copyright-notices in ", htmlFile)
	}

	modules := getModules()

	notices := parseCopyrightNotices(matches[1])
	old := len(notices)

	// match up modules to notices
	matched := map[string]bool{}
	removes := 0
	for i, notice := range notices {
		if notice.Type == TypeJS {
			continue
		}
		found := ""
		for _, module := range modules {
			if strings.Contains(module, notice.Name) {
				found = module

				break
			}
		}
		if found != "" {
			matched[found] = true
			notices[i].Module = found

			continue
		}
		removes++
		fmt.Printf("Removing: %-40s %-55s %s\n", notice.Name, notice.URL, notice.Copyright)
		notices[i].Type = TypeToss
	}

	// add new modules to notices
	adds := 0
	for _, module := range modules {
		_, ok := matched[module]
		if ok {
			continue
		}

		adds++
		notice := CopyrightNotice{}
		notice.Name = module
		if strings.HasPrefix(notice.Name, "github.com/") {
			notice.Name = strings.ReplaceAll(notice.Name, "github.com/", "")
		}
		notice.Type = TypeNew

		url, ok := urlMap[module]
		if ok {
			notice.URL = url
			notice.RepoURL = url
		} else {
			notice.URL = "https://" + module
			notice.RepoURL = "https://" + module
		}
		notices = append(notices, notice)
	}

	if removes == 0 && adds == 0 {
		// authors.go is quiet, so let's be quiet too.
		// fmt.Printf("No changes detected in %d modules and %d notices\n", len(modules), len(notices))
		os.Exit(0)
	}

	// get copyrights via Github API for new modules
	notfound := 0
	for i, n := range notices {
		if n.Type != TypeNew {
			continue
		}

		copyright, ok := copyrightMap[n.Name]
		if ok {
			notices[i].Copyright = copyright

			continue
		}
		notices[i].Copyright = defaultCopyright(n)

		if strings.Contains(n.URL, "github.com/") {
			notices[i].RepoURL = notices[i].URL
			owner, repo := parseGitHubURL(n.URL)
			licenseText := getLicenseText(owner, repo)
			notices[i].RepoCopyrights = extractCopyrights(licenseText, n)

			if len(notices[i].RepoCopyrights) > 0 {
				notices[i].Copyright = notices[i].RepoCopyrights[0]
			}

			notices[i].HTML = fmt.Sprintf("<li><a href=\"%s\">%s</a>, %s.</li>", n.URL, n.Name, notices[i].Copyright)
			if len(notices[i].RepoCopyrights) > 0 {
				continue
			}
		}
		fmt.Printf("Copyright not found: %-30s : using %q\n", n.Name, notices[i].Copyright)
		notfound++
	}

	replacements := write(notices, bs)
	fmt.Printf("Removed:              %3d\n", removes)
	fmt.Printf("Added:                %3d\n", adds)
	fmt.Printf("Copyrights not found: %3d\n", notfound)
	fmt.Printf("Old package count:    %3d\n", old)
	fmt.Printf("New package count:    %3d\n", replacements)
}

func write(notices []CopyrightNotice, bs []byte) int {
	keys := make([]string, 0, len(notices))

	noticeMap := make(map[string]CopyrightNotice, 0)

	for _, n := range notices {
		if n.Type != TypeKeep && n.Type != TypeNew {
			continue
		}
		if n.Type == TypeNew {
			fmt.Printf("Adding: %-40s %-55s %s\n", n.Name, n.URL, n.Copyright)
		}
		keys = append(keys, n.Name)
		noticeMap[n.Name] = n
	}

	slices.Sort(keys)

	indent := "          "
	replacements := []string{}
	for _, n := range notices {
		if n.Type != TypeJS {
			continue
		}
		replacements = append(replacements, indent+n.HTML)
	}

	for _, k := range keys {
		n := noticeMap[k]
		line := fmt.Sprintf("%s<li><a href=\"%s\">%s</a>, %s.</li>", indent, n.URL, n.Name, n.Copyright)
		replacements = append(replacements, line)
	}
	replacement := strings.Join(replacements, "\n")

	bs = copyrightRe.ReplaceAll(bs, []byte("id=\"copyright-notices\">\n"+replacement+"\n        </ul>"))
	writeFile(htmlFile, string(bs))

	return len(replacements)
}

func readAll(path string) []byte {
	fd, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	bs, err := io.ReadAll(fd)
	if err != nil {
		log.Fatal(err)
	}

	return bs
}

func writeFile(path string, data string) {
	err := os.WriteFile(path, []byte(data), 0o644)
	if err != nil {
		log.Fatal(err)
	}
}

func getModules() []string {
	ignoreRe := regexp.MustCompile(`golang\.org/x/|github\.com/syncthing|^[^.]+(/|$)`)

	// List all modules (used for mapping packages to modules)
	data, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		log.Fatalf("go list -m all: %v", err)
	}
	modules := strings.Split(string(data), "\n")
	for i := range modules {
		modules[i], _, _ = strings.Cut(modules[i], " ")
	}
	modules = slices.DeleteFunc(modules, func(s string) bool { return s == "" })

	// List all packages in use by the syncthing binary, map them to modules
	data, err = exec.Command("go", "list", "-deps", "./cmd/syncthing").Output()
	if err != nil {
		log.Fatalf("go list -deps ./cmd/syncthing: %v", err)
	}
	packages := strings.Split(string(data), "\n")
	packages = slices.DeleteFunc(packages, func(s string) bool { return s == "" })

	seen := make(map[string]struct{})
	for _, pkg := range packages {
		if ignoreRe.MatchString(pkg) {
			continue
		}

		// Find module for package
		modIdx := slices.IndexFunc(modules, func(mod string) bool {
			return strings.HasPrefix(pkg, mod)
		})
		if modIdx < 0 {
			log.Println("no module for", pkg)
			continue
		}
		module := modules[modIdx]
		seen[module] = struct{}{}
	}

	adds := make([]string, 0)
	for k := range seen {
		adds = append(adds, k)
	}

	slices.Sort(adds)

	return adds
}

func parseCopyrightNotices(input string) []CopyrightNotice {
	doc, err := html.Parse(strings.NewReader("<ul>" + input + "</ul>"))
	if err != nil {
		log.Fatal(err)
	}

	var notices []CopyrightNotice

	typ := TypeJS

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" {
			var notice CopyrightNotice
			var aFound bool

			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "a" {
					aFound = true
					for _, attr := range c.Attr {
						if attr.Key == "href" {
							notice.URL = attr.Val
						}
					}
					if c.FirstChild != nil && c.FirstChild.Type == html.TextNode {
						notice.Name = strings.TrimSpace(c.FirstChild.Data)
					}
				} else if c.Type == html.TextNode && aFound {
					// Anything after <a> is considered the copyright
					notice.Copyright = strings.TrimSpace(html.UnescapeString(c.Data))
					notice.Copyright = strings.Trim(notice.Copyright, "., ")
				}
				if typ == TypeJS && strings.Contains(notice.URL, "AudriusButkevicius") {
					typ = TypeKeep
				}
				notice.Type = typ
				var buf strings.Builder
				_ = html.Render(&buf, n)
				notice.HTML = buf.String()
			}

			notice.Copyright = strings.ReplaceAll(notice.Copyright, "©", "&copy;")
			notice.HTML = strings.ReplaceAll(notice.HTML, "©", "&copy;")
			notices = append(notices, notice)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)

	return notices
}

func parseGitHubURL(u string) (string, string) {
	parsed, err := url.Parse(u)
	if err != nil {
		log.Fatal(err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		log.Fatal(fmt.Errorf("invalid GitHub URL: %q", parsed.Path))
	}

	return parts[0], parts[1]
}

func getLicenseText(owner, repo string) string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/license", owner, repo)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return ""
	}
	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	body, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Fatal(err)
	}

	if result.Encoding != "base64" {
		log.Fatal(fmt.Sprintf("unexpected encoding: %q", result.Encoding))
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		log.Fatal(err)
	}

	return string(decoded)
}

func extractCopyrights(license string, notice CopyrightNotice) []string {
	lines := strings.Split(license, "\n")

	re := regexp.MustCompile(`(?i)^\s*(copyright\s*(?:©|\(c\)|&copy;|19|20).*)$`)

	copyrights := []string{}

	for _, line := range lines {
		if matches := re.FindStringSubmatch(strings.TrimSpace(line)); len(matches) == 2 {
			copyright := strings.TrimSpace(matches[1])
			re := regexp.MustCompile(`(?i)all rights reserved`)
			copyright = re.ReplaceAllString(copyright, "")
			copyright = strings.ReplaceAll(copyright, "©", "&copy;")
			copyright = strings.ReplaceAll(copyright, "(C)", "&copy;")
			copyright = strings.ReplaceAll(copyright, "(c)", "&copy;")
			copyright = strings.Trim(copyright, "., ")
			copyrights = append(copyrights, copyright)
		}
	}

	if len(copyrights) > 0 {
		return copyrights
	}

	return []string{}
}

func defaultCopyright(n CopyrightNotice) string {
	year := time.Now().Format("2006")

	return fmt.Sprintf("Copyright &copy; %v, the %s authors", year, n.Name)
}

func writeNotices(path string, notices []CopyrightNotice) {
	s := ""
	for i, n := range notices {
		s += "#        : " + strconv.Itoa(i) + "\n" + n.String()
	}
	writeFile(path, s)
}

func (n CopyrightNotice) String() string {
	return fmt.Sprintf("Type     : %v\nHTML     : %v\nName     : %v\nModule   : %v\nURL      : %v\nCopyright: %v\nRepoURL  : %v\nRepoCopys: %v\n\n",
		n.Type, n.HTML, n.Name, n.Module, n.URL, n.Copyright, n.RepoURL, strings.Join(n.RepoCopyrights, ","))
}

func (t Type) String() string {
	switch t {
	case TypeJS:
		return "TypeJS"
	case TypeKeep:
		return "TypeKeep"
	case TypeToss:
		return "TypeToss"
	case TypeNew:
		return "TypeNew"
	default:
		return "unknown"
	}
}
