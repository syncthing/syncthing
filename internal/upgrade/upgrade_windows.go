// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build windows,!noupgrade

package upgrade

import (
	"archive/zip"
	"bytes"
	"encoding/json"
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

// Upgrade to the given release, saving the previous binary with a ".old" extension.
func upgradeTo(rel Release, archExtra string) error {
	path, err := osext.Executable()
	if err != nil {
		return err
	}
	expectedRelease := fmt.Sprintf("syncthing-%s-%s%s-%s.", runtime.GOOS, runtime.GOARCH, archExtra, rel.Tag)
	if debug {
		l.Debugf("expected release asset %q", expectedRelease)
	}
	for _, asset := range rel.Assets {
		if debug {
			l.Debugln("considering release", asset)
		}
		if strings.HasPrefix(asset.Name, expectedRelease) {
			if strings.HasSuffix(asset.Name, ".zip") {
				fname, err := readZip(asset.URL, filepath.Dir(path))
				if err != nil {
					return err
				}

				old := path + ".old"

				os.Remove(old)
				err = os.Rename(path, old)
				if err != nil {
					return err
				}
				err = os.Rename(fname, path)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}

	return ErrVersionUnknown
}

// Returns the latest release, including prereleases or not depending on the argument
func LatestRelease(prerelease bool) (Release, error) {
	resp, err := http.Get("https://api.github.com/repos/syncthing/syncthing/releases?per_page=10")
	if err != nil {
		return Release{}, err
	}
	if resp.StatusCode > 299 {
		return Release{}, fmt.Errorf("API call returned HTTP error: %s", resp.Status)
	}

	var rels []Release
	json.NewDecoder(resp.Body).Decode(&rels)
	resp.Body.Close()

	if len(rels) == 0 {
		return Release{}, ErrVersionUnknown
	}

	if prerelease {
		// We are a beta version. Use the latest.
		return rels[0], nil
	} else {
		// We are a regular release. Only consider non-prerelease versions for upgrade.
		for _, rel := range rels {
			if !rel.Prerelease {
				return rel, nil
			}
		}
		return Release{}, ErrVersionUnknown
	}
}

func readZip(url, dir string) (string, error) {
	if debug {
		l.Debugf("loading %q", url)
	}

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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	archive, err := zip.NewReader(bytes.NewReader(body), resp.ContentLength)
	if err != nil {
		return "", err
	}

	// Iterate through the files in the archive.
	for _, file := range archive.File {

		if debug {
			l.Debugf("considering file %q", file.Name)
		}

		if path.Base(file.Name) == "syncthing.exe" {
			infile, err := file.Open()
			if err != nil {
				return "", err
			}

			outfile, err := ioutil.TempFile(dir, "syncthing")
			if err != nil {
				return "", err
			}

			_, err = io.Copy(outfile, infile)
			if err != nil {
				return "", err
			}

			err = infile.Close()
			if err != nil {
				return "", err
			}

			err = outfile.Close()
			if err != nil {
				os.Remove(outfile.Name())
				return "", err
			}

			os.Chmod(outfile.Name(), file.Mode())
			return outfile.Name(), nil
		}
	}

	return "", fmt.Errorf("No upgrade found")
}
