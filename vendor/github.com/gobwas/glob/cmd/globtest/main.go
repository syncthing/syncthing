package main

import (
	"flag"
	"fmt"
	"github.com/gobwas/glob"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func benchString(r testing.BenchmarkResult) string {
	nsop := r.NsPerOp()
	ns := fmt.Sprintf("%10d ns/op", nsop)
	allocs := "0"
	if r.N > 0 {
		if nsop < 100 {
			// The format specifiers here make sure that
			// the ones digits line up for all three possible formats.
			if nsop < 10 {
				ns = fmt.Sprintf("%13.2f ns/op", float64(r.T.Nanoseconds())/float64(r.N))
			} else {
				ns = fmt.Sprintf("%12.1f ns/op", float64(r.T.Nanoseconds())/float64(r.N))
			}
		}

		allocs = fmt.Sprintf("%d", r.MemAllocs/uint64(r.N))
	}

	return fmt.Sprintf("%8d\t%s\t%s allocs", r.N, ns, allocs)
}

func main() {
	pattern := flag.String("p", "", "pattern to draw")
	sep := flag.String("s", "", "comma separated list of separators")
	fixture := flag.String("f", "", "fixture")
	verbose := flag.Bool("v", false, "verbose")
	flag.Parse()

	if *pattern == "" {
		flag.Usage()
		os.Exit(1)
	}

	var separators []rune
	for _, c := range strings.Split(*sep, ",") {
		if r, w := utf8.DecodeRuneInString(c); len(c) > w {
			fmt.Println("only single charactered separators are allowed")
			os.Exit(1)
		} else {
			separators = append(separators, r)
		}
	}

	g, err := glob.Compile(*pattern, separators...)
	if err != nil {
		fmt.Println("could not compile pattern:", err)
		os.Exit(1)
	}

	if !*verbose {
		fmt.Println(g.Match(*fixture))
		return
	}

	fmt.Printf("result: %t\n", g.Match(*fixture))

	cb := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			glob.Compile(*pattern, separators...)
		}
	})
	fmt.Println("compile:", benchString(cb))

	mb := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			g.Match(*fixture)
		}
	})
	fmt.Println("match:    ", benchString(mb))
}
