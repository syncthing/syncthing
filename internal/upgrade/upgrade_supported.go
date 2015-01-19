// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build !noupgrade

package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/md5"
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
)

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
	}

	// We are a regular release. Only consider non-prerelease versions for upgrade.
	for _, rel := range rels {
		if !rel.Prerelease {
			return rel, nil
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
	resp, err := http.DefaultClient.Do(req)
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

	var tempName, actualMD5, expectedMD5 string

	// Iterate through the files in the archive.
fileLoop:
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

		switch shortName {
		case "syncthing":
			if debug {
				l.Debugln("writing and hashing binary")
			}
			tempName, actualMD5, err = writeBinary(dir, tr)
			if err != nil {
				return "", err
			}

			if expectedMD5 != "" {
				// We're done
				break fileLoop
			}

		case "syncthing.md5":
			bs, err := ioutil.ReadAll(tr)
			if err != nil {
				return "", err
			}

			expectedMD5 = strings.TrimSpace(string(bs))
			if debug {
				l.Debugln("expected md5 is", actualMD5)
			}

			if actualMD5 != "" {
				// We're done
				break fileLoop
			}
		}
	}

	if tempName != "" {
		// We found and saved something to disk.
		if expectedMD5 == "" || actualMD5 == expectedMD5 {
			return tempName, nil
		}
		os.Remove(tempName)
		// There was an md5 file included in the archive, and it doesn't
		// match what we just wrote to disk.
		return "", fmt.Errorf("incorrect MD5 checksum")
	}
	return "", fmt.Errorf("no upgrade found")
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

	var tempName, actualMD5, expectedMD5 string

	// Iterate through the files in the archive.
fileLoop:
	for _, file := range archive.File {
		shortName := path.Base(file.Name)

		if debug {
			l.Debugf("considering file %q", shortName)
		}

		switch shortName {
		case "syncthing.exe":
			if debug {
				l.Debugln("writing and hashing binary")
			}

			inFile, err := file.Open()
			if err != nil {
				return "", err
			}
			tempName, actualMD5, err = writeBinary(dir, inFile)
			if err != nil {
				return "", err
			}

			if expectedMD5 != "" {
				// We're done
				break fileLoop
			}

		case "syncthing.exe.md5":
			inFile, err := file.Open()
			if err != nil {
				return "", err
			}
			bs, err := ioutil.ReadAll(inFile)
			if err != nil {
				return "", err
			}

			expectedMD5 = strings.TrimSpace(string(bs))
			if debug {
				l.Debugln("expected md5 is", actualMD5)
			}

			if actualMD5 != "" {
				// We're done
				break fileLoop
			}
		}
	}

	if tempName != "" {
		// We found and saved something to disk.
		if expectedMD5 == "" || actualMD5 == expectedMD5 {
			return tempName, nil
		}
		os.Remove(tempName)
		// There was an md5 file included in the archive, and it doesn't
		// match what we just wrote to disk.
		return "", fmt.Errorf("incorrect MD5 checksum")
	}
	return "", fmt.Errorf("No upgrade found")
}

func writeBinary(dir string, inFile io.Reader) (filename, md5sum string, err error) {
	outFile, err := ioutil.TempFile(dir, "syncthing")
	if err != nil {
		return "", "", err
	}

	// Write the binary both a temporary file and to the MD5 hasher.

	h := md5.New()
	mw := io.MultiWriter(h, outFile)

	_, err = io.Copy(mw, inFile)
	if err != nil {
		os.Remove(outFile.Name())
		return "", "", err
	}

	err = outFile.Close()
	if err != nil {
		os.Remove(outFile.Name())
		return "", "", err
	}

	err = os.Chmod(outFile.Name(), os.FileMode(0755))
	if err != nil {
		os.Remove(outFile.Name())
		return "", "", err
	}

	actualMD5 := fmt.Sprintf("%x", h.Sum(nil))
	if debug {
		l.Debugln("actual md5 is", actualMD5)
	}

	return outFile.Name(), actualMD5, nil
}
