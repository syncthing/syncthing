// Command tomlv validates TOML documents and prints each key's type.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"text/tabwriter"

	"github.com/BurntSushi/toml"
)

var (
	flagTypes = false
)

func init() {
	log.SetFlags(0)

	flag.BoolVar(&flagTypes, "types", flagTypes,
		"When set, the types of every defined key will be shown.")

	flag.Usage = usage
	flag.Parse()
}

func usage() {
	log.Printf("Usage: %s toml-file [ toml-file ... ]\n",
		path.Base(os.Args[0]))
	flag.PrintDefaults()

	os.Exit(1)
}

func main() {
	if flag.NArg() < 1 {
		flag.Usage()
	}
	for _, f := range flag.Args() {
		var tmp interface{}
		md, err := toml.DecodeFile(f, &tmp)
		if err != nil {
			log.Fatalf("Error in '%s': %s", f, err)
		}
		if flagTypes {
			printTypes(md)
		}
	}
}

func printTypes(md toml.MetaData) {
	tabw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, key := range md.Keys() {
		fmt.Fprintf(tabw, "%s%s\t%s\n",
			strings.Repeat("    ", len(key)-1), key, md.Type(key...))
	}
	tabw.Flush()
}
