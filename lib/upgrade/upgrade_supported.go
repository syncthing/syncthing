// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !noupgrade

package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/signature"
)

// This is an HTTP/HTTPS client that does *not* perform certificate
// validation. We do this because some systems where Syncthing runs have
// issues with old or missing CA roots. It doesn't actually matter that we
// load the upgrade insecurely as we verify an ECDSA signature of the actual
// binary contents before accepting the upgrade.
var insecureHTTP = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

// LatestGithubReleases returns the latest releases, including prereleases or
// not depending on the argument
func LatestGithubReleases(version string) ([]Release, error) {
	resp, err := insecureHTTP.Get("https://api.github.com/repos/syncthing/syncthing/releases?per_page=30")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("API call returned HTTP error: %s", resp.Status)
	}

	var rels []Release
	json.NewDecoder(resp.Body).Decode(&rels)
	resp.Body.Close()

	return rels, nil
}

type SortByRelease []Release

func (s SortByRelease) Len() int {
	return len(s)
}
func (s SortByRelease) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s SortByRelease) Less(i, j int) bool {
	return CompareVersions(s[i].Tag, s[j].Tag) > 0
}

func LatestRelease(version string) (Release, error) {
	rels, _ := LatestGithubReleases(version)
	return SelectLatestRelease(version, rels)
}

func SelectLatestRelease(version string, rels []Release) (Release, error) {
	if len(rels) == 0 {
		return Release{}, ErrVersionUnknown
	}

	sort.Sort(SortByRelease(rels))
	// Check for a beta build
	beta := strings.Contains(version, "-beta")

	for _, rel := range rels {
		if rel.Prerelease && !beta {
			continue
		}
		for _, asset := range rel.Assets {
			assetName := path.Base(asset.Name)
			// Check for the architecture
			expectedRelease := releaseName(rel.Tag)
			if debug {
				l.Debugf("expected release asset %q", expectedRelease)
			}
			if debug {
				l.Debugln("considering release", assetName)
			}
			if strings.HasPrefix(assetName, expectedRelease) {
				return rel, nil
			}
		}
	}
	return Release{}, ErrVersionUnknown
}

// Upgrade to the given release, saving the previous binary with a ".old" extension.
func upgradeTo(binary string, rel Release) error {
	expectedRelease := releaseName(rel.Tag)
	if debug {
		l.Debugf("expected release asset %q", expectedRelease)
	}
	for _, asset := range rel.Assets {
		assetName := path.Base(asset.Name)
		if debug {
			l.Debugln("considering release", assetName)
		}

		if strings.HasPrefix(assetName, expectedRelease) {
			return upgradeToURL(binary, asset.URL)
		}
	}

	return ErrVersionUnknown
}

// Upgrade to the given release, saving the previous binary with a ".old" extension.
func upgradeToURL(binary string, url string) error {
	fname, err := readRelease(filepath.Dir(binary), url)
	if err != nil {
		return err
	}

	old := binary + ".old"
	os.Remove(old)
	err = os.Rename(binary, old)
	if err != nil {
		return err
	}
	err = os.Rename(fname, binary)
	if err != nil {
		return err
	}
	return nil
}

func readRelease(dir, url string) (string, error) {
	if debug {
		l.Debugf("loading %q", url)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Accept", "application/octet-stream")
	resp, err := insecureHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch runtime.GOOS {
	case "windows":
		return readZip(dir, resp.Body)
	default:
		return readTarGz(dir, resp.Body)
	}
}

func readTarGz(dir string, r io.Reader) (string, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}

	tr := tar.NewReader(gr)

	var tempName string
	var sig []byte

	// Iterate through the files in the archive.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return "", err
		}

		shortName := path.Base(hdr.Name)

		if debug {
			l.Debugf("considering file %q", shortName)
		}

		err = archiveFileVisitor(dir, &tempName, &sig, shortName, tr)
		if err != nil {
			return "", err
		}

		if tempName != "" && sig != nil {
			break
		}
	}

	if err := verifyUpgrade(tempName, sig); err != nil {
		return "", err
	}

	return tempName, nil
}

func readZip(dir string, r io.Reader) (string, error) {
	body, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}

	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", err
	}

	var tempName string
	var sig []byte

	// Iterate through the files in the archive.
	for _, file := range archive.File {
		shortName := path.Base(file.Name)

		if debug {
			l.Debugf("considering file %q", shortName)
		}

		inFile, err := file.Open()
		if err != nil {
			return "", err
		}

		err = archiveFileVisitor(dir, &tempName, &sig, shortName, inFile)
		inFile.Close()
		if err != nil {
			return "", err
		}

		if tempName != "" && sig != nil {
			break
		}
	}

	if err := verifyUpgrade(tempName, sig); err != nil {
		return "", err
	}

	return tempName, nil
}

// archiveFileVisitor is called for each file in an archive. It may set
// tempFile and signature.
func archiveFileVisitor(dir string, tempFile *string, signature *[]byte, filename string, filedata io.Reader) error {
	var err error
	switch filename {
	case "syncthing", "syncthing.exe":
		if debug {
			l.Debugln("reading binary")
		}
		*tempFile, err = writeBinary(dir, filedata)
		if err != nil {
			return err
		}

	case "syncthing.sig", "syncthing.exe.sig":
		if debug {
			l.Debugln("reading signature")
		}
		*signature, err = ioutil.ReadAll(filedata)
		if err != nil {
			return err
		}
	}

	return nil
}

func verifyUpgrade(tempName string, sig []byte) error {
	if tempName == "" {
		return fmt.Errorf("no upgrade found")
	}
	if sig == nil {
		return fmt.Errorf("no signature found")
	}

	if debug {
		l.Debugf("checking signature\n%s", sig)
	}

	fd, err := os.Open(tempName)
	if err != nil {
		return err
	}
	err = signature.Verify(SigningKey, sig, fd)
	fd.Close()

	if err != nil {
		os.Remove(tempName)
		return err
	}

	return nil
}

func writeBinary(dir string, inFile io.Reader) (filename string, err error) {
	// Write the binary to a temporary file.

	outFile, err := ioutil.TempFile(dir, "syncthing")
	if err != nil {
		return "", err
	}

	_, err = io.Copy(outFile, inFile)
	if err != nil {
		os.Remove(outFile.Name())
		return "", err
	}

	err = outFile.Close()
	if err != nil {
		os.Remove(outFile.Name())
		return "", err
	}

	err = os.Chmod(outFile.Name(), os.FileMode(0755))
	if err != nil {
		os.Remove(outFile.Name())
		return "", err
	}

	return outFile.Name(), nil
}
