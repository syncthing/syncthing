package deadlock

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

func callers(skip int) []uintptr {
	s := make([]uintptr, 50) // Most relevant context seem to appear near the top of the stack.
	return s[:runtime.Callers(2+skip, s)]
}

func printStack(w io.Writer, stack []uintptr) {
	home := os.Getenv("HOME")
	usr, err := user.Current()
	if err == nil {
		home = usr.HomeDir
	}
	cwd, _ := os.Getwd()

	for i, pc := range stack {
		f := runtime.FuncForPC(pc)
		name := f.Name()
		pkg := ""
		if pos := strings.LastIndex(name, "/"); pos >= 0 {
			name = name[pos+1:]
		}
		if pos := strings.Index(name, "."); pos >= 0 {
			pkg = name[:pos]
			name = name[pos+1:]
		}
		file, line := f.FileLine(pc - 1)
		if (pkg == "runtime" && name == "goexit") || (pkg == "testing" && name == "tRunner") {
			fmt.Fprintln(w)
			return
		}
		tail := ""
		if i == 0 {
			tail = " <<<<<" // Make the line performing a lock prominent.
		}
		// Shorten the file name.
		clean := file
		if cwd != "" {
			cl, err := filepath.Rel(cwd, file)
			if err == nil {
				clean = cl
			}
		}
		if home != "" {
			s2 := strings.Replace(file, home, "~", 1)
			if len(clean) > len(s2) {
				clean = s2
			}
		}
		fmt.Fprintf(w, "%s:%d %s.%s %s%s\n", clean, line, pkg, name, code(file, line), tail)
	}
	fmt.Fprintln(w)
}

var fileSources struct {
	sync.Mutex
	lines map[string][][]byte
}

// Reads souce file lines from disk if not cached already.
func getSourceLines(file string) [][]byte {
	fileSources.Lock()
	defer fileSources.Unlock()
	if fileSources.lines == nil {
		fileSources.lines = map[string][][]byte{}
	}
	if lines, ok := fileSources.lines[file]; ok {
		return lines
	}
	text, _ := ioutil.ReadFile(file)
	fileSources.lines[file] = bytes.Split(text, []byte{'\n'})
	return fileSources.lines[file]
}

func code(file string, line int) string {
	lines := getSourceLines(file)
	// lines are 1 based.
	if line >= len(lines) || line <= 0 {
		return "???"
	}
	return "{ " + string(bytes.TrimSpace(lines[line-1])) + " }"
}

// Stacktraces for all goroutines.
func stacks() []byte {
	buf := make([]byte, 1024*16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}
