// +build mage

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/magefile/mage/sh"

	"github.com/magefile/mage/mg"
)

// Spinners generates easy/accessible Go types for spinners from
// cli-spinners/spinners.json.
func Spinners() error {
	pkg := "spin"
	mg.Deps(func() error {
		_, err := os.Stat(pkg)
		if err != nil {
			if os.IsNotExist(err) {
				return os.Mkdir(pkg, 0777)
			}
			return err
		}
		return nil
	})
	b, err := ioutil.ReadFile("cli-spinners/spinners.json")
	if err != nil {
		return err
	}
	o := make(map[string]interface{})
	err = json.Unmarshal(b, &o)
	if err != nil {
		return err
	}
	tpl, err := template.New("spinner").Funcs(helpers()).Parse(spinnersTpl)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = tpl.Execute(&buf, o)
	if err != nil {
		return err
	}
	bo, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(pkg, "spinners.go"), bo, 0600)
}

// ExampleAll generates example executable file to demo all spinners
func ExampleAll() error {
	b, err := ioutil.ReadFile("cli-spinners/spinners.json")
	if err != nil {
		return err
	}
	o := make(map[string]interface{})
	err = json.Unmarshal(b, &o)
	if err != nil {
		return err
	}

	// Generate example/all/main.go
	tpl, err := template.New("example-all").Funcs(helpers()).Parse(exampleAllTpl)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = tpl.Execute(&buf, o)
	if err != nil {
		return err
	}
	bo, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return ioutil.WriteFile("example/all/main.go", bo, 0600)
}

func helpers() template.FuncMap {
	return template.FuncMap{
		"title": strings.Title,
		"stringify": func(a []interface{}) string {
			o := ""
			switch len(a) {
			case 0:
				return ""
			case 1:
				return fmt.Sprintf("\"%s\"", a[0])
			default:
				for k, v := range a {
					if k == 0 {
						o += fmt.Sprintf("`%s`", v)
					} else {
						if v == "`" {
							o += fmt.Sprintf(",\"%s\"", v)

						} else {
							o += fmt.Sprintf(",`%s`", v)
						}

					}
				}

				return fmt.Sprintf("[]string{ %v }", o)
			}
		},
		"all": func(a map[string]interface{}) string {
			o := ""
			for k := range a {
				if o == "" {
					o += fmt.Sprintf("%s", strings.Title(k))
				} else {
					o += fmt.Sprintf(",%s", strings.Title(k))
				}
			}
			return fmt.Sprintf("[]Name{%s}", o)
		},
		"allwithpkg": func(a map[string]interface{}) string {
			o := ""
			for k := range a {
				if o == "" {
					o += fmt.Sprintf("spin.%s", strings.Title(k))
				} else {
					o += fmt.Sprintf(",spin.%s", strings.Title(k))
				}
			}
			return fmt.Sprintf("[]spin.Name{%s}", o)
		},
	}
}

const spinnersTpl = `//DO NOT EDIT : this file was automatically generated.
package spin

// Spinner defines a spinner widget
type Spinner struct{
	Name Name
	Interval int
	Frames []string
}

// Name  represents a name for a spinner item.
type Name uint

// available spinners
const(
	Unknown Name=iota
	{{range $k,$v:= .}}
	{{- $k|title}}
	{{end}}
)

func (s Name)String()string{
	switch s{
		{{- range $k,$v:=.}}
	case {{$k|title}} :
		return "{{$k}}"
		{{- end}}
	default:
		return ""
	}
}

func Get( name Name)Spinner{
	switch name{
		{{- range $k,$v:=.}}
	case {{$k|title}} :
		return Spinner{
			Name: {{$k|title}},
			Interval: {{$v.interval}},
			Frames: {{$v.frames|stringify }},
		}
		{{- end}}
	default:
		return Spinner{}
	}
}
`

const exampleAllTpl = `//DO NOT EDIT : this file was automatically generated.
package main

import (
	"os"
	"time"

	"github.com/gernest/wow"
	"github.com/gernest/wow/spin"
)

var all = {{allwithpkg .}}

func main() {
	for _, v := range all {
		w := wow.New(os.Stdout, spin.Get(v), " "+v.String())
		w.Start()
		time.Sleep(2)
		w.Persist()
	}
}

`

// Update updates cli-spinners to get latest changes to the spinners.json file.
func Update() error {
	return sh.Run("git", "submodule", "update", "--remote", "cli-spinners")
}

//Setup prepares the project for local development.
//
//  This runs git submodule init && git submodule update
func Setup() error {
	err := sh.Run("git", "submodule", "init")
	if err != nil {
		return err
	}
	return sh.Run("git", "submodule", "update")
}
