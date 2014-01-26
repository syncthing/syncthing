package main

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type Options struct {
	Listen            string        `ini:"listen-address" default:":22000" description:"ip:port to for incoming sync connections"`
	ReadOnly          bool          `ini:"read-only" description:"Allow changes to the local repository"`
	Delete            bool          `ini:"allow-delete" default:"true" description:"Allow deletes of files in the local repository"`
	Symlinks          bool          `ini:"follow-symlinks" default:"true" description:"Follow symbolic links at the top level of the repository"`
	GUI               bool          `ini:"gui-enabled" default:"true" description:"Enable the HTTP GUI"`
	GUIAddr           string        `ini:"gui-address" default:"127.0.0.1:8080" description:"ip:port for GUI connections"`
	ExternalServer    string        `ini:"global-announce-server" default:"syncthing.nym.se:22025" description:"Global server for announcements"`
	ExternalDiscovery bool          `ini:"global-announce-enabled" default:"true" description:"Announce to the global announce server"`
	LocalDiscovery    bool          `ini:"local-announce-enabled" default:"true" description:"Announce to the local network"`
	ParallelRequests  int           `ini:"parallel-requests" default:"16" description:"Maximum number of blocks to request in parallel"`
	LimitRate         int           `ini:"max-send-kbps" description:"Limit outgoing data rate (kbyte/s)"`
	ScanInterval      time.Duration `ini:"rescan-interval" default:"60s" description:"Scan repository for changes this often"`
	ConnInterval      time.Duration `ini:"reconnection-interval" default:"60s" description:"Attempt to (re)connect to peers this often"`
	MaxChangeBW       int           `ini:"max-change-bw" default:"1000" description:"Suppress files changing more than this (kbyte/s)"`
}

func loadConfig(m map[string]string, data interface{}) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		name := tag.Get("ini")
		if len(name) == 0 {
			name = strings.ToLower(t.Field(i).Name)
		}

		v, ok := m[name]
		if !ok {
			v = tag.Get("default")
		}
		if len(v) > 0 {
			switch f.Interface().(type) {
			case time.Duration:
				d, err := time.ParseDuration(v)
				if err != nil {
					return err
				}
				f.SetInt(int64(d))

			case string:
				f.SetString(v)

			case int:
				i, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return err
				}
				f.SetInt(i)

			case bool:
				f.SetBool(v == "true")

			default:
				panic(f.Type())
			}
		}
	}
	return nil
}

type cfg struct {
	Key     string
	Value   string
	Comment string
}

func structToValues(data interface{}) []cfg {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	var vals []cfg
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		var c cfg
		c.Key = tag.Get("ini")
		if len(c.Key) == 0 {
			c.Key = strings.ToLower(t.Field(i).Name)
		}
		c.Value = fmt.Sprint(f.Interface())
		c.Comment = tag.Get("description")
		vals = append(vals, c)
	}
	return vals
}

var configTemplateStr = `[repository]
{{if .comments}}; The directory to synchronize. Will be created if it does not exist.
{{end}}dir = {{.dir}}

[nodes]
{{if .comments}}; Map of node ID to addresses, or "dynamic" for automatic discovery. Examples:
; J3MZ4G5O4CLHJKB25WX47K5NUJUWDOLO2TTNY3TV3NRU4HVQRKEQ = 172.16.32.24:22000
; ZNJZRXQKYHF56A2VVNESRZ6AY4ZOWGFJCV6FXDZJUTRVR3SNBT6Q = dynamic
{{end}}{{range $n, $a := .nodes}}{{$n}} = {{$a}}
{{end}}
[settings]
{{range $v := .settings}}; {{$v.Comment}}
{{$v.Key}} = {{$v.Value}}
{{end}}
`

var configTemplate = template.Must(template.New("config").Parse(configTemplateStr))

func writeConfig(wr io.Writer, dir string, nodes map[string]string, opts Options, comments bool) {
	configTemplate.Execute(wr, map[string]interface{}{
		"dir":      dir,
		"nodes":    nodes,
		"settings": structToValues(&opts),
		"comments": comments,
	})
}
