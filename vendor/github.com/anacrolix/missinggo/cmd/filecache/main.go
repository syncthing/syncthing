package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/tagflag"
	"github.com/dustin/go-humanize"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/filecache"
)

var c *filecache.Cache

func handleNewData(w http.ResponseWriter, path string, offset int64, r io.Reader) (served bool) {
	f, err := c.OpenFile(path, os.O_CREATE|os.O_WRONLY)
	if err != nil {
		log.Print(err)
		http.Error(w, "couldn't open file", http.StatusInternalServerError)
		return true
	}
	defer f.Close()
	f.Seek(offset, os.SEEK_SET)
	_, err = io.Copy(f, r)
	if err != nil {
		log.Print(err)
		f.Remove()
		http.Error(w, "didn't complete", http.StatusInternalServerError)
		return true
	}
	return
}

// Parses out the first byte from a Content-Range header. Returns 0 if it
// isn't found, which is what is implied if there is no header.
func parseContentRangeFirstByte(s string) int64 {
	matches := regexp.MustCompile(`(\d+)-`).FindStringSubmatch(s)
	if matches == nil {
		return 0
	}
	ret, _ := strconv.ParseInt(matches[1], 0, 64)
	return ret
}

func handleDelete(w http.ResponseWriter, path string) {
	err := c.Remove(path)
	if err != nil {
		log.Print(err)
		http.Error(w, "didn't work", http.StatusInternalServerError)
		return
	}
}

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	args := struct {
		Capacity tagflag.Bytes `short:"c"`
		Addr     string
	}{
		Capacity: -1,
		Addr:     "localhost:2076",
	}
	tagflag.Parse(&args)
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("cache root at %q", root)
	c, err = filecache.NewCache(root)
	if err != nil {
		log.Fatalf("error creating cache: %s", err)
	}
	if args.Capacity < 0 {
		log.Printf("no capacity set, no evictions will occur")
	} else {
		c.SetCapacity(args.Capacity.Int64())
		log.Printf("setting capacity to %s bytes", humanize.Comma(args.Capacity.Int64()))
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path[1:]
		switch r.Method {
		case "DELETE":
			log.Printf("%s %s", r.Method, r.RequestURI)
			handleDelete(w, p)
			return
		case "PUT", "PATCH", "POST":
			contentRange := r.Header.Get("Content-Range")
			firstByte := parseContentRangeFirstByte(contentRange)
			log.Printf("%s (%d-) %s", r.Method, firstByte, r.RequestURI)
			handleNewData(w, p, firstByte, r.Body)
			return
		}
		log.Printf("%s %s %s", r.Method, r.Header.Get("Range"), r.RequestURI)
		f, err := c.OpenFile(p, os.O_RDONLY)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("couldn't open requested file: %s", err)
			http.Error(w, "couldn't open file", http.StatusInternalServerError)
			return
		}
		defer func() {
			go f.Close()
		}()
		info, _ := f.Stat()
		w.Header().Set("Content-Range", fmt.Sprintf("*/%d", info.Size()))
		http.ServeContent(w, r, p, info.ModTime(), f)
	})
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		info := c.Info()
		fmt.Fprintf(w, "Capacity: %d\n", info.Capacity)
		fmt.Fprintf(w, "Current Size: %d\n", info.Filled)
		fmt.Fprintf(w, "Item Count: %d\n", info.NumItems)
	})
	http.HandleFunc("/lru", func(w http.ResponseWriter, r *http.Request) {
		c.WalkItems(func(item filecache.ItemInfo) {
			fmt.Fprintf(w, "%s\t%d\t%s\n", item.Accessed, item.Size, item.Path)
		})
	})
	cert, err := missinggo.NewSelfSignedCertificate()
	if err != nil {
		log.Fatal(err)
	}
	srv := http.Server{
		Addr: args.Addr,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}
	log.Fatal(srv.ListenAndServeTLS("", ""))
}
