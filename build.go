// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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
	flag.BoolVar(&noupgrade, "no-upgrade", false, "Disable upgrade functionality")
	flag.StringVar(&version, "version", getVersion(), "Set compiled in version string")
	flag.Parse()

	switch goarch {
	case "386", "amd64", "armv5", "armv6", "armv7":
		break
	case "arm":
		log.Println("Invalid goarch \"arm\". Use one of \"armv5\", \"armv6\", \"armv7\".")
		log.Fatalln("Note that producing a correct \"armv5\" binary requires a rebuilt stdlib.")
	default:
		log.Printf("Unknown goarch %q; proceed with caution!", goarch)
	}

	checkRequiredGoVersion()

	if flag.NArg() == 0 {
		install("./cmd/...")
		return
	}

	for _, cmd := range flag.Args() {
		switch cmd {
		case "setup":
			setup()

		case "install":
			pkg := "./cmd/..."
			install(pkg)

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
			log.Printf("*** Could parse Go version out of %q.\n*** This isn't known to work, proceed on your own risk.", vs)
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
	runPrint("go", "get", "-v", "code.google.com/p/go.tools/cmd/cover")
	runPrint("go", "get", "-v", "code.google.com/p/go.tools/cmd/vet")
	runPrint("go", "get", "-v", "code.google.com/p/go.net/html")
	runPrint("go", "get", "-v", "github.com/tools/godep")
}

func test(pkg string) {
	setBuildEnv()
	runPrint("go", "test", "-short", "-timeout", "10s", pkg)
}

func install(pkg string) {
	os.Setenv("GOBIN", "./bin")
	setBuildEnv()
	runPrint("go", "install", "-v", "-ldflags", ldflags(), pkg)
}

func build(pkg string, tags []string) {
	rmr("syncthing", "syncthing.exe")
	args := []string{"build", "-ldflags", ldflags()}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, pkg)
	setBuildEnv()
	runPrint("go", args...)
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
	tarGz(filename, []archiveFile{
		{"README.md", name + "/README.txt"},
		{"LICENSE", name + "/LICENSE.txt"},
		{"CONTRIBUTORS", name + "/CONTRIBUTORS.txt"},
		{"syncthing", name + "/syncthing"},
	})
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
	zipFile(filename, []archiveFile{
		{"README.md", name + "/README.txt"},
		{"LICENSE", name + "/LICENSE.txt"},
		{"CONTRIBUTORS", name + "/CONTRIBUTORS.txt"},
		{"syncthing.exe", name + "/syncthing.exe"},
	})
	log.Println(filename)
}

func setBuildEnv() {
	os.Setenv("GOOS", goos)
	if strings.HasPrefix(goarch, "arm") {
		os.Setenv("GOARCH", "arm")
		os.Setenv("GOARM", goarch[4:])
	} else {
		os.Setenv("GOARCH", goarch)
	}
	if goarch == "386" {
		os.Setenv("GO386", "387")
	}
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
	for _, f := range []string{"internal/discover/packets", "internal/files/leveldb", "internal/protocol/message"} {
		runPipe(f+"_xdr.go", "go", "run", "./Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go", "--", f+".go")
	}
}

func translate() {
	os.Chdir("gui/lang")
	runPipe("lang-en-new.json", "go", "run", "../../cmd/translate/main.go", "lang-en.json", "../index.html")
	os.Remove("lang-en.json")
	err := os.Rename("lang-en-new.json", "lang-en.json")
	if err != nil {
		log.Fatal(err)
	}
	os.Chdir("../..")
}

func transifex() {
	os.Chdir("gui/lang")
	runPrint("go", "run", "../../cmd/transifexdl/main.go")
	os.Chdir("../..")
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
	if strings.HasPrefix(goarch, "arm") {
		b.WriteString(fmt.Sprintf(" -X main.GoArchExtra %s", goarch[3:]))
	}
	return b.String()
}

func rmr(paths ...string) {
	for _, path := range paths {
		log.Println("rm -r", path)
		os.RemoveAll(path)
	}
}

func getVersion() string {
	v, err := runError("git", "describe", "--always", "--dirty")
	if err != nil {
		return "unknown-dev"
	}
	v = versionRe.ReplaceAllFunc(v, func(s []byte) []byte {
		s[0] = '+'
		return s
	})
	return string(v)
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
