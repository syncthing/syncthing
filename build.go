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
)

var (
	versionRe = regexp.MustCompile(`-[0-9]{1,3}-g[0-9a-f]{5,10}`)
	goarch    string
	goos      string
	noupgrade bool
)

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
	flag.Parse()

	if check() != nil {
		setup()
	}

	if flag.NArg() == 0 {
		install("./cmd/...")
		return
	}

	switch flag.Arg(0) {
	case "install":
		pkg := "./cmd/..."
		if flag.NArg() > 2 {
			pkg = flag.Arg(1)
		}
		install(pkg)

	case "build":
		pkg := "./cmd/syncthing"
		if flag.NArg() > 2 {
			pkg = flag.Arg(1)
		}
		var tags []string
		if noupgrade {
			tags = []string{"noupgrade"}
		}
		build(pkg, tags)

	case "test":
		pkg := "./..."
		if flag.NArg() > 2 {
			pkg = flag.Arg(1)
		}
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
		log.Fatalf("Unknown command %q", flag.Arg(0))
	}
}

func check() error {
	_, err := exec.LookPath("godep")
	return err
}

func setup() {
	runPrint("go", "get", "-v", "code.google.com/p/go.tools/cmd/cover")
	runPrint("go", "get", "-v", "code.google.com/p/go.tools/cmd/vet")
	runPrint("go", "get", "-v", "code.google.com/p/go.net/html")
	runPrint("go", "get", "-v", "github.com/mattn/goveralls")
	runPrint("go", "get", "-v", "github.com/tools/godep")
}

func test(pkg string) {
	runPrint("godep", "go", "test", pkg)
}

func install(pkg string) {
	os.Setenv("GOBIN", "./bin")
	runPrint("godep", "go", "install", "-ldflags", ldflags(), pkg)
}

func build(pkg string, tags []string) {
	rmr("syncthing", "syncthing.exe")
	args := []string{"go", "build", "-ldflags", ldflags()}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, pkg)
	setBuildEnv()
	runPrint("godep", args...)
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
	} else {
		os.Setenv("GOARCH", goarch)
	}
}

func assets() {
	runPipe("auto/gui.files.go", "godep", "go", "run", "cmd/genassets/main.go", "gui")
}

func xdr() {
	for _, f := range []string{"discover/packets", "files/leveldb", "protocol/message"} {
		runPipe(f+"_xdr.go", "go", "run", "./Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go", "--", f+".go")
	}
}

func translate() {
	os.Chdir("gui")
	runPipe("lang-en-new.json", "go", "run", "../cmd/translate/main.go", "lang-en.json", "index.html")
	os.Remove("lang-en.json")
	err := os.Rename("lang-en-new.json", "lang-en.json")
	if err != nil {
		log.Fatal(err)
	}
	os.Chdir("..")
}

func transifex() {
	os.Chdir("gui")
	runPrint("go", "run", "../cmd/transifexdl/main.go")
	os.Chdir("..")
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
	b.WriteString(fmt.Sprintf(" -X main.Version %s", version()))
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

func version() string {
	v := run("git", "describe", "--always", "--dirty")
	v = versionRe.ReplaceAllFunc(v, func(s []byte) []byte {
		s[0] = '+'
		return s
	})
	return string(v)
}

func buildStamp() int64 {
	bs := run("git", "show", "-s", "--format=%ct")
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
	return fmt.Sprintf("syncthing-%s-%s", buildArch(), version())
}

func run(cmd string, args ...string) []byte {
	ecmd := exec.Command(cmd, args...)
	bs, err := ecmd.CombinedOutput()
	if err != nil {
		log.Println(cmd, strings.Join(args, " "))
		log.Println(string(bs))
		log.Fatal(err)
	}
	return bytes.TrimSpace(bs)
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
