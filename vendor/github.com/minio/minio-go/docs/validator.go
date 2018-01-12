// +build ignore

/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2015-2017 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/a8m/mark"
	"github.com/gernest/wow"
	"github.com/gernest/wow/spin"
	"github.com/minio/cli"
)

func init() {
	// Validate go binary.
	if _, err := exec.LookPath("go"); err != nil {
		panic(err)
	}
}

var globalFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "m",
		Value: "API.md",
		Usage: "Path to markdown api documentation.",
	},
	cli.StringFlag{
		Name:  "t",
		Value: "checker.go.template",
		Usage: "Template used for generating the programs.",
	},
	cli.IntFlag{
		Name:  "skip",
		Value: 2,
		Usage: "Skip entries before validating the code.",
	},
}

func runGofmt(path string) (msg string, err error) {
	cmdArgs := []string{"-s", "-w", "-l", path}
	cmd := exec.Command("gofmt", cmdArgs...)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(stdoutStderr), nil
}

func runGoImports(path string) (msg string, err error) {
	cmdArgs := []string{"-w", path}
	cmd := exec.Command("goimports", cmdArgs...)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return string(stdoutStderr), err
	}
	return string(stdoutStderr), nil
}

func runGoBuild(path string) (msg string, err error) {
	// Go build the path.
	cmdArgs := []string{"build", "-o", "/dev/null", path}
	cmd := exec.Command("go", cmdArgs...)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return string(stdoutStderr), err
	}
	return string(stdoutStderr), nil
}

func validatorAction(ctx *cli.Context) error {
	if !ctx.IsSet("m") || !ctx.IsSet("t") {
		return nil
	}
	docPath := ctx.String("m")
	var err error
	docPath, err = filepath.Abs(docPath)
	if err != nil {
		return err
	}
	data, err := ioutil.ReadFile(docPath)
	if err != nil {
		return err
	}

	templatePath := ctx.String("t")
	templatePath, err = filepath.Abs(templatePath)
	if err != nil {
		return err
	}

	skipEntries := ctx.Int("skip")
	m := mark.New(string(data), &mark.Options{
		Gfm: true, // Github markdown support is enabled by default.
	})

	t, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "md-verifier")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	entryN := 1
	for i := mark.NodeText; i < mark.NodeCheckbox; i++ {
		if mark.NodeCode != mark.NodeType(i) {
			m.AddRenderFn(mark.NodeType(i), func(node mark.Node) (s string) {
				return ""
			})
			continue
		}
		m.AddRenderFn(mark.NodeCode, func(node mark.Node) (s string) {
			p, ok := node.(*mark.CodeNode)
			if !ok {
				return
			}
			p.Text = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&").Replace(p.Text)
			if skipEntries > 0 {
				skipEntries--
				return
			}

			testFilePath := filepath.Join(tmpDir, "example.go")
			w, werr := os.Create(testFilePath)
			if werr != nil {
				panic(werr)
			}
			t.Execute(w, p)
			w.Sync()
			w.Close()
			entryN++

			msg, err := runGofmt(testFilePath)
			if err != nil {
				fmt.Printf("Failed running gofmt on %s, with (%s):(%s)\n", testFilePath, msg, err)
				os.Exit(-1)
			}

			msg, err = runGoImports(testFilePath)
			if err != nil {
				fmt.Printf("Failed running gofmt on %s, with (%s):(%s)\n", testFilePath, msg, err)
				os.Exit(-1)
			}

			msg, err = runGoBuild(testFilePath)
			if err != nil {
				fmt.Printf("Failed running gobuild on %s, with (%s):(%s)\n", testFilePath, msg, err)
				fmt.Printf("Code with possible issue in %s:\n%s", docPath, p.Text)
				fmt.Printf("To test `go build %s`\n", testFilePath)
				os.Exit(-1)
			}

			// Once successfully built remove the test file
			os.Remove(testFilePath)
			return
		})
	}

	w := wow.New(os.Stdout, spin.Get(spin.Moon), fmt.Sprintf(" Running validation tests in %s", tmpDir))

	w.Start()
	// Render markdown executes our checker on each code blocks.
	_ = m.Render()
	w.PersistWith(spin.Get(spin.Runner), " Successfully finished tests")
	w.Stop()

	return nil
}

func main() {
	app := cli.NewApp()
	app.Action = validatorAction
	app.HideVersion = true
	app.HideHelpCommand = true
	app.Usage = "Validates code block sections inside API.md"
	app.Author = "Minio.io"
	app.Flags = globalFlags
	// Help template for validator
	app.CustomAppHelpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

USAGE:
  {{.Name}} {{if .VisibleFlags}}[FLAGS] {{end}}COMMAND{{if .VisibleFlags}} [COMMAND FLAGS | -h]{{end}} [ARGUMENTS...]

COMMANDS:
  {{range .VisibleCommands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
  {{end}}{{if .VisibleFlags}}
FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
TEMPLATE:
  Validator uses Go's 'text/template' formatting so you need to ensure 
  your template is formatted correctly, check 'docs/checker.go.template'

USAGE:
  go run docs/validator.go -m docs/API.md -t /tmp/mycode.go.template

`
	app.Run(os.Args)

}
