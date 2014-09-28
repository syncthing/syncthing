// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build !solaris,!windows,!noupgrade

package upgrade

import (
	"archive/tar"
	"compress/gzip"
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
	osName := runtime.GOOS
	if osName == "darwin" {
		// We call the darwin release bundles macosx because that makes more
		// sense for people downloading them
		osName = "macosx"
	}
	expectedRelease := fmt.Sprintf("syncthing-%s-%s%s-%s.", osName, runtime.GOARCH, archExtra, rel.Tag)
	if debug {
		l.Debugf("expected release asset %q", expectedRelease)
	}
	for _, asset := range rel.Assets {
		if debug {
			l.Debugln("considering release", asset)
		}
		if strings.HasPrefix(asset.Name, expectedRelease) {
			if strings.HasSuffix(asset.Name, ".tar.gz") {
				fname, err := readTarGZ(asset.URL, filepath.Dir(path))
				if err != nil {
					return err
				}

				old := path + ".old"
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

func readTarGZ(url string, dir string) (string, error) {
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
		if debug {
			l.Debugf("considering file %q", hdr.Name)
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
