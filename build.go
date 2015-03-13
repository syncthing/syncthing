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

// +build ignore

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/md5"
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
	"time"
)

var (
	versionRe = regexp.MustCompile(`-[0-9]{1,3}-g[0-9a-f]{5,10}`)
	goarch    string
	goos      string
	noupgrade bool
	version   string
	race      bool
)

const minGoVersion = 1.3

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	if os.Getenv("GOPATH") == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		gopath := filepath.Clean(filepath.Join(cwd, "../../../../"))
		log.Println("GOPATH is", gopath)
		os.Setenv("GOPATH", gopath)
	}
	os.Setenv("PATH", fmt.Sprintf("%s%cbin%c%s", os.Getenv("GOPATH"), os.PathSeparator, os.PathListSeparator, os.Getenv("PATH")))

	flag.StringVar(&goarch, "goarch", runtime.GOARCH, "GOARCH")
	flag.StringVar(&goos, "goos", runtime.GOOS, "GOOS")
	flag.BoolVar(&noupgrade, "no-upgrade", noupgrade, "Disable upgrade functionality")
	flag.StringVar(&version, "version", getVersion(), "Set compiled in version string")
	flag.BoolVar(&race, "race", race, "Use race detector")
	flag.Parse()

	switch goarch {
	case "386", "amd64", "arm":
		break
	default:
		log.Printf("Unknown goarch %q; proceed with caution!", goarch)
	}

	checkRequiredGoVersion()

	if flag.NArg() == 0 {
		var tags []string
		if noupgrade {
			tags = []string{"noupgrade"}
		}
		install("./cmd/...", tags)
		return
	}

	for _, cmd := range flag.Args() {
		switch cmd {
		case "setup":
			setup()

		case "install":
			pkg := "./cmd/..."
			var tags []string
			if noupgrade {
				tags = []string{"noupgrade"}
			}
			install(pkg, tags)

		case "build":
			pkg := "./cmd/syncthing"
			var tags []string
			if noupgrade {
				tags = []string{"noupgrade"}
			}
			build(pkg, tags)

		case "test":
			pkg := "./..."
			test(pkg)

		case "assets":
			assets()

		case "xdr":
			xdr()

		case "translate":
			translate()

		case "transifex":
			transifex()

		case "deps":
			deps()

		case "tar":
			buildTar()

		case "zip":
			buildZip()

		case "clean":
			clean()

		default:
			log.Fatalf("Unknown command %q", cmd)
		}
	}
}

func checkRequiredGoVersion() {
	ver := run("go", "version")
	re := regexp.MustCompile(`go version go(\d+\.\d+)`)
	if m := re.FindSubmatch(ver); len(m) == 2 {
		vs := string(m[1])
		// This is a standard go build. Verify that it's new enough.
		f, err := strconv.ParseFloat(vs, 64)
		if err != nil {
			log.Printf("*** Couldn't parse Go version out of %q.\n*** This isn't known to work, proceed on your own risk.", vs)
			return
		}
		if f < minGoVersion {
			log.Fatalf("*** Go version %.01f is less than required %.01f.\n*** This is known not to work, not proceeding.", f, minGoVersion)
		}
	} else {
		log.Printf("*** Unknown Go version %q.\n*** This isn't known to work, proceed on your own risk.", ver)
	}
}

func setup() {
	runPrint("go", "get", "-v", "golang.org/x/tools/cmd/cover")
	runPrint("go", "get", "-v", "golang.org/x/tools/cmd/vet")
	runPrint("go", "get", "-v", "golang.org/x/net/html")
	runPrint("go", "get", "-v", "github.com/tools/godep")
}

func test(pkg string) {
	setBuildEnv()
	runPrint("go", "test", "-short", "-timeout", "60s", pkg)
}

func install(pkg string, tags []string) {
	os.Setenv("GOBIN", "./bin")
	args := []string{"install", "-v", "-ldflags", ldflags()}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	if race {
		args = append(args, "-race")
	}
	args = append(args, pkg)
	setBuildEnv()
	runPrint("go", args...)
}

func build(pkg string, tags []string) {
	binary := "syncthing"
	if goos == "windows" {
		binary += ".exe"
	}

	rmr(binary, binary+".md5")
	args := []string{"build", "-ldflags", ldflags()}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	if race {
		args = append(args, "-race")
	}
	args = append(args, pkg)
	setBuildEnv()
	runPrint("go", args...)

	// Create an md5 checksum of the binary, to be included in the archive for
	// automatic upgrades.
	err := md5File(binary)
	if err != nil {
		log.Fatal(err)
	}
}

func buildTar() {
	name := archiveName()
	var tags []string
	if noupgrade {
		tags = []string{"noupgrade"}
		name += "-noupgrade"
	}
	build("./cmd/syncthing", tags)
	filename := name + ".tar.gz"
	files := []archiveFile{
		{src: "README.md", dst: name + "/README.txt"},
		{src: "LICENSE", dst: name + "/LICENSE.txt"},
		{src: "AUTHORS", dst: name + "/AUTHORS.txt"},
		{src: "syncthing", dst: name + "/syncthing"},
		{src: "syncthing.md5", dst: name + "/syncthing.md5"},
	}

	for _, file := range listFiles("etc") {
		files = append(files, archiveFile{src: file, dst: name + "/" + file})
	}
	for _, file := range listFiles("extra") {
		files = append(files, archiveFile{src: file, dst: name + "/" + filepath.Base(file)})
	}

	tarGz(filename, files)
	log.Println(filename)
}

func buildZip() {
	name := archiveName()
	var tags []string
	if noupgrade {
		tags = []string{"noupgrade"}
		name += "-noupgrade"
	}
	build("./cmd/syncthing", tags)
	filename := name + ".zip"
	files := []archiveFile{
		{src: "README.md", dst: name + "/README.txt"},
		{src: "LICENSE", dst: name + "/LICENSE.txt"},
		{src: "AUTHORS", dst: name + "/AUTHORS.txt"},
		{src: "syncthing.exe", dst: name + "/syncthing.exe"},
		{src: "syncthing.exe.md5", dst: name + "/syncthing.exe.md5"},
	}

	for _, file := range listFiles("extra") {
		files = append(files, archiveFile{src: file, dst: name + "/" + filepath.Base(file)})
	}

	zipFile(filename, files)
	log.Println(filename)
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

func setBuildEnv() {
	os.Setenv("GOOS", goos)
	os.Setenv("GOARCH", goarch)
	wd, err := os.Getwd()
	if err != nil {
		log.Println("Warning: can't determine current dir:", err)
		log.Println("Build might not work as expected")
	}
	os.Setenv("GOPATH", fmt.Sprintf("%s%c%s", filepath.Join(wd, "Godeps", "_workspace"), os.PathListSeparator, os.Getenv("GOPATH")))
	log.Println("GOPATH=" + os.Getenv("GOPATH"))
}

func assets() {
	setBuildEnv()
	runPipe("internal/auto/gui.files.go", "go", "run", "cmd/genassets/main.go", "gui")
}

func xdr() {
	runPrint("go", "generate", "./internal/discover", "./internal/db")
}

func translate() {
	os.Chdir("gui/assets/lang")
	runPipe("lang-en-new.json", "go", "run", "../../../cmd/translate/main.go", "lang-en.json", "../../index.html")
	os.Remove("lang-en.json")
	err := os.Rename("lang-en-new.json", "lang-en.json")
	if err != nil {
		log.Fatal(err)
	}
	os.Chdir("../../..")
}

func transifex() {
	os.Chdir("gui/assets/lang")
	runPrint("go", "run", "../../../cmd/transifexdl/main.go")
	os.Chdir("../../..")
	assets()
}

func deps() {
	rmr("Godeps")
	runPrint("godep", "save", "./cmd/...")
}

func clean() {
	rmr("bin", "Godeps/_workspace/pkg", "Godeps/_workspace/bin")
	rmr(filepath.Join(os.Getenv("GOPATH"), fmt.Sprintf("pkg/%s_%s/github.com/syncthing", goos, goarch)))
}

func ldflags() string {
	var b bytes.Buffer
	b.WriteString("-w")
	b.WriteString(fmt.Sprintf(" -X main.Version %s", version))
	b.WriteString(fmt.Sprintf(" -X main.BuildStamp %d", buildStamp()))
	b.WriteString(fmt.Sprintf(" -X main.BuildUser %s", buildUser()))
	b.WriteString(fmt.Sprintf(" -X main.BuildHost %s", buildHost()))
	b.WriteString(fmt.Sprintf(" -X main.BuildEnv %s", buildEnvironment()))
	return b.String()
}

func rmr(paths ...string) {
	for _, path := range paths {
		log.Println("rm -r", path)
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
		return ver
	}
	// This seems to be a dev build.
	return "unknown-dev"
}

func buildStamp() int64 {
	bs, err := runError("git", "show", "-s", "--format=%ct")
	if err != nil {
		return time.Now().Unix()
	}
	s, _ := strconv.ParseInt(string(bs), 10, 64)
	return s
}

func buildUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown-user"
	}
	return strings.Replace(u.Username, " ", "-", -1)
}

func buildHost() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return h
}

func buildEnvironment() string {
	if v := os.Getenv("ENVIRONMENT"); len(v) > 0 {
		return v
	}
	return "default"
}

func buildArch() string {
	os := goos
	if os == "darwin" {
		os = "macosx"
	}
	return fmt.Sprintf("%s-%s", os, goarch)
}

func archiveName() string {
	return fmt.Sprintf("syncthing-%s-%s", buildArch(), version)
}

func run(cmd string, args ...string) []byte {
	bs, err := runError(cmd, args...)
	if err != nil {
		log.Println(cmd, strings.Join(args, " "))
		log.Println(string(bs))
		log.Fatal(err)
	}
	return bytes.TrimSpace(bs)
}

func runError(cmd string, args ...string) ([]byte, error) {
	ecmd := exec.Command(cmd, args...)
	bs, err := ecmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(bs), nil
}

func runPrint(cmd string, args ...string) {
	log.Println(cmd, strings.Join(args, " "))
	ecmd := exec.Command(cmd, args...)
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	err := ecmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func runPipe(file, cmd string, args ...string) {
	log.Println(cmd, strings.Join(args, " "), ">", file)
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

type archiveFile struct {
	src string
	dst string
}

func tarGz(out string, files []archiveFile) {
	fd, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}

	gw := gzip.NewWriter(fd)
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
		fh.Name = f.dst
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

func md5File(file string) error {
	fd, err := os.Open(file)
	if err != nil {
		return err
	}
	defer fd.Close()

	h := md5.New()
	_, err = io.Copy(h, fd)
	if err != nil {
		return err
	}

	out, err := os.Create(file + ".md5")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "%x\n", h.Sum(nil))
	if err != nil {
		return err
	}

	return out.Close()
}
