// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build windows,!noupgrade

// Windows upgrade happens following these steps:
//   1. The old binary will download the new binary as syncthing.exe.new
//   2. The old binary will start syncthing.exe.new and exit
//   3. The new binary will replace syncthing.exe with a copy of itself
//   4. The new binary will start syncthing.exe and exit
//   5. The new syncthing.exe binary will remove syncthing.exe.new and continue
//      the execution
//
// There are multiple problems with this approach:
//  1. As we start and restart the binaries, we need to force them to execute
//     a specific part of the code which takes care of performing the above
//     upgrade steps
//  2. Since we will be running different binaries at different times,
//     we will loose the initial execution state of the old binary.
//     This means that we will find it hard to know if the upgrade was started
//     via -upgrade flag (where it is supposed to upgrade and exit), or if it
//     was started via Web UI (where it is supposed to upgrade and continue
//     execution)
//  3. As we need to swap the binaries around, we need to synchronize between
//     the state of the processes, such that we are sure the old syncthing.exe
//     has exited before we try and remove/replace it.
//  4. The ideal way to work out if the parent process has exited, is by
//     is by checking if the parent process is still alive.
//     Sadly os.Getppid does not work on Windows.
//
// Solutions to the problems:
//  1. We will always divert the execution path to upgrade.LatestRelease which
//     will then recognize the step of execution we are in, and act accordingly
//     To make this happen, we will append -upgrade-check flag to the binaries
//     that we run
//  2. Currently there are only two possible paths how the upgrade is started:
//     * Via -upgrade command line switch
//     * Via web UI
//     We can easily detect that the upgrade was started by the command line
//     switch, and we will set a special flag in the environment to indicate this
//     to the future processes. Once the upgrade process is complete, if the
//     special flag is set, the process will exit instead of continuing
//     normal execution.
//  3. Since Windows does not support signals, we have to resort to using
//     os.FindProcess and Wait(), which is not exactly reliable
//  4. To replace os.Getppid functionality, each parent will pass its own pid
//     to the children as an environment variable.

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
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"bitbucket.org/kardianos/osext"
)

// Upgrade to the given release, saving the previous binary with a ".old" extension.
func UpgradeTo(rel Release) error {
	path, err := osext.Executable()
	if err != nil {
		return err
	}

	expectedRelease := fmt.Sprintf("syncthing-%s-%s-%s.", runtime.GOOS, runtime.GOARCH, rel.Tag)
	for _, asset := range rel.Assets {
		if strings.HasPrefix(asset.Name, expectedRelease) {
			if strings.HasSuffix(asset.Name, ".zip") {
				fname, err := readZip(asset.URL, filepath.Dir(path))
				if err != nil {
					return err
				}

				err = os.Remove(path + ".old")
				if err != nil && !os.IsNotExist(err) {
					return err
				}

				err = copyFile(path, path+".old")
				if err != nil {
					return err
				}

				err = os.Remove(path + ".new")
				if err != nil && !os.IsNotExist(err) {
					return err
				}

				err = os.Rename(fname, path+".new")
				if err != nil {
					return err
				}

				found := false
				// Check if this upgrade was started via command line
				// and add a enviroment variable which will tell future
				// executions of the binary to exit after the upgrade is done.
				for _, arg := range os.Args {
					switch arg {
					case "-upgrade":
						os.Setenv("STEXITPOSTUPGRADE", "1")
						break
					case "-upgrade-check":
						found = true
						break
					}
				}

				// Since we need LatestRelease to be called as we swap the
				// binaries around, make sure the flag is set for future
				// executions of the binary to run that part of the code.
				if !found {
					os.Args = append(os.Args, "-upgrade-check")
				}

				os.Setenv("STPPID", strconv.Itoa(os.Getpid()))
				return runAndExit(path + ".new")
			}
		}
	}

	return ErrVersionUnknown
}

// Returns the latest release, including prereleases or not depending on the argument
func LatestRelease(prerelease bool) (Release, error) {
	if os.Getenv("SMF_FMRI") != "" || os.Getenv("STNORESTART") != "" {
		return Release{}, fmt.Errorf("Cannot perform upgrades under service manager mode")
	}

	err := checkMidUpgrade()
	if err != nil {
		return Release{}, err
	}

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

func readZip(url string, dir string) (string, error) {
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

func checkMidUpgrade() error {
	path, err := osext.Executable()
	if err != nil {
		return err
	}

	if strings.HasSuffix(path, ".new") {
		err = waitForParentExit()
		if err != nil {
			return err
		}

		newpath := path[:len(path)-4]

		err = os.Remove(newpath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		err = copyFile(path, newpath)
		if err != nil {
			return err
		}

		// os.Getppid does not work on Windows, hence pass the PPID as env var
		os.Setenv("STPPID", strconv.Itoa(os.Getpid()))
		return runAndExit(newpath)

	} else {
		_, err = os.Stat(path + ".new")
		if err == nil {
			err = waitForParentExit()
			if err != nil {
				return err
			}

			err := os.Remove(path + ".new")
			if err != nil {
				return err
			}

			// If the initial upgrade was started via command line, we now
			// need to exit, as we did what we were asked to do.
			if os.Getenv("STEXITPOSTUPGRADE") != "" {
				os.Exit(0)
			}

			// Otherwise, the execution should continue
			// Clean up the argument, enviroment, and restart back into
			// the normal mode.
			for i, arg := range os.Args {
				if arg == "-upgrade-check" {
					last := len(os.Args) - 1
					os.Args[i] = os.Args[last]
					os.Args = os.Args[:last]
					break
				}
			}
			os.Setenv("STPPID", "")
			return runAndExit(path)
		}
	}
	return nil
}

func waitForParentExit() error {
	if os.Getenv("STPPID") == "" {
		return nil
	}

	// os.Getppid does not work on Windows, hence get the PPID from an env var
	ppid64, err := strconv.ParseInt(os.Getenv("STPPID"), 10, 64)
	if err != nil {
		return err
	}

	proc, err := os.FindProcess(int(ppid64))
	if err != nil {
		// The process has most likely already exited (fingers crossed)
		return nil
	}

	_, err = proc.Wait()
	return err
}

func runAndExit(path string) error {
	os.Args[0] = path

	pgm, err := exec.LookPath(path)
	if err != nil {
		return err
	}

	proc, err := os.StartProcess(pgm, os.Args, &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return err
	}

	proc.Release()

	os.Exit(0)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
