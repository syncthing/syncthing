// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build !solaris,!windows,!noupgrade

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"bitbucket.org/kardianos/osext"
)

var GoArchExtra string // "", "v5", "v6", "v7"

func upgrade() error {
	if runtime.GOOS == "windows" {
		return errors.New("Upgrade currently unsupported on Windows")
	}

	path, err := osext.Executable()
	if err != nil {
		return err
	}

	rel, err := currentRelease()
	if err != nil {
		return err
	}

	switch compareVersions(rel.Tag, Version) {
	case -1:
		l.Okf("Current version %s is newer than latest release %s. Not upgrading.", Version, rel.Tag)
		return nil
	case 0:
		l.Okf("Already running the latest version, %s. Not upgrading.", Version)
		return nil
	default:
		l.Infof("Attempting upgrade to %s...", rel.Tag)
	}

	expectedRelease := fmt.Sprintf("syncthing-%s-%s%s-%s.", runtime.GOOS, runtime.GOARCH, GoArchExtra, rel.Tag)
	for _, asset := range rel.Assets {
		if strings.HasPrefix(asset.Name, expectedRelease) {
			if strings.HasSuffix(asset.Name, ".tar.gz") {
				l.Infof("Downloading %s...", asset.Name)
				fname, err := readTarGZ(asset.URL, filepath.Dir(path))
				if err != nil {
					return err
				}

				old := path + "." + Version
				err = os.Rename(path, old)
				if err != nil {
					return err
				}
				err = os.Rename(fname, path)
				if err != nil {
					return err
				}

				l.Okf("Upgraded %q to %s.", path, rel.Tag)
				l.Okf("Previous version saved in %q.", old)

				return nil
			}
		}
	}

	return fmt.Errorf("Found no asset for %q", expectedRelease)
}

func currentRelease() (githubRelease, error) {
	resp, err := http.Get("https://api.github.com/repos/calmh/syncthing/releases?per_page=1")
	if err != nil {
		return githubRelease{}, err
	}

	var rels []githubRelease
	json.NewDecoder(resp.Body).Decode(&rels)
	resp.Body.Close()

	if len(rels) != 1 {
		return githubRelease{}, fmt.Errorf("Unexpected number of releases: %d", len(rels))
	}
	return rels[0], nil
}

func readTarGZ(url string, dir string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Accept", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}

	tr := tar.NewReader(gr)
	if err != nil {
		return "", err
	}

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

		if path.Base(hdr.Name) == "syncthing" {
			of, err := ioutil.TempFile(dir, "syncthing")
			if err != nil {
				return "", err
			}
			io.Copy(of, tr)
			err = of.Close()
			if err != nil {
				os.Remove(of.Name())
				return "", err
			}

			os.Chmod(of.Name(), os.FileMode(hdr.Mode))
			return of.Name(), nil
		}
	}

	return "", fmt.Errorf("No upgrade found")
}
