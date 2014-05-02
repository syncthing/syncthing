//+build !guidev

package main

import (
	"fmt"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/calmh/syncthing/auto"
)

func embeddedStatic() interface{} {
	var modt = time.Now().UTC().Format(http.TimeFormat)

	return func(res http.ResponseWriter, req *http.Request, log *log.Logger) {
		file := req.URL.Path

		if file[0] == '/' {
			file = file[1:]
		}

		bs, ok := auto.Assets[file]
		if !ok {
			return
		}

		mtype := mime.TypeByExtension(filepath.Ext(req.URL.Path))
		if len(mtype) != 0 {
			res.Header().Set("Content-Type", mtype)
		}
		res.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))
		res.Header().Set("Last-Modified", modt)

		res.Write(bs)
	}
}
