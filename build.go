// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var (
	versionRe     = regexp.MustCompile(`-[0-9]{1,3}-g[0-9a-f]{5,10}`)
	goarch        string
	goos          string
	noupgrade     bool
	version       string
	goVersion     float64
	race          bool
	debug         = os.Getenv("BUILDDEBUG") != ""
	noBuildGopath bool
	extraTags     string
	installSuffix string
	pkgdir        string
	debugBinary   bool
	timeout       = "120s"
)

type target struct {
	name              string
	debname           string
	debdeps           []string
	debpost           string
	description       string
	buildPkg          string
	binaryName        string
	archiveFiles      []archiveFile
	installationFiles []archiveFile
	tags              []string
}

type archiveFile struct {
	src  string
	dst  string
	perm os.FileMode
}

var targets = map[string]target{
	"all": {
		// Only valid for the "build" and "install" commands as it lacks all
		// the archive creation stuff.
		buildPkg: "github.com/syncthing/syncthing/cmd/...",
		tags:     []string{"purego"},
	},
	"syncthing": {
		// The default target for "build", "install", "tar", "zip", "deb", etc.
		name:        "syncthing",
		debname:     "syncthing",
		debdeps:     []string{"libc6", "procps"},
		debpost:     "script/post-upgrade",
		description: "Open Source Continuous File Synchronization",
		buildPkg:    "github.com/syncthing/syncthing/cmd/syncthing",
		binaryName:  "syncthing", // .exe will be added automatically for Windows builds
		archiveFiles: []archiveFile{
			{src: "{{binary}}", dst: "{{binary}}", perm: 0755},
			{src: "README.md", dst: "README.txt", perm: 0644},
			{src: "LICENSE", dst: "LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "AUTHORS.txt", perm: 0644},
			// All files from etc/ and extra/ added automatically in init().
		},
		installationFiles: []archiveFile{
			{src: "{{binary}}", dst: "deb/usr/bin/{{binary}}", perm: 0755},
			{src: "README.md", dst: "deb/usr/share/doc/syncthing/README.txt", perm: 0644},
			{src: "LICENSE", dst: "deb/usr/share/doc/syncthing/LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "deb/usr/share/doc/syncthing/AUTHORS.txt", perm: 0644},
			{src: "man/syncthing.1", dst: "deb/usr/share/man/man1/syncthing.1", perm: 0644},
			{src: "man/syncthing-config.5", dst: "deb/usr/share/man/man5/syncthing-config.5", perm: 0644},
			{src: "man/syncthing-stignore.5", dst: "deb/usr/share/man/man5/syncthing-stignore.5", perm: 0644},
			{src: "man/syncthing-device-ids.7", dst: "deb/usr/share/man/man7/syncthing-device-ids.7", perm: 0644},
			{src: "man/syncthing-event-api.7", dst: "deb/usr/share/man/man7/syncthing-event-api.7", perm: 0644},
			{src: "man/syncthing-faq.7", dst: "deb/usr/share/man/man7/syncthing-faq.7", perm: 0644},
			{src: "man/syncthing-networking.7", dst: "deb/usr/share/man/man7/syncthing-networking.7", perm: 0644},
			{src: "man/syncthing-rest-api.7", dst: "deb/usr/share/man/man7/syncthing-rest-api.7", perm: 0644},
			{src: "man/syncthing-security.7", dst: "deb/usr/share/man/man7/syncthing-security.7", perm: 0644},
			{src: "man/syncthing-versioning.7", dst: "deb/usr/share/man/man7/syncthing-versioning.7", perm: 0644},
			{src: "etc/linux-systemd/system/syncthing@.service", dst: "deb/lib/systemd/system/syncthing@.service", perm: 0644},
			{src: "etc/linux-systemd/system/syncthing-resume.service", dst: "deb/lib/systemd/system/syncthing-resume.service", perm: 0644},
			{src: "etc/linux-systemd/user/syncthing.service", dst: "deb/usr/lib/systemd/user/syncthing.service", perm: 0644},
			{src: "etc/firewall-ufw/syncthing", dst: "deb/etc/ufw/applications.d/syncthing", perm: 0644},
		},
	},
	"stdiscosrv": {
		name:        "stdiscosrv",
		debname:     "syncthing-discosrv",
		debdeps:     []string{"libc6"},
		description: "Syncthing Discovery Server",
		buildPkg:    "github.com/syncthing/syncthing/cmd/stdiscosrv",
		binaryName:  "stdiscosrv", // .exe will be added automatically for Windows builds
		archiveFiles: []archiveFile{
			{src: "{{binary}}", dst: "{{binary}}", perm: 0755},
			{src: "cmd/stdiscosrv/README.md", dst: "README.txt", perm: 0644},
			{src: "LICENSE", dst: "LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "AUTHORS.txt", perm: 0644},
		},
		installationFiles: []archiveFile{
			{src: "{{binary}}", dst: "deb/usr/bin/{{binary}}", perm: 0755},
			{src: "cmd/stdiscosrv/README.md", dst: "deb/usr/share/doc/syncthing-discosrv/README.txt", perm: 0644},
			{src: "cmd/stdiscosrv/LICENSE", dst: "deb/usr/share/doc/syncthing-discosrv/LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "deb/usr/share/doc/syncthing-discosrv/AUTHORS.txt", perm: 0644},
			{src: "man/stdiscosrv.1", dst: "deb/usr/share/man/man1/stdiscosrv.1", perm: 0644},
		},
		tags: []string{"purego"},
	},
	"strelaysrv": {
		name:        "strelaysrv",
		debname:     "syncthing-relaysrv",
		debdeps:     []string{"libc6"},
		description: "Syncthing Relay Server",
		buildPkg:    "github.com/syncthing/syncthing/cmd/strelaysrv",
		binaryName:  "strelaysrv", // .exe will be added automatically for Windows builds
		archiveFiles: []archiveFile{
			{src: "{{binary}}", dst: "{{binary}}", perm: 0755},
			{src: "cmd/strelaysrv/README.md", dst: "README.txt", perm: 0644},
			{src: "cmd/strelaysrv/LICENSE", dst: "LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "AUTHORS.txt", perm: 0644},
		},
		installationFiles: []archiveFile{
			{src: "{{binary}}", dst: "deb/usr/bin/{{binary}}", perm: 0755},
			{src: "cmd/strelaysrv/README.md", dst: "deb/usr/share/doc/syncthing-relaysrv/README.txt", perm: 0644},
			{src: "cmd/strelaysrv/LICENSE", dst: "deb/usr/share/doc/syncthing-relaysrv/LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "deb/usr/share/doc/syncthing-relaysrv/AUTHORS.txt", perm: 0644},
			{src: "man/strelaysrv.1", dst: "deb/usr/share/man/man1/strelaysrv.1", perm: 0644},
		},
	},
	"strelaypoolsrv": {
		name:        "strelaypoolsrv",
		debname:     "syncthing-relaypoolsrv",
		debdeps:     []string{"libc6"},
		description: "Syncthing Relay Pool Server",
		buildPkg:    "github.com/syncthing/syncthing/cmd/strelaypoolsrv",
		binaryName:  "strelaypoolsrv", // .exe will be added automatically for Windows builds
		archiveFiles: []archiveFile{
			{src: "{{binary}}", dst: "{{binary}}", perm: 0755},
			{src: "cmd/strelaypoolsrv/README.md", dst: "README.txt", perm: 0644},
			{src: "cmd/strelaypoolsrv/LICENSE", dst: "LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "AUTHORS.txt", perm: 0644},
		},
		installationFiles: []archiveFile{
			{src: "{{binary}}", dst: "deb/usr/bin/{{binary}}", perm: 0755},
			{src: "cmd/strelaypoolsrv/README.md", dst: "deb/usr/share/doc/syncthing-relaypoolsrv/README.txt", perm: 0644},
			{src: "cmd/strelaypoolsrv/LICENSE", dst: "deb/usr/share/doc/syncthing-relaypoolsrv/LICENSE.txt", perm: 0644},
			{src: "AUTHORS", dst: "deb/usr/share/doc/syncthing-relaypoolsrv/AUTHORS.txt", perm: 0644},
		},
	},
}

func init() {
	// The "syncthing" target includes a few more files found in the "etc"
	// and "extra" dirs.
	syncthingPkg := targets["syncthing"]
	for _, file := range listFiles("etc") {
		syncthingPkg.archiveFiles = append(syncthingPkg.archiveFiles, archiveFile{src: file, dst: file, perm: 0644})
	}
	for _, file := range listFiles("extra") {
		syncthingPkg.archiveFiles = append(syncthingPkg.archiveFiles, archiveFile{src: file, dst: file, perm: 0644})
	}
	for _, file := range listFiles("extra") {
		syncthingPkg.installationFiles = append(syncthingPkg.installationFiles, archiveFile{src: file, dst: "deb/usr/share/doc/syncthing/" + filepath.Base(file), perm: 0644})
	}
	targets["syncthing"] = syncthingPkg
}

func main() {
	log.SetFlags(0)

	parseFlags()

	if debug {
		t0 := time.Now()
		defer func() {
			log.Println("... build completed in", time.Since(t0))
		}()
	}

	if gopath := gopath(); gopath == "" {
		gopath, err := temporaryBuildDir()
		if err != nil {
			log.Fatal(err)
		}
		if !noBuildGopath {
			lazyRebuildAssets()
			if err := buildGOPATH(gopath); err != nil {
				log.Fatal(err)
			}
		}
		os.Setenv("GOPATH", gopath)
		log.Println("GOPATH is", gopath)
	} else {
		inside := false
		wd, _ := os.Getwd()
		wd, _ = filepath.EvalSymlinks(wd)
		for _, p := range filepath.SplitList(gopath) {
			p, _ = filepath.EvalSymlinks(p)
			if filepath.Join(p, "src/github.com/syncthing/syncthing") == wd {
				inside = true
				break
			}
		}
		if !inside {
			fmt.Println("You seem to have GOPATH set but the Syncthing source not placed correctly within it, which may cause problems.")
		}
	}

	// Set path to $GOPATH/bin:$PATH so that we can for sure find tools we
	// might have installed during "build.go setup".
	os.Setenv("PATH", fmt.Sprintf("%s%cbin%c%s", os.Getenv("GOPATH"), os.PathSeparator, os.PathListSeparator, os.Getenv("PATH")))

	// Invoking build.go with no parameters at all builds everything (incrementally),
	// which is what you want for maximum error checking during development.
	if flag.NArg() == 0 {
		runCommand("install", targets["all"])
	} else {
		// with any command given but not a target, the target is
		// "syncthing". So "go run build.go install" is "go run build.go install
		// syncthing" etc.
		targetName := "syncthing"
		if flag.NArg() > 1 {
			targetName = flag.Arg(1)
		}
		target, ok := targets[targetName]
		if !ok {
			log.Fatalln("Unknown target", target)
		}

		runCommand(flag.Arg(0), target)
	}
}

func runCommand(cmd string, target target) {
	switch cmd {
	case "setup":
		setup()

	case "install":
		var tags []string
		if noupgrade {
			tags = []string{"noupgrade"}
		}
		tags = append(tags, strings.Fields(extraTags)...)
		install(target, tags)
		metalintShort()

	case "build":
		var tags []string
		if noupgrade {
			tags = []string{"noupgrade"}
		}
		tags = append(tags, strings.Fields(extraTags)...)
		build(target, tags)

	case "test":
		test("github.com/syncthing/syncthing/lib/...", "github.com/syncthing/syncthing/cmd/...")

	case "bench":
		bench("github.com/syncthing/syncthing/lib/...", "github.com/syncthing/syncthing/cmd/...")

	case "assets":
		rebuildAssets()

	case "proto":
		proto()

	case "translate":
		translate()

	case "transifex":
		transifex()

	case "tar":
		buildTar(target)

	case "zip":
		buildZip(target)

	case "deb":
		buildDeb(target)

	case "snap":
		buildSnap(target)

	case "clean":
		clean()

	case "vet":
		metalintShort()

	case "lint":
		metalintShort()

	case "metalint":
		metalint()

	case "version":
		fmt.Println(getVersion())

	case "gopath":
		gopath, err := temporaryBuildDir()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(gopath)

	default:
		log.Fatalf("Unknown command %q", cmd)
	}
}

func parseFlags() {
	flag.StringVar(&goarch, "goarch", runtime.GOARCH, "GOARCH")
	flag.StringVar(&goos, "goos", runtime.GOOS, "GOOS")
	flag.BoolVar(&noupgrade, "no-upgrade", noupgrade, "Disable upgrade functionality")
	flag.StringVar(&version, "version", getVersion(), "Set compiled in version string")
	flag.BoolVar(&race, "race", race, "Use race detector")
	flag.BoolVar(&noBuildGopath, "no-build-gopath", noBuildGopath, "Don't build GOPATH, assume it's OK")
	flag.StringVar(&extraTags, "tags", extraTags, "Extra tags, space separated")
	flag.StringVar(&installSuffix, "installsuffix", installSuffix, "Install suffix, optional")
	flag.StringVar(&pkgdir, "pkgdir", "", "Set -pkgdir parameter for `go build`")
	flag.BoolVar(&debugBinary, "debug-binary", debugBinary, "Create unoptimized binary to use with delve, set -gcflags='-N -l' and omit -ldflags")
	flag.Parse()
}

func setup() {
	packages := []string{
		"github.com/alecthomas/gometalinter",
		"github.com/AlekSi/gocov-xml",
		"github.com/axw/gocov/gocov",
		"github.com/FiloSottile/gvt",
		"github.com/golang/lint/golint",
		"github.com/gordonklaus/ineffassign",
		"github.com/mdempsky/unconvert",
		"github.com/mitchellh/go-wordwrap",
		"github.com/opennota/check/cmd/...",
		"github.com/tsenart/deadcode",
		"golang.org/x/net/html",
		"golang.org/x/tools/cmd/cover",
		"honnef.co/go/tools/cmd/gosimple",
		"honnef.co/go/tools/cmd/staticcheck",
		"honnef.co/go/tools/cmd/unused",
		"github.com/josephspurrier/goversioninfo",
	}
	for _, pkg := range packages {
		fmt.Println(pkg)
		runPrint("go", "get", "-u", pkg)
	}

	runPrint("go", "install", "-v", "github.com/syncthing/syncthing/vendor/github.com/gogo/protobuf/protoc-gen-gogofast")
}

func test(pkgs ...string) {
	lazyRebuildAssets()

	useRace := runtime.GOARCH == "amd64"
	switch runtime.GOOS {
	case "darwin", "linux", "freebsd", "windows":
	default:
		useRace = false
	}

	if useRace {
		runPrint("go", append([]string{"test", "-short", "-race", "-timeout", timeout, "-tags", "purego"}, pkgs...)...)
	} else {
		runPrint("go", append([]string{"test", "-short", "-timeout", timeout, "-tags", "purego"}, pkgs...)...)
	}
}

func bench(pkgs ...string) {
	lazyRebuildAssets()
	runPrint("go", append([]string{"test", "-run", "NONE", "-bench", "."}, pkgs...)...)
}

func install(target target, tags []string) {
	lazyRebuildAssets()

	tags = append(target.tags, tags...)

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	os.Setenv("GOBIN", filepath.Join(cwd, "bin"))

	args := []string{"install", "-v"}
	args = appendParameters(args, tags, target)

	os.Setenv("GOOS", goos)
	os.Setenv("GOARCH", goarch)

	// On Windows generate a special file which the Go compiler will
	// automatically use when generating Windows binaries to set things like
	// the file icon, version, etc.
	if goos == "windows" {
		sysoPath, err := shouldBuildSyso(cwd)
		if err != nil {
			log.Printf("Warning: Windows binaries will not have file information encoded: %v", err)
		}
		defer shouldCleanupSyso(sysoPath)
	}

	runPrint("go", args...)
}

func build(target target, tags []string) {
	lazyRebuildAssets()

	tags = append(target.tags, tags...)

	rmr(target.BinaryName())

	args := []string{"build", "-i", "-v"}
	args = appendParameters(args, tags, target)

	os.Setenv("GOOS", goos)
	os.Setenv("GOARCH", goarch)

	// On Windows generate a special file which the Go compiler will
	// automatically use when generating Windows binaries to set things like
	// the file icon, version, etc.
	if goos == "windows" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		sysoPath, err := shouldBuildSyso(cwd)
		if err != nil {
			log.Printf("Warning: Windows binaries will not have file information encoded: %v", err)
		}
		defer shouldCleanupSyso(sysoPath)
	}

	runPrint("go", args...)
}

func appendParameters(args []string, tags []string, target target) []string {
	if pkgdir != "" {
		args = append(args, "-pkgdir", pkgdir)
	}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, " "))
	}
	if installSuffix != "" {
		args = append(args, "-installsuffix", installSuffix)
	}
	if race {
		args = append(args, "-race")
	}

	if !debugBinary {
		// Regular binaries get version tagged and skip some debug symbols
		args = append(args, "-ldflags", ldflags())
	} else {
		// -gcflags to disable optimizations and inlining. Skip -ldflags
		// because `Could not launch program: decoding dwarf section info at
		// offset 0x0: too short` on 'dlv exec ...' see
		// https://github.com/derekparker/delve/issues/79
		args = append(args, "-gcflags", "-N -l")
	}

	return append(args, target.buildPkg)
}

func buildTar(target target) {
	name := archiveName(target)
	filename := name + ".tar.gz"

	var tags []string
	if noupgrade {
		tags = []string{"noupgrade"}
		name += "-noupgrade"
	}

	build(target, tags)

	if goos == "darwin" {
		macosCodesign(target.BinaryName())
	}

	for i := range target.archiveFiles {
		target.archiveFiles[i].src = strings.Replace(target.archiveFiles[i].src, "{{binary}}", target.BinaryName(), 1)
		target.archiveFiles[i].dst = strings.Replace(target.archiveFiles[i].dst, "{{binary}}", target.BinaryName(), 1)
		target.archiveFiles[i].dst = name + "/" + target.archiveFiles[i].dst
	}

	tarGz(filename, target.archiveFiles)
	fmt.Println(filename)
}

func buildZip(target target) {
	name := archiveName(target)
	filename := name + ".zip"

	var tags []string
	if noupgrade {
		tags = []string{"noupgrade"}
		name += "-noupgrade"
	}

	build(target, tags)

	if goos == "windows" {
		windowsCodesign(target.BinaryName())
	}

	for i := range target.archiveFiles {
		target.archiveFiles[i].src = strings.Replace(target.archiveFiles[i].src, "{{binary}}", target.BinaryName(), 1)
		target.archiveFiles[i].dst = strings.Replace(target.archiveFiles[i].dst, "{{binary}}", target.BinaryName(), 1)
		target.archiveFiles[i].dst = name + "/" + target.archiveFiles[i].dst
	}

	zipFile(filename, target.archiveFiles)
	fmt.Println(filename)
}

func buildDeb(target target) {
	os.RemoveAll("deb")

	// "goarch" here is set to whatever the Debian packages expect. We correct
	// it to what we actually know how to build and keep the Debian variant
	// name in "debarch".
	debarch := goarch
	switch goarch {
	case "i386":
		goarch = "386"
	case "armel", "armhf":
		goarch = "arm"
	}

	build(target, []string{"noupgrade"})

	for i := range target.installationFiles {
		target.installationFiles[i].src = strings.Replace(target.installationFiles[i].src, "{{binary}}", target.BinaryName(), 1)
		target.installationFiles[i].dst = strings.Replace(target.installationFiles[i].dst, "{{binary}}", target.BinaryName(), 1)
	}

	for _, af := range target.installationFiles {
		if err := copyFile(af.src, af.dst, af.perm); err != nil {
			log.Fatal(err)
		}
	}

	maintainer := "Syncthing Release Management <release@syncthing.net>"
	debver := version
	if strings.HasPrefix(debver, "v") {
		debver = debver[1:]
		// Debian interprets dashes as separator between main version and
		// Debian package version, and thus thinks 0.14.26-rc.1 is better
		// than just 0.14.26. This rectifies that.
		debver = strings.Replace(debver, "-", "~", -1)
	}
	args := []string{
		"-t", "deb",
		"-s", "dir",
		"-C", "deb",
		"-n", target.debname,
		"-v", debver,
		"-a", debarch,
		"-m", maintainer,
		"--vendor", maintainer,
		"--description", target.description,
		"--url", "https://syncthing.net/",
		"--license", "MPL-2",
	}
	for _, dep := range target.debdeps {
		args = append(args, "-d", dep)
	}
	if target.debpost != "" {
		args = append(args, "--after-upgrade", target.debpost)
	}
	runPrint("fpm", args...)
}

func buildSnap(target target) {
	os.RemoveAll("snap")

	tmpl, err := template.ParseFiles("snapcraft.yaml.template")
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("snapcraft.yaml")
	defer f.Close()
	if err != nil {
		log.Fatal(err)
	}

	snaparch := goarch
	if snaparch == "armhf" {
		goarch = "arm"
	}
	snapver := version
	if strings.HasPrefix(snapver, "v") {
		snapver = snapver[1:]
	}
	snapgrade := "devel"
	if matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+(-rc.\d+)?$`, snapver); matched {
		snapgrade = "stable"
	}
	err = tmpl.Execute(f, map[string]string{
		"Version":      snapver,
		"Architecture": snaparch,
		"Grade":        snapgrade,
	})
	if err != nil {
		log.Fatal(err)
	}
	runPrint("snapcraft", "clean")
	build(target, []string{"noupgrade"})
	runPrint("snapcraft")
}

func shouldBuildSyso(dir string) (string, error) {
	jsonPath := filepath.Join(dir, "versioninfo.json")
	file, err := os.Create(filepath.Join(dir, "versioninfo.json"))
	if err != nil {
		return "", errors.New("failed to create " + jsonPath + ": " + err.Error())
	}

	major, minor, patch, build := semanticVersion()
	fmt.Fprintf(file, `{
    "FixedFileInfo": {
        "FileVersion": {
            "Major": %s,
            "Minor": %s,
            "Patch": %s,
            "Build": %s
        }
    },
    "StringFileInfo": {
        "FileDescription": "Open Source Continuous File Synchronization",
        "LegalCopyright": "The Syncthing Authors",
        "ProductVersion": "%s",
        "ProductName": "Syncthing"
    },
    "IconPath": "assets/logo.ico"
}`, major, minor, patch, build, getVersion())
	file.Close()
	defer func() {
		if err := os.Remove(jsonPath); err != nil {
			log.Printf("Warning: unable to remove generated %s: %v. Please remove it manually.", jsonPath, err)
		}
	}()

	sysoPath := filepath.Join(dir, "cmd", "syncthing", "resource.syso")

	if _, err := runError("goversioninfo", "-o", sysoPath); err != nil {
		return "", errors.New("failed to create " + sysoPath + ": " + err.Error())
	}

	return sysoPath, nil
}

func shouldCleanupSyso(sysoFilePath string) {
	if sysoFilePath == "" {
		return
	}
	if err := os.Remove(sysoFilePath); err != nil {
		log.Printf("Warning: unable to remove generated %s: %v. Please remove it manually.", sysoFilePath, err)
	}
}

// copyFile copies a file from src to dst, ensuring the containing directory
// exists. The permission bits are copied as well. If dst already exists and
// the contents are identical to src the modification time is not updated.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	out, err := ioutil.ReadFile(dst)
	if err != nil {
		// The destination probably doesn't exist, we should create
		// it.
		goto copy
	}

	if bytes.Equal(in, out) {
		// The permission bits may have changed without the contents
		// changing so we always mirror them.
		os.Chmod(dst, perm)
		return nil
	}

copy:
	os.MkdirAll(filepath.Dir(dst), 0777)
	if err := ioutil.WriteFile(dst, in, perm); err != nil {
		return err
	}

	return nil
}

func listFiles(dir string) []string {
	var res []string
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			res = append(res, path)
		}
		return nil
	})
	return res
}

func rebuildAssets() {
	os.Setenv("SOURCE_DATE_EPOCH", fmt.Sprint(buildStamp()))
	runPrint("go", "generate", "github.com/syncthing/syncthing/lib/auto", "github.com/syncthing/syncthing/cmd/strelaypoolsrv/auto")
}

func lazyRebuildAssets() {
	if shouldRebuildAssets("lib/auto/gui.files.go", "gui") || shouldRebuildAssets("cmd/strelaypoolsrv/auto/gui.files.go", "cmd/strelaypoolsrv/auto/gui") {
		rebuildAssets()
	}
}

func shouldRebuildAssets(target, srcdir string) bool {
	info, err := os.Stat(target)
	if err != nil {
		// If the file doesn't exist, we must rebuild it
		return true
	}

	// Check if any of the files in gui/ are newer than the asset file. If
	// so we should rebuild it.
	currentBuild := info.ModTime()
	assetsAreNewer := false
	stop := errors.New("no need to iterate further")
	filepath.Walk(srcdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.ModTime().After(currentBuild) {
			assetsAreNewer = true
			return stop
		}
		return nil
	})

	return assetsAreNewer
}

func proto() {
	runPrint("go", "generate", "github.com/syncthing/syncthing/lib/...", "github.com/syncthing/syncthing/cmd/stdiscosrv")
}

func translate() {
	os.Chdir("gui/default/assets/lang")
	runPipe("lang-en-new.json", "go", "run", "../../../../script/translate.go", "lang-en.json", "../../../")
	os.Remove("lang-en.json")
	err := os.Rename("lang-en-new.json", "lang-en.json")
	if err != nil {
		log.Fatal(err)
	}
	os.Chdir("../../../..")
}

func transifex() {
	os.Chdir("gui/default/assets/lang")
	runPrint("go", "run", "../../../../script/transifexdl.go")
}

func clean() {
	rmr("bin")
	rmr(filepath.Join(os.Getenv("GOPATH"), fmt.Sprintf("pkg/%s_%s/github.com/syncthing", goos, goarch)))
}

func ldflags() string {
	sep := '='
	if goVersion > 0 && goVersion < 1.5 {
		sep = ' '
	}

	b := new(bytes.Buffer)
	b.WriteString("-w")
	fmt.Fprintf(b, " -X main.Version%c%s", sep, version)
	fmt.Fprintf(b, " -X main.BuildStamp%c%d", sep, buildStamp())
	fmt.Fprintf(b, " -X main.BuildUser%c%s", sep, buildUser())
	fmt.Fprintf(b, " -X main.BuildHost%c%s", sep, buildHost())
	return b.String()
}

func rmr(paths ...string) {
	for _, path := range paths {
		if debug {
			log.Println("rm -r", path)
		}
		os.RemoveAll(path)
	}
}

func getReleaseVersion() (string, error) {
	fd, err := os.Open("RELEASE")
	if err != nil {
		return "", err
	}
	defer fd.Close()

	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(bs)), nil
}

func getGitVersion() (string, error) {
	v, err := runError("git", "describe", "--always", "--dirty")
	if err != nil {
		return "", err
	}
	v = versionRe.ReplaceAllFunc(v, func(s []byte) []byte {
		s[0] = '+'
		return s
	})
	return string(v), nil
}

func getVersion() string {
	// First try for a RELEASE file,
	if ver, err := getReleaseVersion(); err == nil {
		return ver
	}
	// ... then see if we have a Git tag.
	if ver, err := getGitVersion(); err == nil {
		if strings.Contains(ver, "-") {
			// The version already contains a hash and stuff. See if we can
			// find a current branch name to tack onto it as well.
			return ver + getBranchSuffix()
		}
		return ver
	}
	// This seems to be a dev build.
	return "unknown-dev"
}

func semanticVersion() (major, minor, patch, build string) {
	r := regexp.MustCompile(`v(?P<Major>\d+)\.(?P<Minor>\d+).(?P<Patch>\d+).*\+(?P<CommitsAhead>\d+)`)
	matches := r.FindStringSubmatch(getVersion())
	if len(matches) != 5 {
		return "0", "0", "0", "0"
	}
	return matches[1], matches[2], matches[3], matches[4]
}

func getBranchSuffix() string {
	bs, err := runError("git", "branch", "-a", "--contains")
	if err != nil {
		return ""
	}

	branches := strings.Split(string(bs), "\n")
	if len(branches) == 0 {
		return ""
	}

	branch := ""
	for i, candidate := range branches {
		if strings.HasPrefix(candidate, "*") {
			// This is the current branch. Select it!
			branch = strings.TrimLeft(candidate, " \t*")
			break
		} else if i == 0 {
			// Otherwise the first branch in the list will do.
			branch = strings.TrimSpace(branch)
		}
	}

	if branch == "" {
		return ""
	}

	// The branch name may be on the form "remotes/origin/foo" from which we
	// just want "foo".
	parts := strings.Split(branch, "/")
	if len(parts) == 0 || len(parts[len(parts)-1]) == 0 {
		return ""
	}

	branch = parts[len(parts)-1]
	switch branch {
	case "master", "release":
		// these are not special
		return ""
	}

	validBranchRe := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !validBranchRe.MatchString(branch) {
		// There's some odd stuff in the branch name. Better skip it.
		return ""
	}

	return "-" + branch
}

func buildStamp() int64 {
	// If SOURCE_DATE_EPOCH is set, use that.
	if s, _ := strconv.ParseInt(os.Getenv("SOURCE_DATE_EPOCH"), 10, 64); s > 0 {
		return s
	}

	// Try to get the timestamp of the latest commit.
	bs, err := runError("git", "show", "-s", "--format=%ct")
	if err != nil {
		// Fall back to "now".
		return time.Now().Unix()
	}

	s, _ := strconv.ParseInt(string(bs), 10, 64)
	return s
}

func buildUser() string {
	if v := os.Getenv("BUILD_USER"); v != "" {
		return v
	}

	u, err := user.Current()
	if err != nil {
		return "unknown-user"
	}
	return strings.Replace(u.Username, " ", "-", -1)
}

func buildHost() string {
	if v := os.Getenv("BUILD_HOST"); v != "" {
		return v
	}

	h, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return h
}

func buildArch() string {
	os := goos
	if os == "darwin" {
		os = "macos"
	}
	return fmt.Sprintf("%s-%s", os, goarch)
}

func archiveName(target target) string {
	return fmt.Sprintf("%s-%s-%s", target.name, buildArch(), version)
}

func runError(cmd string, args ...string) ([]byte, error) {
	if debug {
		t0 := time.Now()
		log.Println("runError:", cmd, strings.Join(args, " "))
		defer func() {
			log.Println("... in", time.Since(t0))
		}()
	}
	ecmd := exec.Command(cmd, args...)
	bs, err := ecmd.CombinedOutput()
	return bytes.TrimSpace(bs), err
}

func runPrint(cmd string, args ...string) {
	if debug {
		t0 := time.Now()
		log.Println("runPrint:", cmd, strings.Join(args, " "))
		defer func() {
			log.Println("... in", time.Since(t0))
		}()
	}
	ecmd := exec.Command(cmd, args...)
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	err := ecmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func runPipe(file, cmd string, args ...string) {
	if debug {
		t0 := time.Now()
		log.Println("runPipe:", cmd, strings.Join(args, " "))
		defer func() {
			log.Println("... in", time.Since(t0))
		}()
	}
	fd, err := os.Create(file)
	if err != nil {
		log.Fatal(err)
	}
	ecmd := exec.Command(cmd, args...)
	ecmd.Stdout = fd
	ecmd.Stderr = os.Stderr
	err = ecmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()
}

func tarGz(out string, files []archiveFile) {
	fd, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}

	gw, err := gzip.NewWriterLevel(fd, gzip.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	tw := tar.NewWriter(gw)

	for _, f := range files {
		sf, err := os.Open(f.src)
		if err != nil {
			log.Fatal(err)
		}

		info, err := sf.Stat()
		if err != nil {
			log.Fatal(err)
		}
		h := &tar.Header{
			Name:    f.dst,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}

		err = tw.WriteHeader(h)
		if err != nil {
			log.Fatal(err)
		}
		_, err = io.Copy(tw, sf)
		if err != nil {
			log.Fatal(err)
		}
		sf.Close()
	}

	err = tw.Close()
	if err != nil {
		log.Fatal(err)
	}
	err = gw.Close()
	if err != nil {
		log.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func zipFile(out string, files []archiveFile) {
	fd, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}

	zw := zip.NewWriter(fd)

	var fw *flate.Writer

	// Register the deflator.
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		var err error
		if fw == nil {
			// Creating a flate compressor for every file is
			// expensive, create one and reuse it.
			fw, err = flate.NewWriter(out, flate.BestCompression)
		} else {
			fw.Reset(out)
		}
		return fw, err
	})

	for _, f := range files {
		sf, err := os.Open(f.src)
		if err != nil {
			log.Fatal(err)
		}

		info, err := sf.Stat()
		if err != nil {
			log.Fatal(err)
		}

		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			log.Fatal(err)
		}
		fh.Name = filepath.ToSlash(f.dst)
		fh.Method = zip.Deflate

		if strings.HasSuffix(f.dst, ".txt") {
			// Text file. Read it and convert line endings.
			bs, err := ioutil.ReadAll(sf)
			if err != nil {
				log.Fatal(err)
			}
			bs = bytes.Replace(bs, []byte{'\n'}, []byte{'\n', '\r'}, -1)
			fh.UncompressedSize = uint32(len(bs))
			fh.UncompressedSize64 = uint64(len(bs))

			of, err := zw.CreateHeader(fh)
			if err != nil {
				log.Fatal(err)
			}
			of.Write(bs)
		} else {
			// Binary file. Copy verbatim.
			of, err := zw.CreateHeader(fh)
			if err != nil {
				log.Fatal(err)
			}
			_, err = io.Copy(of, sf)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	err = zw.Close()
	if err != nil {
		log.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func macosCodesign(file string) {
	if pass := os.Getenv("CODESIGN_KEYCHAIN_PASS"); pass != "" {
		bs, err := runError("security", "unlock-keychain", "-p", pass)
		if err != nil {
			log.Println("Codesign: unlocking keychain failed:", string(bs))
			return
		}
	}

	if id := os.Getenv("CODESIGN_IDENTITY"); id != "" {
		bs, err := runError("codesign", "-s", id, file)
		if err != nil {
			log.Println("Codesign: signing failed:", string(bs))
			return
		}
		log.Println("Codesign: successfully signed", file)
	}
}

func windowsCodesign(file string) {
	st := "signtool.exe"

	if path := os.Getenv("CODESIGN_SIGNTOOL"); path != "" {
		st = path
	}

	for i, algo := range []string{"sha1", "sha256"} {
		args := []string{"sign", "/fd", algo}
		if f := os.Getenv("CODESIGN_CERTIFICATE_FILE"); f != "" {
			args = append(args, "/f", f)
		}
		if p := os.Getenv("CODESIGN_CERTIFICATE_PASSWORD"); p != "" {
			args = append(args, "/p", p)
		}
		if tr := os.Getenv("CODESIGN_TIMESTAMP_SERVER"); tr != "" {
			switch algo {
			case "sha256":
				args = append(args, "/tr", tr, "/td", algo)
			default:
				args = append(args, "/t", tr)
			}
		}
		if i > 0 {
			args = append(args, "/as")
		}
		args = append(args, file)

		bs, err := runError(st, args...)
		if err != nil {
			log.Println("Codesign: signing failed:", string(bs))
			return
		}
		log.Println("Codesign: successfully signed", file, "using", algo)
	}
}

func metalint() {
	lazyRebuildAssets()
	runPrint("go", "test", "-run", "Metalint", "./meta")
}

func metalintShort() {
	lazyRebuildAssets()
	runPrint("go", "test", "-short", "-run", "Metalint", "./meta")
}

func temporaryBuildDir() (string, error) {
	// The base of our temp dir is "syncthing-xxxxxxxx" where the x:es
	// are eight bytes from the sha256 of our working directory. We do
	// this because we want a name in the global temp dir that doesn't
	// conflict with someone else building syncthing on the same
	// machine, yet is persistent between runs from the same source
	// directory.
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(wd))
	base := fmt.Sprintf("syncthing-%x", hash[:4])

	// The temp dir is taken from $STTMPDIR if set, otherwise the system
	// default (potentially infrluenced by $TMPDIR on unixes).
	var tmpDir string
	if t := os.Getenv("STTMPDIR"); t != "" {
		tmpDir = t
	} else {
		tmpDir = os.TempDir()
	}

	return filepath.Join(tmpDir, base), nil
}

func buildGOPATH(gopath string) error {
	pkg := filepath.Join(gopath, "src/github.com/syncthing/syncthing")
	dirs := []string{"cmd", "lib", "meta", "script", "test", "vendor"}

	if debug {
		t0 := time.Now()
		log.Println("build temporary GOPATH in", gopath)
		defer func() {
			log.Println("... in", time.Since(t0))
		}()
	}

	// Walk the sources and copy the files into the temporary GOPATH.
	// Remember which files are supposed to be present so we can clean
	// out everything else in the next step. The copyFile() step will
	// only actually copy the file if it doesn't exist or the contents
	// differ.

	exists := map[string]struct{}{}
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			dst := filepath.Join(pkg, path)
			exists[dst] = struct{}{}

			if err := copyFile(path, dst, info.Mode()); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	// Walk the temporary GOPATH and remove any files that we wouldn't
	// have copied there in the previous step.

	filepath.Walk(pkg, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if _, ok := exists[path]; !ok {
			os.Remove(path)
		}
		return nil
	})

	return nil
}

func gopath() string {
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		// The env var is set, use that.
		return gopath
	}

	// Ask Go what it thinks.
	bs, err := runError("go", "env", "GOPATH")
	if err != nil {
		return ""
	}

	// We got something. Check if we are in fact available in that location.
	gopath := string(bs)
	if _, err := os.Stat(filepath.Join(gopath, "src/github.com/syncthing/syncthing/build.go")); err == nil {
		// That seems to be the gopath.
		return gopath
	}

	// The gopath is not valid.
	return ""
}

func (t target) BinaryName() string {
	if goos == "windows" {
		return t.binaryName + ".exe"
	}
	return t.binaryName
}
